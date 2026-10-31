package runner

import (
	"context"
	"math"
	"strings"
	"sync"
	"time"

	"github.com/solomonadzape95/workbench/internal/core"
)

// Job is the live state of one pipeline execution. Its exported methods are
// safe to call from the TUI while the job runs.
type Job struct {
	id       string
	name     string
	pipeline core.Pipeline
	target   core.Target

	ctx    context.Context
	cancel context.CancelFunc

	mu         sync.RWMutex
	status     core.Status
	items      []*item
	steps      []*stepState
	err        error
	queuedAt   time.Time
	startedAt  time.Time
	finishedAt time.Time
	metrics    core.Metrics
	tempDir    string
	logPath    string
	logs       []string
	samples    []int
	aggregate  []core.Artifact
}

type item struct {
	id      string
	index   int
	input   core.Artifact
	current core.Artifact
	status  core.ItemStatus
	stage   string
	done    int
	total   int
	outputs []core.Artifact
	err     error
	step    int
}

type stepState struct {
	id       string
	op       string
	title    string
	index    int
	status   core.Status
	err      error
	started  time.Time
	finished time.Time
}

type ItemView struct {
	ID      string
	Input   core.Artifact
	Current core.Artifact
	Status  core.ItemStatus
	Stage   string
	Done    int
	Total   int
	Outputs []core.Artifact
	Err     error
	Step    int
}

type StepView struct {
	ID     string
	Op     string
	Title  string
	Index  int
	Status core.Status
	Err    error
}

type JobView struct {
	ID         string
	Name       string
	Status     core.Status
	Items      []ItemView
	Steps      []StepView
	Err        error
	QueuedAt   time.Time
	StartedAt  time.Time
	FinishedAt time.Time
	Metrics    core.Metrics
	Logs       []string
	Samples    []int
	Target     core.Target
	Aggregate  []core.Artifact

	// Historical is true when the view was loaded from disk instead of the
	// live runner. LogPath points at the persisted log for those views.
	Historical bool
	LogPath    string
	Pipeline   core.Pipeline
}

type Counts struct {
	Done     int
	Failed   int
	Skipped  int
	Canceled int
	Active   int
	Total    int
}

func (v JobView) Counts() Counts {
	var c Counts
	c.Total = len(v.Items)
	for _, it := range v.Items {
		switch it.Status {
		case core.ItemStatusDone:
			c.Done++
		case core.ItemStatusFailed:
			c.Failed++
		case core.ItemStatusSkipped:
			c.Skipped++
		case core.ItemStatusCanceled:
			c.Canceled++
		default:
			c.Active++
		}
	}
	return c
}

func (v JobView) Settled() int {
	c := v.Counts()
	return c.Done + c.Failed + c.Skipped + c.Canceled
}

// Percent is the aggregate visual progress. Terminal items count as complete;
// running items contribute their own fraction; failed and cancelled items keep
// the fraction they reached so a cancelled job doesn't snap to 100%.
func (v JobView) Percent() int {
	if len(v.Items) == 0 {
		return 0
	}
	sum := 0.0
	for _, it := range v.Items {
		switch it.Status {
		case core.ItemStatusDone, core.ItemStatusSkipped:
			sum += 1
		case core.ItemStatusRunning, core.ItemStatusFailed, core.ItemStatusCanceled:
			if it.Total > 0 {
				sum += math.Min(float64(it.Done)/float64(it.Total), 1)
			} else if it.Status == core.ItemStatusFailed || it.Status == core.ItemStatusCanceled {
				sum += 1
			}
		}
	}
	return int(math.Round(sum / float64(len(v.Items)) * 100))
}

func (v JobView) Duration() time.Duration {
	if v.StartedAt.IsZero() {
		return 0
	}
	end := v.FinishedAt
	if end.IsZero() {
		end = time.Now()
	}
	return end.Sub(v.StartedAt)
}

func (j *Job) ID() string   { return j.id }
func (j *Job) Name() string { return j.name }

func (j *Job) Status() core.Status {
	j.mu.RLock()
	defer j.mu.RUnlock()
	return j.status
}

// Cancel asks the job to stop. Cancellation is cooperative: operations see a
// canceled context and the runner removes the job's temp directory.
func (j *Job) Cancel() {
	j.mu.Lock()
	if j.status == core.StatusQueued || j.status == core.StatusRunning {
		j.status = core.StatusCancelling
	}
	cancel := j.cancel
	j.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}

func (j *Job) Snapshot() JobView {
	j.mu.RLock()
	defer j.mu.RUnlock()

	view := JobView{
		ID:         j.id,
		Name:       j.name,
		Status:     j.status,
		Err:        j.err,
		QueuedAt:   j.queuedAt,
		StartedAt:  j.startedAt,
		FinishedAt: j.finishedAt,
		Metrics:    j.metrics,
		Pipeline:   j.pipeline,
		LogPath:    j.logPath,
	}
	view.Items = make([]ItemView, len(j.items))
	for i, it := range j.items {
		view.Items[i] = ItemView{
			ID:      it.id,
			Input:   it.input,
			Current: it.current,
			Status:  it.status,
			Stage:   it.stage,
			Done:    it.done,
			Total:   it.total,
			Outputs: append([]core.Artifact(nil), it.outputs...),
			Err:     it.err,
			Step:    it.step,
		}
	}
	view.Steps = make([]StepView, len(j.steps))
	for i, st := range j.steps {
		view.Steps[i] = StepView{
			ID: st.id, Op: st.op, Title: st.title, Index: st.index,
			Status: st.status, Err: st.err,
		}
	}
	view.Logs = append([]string(nil), j.logs...)
	view.Samples = append([]int(nil), j.samples...)
	view.Target = j.target
	view.Aggregate = append([]core.Artifact(nil), j.aggregate...)
	return view
}

func (j *Job) activeItems() []*item {
	j.mu.RLock()
	defer j.mu.RUnlock()
	var out []*item
	for _, it := range j.items {
		if it.current.Path == "" {
			continue
		}
		switch it.status {
		case core.ItemStatusFailed, core.ItemStatusSkipped, core.ItemStatusCanceled:
			continue
		}
		out = append(out, it)
	}
	return out
}

func (j *Job) setRunning(at time.Time) {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.status = core.StatusRunning
	j.startedAt = at
}

func (j *Job) startStep(index int, id, op, title string) {
	j.mu.Lock()
	defer j.mu.Unlock()
	t := time.Now()
	j.steps = append(j.steps, &stepState{
		id: id, op: op, title: title, index: index,
		status: core.StatusRunning, started: t,
	})
	for _, it := range j.items {
		if it.current.Path == "" || it.status == core.ItemStatusFailed ||
			it.status == core.ItemStatusSkipped || it.status == core.ItemStatusCanceled {
			continue
		}
		it.status = core.ItemStatusRunning
		it.stage = ""
		it.done = 0
		it.total = 0
		it.step = index + 1
	}
}

func (j *Job) finishStep(index int, err error) {
	j.mu.Lock()
	defer j.mu.Unlock()
	if index < 0 || index >= len(j.steps) {
		return
	}
	st := j.steps[index]
	st.err = err
	if err != nil {
		st.status = core.StatusFailed
	} else {
		st.status = core.StatusDone
	}
	st.finished = time.Now()
}

func (j *Job) setProgress(it *item, done, total int, stage string) {
	j.mu.Lock()
	defer j.mu.Unlock()
	it.done = done
	it.total = total
	if stage != "" {
		it.stage = stage
	}
	j.recordSampleLocked()
}

func (j *Job) setItemStatus(it *item, status core.ItemStatus, err error) {
	j.mu.Lock()
	defer j.mu.Unlock()
	it.status = status
	it.err = err
	if status == core.ItemStatusDone || status == core.ItemStatusSkipped || status == core.ItemStatusFailed {
		if it.total > 0 {
			it.done = it.total
		}
	}
	j.recordSampleLocked()
}

func (j *Job) setItemOutputs(it *item, outputs []core.Artifact) {
	j.mu.Lock()
	defer j.mu.Unlock()
	it.outputs = append([]core.Artifact(nil), outputs...)
}

func (j *Job) setItemCurrent(it *item, art core.Artifact) {
	j.mu.Lock()
	defer j.mu.Unlock()
	it.current = art
}

func (j *Job) setAggregateOutputs(outputs []core.Artifact) {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.aggregate = append([]core.Artifact(nil), outputs...)
}

func (j *Job) setMetrics(m core.Metrics) {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.metrics.BytesIn += m.BytesIn
	j.metrics.BytesOut += m.BytesOut
	if m.Engine != "" {
		j.metrics.Engine = m.Engine
	}
	if m.Model != "" {
		j.metrics.Model = m.Model
	}
}

func (j *Job) fail(err error) {
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.err == nil {
		j.err = err
	}
}

func (j *Job) setTempDir(dir string) {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.tempDir = dir
}

func (j *Job) setLogPath(path string) {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.logPath = path
}

// finalize computes the terminal job status and duration.
func (j *Job) finalize(ctx context.Context) (core.Status, time.Duration, core.Metrics, error) {
	j.mu.Lock()
	defer j.mu.Unlock()

	var done, failed, skipped, active int
	for _, it := range j.items {
		switch it.status {
		case core.ItemStatusDone:
			done++
		case core.ItemStatusSkipped:
			skipped++
		case core.ItemStatusFailed, core.ItemStatusCanceled:
			failed++
		default:
			active++
		}
	}

	switch {
	case ctx.Err() != nil:
		j.status = core.StatusCanceled
	case failed == 0 && active == 0 && done+skipped > 0:
		j.status = core.StatusDone
	case done+skipped > 0:
		j.status = core.StatusPartial
	default:
		j.status = core.StatusFailed
	}

	j.finishedAt = time.Now()
	duration := j.finishedAt.Sub(j.startedAt)
	if j.startedAt.IsZero() {
		duration = 0
	}
	j.metrics.Duration = duration
	return j.status, duration, j.metrics, j.err
}

func (j *Job) addLog(level core.Level, message string) {
	message = strings.TrimRight(message, "\n")
	if message == "" {
		return
	}
	prefix := time.Now().Format("15:04:05") + " " + strings.ToUpper(string(level)) + " "
	j.mu.Lock()
	defer j.mu.Unlock()
	for _, line := range strings.Split(message, "\n") {
		j.logs = append(j.logs, prefix+line)
	}
	const maxLogLines = 2000
	if len(j.logs) > maxLogLines {
		j.logs = append([]string(nil), j.logs[len(j.logs)-maxLogLines:]...)
	}
}

func (j *Job) logText() string {
	j.mu.RLock()
	defer j.mu.RUnlock()
	if len(j.logs) == 0 {
		return ""
	}
	return strings.Join(j.logs, "\n") + "\n"
}

// recordSampleLocked appends the current aggregate percent when it changes.
func (j *Job) recordSampleLocked() {
	if len(j.items) == 0 {
		return
	}
	sum := 0.0
	for _, it := range j.items {
		switch it.status {
		case core.ItemStatusDone, core.ItemStatusSkipped:
			sum += 1
		case core.ItemStatusRunning, core.ItemStatusFailed, core.ItemStatusCanceled:
			if it.total > 0 {
				sum += math.Min(float64(it.done)/float64(it.total), 1)
			} else if it.status == core.ItemStatusFailed || it.status == core.ItemStatusCanceled {
				sum += 1
			}
		}
	}
	percent := int(math.Round(sum / float64(len(j.items)) * 100))
	if len(j.samples) > 0 && j.samples[len(j.samples)-1] == percent {
		return
	}
	j.samples = append(j.samples, percent)
	if len(j.samples) > 48 {
		j.samples = append([]int(nil), j.samples[len(j.samples)-48:]...)
	}
}

func (j *Job) cancelIfQueued() {
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.status == core.StatusQueued {
		j.status = core.StatusCanceled
		j.finishedAt = time.Now()
	}
}
