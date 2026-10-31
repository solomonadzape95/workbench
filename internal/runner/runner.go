// Package runner executes pipelines. It owns job lifecycle, concurrency,
// progress events, temp workspaces, and atomic output commits.
package runner

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"github.com/solomonadzape95/workbench/internal/core"
)

type Options struct {
	Operations   *core.Registry
	CacheDir     string
	LogDir       string
	UndoDir      string
	MaxJobs      int
	EngineLimits map[string]int
	History      Recorder
}

// Record is one finished job, in the shape the history store persists. The
// runner owns it so the runner never imports the storage package.
type Record struct {
	Version int
	View    JobView
}

// Recorder is implemented by the history store. A nil recorder is fine.
type Recorder interface {
	Append(Record) error
}

type Runner struct {
	ops      *core.Registry
	cacheDir string
	logDir   string
	undoDir  string
	history  Recorder

	events   chan core.Event
	queue    chan *Job
	jobSlots *semaphore

	engineMu    sync.Mutex
	engineSlots map[core.EngineID]*semaphore
	limits      map[core.EngineID]int

	// commitMu serializes output naming and rename across all jobs so two
	// jobs cannot pick the same destination.
	commitMu sync.Mutex

	ctx    context.Context
	cancel context.CancelFunc

	mu     sync.Mutex
	jobs   map[string]*Job
	order  []string
	closed bool

	seq atomic.Uint64

	wg         sync.WaitGroup // running jobs
	dispatchWG sync.WaitGroup
	closeOnce  sync.Once
}

func New(opts Options) *Runner {
	if opts.Operations == nil {
		opts.Operations = core.NewRegistry()
	}
	if opts.UndoDir == "" && opts.CacheDir != "" {
		opts.UndoDir = filepath.Join(opts.CacheDir, "undo")
	}
	ctx, cancel := context.WithCancel(context.Background())

	r := &Runner{
		ops:         opts.Operations,
		cacheDir:    opts.CacheDir,
		logDir:      opts.LogDir,
		undoDir:     opts.UndoDir,
		history:     opts.History,
		events:      make(chan core.Event, 1024),
		queue:       make(chan *Job, 256),
		engineSlots: map[core.EngineID]*semaphore{},
		limits:      map[core.EngineID]int{},
		ctx:         ctx,
		cancel:      cancel,
		jobs:        map[string]*Job{},
	}
	for name, limit := range opts.EngineLimits {
		r.limits[core.EngineID(name)] = limit
	}
	slotCount := opts.MaxJobs
	if slotCount <= 0 {
		slotCount = 0 // unlimited
	}
	r.jobSlots = newSemaphore(slotCount)

	r.dispatchWG.Add(1)
	go r.dispatch()

	return r
}

// Start validates the pipeline against the inputs, registers a queued job, and
// returns immediately. Execution happens on the runner's goroutines.
func (r *Runner) Start(p core.Pipeline, inputs []core.Artifact, target core.Target) (*Job, error) {
	if len(inputs) == 0 {
		return nil, core.NewError(core.CodeUsage, "", "", "no input files",
			"pick files with a path, glob, or alias")
	}
	if err := p.Validate(r.ops); err != nil {
		return nil, err
	}

	target = normalizeTarget(target)
	if target.Dir == "" {
		return nil, core.NewError(core.CodeUsage, "", "", "no output directory", "")
	}

	first, _ := r.ops.Get(p.Steps[0].Op)
	firstParams := mergeParams(first.Spec().DefaultValues(), p.Steps[0].Params)
	if err := first.Validate(r.ctx, core.Request{Items: inputs, Params: firstParams, Target: target}); err != nil {
		return nil, err
	}

	jobCtx, cancel := context.WithCancel(r.ctx)
	job := newJob(r.nextID(), p, inputs, target, jobCtx, cancel)

	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		cancel()
		return nil, errors.New("runner is closed")
	}
	r.jobs[job.id] = job
	r.order = append(r.order, job.id)
	r.mu.Unlock()

	r.emit(core.JobQueued{JobID: job.id, Name: job.name, Items: len(inputs)})

	select {
	case r.queue <- job:
	case <-r.ctx.Done():
		cancel()
		return nil, context.Canceled
	}
	return job, nil
}

func newJob(id string, p core.Pipeline, inputs []core.Artifact, target core.Target, ctx context.Context, cancel context.CancelFunc) *Job {
	name := p.Name
	if name == "" && len(p.Steps) > 0 {
		name = p.Steps[0].Op
	}
	job := &Job{
		id:       id,
		name:     name,
		pipeline: p,
		target:   target,
		ctx:      ctx,
		cancel:   cancel,
		status:   core.StatusQueued,
		queuedAt: time.Now(),
	}
	for i, in := range inputs {
		job.items = append(job.items, &item{
			id:      fmt.Sprintf("item-%d", i+1),
			index:   i,
			input:   in,
			current: in,
			status:  core.ItemStatusPending,
		})
	}
	return job
}

func (r *Runner) nextID() string {
	return fmt.Sprintf("job-%d-%03d", time.Now().UnixNano(), r.seq.Add(1))
}

func normalizeTarget(t core.Target) core.Target {
	if t.Collision == "" {
		t.Collision = core.CollisionSuffix
	}
	if t.Template == "" {
		t.Template = "{stem}.{ext}"
	}
	return t
}

// Events returns the stream of job events. The channel closes when the runner
// is closed.
func (r *Runner) Events() <-chan core.Event { return r.events }

// Jobs returns snapshots, newest first.
func (r *Runner) Jobs() []JobView {
	r.mu.Lock()
	ids := append([]string(nil), r.order...)
	live := make([]*Job, 0, len(ids))
	for _, id := range ids {
		if job, ok := r.jobs[id]; ok {
			live = append(live, job)
		}
	}
	r.mu.Unlock()

	out := make([]JobView, 0, len(live))
	for i := len(live) - 1; i >= 0; i-- {
		out = append(out, live[i].Snapshot())
	}
	return out
}

func (r *Runner) Job(id string) (*Job, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	job, ok := r.jobs[id]
	return job, ok
}

// RetryFailed starts a new job using only the failed or cancelled items.
func (r *Runner) RetryFailed(id string) (*Job, error) {
	job, ok := r.Job(id)
	if !ok {
		return nil, errors.New("job not found")
	}
	view := job.Snapshot()
	if isActiveStatus(view.Status) {
		return nil, errors.New("job is still running")
	}
	var inputs []core.Artifact
	for _, item := range view.Items {
		if item.Status == core.ItemStatusFailed || item.Status == core.ItemStatusCanceled {
			inputs = append(inputs, item.Input)
		}
	}
	if len(inputs) == 0 {
		return nil, errors.New("this job has no failed items")
	}
	pipeline := job.pipeline
	pipeline.Name = view.Name + " retry"
	return r.Start(pipeline, inputs, view.Target)
}

// Rerun starts the same pipeline again with all original inputs.
func (r *Runner) Rerun(id string) (*Job, error) {
	job, ok := r.Job(id)
	if !ok {
		return nil, errors.New("job not found")
	}
	view := job.Snapshot()
	if isActiveStatus(view.Status) {
		return nil, errors.New("job is still running")
	}
	inputs := make([]core.Artifact, 0, len(view.Items))
	for _, item := range view.Items {
		inputs = append(inputs, item.Input)
	}
	if len(inputs) == 0 {
		return nil, errors.New("this job has no inputs")
	}
	return r.Start(job.pipeline, inputs, view.Target)
}

func isActiveStatus(status core.Status) bool {
	return status == core.StatusQueued || status == core.StatusRunning || status == core.StatusCancelling
}

func (r *Runner) Cancel(id string) bool {
	job, ok := r.Job(id)
	if ok {
		job.Cancel()
	}
	return ok
}

func (r *Runner) Close() {
	r.closeOnce.Do(func() {
		r.mu.Lock()
		r.closed = true
		r.mu.Unlock()

		r.cancel()
		r.dispatchWG.Wait()
		r.wg.Wait()

		r.mu.Lock()
		for _, job := range r.jobs {
			job.cancelIfQueued()
		}
		r.mu.Unlock()

		close(r.events)
	})
}

func (r *Runner) dispatch() {
	defer r.dispatchWG.Done()
	for {
		select {
		case <-r.ctx.Done():
			return
		case job := <-r.queue:
			if err := r.jobSlots.acquire(r.ctx); err != nil {
				return
			}
			r.wg.Add(1)
			go func() {
				defer r.wg.Done()
				defer r.jobSlots.release()
				r.execute(job)
			}()
		}
	}
}

func (r *Runner) log(job *Job, level core.Level, format string, args ...any) {
	line := fmt.Sprintf(format, args...)
	job.addLog(level, line)
	r.emit(core.JobLog{JobID: job.id, Level: level, Line: line})
}

func (r *Runner) emit(event core.Event) {
	select {
	case r.events <- event:
	case <-r.ctx.Done():
	}
}

func (r *Runner) engineLimit(id core.EngineID) int {
	if n, ok := r.limits[id]; ok && n > 0 {
		return n
	}
	return runtime.NumCPU()
}

func (r *Runner) engineSlot(id core.EngineID) *semaphore {
	r.engineMu.Lock()
	defer r.engineMu.Unlock()
	slot, ok := r.engineSlots[id]
	if !ok {
		slot = newSemaphore(r.engineLimit(id))
		r.engineSlots[id] = slot
	}
	return slot
}

func (r *Runner) tempRoot(job *Job) string {
	return filepath.Join(r.cacheDir, "jobs", job.id)
}

// CleanStale removes job temp directories left behind by a crash. Active jobs
// from another process are protected by the age check; only directories older
// than maxAge are removed.
func CleanStale(cacheDir string, maxAge time.Duration) {
	if cacheDir == "" || maxAge <= 0 {
		return
	}
	root := filepath.Join(cacheDir, "jobs")
	entries, err := os.ReadDir(root)
	if err != nil {
		return
	}
	cutoff := time.Now().Add(-maxAge)
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		info, err := entry.Info()
		if err != nil || info.ModTime().After(cutoff) {
			continue
		}
		_ = os.RemoveAll(filepath.Join(root, entry.Name()))
	}
}

func (r *Runner) ensureCacheDir() error {
	if r.cacheDir == "" {
		return errors.New("runner cache directory is not configured")
	}
	return os.MkdirAll(r.cacheDir, 0o755)
}

func mergeParams(defaults, override core.Values) core.Values {
	merged := make(core.Values, len(defaults)+len(override))
	for k, v := range defaults {
		merged[k] = v
	}
	for k, v := range override {
		merged[k] = v
	}
	return merged
}

func primaryEngine(spec core.Spec) core.EngineID {
	if len(spec.Engines) > 0 {
		return spec.Engines[0]
	}
	return core.EngineID("internal")
}

// semaphore is a counting limiter. A nil channel means unlimited.
type semaphore struct {
	ch chan struct{}
}

func newSemaphore(n int) *semaphore {
	if n <= 0 {
		return &semaphore{}
	}
	return &semaphore{ch: make(chan struct{}, n)}
}

func (s *semaphore) acquire(ctx context.Context) error {
	if s.ch == nil {
		return nil
	}
	select {
	case s.ch <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (s *semaphore) release() {
	if s.ch != nil {
		<-s.ch
	}
}
