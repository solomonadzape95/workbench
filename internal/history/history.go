// Package history persists finished jobs as an append-only JSONL file. It is
// deliberately a log, not a database: one line per job, newest at the end,
// rotated at 5 MB. The runner owns the live state; this package only records
// what already happened.
package history

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/solomonadzape95/workbench/internal/core"
	"github.com/solomonadzape95/workbench/internal/runner"
)

const defaultMaxBytes int64 = 5 << 20

type Store struct {
	Path     string
	MaxBytes int64

	mu sync.Mutex
}

func New(dir string) *Store {
	return &Store{
		Path:     filepath.Join(dir, "history.jsonl"),
		MaxBytes: defaultMaxBytes,
	}
}

// Append records one finished job. A failure is reported to the caller, but
// the runner treats history as best-effort and never fails a job over it.
func (s *Store) Append(rec runner.Record) error {
	if s == nil || s.Path == "" {
		return nil
	}
	disk := toDisk(rec)
	line, err := json.Marshal(disk)
	if err != nil {
		return err
	}
	line = append(line, '\n')

	s.mu.Lock()
	defer s.mu.Unlock()

	if err := os.MkdirAll(filepath.Dir(s.Path), 0o755); err != nil {
		return err
	}
	max := s.MaxBytes
	if max <= 0 {
		max = defaultMaxBytes
	}
	if info, err := os.Stat(s.Path); err == nil && info.Size()+int64(len(line)) > max {
		old := s.Path + ".1"
		_ = os.Remove(old)
		if err := os.Rename(s.Path, old); err != nil {
			return err
		}
	}

	f, err := os.OpenFile(s.Path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err := f.Write(line); err != nil {
		return err
	}
	return f.Sync()
}

// Recent returns up to n records, newest first. Malformed lines are skipped so
// one bad append cannot hide the rest of the history.
func (s *Store) Recent(n int) ([]runner.Record, error) {
	if s == nil || s.Path == "" {
		return nil, nil
	}
	var out []runner.Record
	for _, path := range []string{s.Path + ".1", s.Path} {
		records, err := readRecords(path)
		if err != nil {
			return nil, err
		}
		out = append(out, records...)
	}
	if n > 0 && len(out) > n {
		out = out[len(out)-n:]
	}
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out, nil
}

func readRecords(path string) ([]runner.Record, error) {
	f, err := os.Open(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var out []runner.Record
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 64*1024), 16<<20)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var disk diskRecord
		if err := json.Unmarshal([]byte(line), &disk); err != nil {
			continue
		}
		out = append(out, fromDisk(disk))
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *Store) Clear() error {
	if s == nil || s.Path == "" {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	var errs []error
	for _, path := range []string{s.Path, s.Path + ".1"} {
		if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

type diskRecord struct {
	Version int      `json:"version"`
	View    diskView `json:"view"`
}

type diskView struct {
	ID         string          `json:"id"`
	Name       string          `json:"name"`
	Status     core.Status     `json:"status"`
	QueuedAt   time.Time       `json:"queued_at"`
	StartedAt  time.Time       `json:"started_at"`
	FinishedAt time.Time       `json:"finished_at"`
	Metrics    core.Metrics    `json:"metrics"`
	Target     core.Target     `json:"target"`
	Pipeline   core.Pipeline   `json:"pipeline"`
	Aggregate  []core.Artifact `json:"aggregate,omitempty"`
	Items      []diskItem      `json:"items,omitempty"`
	Steps      []diskStep      `json:"steps,omitempty"`
	Samples    []int           `json:"samples,omitempty"`
	Err        string          `json:"error,omitempty"`
	Hint       string          `json:"hint,omitempty"`
	LogPath    string          `json:"log_path,omitempty"`
}

type diskItem struct {
	ID      string          `json:"id"`
	Input   core.Artifact   `json:"input"`
	Current core.Artifact   `json:"current"`
	Status  core.ItemStatus `json:"status"`
	Stage   string          `json:"stage,omitempty"`
	Done    int             `json:"done,omitempty"`
	Total   int             `json:"total,omitempty"`
	Outputs []core.Artifact `json:"outputs,omitempty"`
	Err     string          `json:"error,omitempty"`
	Step    int             `json:"step,omitempty"`
}

type diskStep struct {
	ID     string      `json:"id"`
	Op     string      `json:"op"`
	Title  string      `json:"title,omitempty"`
	Index  int         `json:"index"`
	Status core.Status `json:"status"`
	Err    string      `json:"error,omitempty"`
}

func toDisk(rec runner.Record) diskRecord {
	v := rec.View
	out := diskRecord{
		Version: rec.Version,
		View: diskView{
			ID:         v.ID,
			Name:       v.Name,
			Status:     v.Status,
			QueuedAt:   v.QueuedAt,
			StartedAt:  v.StartedAt,
			FinishedAt: v.FinishedAt,
			Metrics:    v.Metrics,
			Target:     v.Target,
			Pipeline:   v.Pipeline,
			Aggregate:  append([]core.Artifact(nil), v.Aggregate...),
			Samples:    append([]int(nil), v.Samples...),
			LogPath:    v.LogPath,
		},
	}
	if v.Err != nil {
		out.View.Err = v.Err.Error()
		out.View.Hint = core.HintOf(v.Err)
	}
	for _, it := range v.Items {
		item := diskItem{
			ID: it.ID, Input: it.Input, Current: it.Current, Status: it.Status,
			Stage: it.Stage, Done: it.Done, Total: it.Total,
			Outputs: append([]core.Artifact(nil), it.Outputs...), Step: it.Step,
		}
		if it.Err != nil {
			item.Err = it.Err.Error()
		}
		out.View.Items = append(out.View.Items, item)
	}
	for _, st := range v.Steps {
		step := diskStep{
			ID: st.ID, Op: st.Op, Title: st.Title, Index: st.Index, Status: st.Status,
		}
		if st.Err != nil {
			step.Err = st.Err.Error()
		}
		out.View.Steps = append(out.View.Steps, step)
	}
	return out
}

func fromDisk(d diskRecord) runner.Record {
	v := runner.JobView{
		ID:         d.View.ID,
		Name:       d.View.Name,
		Status:     d.View.Status,
		QueuedAt:   d.View.QueuedAt,
		StartedAt:  d.View.StartedAt,
		FinishedAt: d.View.FinishedAt,
		Metrics:    d.View.Metrics,
		Target:     d.View.Target,
		Pipeline:   normalizePipeline(d.View.Pipeline),
		Aggregate:  append([]core.Artifact(nil), d.View.Aggregate...),
		Samples:    append([]int(nil), d.View.Samples...),
		LogPath:    d.View.LogPath,
		Historical: true,
	}
	if d.View.Err != "" {
		v.Err = core.NewError(core.CodeInternal, "", "", d.View.Err, d.View.Hint)
	}
	for _, it := range d.View.Items {
		item := runner.ItemView{
			ID: it.ID, Input: it.Input, Current: it.Current, Status: it.Status,
			Stage: it.Stage, Done: it.Done, Total: it.Total,
			Outputs: append([]core.Artifact(nil), it.Outputs...), Step: it.Step,
		}
		if it.Err != "" {
			item.Err = errors.New(it.Err)
		}
		v.Items = append(v.Items, item)
	}
	for _, st := range d.View.Steps {
		step := runner.StepView{
			ID: st.ID, Op: st.Op, Title: st.Title, Index: st.Index, Status: st.Status,
		}
		if st.Err != "" {
			step.Err = errors.New(st.Err)
		}
		v.Steps = append(v.Steps, step)
	}
	return runner.Record{Version: d.Version, View: v}
}

// normalizePipeline repairs JSON's lossy number decoding. A ParamInt written as
// 12 comes back as float64(12); without this, re-running an old pipeline would
// fail validation.
func normalizePipeline(p core.Pipeline) core.Pipeline {
	for i := range p.Steps {
		p.Steps[i].Params = normalizeValues(p.Steps[i].Params)
	}
	return p
}

func normalizeValues(in core.Values) core.Values {
	if in == nil {
		return nil
	}
	out := make(core.Values, len(in))
	for key, value := range in {
		out[key] = normalizeValue(value)
	}
	return out
}

func normalizeValue(value any) any {
	switch v := value.(type) {
	case float64:
		if v == float64(int64(v)) {
			return int(v)
		}
		return v
	case float32:
		return normalizeValue(float64(v))
	case map[string]any:
		for key, item := range v {
			v[key] = normalizeValue(item)
		}
		return v
	case []any:
		for i, item := range v {
			v[i] = normalizeValue(item)
		}
		return v
	default:
		return value
	}
}

// PathFor is a small convenience for callers that want to show the file path.
func (s *Store) PathFor() string {
	if s == nil {
		return ""
	}
	return s.Path
}

// String makes a zero store safe to log without a nil panic.
func (s *Store) String() string {
	if s == nil {
		return "history: disabled"
	}
	return fmt.Sprintf("history: %s", s.Path)
}
