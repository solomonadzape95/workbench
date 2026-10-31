package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/solomonadzape95/workbench/internal/core"
	"github.com/solomonadzape95/workbench/internal/engines"
	"github.com/solomonadzape95/workbench/internal/pipeline"
	"github.com/solomonadzape95/workbench/internal/runner"
)

type errorBody struct {
	Error errorInfo `json:"error"`
}

type errorInfo struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Hint    string `json:"hint,omitempty"`
	Path    string `json:"path,omitempty"`
	Op      string `json:"operation,omitempty"`
}

type operationInfo struct {
	ID             string       `json:"id"`
	Title          string       `json:"title"`
	Summary        string       `json:"summary,omitempty"`
	Domain         string       `json:"domain"`
	Source         string       `json:"source"`
	Batchable      bool         `json:"batchable"`
	Cost           string       `json:"cost,omitempty"`
	Engines        []string     `json:"engines,omitempty"`
	RequireEngines bool         `json:"require_engines,omitempty"`
	Mode           string       `json:"mode,omitempty"`
	Accepts        []acceptInfo `json:"accepts,omitempty"`
	Params         []paramInfo  `json:"params,omitempty"`
	Output         outputInfo   `json:"output"`
}

type acceptInfo struct {
	Kind    string   `json:"kind"`
	Formats []string `json:"formats,omitempty"`
}

type optionInfo struct {
	Value string `json:"value"`
	Label string `json:"label,omitempty"`
}

type paramInfo struct {
	Key      string       `json:"key"`
	Label    string       `json:"label,omitempty"`
	Type     string       `json:"type"`
	Default  any          `json:"default,omitempty"`
	Options  []optionInfo `json:"options,omitempty"`
	Required bool         `json:"required,omitempty"`
	Advanced bool         `json:"advanced,omitempty"`
	Min      *float64     `json:"min,omitempty"`
	Max      *float64     `json:"max,omitempty"`
	Help     string       `json:"help,omitempty"`
}

type outputInfo struct {
	Kind         string `json:"kind,omitempty"`
	FormatParam  string `json:"format_param,omitempty"`
	StaticFormat string `json:"static_format,omitempty"`
}

type engineInfo struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Path      string `json:"path,omitempty"`
	Version   string `json:"version,omitempty"`
	Available bool   `json:"available"`
	Detail    string `json:"detail,omitempty"`
	Hint      string `json:"hint,omitempty"`
	Error     string `json:"error,omitempty"`
}

type pipelineInfo struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Steps       int    `json:"steps"`
	Error       string `json:"error,omitempty"`
}

type pipelineDetail struct {
	Name        string     `json:"name"`
	Version     int        `json:"version"`
	Description string     `json:"description,omitempty"`
	Steps       []stepInfo `json:"steps"`
}

type stepInfo struct {
	ID     string         `json:"id,omitempty"`
	Op     string         `json:"operation"`
	Params map[string]any `json:"params,omitempty"`
}

type jobInfo struct {
	ID         string         `json:"id"`
	Name       string         `json:"name"`
	Status     string         `json:"status"`
	Percent    int            `json:"percent"`
	QueuedAt   time.Time      `json:"queued_at"`
	StartedAt  *time.Time     `json:"started_at,omitempty"`
	FinishedAt *time.Time     `json:"finished_at,omitempty"`
	DurationMS int64          `json:"duration_ms"`
	Counts     countsInfo     `json:"counts"`
	Steps      []stepState    `json:"steps,omitempty"`
	Items      []itemInfo     `json:"items,omitempty"`
	Error      *errorInfo     `json:"error,omitempty"`
	Metrics    metricsInfo    `json:"metrics"`
	Target     targetInfo     `json:"target"`
	Aggregate  []artifactInfo `json:"aggregate,omitempty"`
	Logs       []string       `json:"logs,omitempty"`
}

type countsInfo struct {
	Total    int `json:"total"`
	Done     int `json:"done"`
	Failed   int `json:"failed"`
	Skipped  int `json:"skipped"`
	Canceled int `json:"canceled"`
	Active   int `json:"active"`
}

type stepState struct {
	ID     string `json:"id"`
	Op     string `json:"operation"`
	Title  string `json:"title,omitempty"`
	Index  int    `json:"index"`
	Status string `json:"status"`
	Error  string `json:"error,omitempty"`
}

type itemInfo struct {
	ID      string         `json:"id"`
	Input   artifactInfo   `json:"input"`
	Current artifactInfo   `json:"current"`
	Status  string         `json:"status"`
	Stage   string         `json:"stage,omitempty"`
	Percent int            `json:"percent"`
	Step    int            `json:"step"`
	Outputs []artifactInfo `json:"outputs,omitempty"`
	Error   *errorInfo     `json:"error,omitempty"`
}

type artifactInfo struct {
	Name          string         `json:"name"`
	Path          string         `json:"path"`
	Kind          string         `json:"kind"`
	Format        string         `json:"format,omitempty"`
	Size          int64          `json:"size,omitempty"`
	SuggestedName string         `json:"suggested_name,omitempty"`
	Meta          map[string]any `json:"meta,omitempty"`
}

type metricsInfo struct {
	BytesIn    int64  `json:"bytes_in,omitempty"`
	BytesOut   int64  `json:"bytes_out,omitempty"`
	DurationMS int64  `json:"duration_ms,omitempty"`
	Engine     string `json:"engine,omitempty"`
	Model      string `json:"model,omitempty"`
}

type targetInfo struct {
	Dir       string `json:"dir"`
	Template  string `json:"template,omitempty"`
	Collision string `json:"collision,omitempty"`
}

func operationDTO(op core.Operation) operationInfo {
	spec := op.Spec()
	out := operationInfo{
		ID:             spec.ID,
		Title:          spec.Title,
		Summary:        spec.Summary,
		Domain:         string(spec.Domain),
		Source:         string(spec.SourceKind()),
		Batchable:      spec.Batchable,
		Cost:           string(spec.Cost),
		RequireEngines: spec.RequireEngines,
		Mode:           string(spec.Mode),
		Output: outputInfo{
			Kind:         string(spec.OutputKind()),
			FormatParam:  spec.Output.FormatParam,
			StaticFormat: spec.Output.StaticFormat,
		},
	}
	for _, accept := range spec.Accepts {
		out.Accepts = append(out.Accepts, acceptInfo{Kind: string(accept.Kind), Formats: accept.Formats})
	}
	for _, engine := range spec.Engines {
		out.Engines = append(out.Engines, string(engine))
	}
	for _, param := range spec.Params {
		p := paramInfo{
			Key: param.Key, Label: param.Label, Type: string(param.Type),
			Default: param.Default, Required: param.Required, Advanced: param.Advanced,
			Min: param.Min, Max: param.Max, Help: param.Help,
		}
		for _, option := range param.Options {
			p.Options = append(p.Options, optionInfo{Value: option.Value, Label: option.Label})
		}
		out.Params = append(out.Params, p)
	}
	return out
}

func engineDTO(info engines.Info) engineInfo {
	out := engineInfo{
		ID: string(info.ID), Name: info.Name, Path: info.Path, Version: info.Version,
		Available: info.Available, Detail: info.Detail, Hint: info.Hint,
	}
	if info.Err != nil {
		out.Error = info.Err.Error()
	}
	return out
}

func pipelineDTO(meta pipeline.Meta) pipelineInfo {
	out := pipelineInfo{Name: meta.Name, Description: meta.Description, Steps: meta.Steps}
	if meta.Err != nil {
		out.Error = meta.Err.Error()
	}
	return out
}

func pipelineDetailDTO(p core.Pipeline) pipelineDetail {
	out := pipelineDetail{Name: p.Name, Version: p.Version, Description: p.Description}
	for _, step := range p.Steps {
		out.Steps = append(out.Steps, stepInfo{ID: step.ID, Op: step.Op, Params: step.Params})
	}
	return out
}

func jobDTO(view runner.JobView, logs bool) jobInfo {
	counts := view.Counts()
	out := jobInfo{
		ID:         view.ID,
		Name:       view.Name,
		Status:     string(view.Status),
		Percent:    view.Percent(),
		QueuedAt:   view.QueuedAt,
		StartedAt:  timePtr(view.StartedAt),
		FinishedAt: timePtr(view.FinishedAt),
		DurationMS: view.Duration().Milliseconds(),
		Counts: countsInfo{
			Total: counts.Total, Done: counts.Done, Failed: counts.Failed,
			Skipped: counts.Skipped, Canceled: counts.Canceled, Active: counts.Active,
		},
		Metrics: metricsInfo{
			BytesIn: view.Metrics.BytesIn, BytesOut: view.Metrics.BytesOut,
			DurationMS: view.Metrics.Duration.Milliseconds(),
			Engine:     string(view.Metrics.Engine),
			Model:      view.Metrics.Model,
		},
		Target: targetInfo{Dir: view.Target.Dir, Template: view.Target.Template, Collision: string(view.Target.Collision)},
	}
	if view.Err != nil {
		out.Error = coreErrorInfo(view.Err)
	}
	for _, step := range view.Steps {
		st := stepState{
			ID: step.ID, Op: step.Op, Title: step.Title, Index: step.Index, Status: string(step.Status),
		}
		if step.Err != nil {
			st.Error = step.Err.Error()
		}
		out.Steps = append(out.Steps, st)
	}
	for _, artifact := range view.Aggregate {
		out.Aggregate = append(out.Aggregate, artifactDTO(artifact))
	}
	for _, item := range view.Items {
		it := itemInfo{
			ID:      item.ID,
			Input:   artifactDTO(item.Input),
			Current: artifactDTO(item.Current),
			Status:  string(item.Status),
			Stage:   item.Stage,
			Step:    item.Step,
			Percent: itemPercent(item),
		}
		if item.Err != nil {
			it.Error = coreErrorInfo(item.Err)
		}
		for _, output := range item.Outputs {
			it.Outputs = append(it.Outputs, artifactDTO(output))
		}
		out.Items = append(out.Items, it)
	}
	if logs {
		out.Logs = view.Logs
		if len(out.Logs) == 0 && view.LogPath != "" {
			if data, err := os.ReadFile(view.LogPath); err == nil {
				out.Logs = strings.Split(strings.TrimRight(string(data), "\n"), "\n")
			}
		}
	}
	return out
}

func itemPercent(item runner.ItemView) int {
	if item.Status == core.ItemStatusDone || item.Status == core.ItemStatusSkipped {
		return 100
	}
	if item.Total <= 0 {
		return 0
	}
	p := item.Done * 100 / item.Total
	if p < 0 {
		return 0
	}
	if p > 100 {
		return 100
	}
	return p
}

func artifactDTO(a core.Artifact) artifactInfo {
	return artifactInfo{
		Name: a.Name(), Path: a.Path, Kind: string(a.Kind), Format: a.Format,
		Size: a.Size, SuggestedName: a.SuggestedName, Meta: a.Meta,
	}
}

func coreErrorInfo(err error) *errorInfo {
	var e *core.Error
	if errors.As(err, &e) {
		return &errorInfo{Code: string(e.Code), Message: e.Message, Hint: e.Hint, Path: e.Path, Op: e.Op}
	}
	return &errorInfo{Code: string(core.CodeOf(err)), Message: err.Error()}
}

func timePtr(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	return &t
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, errorBody{Error: *coreErrorInfo(err)})
}

func statusFor(err error) int {
	switch core.CodeOf(err) {
	case core.CodeUsage, core.CodeInput:
		return http.StatusBadRequest
	case core.CodeEngineMissing:
		return http.StatusConflict
	case core.CodeEngineFailed:
		return http.StatusBadGateway
	case core.CodeOutput:
		return http.StatusUnprocessableEntity
	case core.CodeCanceled:
		return http.StatusConflict
	default:
		return http.StatusInternalServerError
	}
}
