package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/url"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/solomonadzape95/workbench/internal/alias"
	"github.com/solomonadzape95/workbench/internal/config"
	"github.com/solomonadzape95/workbench/internal/core"
	"github.com/solomonadzape95/workbench/internal/notify"
	"github.com/solomonadzape95/workbench/internal/runner"
)

type createJobRequest struct {
	Operation   string         `json:"operation,omitempty"`
	Pipeline    string         `json:"pipeline,omitempty"`
	Steps       []stepRequest  `json:"steps,omitempty"`
	Inputs      []string       `json:"inputs"`
	Params      map[string]any `json:"params,omitempty"`
	Name        string         `json:"name,omitempty"`
	Output      string         `json:"output,omitempty"`
	Target      *targetRequest `json:"target,omitempty"`
	CallbackURL string         `json:"callback_url,omitempty"`
}

type stepRequest struct {
	Operation string         `json:"operation"`
	Params    map[string]any `json:"params,omitempty"`
}

type targetRequest struct {
	Dir       string `json:"dir,omitempty"`
	Template  string `json:"template,omitempty"`
	Collision string `json:"collision,omitempty"`
}

func (s *Server) listOperations(w http.ResponseWriter, _ *http.Request) {
	ops := s.ops.All()
	out := make([]operationInfo, 0, len(ops))
	for _, op := range ops {
		out = append(out, operationDTO(op))
	}
	writeJSON(w, http.StatusOK, map[string]any{"operations": out})
}

func (s *Server) getOperation(w http.ResponseWriter, r *http.Request) {
	op, ok := s.ops.Get(r.PathValue("id"))
	if !ok {
		writeError(w, http.StatusNotFound, core.NewError(core.CodeUsage, r.PathValue("id"), "",
			"operation not found", "list operations with GET /v1/operations"))
		return
	}
	writeJSON(w, http.StatusOK, operationDTO(op))
}

func (s *Server) listEngines(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	var out []engineInfo
	if s.engines != nil {
		for _, info := range s.engines.ProbeAll(ctx) {
			out = append(out, engineDTO(info))
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"engines": out})
}

func (s *Server) listPipelines(w http.ResponseWriter, _ *http.Request) {
	if s.pipelines == nil {
		writeJSON(w, http.StatusOK, map[string]any{"pipelines": []pipelineInfo{}})
		return
	}
	metas, err := s.pipelines.List()
	if err != nil {
		writeError(w, statusFor(err), err)
		return
	}
	out := make([]pipelineInfo, 0, len(metas))
	for _, meta := range metas {
		out = append(out, pipelineDTO(meta))
	}
	writeJSON(w, http.StatusOK, map[string]any{"pipelines": out})
}

func (s *Server) getPipeline(w http.ResponseWriter, r *http.Request) {
	if s.pipelines == nil {
		writeError(w, http.StatusNotFound, core.NewError(core.CodeUsage, "", "", "no pipeline store", ""))
		return
	}
	p, err := s.pipelines.Load(r.PathValue("name"))
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			writeError(w, http.StatusNotFound, core.NewError(core.CodeUsage, "", r.PathValue("name"),
				"pipeline not found", "list saved pipelines with GET /v1/pipelines"))
			return
		}
		writeError(w, statusFor(err), err)
		return
	}
	writeJSON(w, http.StatusOK, pipelineDetailDTO(p))
}

func (s *Server) createJob(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	var req createJobRequest
	dec := json.NewDecoder(r.Body)
	if err := dec.Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, core.NewError(core.CodeUsage, "", "",
			"invalid JSON: "+err.Error(), "send a JSON object; see docs/agent-api.md"))
		return
	}
	if err := ensureEOF(dec); err != nil {
		writeError(w, http.StatusBadRequest, core.NewError(core.CodeUsage, "", "",
			"invalid JSON: "+err.Error(), "send exactly one JSON object"))
		return
	}

	p, first, err := s.buildPipeline(req)
	if err != nil {
		writeError(w, statusFor(err), err)
		return
	}
	inputs, err := s.resolveInputs(req.Inputs, first.Spec())
	if err != nil {
		writeError(w, statusFor(err), err)
		return
	}
	target, err := s.resolveTarget(req)
	if err != nil {
		writeError(w, statusFor(err), err)
		return
	}

	job, err := s.runner.Start(p, inputs, target)
	if err != nil {
		writeError(w, statusFor(err), err)
		return
	}
	if strings.TrimSpace(req.CallbackURL) != "" {
		if err := validateCallback(req.CallbackURL); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		go s.callback(req.CallbackURL, job.ID())
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"job": jobDTO(job.Snapshot(), true)})
}

func (s *Server) buildPipeline(req createJobRequest) (core.Pipeline, core.Operation, error) {
	modes := 0
	if strings.TrimSpace(req.Operation) != "" {
		modes++
	}
	if strings.TrimSpace(req.Pipeline) != "" {
		modes++
	}
	if len(req.Steps) > 0 {
		modes++
	}
	if modes != 1 {
		return core.Pipeline{}, nil, core.NewError(core.CodeUsage, "", "",
			"specify exactly one of operation, pipeline, or steps",
			"see GET /v1/operations for available operations")
	}

	var p core.Pipeline
	switch {
	case strings.TrimSpace(req.Operation) != "":
		op, ok := s.ops.Get(strings.TrimSpace(req.Operation))
		if !ok {
			return core.Pipeline{}, nil, core.NewError(core.CodeUsage, req.Operation, "", "unknown operation", "list operations with GET /v1/operations")
		}
		p = core.Pipeline{
			Name:    jobName(req.Name, op.Spec().Title),
			Version: core.PipelineVersion,
			Steps:   []core.Step{{Op: op.Spec().ID, Params: core.Values(req.Params)}},
		}
	case strings.TrimSpace(req.Pipeline) != "":
		if s.pipelines == nil {
			return core.Pipeline{}, nil, core.NewError(core.CodeUsage, req.Pipeline, "", "no pipeline store", "")
		}
		loaded, err := s.pipelines.Load(strings.TrimSpace(req.Pipeline))
		if err != nil {
			return core.Pipeline{}, nil, core.Wrap(core.CodeUsage, "", req.Pipeline, err, "list saved pipelines with GET /v1/pipelines")
		}
		p = loaded
		if strings.TrimSpace(req.Name) != "" {
			p.Name = req.Name
		}
		if len(req.Params) > 0 {
			merged := core.Values{}
			for key, value := range p.Steps[0].Params {
				merged[key] = value
			}
			for key, value := range req.Params {
				merged[key] = value
			}
			p.Steps[0].Params = merged
		}
	default:
		if len(req.Params) > 0 {
			return core.Pipeline{}, nil, core.NewError(core.CodeUsage, "", "",
				"params belong on each step when using steps", "put params inside the step objects")
		}
		steps := make([]core.Step, 0, len(req.Steps))
		for i, step := range req.Steps {
			op, ok := s.ops.Get(strings.TrimSpace(step.Operation))
			if !ok {
				return core.Pipeline{}, nil, core.NewError(core.CodeUsage, step.Operation, "",
					fmt.Sprintf("step %d uses an unknown operation", i+1), "list operations with GET /v1/operations")
			}
			steps = append(steps, core.Step{Op: op.Spec().ID, Params: core.Values(step.Params)})
		}
		p = core.Pipeline{Name: jobName(req.Name, "pipeline"), Version: core.PipelineVersion, Steps: steps}
	}

	if err := p.Validate(s.ops); err != nil {
		return core.Pipeline{}, nil, err
	}
	first, ok := s.ops.Get(p.Steps[0].Op)
	if !ok {
		return core.Pipeline{}, nil, core.NewError(core.CodeInternal, p.Steps[0].Op, "",
			"first operation disappeared from the registry", "this is a Workbench bug")
	}
	return p, first, nil
}

func (s *Server) resolveInputs(raw []string, first core.Spec) ([]core.Artifact, error) {
	var inputs []string
	for _, value := range raw {
		if value = strings.TrimSpace(value); value != "" {
			inputs = append(inputs, value)
		}
	}
	if len(inputs) == 0 {
		return nil, core.NewError(core.CodeUsage, first.ID, "", "inputs is required",
			"send file paths/globs or http(s) URLs in the inputs array")
	}

	if first.SourceKind() == core.SourceURLs {
		out := make([]core.Artifact, 0, len(inputs))
		for _, input := range inputs {
			parsed, err := url.Parse(input)
			if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") {
				return nil, core.NewError(core.CodeInput, first.ID, input,
					"input is not an http(s) URL", "URL operations require full http:// or https:// links")
			}
			out = append(out, core.Artifact{
				Path:   input,
				Label:  input,
				Kind:   core.Unknown,
				Format: strings.TrimPrefix(filepath.Ext(parsed.Path), "."),
			})
		}
		return out, nil
	}

	out, err := alias.Resolve(strings.Join(inputs, "\n"), s.cfg.Aliases)
	if err != nil {
		return nil, core.Wrap(core.CodeInput, first.ID, "", err,
			"use absolute paths, ~/ paths, globs, or @aliases")
	}
	if len(out) == 0 {
		return nil, core.NewError(core.CodeInput, first.ID, "", "no input files matched", "")
	}
	return out, nil
}

func (s *Server) resolveTarget(req createJobRequest) (core.Target, error) {
	dir := s.cfg.Output.Dir
	template := s.cfg.Output.Template
	collision := s.cfg.Output.Collision
	if strings.TrimSpace(dir) == "" {
		dir = "~/Workbench/{date}"
	}
	if strings.TrimSpace(template) == "" {
		template = "{stem}.{ext}"
	}
	if strings.TrimSpace(collision) == "" {
		collision = string(core.CollisionSuffix)
	}
	if strings.TrimSpace(req.Output) != "" {
		dir = req.Output
	}
	if req.Target != nil {
		if strings.TrimSpace(req.Target.Dir) != "" {
			dir = req.Target.Dir
		}
		if strings.TrimSpace(req.Target.Template) != "" {
			template = req.Target.Template
		}
		if strings.TrimSpace(req.Target.Collision) != "" {
			collision = req.Target.Collision
		}
	}

	dir = strings.ReplaceAll(dir, "{date}", time.Now().Format("2006-01-02"))
	dir = config.ExpandHome(strings.TrimSpace(dir))
	if dir == "" {
		return core.Target{}, core.NewError(core.CodeUsage, "", "", "output directory is empty", "")
	}
	switch core.Collision(collision) {
	case core.CollisionSuffix, core.CollisionOverwrite, core.CollisionSkip, core.CollisionFail:
	default:
		return core.Target{}, core.NewError(core.CodeUsage, "", "",
			fmt.Sprintf("unknown collision policy %q", collision),
			"use suffix, overwrite, skip, or fail")
	}
	return core.Target{Dir: dir, Template: template, Collision: core.Collision(collision)}, nil
}

func (s *Server) listJobs(w http.ResponseWriter, r *http.Request) {
	limit := 100
	if raw := r.URL.Query().Get("limit"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 && n <= 1000 {
			limit = n
		}
	}
	live := s.runner.Jobs()
	jobs := append([]runner.JobView(nil), live...)
	seen := make(map[string]bool, len(live))
	for _, job := range live {
		seen[job.ID] = true
	}
	if s.history != nil {
		if records, err := s.history.Recent(limit * 2); err == nil {
			for _, record := range records {
				if seen[record.View.ID] {
					continue
				}
				seen[record.View.ID] = true
				jobs = append(jobs, record.View)
			}
		}
	}
	sort.SliceStable(jobs, func(i, j int) bool {
		return jobs[i].QueuedAt.After(jobs[j].QueuedAt)
	})
	if len(jobs) > limit {
		jobs = jobs[:limit]
	}
	out := make([]jobInfo, 0, len(jobs))
	for _, job := range jobs {
		out = append(out, jobDTO(job, false))
	}
	writeJSON(w, http.StatusOK, map[string]any{"jobs": out})
}

func (s *Server) getJob(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	wait := 0
	if raw := r.URL.Query().Get("wait"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 {
			wait = n
			if wait > 60 {
				wait = 60
			}
		}
	}
	deadline := time.Now().Add(time.Duration(wait) * time.Second)
	for {
		job, ok := s.runner.Job(id)
		if !ok {
			if view, found := s.historicalJob(id); found {
				writeJSON(w, http.StatusOK, jobDTO(view, true))
				return
			}
			writeError(w, http.StatusNotFound, core.NewError(core.CodeUsage, "", "",
				"job not found", "list jobs with GET /v1/jobs"))
			return
		}
		view := job.Snapshot()
		if wait == 0 || isTerminal(view.Status) || time.Now().After(deadline) {
			writeJSON(w, http.StatusOK, jobDTO(view, true))
			return
		}
		select {
		case <-r.Context().Done():
			return
		case <-time.After(150 * time.Millisecond):
		}
	}
}

func (s *Server) cancelJob(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	job, ok := s.runner.Job(id)
	if !ok {
		writeError(w, http.StatusNotFound, core.NewError(core.CodeUsage, "", "", "job not found", ""))
		return
	}
	job.Cancel()
	writeJSON(w, http.StatusAccepted, map[string]any{"job": jobDTO(job.Snapshot(), true)})
}

func (s *Server) retryJob(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if _, ok := s.runner.Job(id); !ok {
		view, found := s.historicalJob(id)
		if !found {
			writeError(w, http.StatusNotFound, core.NewError(core.CodeUsage, "", "", "job not found", ""))
			return
		}
		job, err := s.startHistorical(view, false)
		if err != nil {
			writeError(w, http.StatusConflict, core.NewError(core.CodeUsage, "", "", err.Error(), "retry is only possible after a job has failed items"))
			return
		}
		writeJSON(w, http.StatusAccepted, map[string]any{"job": jobDTO(job.Snapshot(), true)})
		return
	}
	job, err := s.runner.RetryFailed(id)
	if err != nil {
		writeError(w, http.StatusConflict, core.NewError(core.CodeUsage, "", "", err.Error(), "retry is only possible after a job has failed items"))
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"job": jobDTO(job.Snapshot(), true)})
}

func (s *Server) rerunJob(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if _, ok := s.runner.Job(id); !ok {
		view, found := s.historicalJob(id)
		if !found {
			writeError(w, http.StatusNotFound, core.NewError(core.CodeUsage, "", "", "job not found", ""))
			return
		}
		job, err := s.startHistorical(view, true)
		if err != nil {
			writeError(w, http.StatusConflict, core.NewError(core.CodeUsage, "", "", err.Error(), "rerun is only possible after a job finishes"))
			return
		}
		writeJSON(w, http.StatusAccepted, map[string]any{"job": jobDTO(job.Snapshot(), true)})
		return
	}
	job, err := s.runner.Rerun(id)
	if err != nil {
		writeError(w, http.StatusConflict, core.NewError(core.CodeUsage, "", "", err.Error(), "rerun is only possible after a job finishes"))
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"job": jobDTO(job.Snapshot(), true)})
}

type artifactRef struct {
	Index  int    `json:"index"`
	ItemID string `json:"item_id"`
	Name   string `json:"name"`
	Path   string `json:"path"`
	Kind   string `json:"kind"`
	Format string `json:"format,omitempty"`
	Size   int64  `json:"size,omitempty"`
}

func (s *Server) jobView(id string) (runner.JobView, bool) {
	if s.runner != nil {
		if job, ok := s.runner.Job(id); ok {
			return job.Snapshot(), true
		}
	}
	return s.historicalJob(id)
}

func (s *Server) artifactRefs(view runner.JobView) []artifactRef {
	var out []artifactRef
	for _, item := range view.Items {
		for _, artifact := range item.Outputs {
			out = append(out, artifactRef{
				Index:  len(out) + 1,
				ItemID: item.ID,
				Name:   artifact.Name(),
				Path:   artifact.Path,
				Kind:   string(artifact.Kind),
				Format: artifact.Format,
				Size:   artifact.Size,
			})
		}
	}
	return out
}

func (s *Server) listArtifacts(w http.ResponseWriter, r *http.Request) {
	view, ok := s.jobView(r.PathValue("id"))
	if !ok {
		writeError(w, http.StatusNotFound, core.NewError(core.CodeUsage, "", "", "job not found", "list jobs with GET /v1/jobs"))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"artifacts": s.artifactRefs(view)})
}

func (s *Server) getArtifact(w http.ResponseWriter, r *http.Request) {
	view, ok := s.jobView(r.PathValue("id"))
	if !ok {
		writeError(w, http.StatusNotFound, core.NewError(core.CodeUsage, "", "", "job not found", "list jobs with GET /v1/jobs"))
		return
	}
	index, err := strconv.Atoi(r.PathValue("n"))
	if err != nil || index < 1 {
		writeError(w, http.StatusBadRequest, core.NewError(core.CodeUsage, "", "", "artifact number must start at 1", ""))
		return
	}
	refs := s.artifactRefs(view)
	if index > len(refs) {
		writeError(w, http.StatusNotFound, core.NewError(core.CodeUsage, "", "", "artifact not found", "list artifacts with GET /v1/jobs/{id}/artifacts"))
		return
	}
	http.ServeFile(w, r, refs[index-1].Path)
}

func (s *Server) jobEvents(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, core.NewError(core.CodeInternal, "", "", "streaming is not supported", ""))
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	id := r.PathValue("id")
	ticker := time.NewTicker(300 * time.Millisecond)
	defer ticker.Stop()
	for {
		view, found := s.jobView(id)
		if !found {
			fmt.Fprint(w, "event: error\ndata: {\"error\":\"job not found\"}\n\n")
			flusher.Flush()
			return
		}
		data, err := json.Marshal(jobDTO(view, true))
		if err != nil {
			return
		}
		fmt.Fprintf(w, "event: job\ndata: %s\n\n", data)
		flusher.Flush()
		if isTerminal(view.Status) {
			return
		}
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
		}
	}
}

func validateCallback(raw string) error {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return core.NewError(core.CodeUsage, "", "", "callback_url must be an http(s) URL", "")
	}
	return nil
}

func (s *Server) callback(rawURL, jobID string) {
	deadline := time.Now().Add(24 * time.Hour)
	for time.Now().Before(deadline) {
		view, ok := s.jobView(jobID)
		if !ok {
			return
		}
		if isTerminal(view.Status) {
			payload := notify.Job{
				ID:         view.ID,
				Name:       view.Name,
				Status:     string(view.Status),
				DurationMS: view.Duration().Milliseconds(),
				BytesIn:    view.Metrics.BytesIn,
				BytesOut:   view.Metrics.BytesOut,
			}
			if view.Err != nil {
				payload.Error = view.Err.Error()
			}
			for _, item := range view.Items {
				for _, output := range item.Outputs {
					payload.Outputs = append(payload.Outputs, output.Path)
				}
			}
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			_ = notify.Webhook(ctx, rawURL, payload)
			cancel()
			return
		}
		time.Sleep(500 * time.Millisecond)
	}
}

func (s *Server) historicalJob(id string) (runner.JobView, bool) {
	if s.history == nil {
		return runner.JobView{}, false
	}
	records, err := s.history.Recent(1000)
	if err != nil {
		return runner.JobView{}, false
	}
	for _, record := range records {
		if record.View.ID == id {
			return record.View, true
		}
	}
	return runner.JobView{}, false
}

func (s *Server) startHistorical(view runner.JobView, all bool) (*runner.Job, error) {
	if len(view.Pipeline.Steps) == 0 {
		return nil, errors.New("history entry has no pipeline")
	}
	var inputs []core.Artifact
	for _, item := range view.Items {
		if all || item.Status == core.ItemStatusFailed || item.Status == core.ItemStatusCanceled {
			inputs = append(inputs, item.Input)
		}
	}
	if len(inputs) == 0 {
		return nil, errors.New("job has no failed items")
	}
	pipeline := view.Pipeline
	pipeline.Name = view.Name
	if !all {
		pipeline.Name += " retry"
	}
	return s.runner.Start(pipeline, inputs, view.Target)
}

func isTerminal(status core.Status) bool {
	switch status {
	case core.StatusDone, core.StatusPartial, core.StatusFailed, core.StatusCanceled:
		return true
	default:
		return false
	}
}

func jobName(given, fallback string) string {
	if name := strings.TrimSpace(given); name != "" {
		return name
	}
	return fallback
}

func ensureEOF(dec *json.Decoder) error {
	var extra any
	if err := dec.Decode(&extra); err == io.EOF {
		return nil
	} else if err != nil {
		return err
	}
	return errors.New("trailing data")
}
