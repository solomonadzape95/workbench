package runner

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/solomonadzape95/workbench/internal/core"
	"github.com/solomonadzape95/workbench/internal/ops/file"
)

func testRunner(t *testing.T, maxJobs int, ops ...core.Operation) *Runner {
	t.Helper()
	r := New(Options{
		Operations:   core.NewRegistry(ops...),
		CacheDir:     t.TempDir(),
		MaxJobs:      maxJobs,
		EngineLimits: map[string]int{"internal": 4},
	})
	t.Cleanup(r.Close)
	return r
}

func writeInput(t *testing.T, dir, name, body string) core.Artifact {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	art, err := core.Probe(path)
	if err != nil {
		t.Fatal(err)
	}
	return art
}

func waitJobs(t *testing.T, r *Runner, n int) (map[string]core.JobDone, []core.Event) {
	t.Helper()
	done := map[string]core.JobDone{}
	var events []core.Event
	timer := time.NewTimer(10 * time.Second)
	defer timer.Stop()

	for len(done) < n {
		select {
		case ev, ok := <-r.Events():
			if !ok {
				t.Fatalf("event channel closed with %d/%d jobs done", len(done), n)
			}
			events = append(events, ev)
			if d, ok := ev.(core.JobDone); ok {
				done[d.JobID] = d
			}
		case <-timer.C:
			t.Fatalf("timed out waiting for %d jobs; got %d", n, len(done))
		}
	}
	return done, events
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestRunnerCopiesAndCommits(t *testing.T) {
	inDir := t.TempDir()
	outDir := t.TempDir()
	inputs := []core.Artifact{
		writeInput(t, inDir, "a.txt", "alpha"),
		writeInput(t, inDir, "b.txt", "beta"),
		writeInput(t, inDir, "c.txt", "gamma"),
	}

	r := testRunner(t, 2, file.Copy{})
	job, err := r.Start(
		core.Pipeline{Name: "copy", Steps: []core.Step{{Op: "file.copy"}}},
		inputs,
		core.Target{Dir: outDir},
	)
	if err != nil {
		t.Fatal(err)
	}

	done, events := waitJobs(t, r, 1)
	if got := done[job.ID()].Status; got != core.StatusDone {
		t.Fatalf("status = %q, want done (err: %v)", got, done[job.ID()].Err)
	}

	for _, in := range inputs {
		out := filepath.Join(outDir, in.Name())
		if got := readFile(t, out); got != readFile(t, in.Path) {
			t.Errorf("%s content = %q, want %q", out, got, readFile(t, in.Path))
		}
	}
	if _, err := os.Stat(filepath.Join(r.cacheDir, "jobs", job.ID())); !os.IsNotExist(err) {
		t.Errorf("job temp directory still exists: %v", err)
	}

	var sawStarted, sawProgress, sawDone bool
	for _, ev := range events {
		switch ev.(type) {
		case core.ItemStarted:
			sawStarted = true
		case core.ItemProgress:
			sawProgress = true
		case core.ItemDone:
			sawDone = true
		}
	}
	if !sawStarted || !sawProgress || !sawDone {
		t.Errorf("events missing lifecycle entries: started=%v progress=%v done=%v", sawStarted, sawProgress, sawDone)
	}

	views := r.Jobs()
	if len(views) != 1 {
		t.Fatalf("Jobs() returned %d, want 1", len(views))
	}
	if counts := views[0].Counts(); counts.Done != 3 || counts.Failed != 0 || counts.Total != 3 {
		t.Errorf("counts = %+v, want 3 done", counts)
	}
}

func TestRunnerCancelLeavesNoOutput(t *testing.T) {
	entered := make(chan struct{}, 2)
	block := testOp{id: "test.block", run: func(ctx context.Context, req core.Request, rep core.Reporter) (core.Result, error) {
		entered <- struct{}{}
		<-ctx.Done()
		return core.Result{}, ctx.Err()
	}}

	inDir := t.TempDir()
	outDir := t.TempDir()
	r := testRunner(t, 2, block)
	job, err := r.Start(
		core.Pipeline{Name: "block", Steps: []core.Step{{Op: "test.block"}}},
		[]core.Artifact{writeInput(t, inDir, "a.txt", "a"), writeInput(t, inDir, "b.txt", "b")},
		core.Target{Dir: outDir},
	)
	if err != nil {
		t.Fatal(err)
	}

	select {
	case <-entered:
	case <-time.After(2 * time.Second):
		t.Fatal("operation never started")
	}
	job.Cancel()

	done, _ := waitJobs(t, r, 1)
	if got := done[job.ID()].Status; got != core.StatusCanceled {
		t.Fatalf("status = %q, want canceled", got)
	}
	entries, err := os.ReadDir(outDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Errorf("output directory has %d entries after cancel, want 0", len(entries))
	}
}

func TestRunnerPartialFailure(t *testing.T) {
	flaky := testOp{id: "test.flaky", run: func(ctx context.Context, req core.Request, rep core.Reporter) (core.Result, error) {
		in := req.Items[0]
		if strings.Contains(in.Path, "bad") {
			return core.Result{}, core.NewError(core.CodeInput, "test.flaky", in.Path, "synthetic failure", "ignore it")
		}
		return core.Result{Artifacts: []core.Artifact{produce(req, ".out", "ok")}}, nil
	}}

	inDir := t.TempDir()
	outDir := t.TempDir()
	r := testRunner(t, 2, flaky)
	job, err := r.Start(
		core.Pipeline{Name: "flaky", Steps: []core.Step{{Op: "test.flaky"}}},
		[]core.Artifact{writeInput(t, inDir, "good.txt", "a"), writeInput(t, inDir, "bad.txt", "b")},
		core.Target{Dir: outDir},
	)
	if err != nil {
		t.Fatal(err)
	}

	done, _ := waitJobs(t, r, 1)
	got := done[job.ID()]
	if got.Status != core.StatusPartial {
		t.Fatalf("status = %q, want partial (err: %v)", got.Status, got.Err)
	}
	if _, err := os.Stat(filepath.Join(outDir, "good.out")); err != nil {
		t.Errorf("successful item was not committed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(outDir, "bad.out")); !os.IsNotExist(err) {
		t.Errorf("failed item produced an output: %v", err)
	}
}

func TestRunnerRecoversOperationPanic(t *testing.T) {
	boom := testOp{id: "test.panic", run: func(ctx context.Context, req core.Request, rep core.Reporter) (core.Result, error) {
		panic("kaboom")
	}}
	inDir := t.TempDir()
	r := testRunner(t, 1, boom)
	job, err := r.Start(
		core.Pipeline{Steps: []core.Step{{Op: "test.panic"}}},
		[]core.Artifact{writeInput(t, inDir, "a.txt", "a")},
		core.Target{Dir: t.TempDir()},
	)
	if err != nil {
		t.Fatal(err)
	}

	done, _ := waitJobs(t, r, 1)
	got := done[job.ID()]
	if got.Status != core.StatusFailed {
		t.Fatalf("status = %q, want failed", got.Status)
	}
	if got.Err == nil || !strings.Contains(got.Err.Error(), "panic") {
		t.Errorf("error = %v, want a panic message", got.Err)
	}
}

func TestRunnerChainsStepsAndKeepsIntermediatesOutOfOutput(t *testing.T) {
	addMid := testOp{id: "test.mid", run: func(ctx context.Context, req core.Request, rep core.Reporter) (core.Result, error) {
		return core.Result{Artifacts: []core.Artifact{produce(req, ".mid", "mid")}}, nil
	}}
	addFinal := testOp{id: "test.final", run: func(ctx context.Context, req core.Request, rep core.Reporter) (core.Result, error) {
		in := req.Items[0]
		if in.Ext() != "mid" {
			return core.Result{}, core.NewError(core.CodeInternal, "test.final", in.Path, "expected the intermediate", "")
		}
		return core.Result{Artifacts: []core.Artifact{produce(req, ".done", "final")}}, nil
	}}

	inDir := t.TempDir()
	outDir := t.TempDir()
	r := testRunner(t, 1, addMid, addFinal)
	job, err := r.Start(
		core.Pipeline{Steps: []core.Step{{Op: "test.mid"}, {Op: "test.final", Input: core.StepInput{From: "previous"}}}},
		[]core.Artifact{writeInput(t, inDir, "a.txt", "a")},
		core.Target{Dir: outDir},
	)
	if err != nil {
		t.Fatal(err)
	}

	done, _ := waitJobs(t, r, 1)
	if got := done[job.ID()].Status; got != core.StatusDone {
		t.Fatalf("status = %q, want done (err: %v)", got, done[job.ID()].Err)
	}
	entries, err := os.ReadDir(outDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != "a.done" {
		t.Errorf("output entries = %v, want only a.done", names(entries))
	}
	if _, err := os.Stat(filepath.Join(r.cacheDir, "jobs", job.ID())); !os.IsNotExist(err) {
		t.Errorf("temp directory survived the chain: %v", err)
	}
}

func TestRunnerCollisionSuffix(t *testing.T) {
	inDir := t.TempDir()
	outDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(outDir, "a.txt"), []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}

	r := testRunner(t, 1, file.Copy{})
	job, err := r.Start(
		core.Pipeline{Steps: []core.Step{{Op: "file.copy"}}},
		[]core.Artifact{writeInput(t, inDir, "a.txt", "new")},
		core.Target{Dir: outDir, Collision: core.CollisionSuffix},
	)
	if err != nil {
		t.Fatal(err)
	}
	done, _ := waitJobs(t, r, 1)
	if got := done[job.ID()].Status; got != core.StatusDone {
		t.Fatalf("status = %q, want done", got)
	}
	if got := readFile(t, filepath.Join(outDir, "a (1).txt")); got != "new" {
		t.Errorf("suffixed output = %q, want new", got)
	}
	if got := readFile(t, filepath.Join(outDir, "a.txt")); got != "old" {
		t.Errorf("existing output was clobbered: %q", got)
	}
}

func TestRunnerRejectsOutputOutsideWorkDir(t *testing.T) {
	outside := t.TempDir()
	evil := testOp{id: "test.evil", run: func(ctx context.Context, req core.Request, rep core.Reporter) (core.Result, error) {
		path := filepath.Join(outside, "stolen.txt")
		if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
			return core.Result{}, err
		}
		return core.Result{Artifacts: []core.Artifact{{Path: path, Format: "txt"}}}, nil
	}}

	r := testRunner(t, 1, evil)
	job, err := r.Start(
		core.Pipeline{Steps: []core.Step{{Op: "test.evil"}}},
		[]core.Artifact{writeInput(t, t.TempDir(), "a.txt", "a")},
		core.Target{Dir: t.TempDir()},
	)
	if err != nil {
		t.Fatal(err)
	}
	done, _ := waitJobs(t, r, 1)
	if got := done[job.ID()].Status; got != core.StatusFailed {
		t.Fatalf("status = %q, want failed", got)
	}
}

func TestRunnerQueuesBeyondMaxJobs(t *testing.T) {
	entered := make(chan struct{}, 2)
	release := make(chan struct{})
	block := testOp{id: "test.block", run: func(ctx context.Context, req core.Request, rep core.Reporter) (core.Result, error) {
		entered <- struct{}{}
		select {
		case <-release:
		case <-ctx.Done():
			return core.Result{}, ctx.Err()
		}
		return core.Result{Artifacts: []core.Artifact{produce(req, ".out", "ok")}}, nil
	}}

	r := testRunner(t, 1, block)
	inputs := []core.Artifact{writeInput(t, t.TempDir(), "a.txt", "a")}
	target := core.Target{Dir: t.TempDir()}
	pipeline := core.Pipeline{Steps: []core.Step{{Op: "test.block"}}}

	job1, err := r.Start(pipeline, inputs, target)
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-entered:
	case <-time.After(2 * time.Second):
		t.Fatal("first job never started")
	}

	job2, err := r.Start(pipeline, inputs, target)
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(50 * time.Millisecond)
	if got := job2.Status(); got != core.StatusQueued {
		t.Errorf("second job status = %q, want queued", got)
	}
	if got := job1.Status(); got != core.StatusRunning {
		t.Errorf("first job status = %q, want running", got)
	}

	close(release)
	done, _ := waitJobs(t, r, 2)
	if done[job1.ID()].Status != core.StatusDone || done[job2.ID()].Status != core.StatusDone {
		t.Errorf("statuses = %q, %q; want done", done[job1.ID()].Status, done[job2.ID()].Status)
	}
}

type testOp struct {
	id  string
	run func(context.Context, core.Request, core.Reporter) (core.Result, error)
}

func (o testOp) Spec() core.Spec {
	return core.Spec{
		ID:        o.id,
		Title:     o.id,
		Engines:   []core.EngineID{"internal"},
		Batchable: true,
	}
}

func (o testOp) Validate(ctx context.Context, req core.Request) error {
	return core.Validate(nil, req.Params)
}

func (o testOp) Run(ctx context.Context, req core.Request, rep core.Reporter) (core.Result, error) {
	return o.run(ctx, req, rep)
}

func produce(req core.Request, ext, body string) core.Artifact {
	in := req.Items[0]
	path := filepath.Join(req.Target.Dir, core.SanitizeStem(in.Stem())+ext)
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		panic(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		panic(err)
	}
	return core.Artifact{Path: path, Kind: in.Kind, Format: strings.TrimPrefix(ext, "."), Size: info.Size()}
}

func names(entries []os.DirEntry) []string {
	out := make([]string, 0, len(entries))
	for _, e := range entries {
		out = append(out, e.Name())
	}
	return out
}

func TestJobViewPercent(t *testing.T) {
	view := JobView{Items: []ItemView{
		{Status: core.ItemStatusRunning, Done: 25, Total: 100},
		{Status: core.ItemStatusDone},
	}}
	if got, want := view.Percent(), 63; got != want {
		t.Errorf("Percent() = %d, want %d", got, want)
	}

	canceled := JobView{Items: []ItemView{{Status: core.ItemStatusCanceled, Done: 42, Total: 100}}}
	if got, want := canceled.Percent(), 42; got != want {
		t.Errorf("cancelled Percent() = %d, want %d", got, want)
	}

	if got := (JobView{}).Percent(); got != 0 {
		t.Errorf("empty Percent() = %d, want 0", got)
	}
}

func TestRunnerCapturesAndPersistsLogs(t *testing.T) {
	logDir := t.TempDir()
	logging := testOp{id: "test.log", run: func(ctx context.Context, req core.Request, rep core.Reporter) (core.Result, error) {
		rep.Log(core.LevelInfo, "hello from the operation")
		return core.Result{}, core.NewError(core.CodeInput, "test.log", req.Items[0].Path, "synthetic failure", "read the log")
	}}
	inDir := t.TempDir()
	r := New(Options{
		Operations: core.NewRegistry(logging),
		CacheDir:   t.TempDir(),
		LogDir:     logDir,
		MaxJobs:    1,
	})
	defer r.Close()

	job, err := r.Start(
		core.Pipeline{Steps: []core.Step{{Op: "test.log"}}},
		[]core.Artifact{writeInput(t, inDir, "a.txt", "a")},
		core.Target{Dir: t.TempDir()},
	)
	if err != nil {
		t.Fatal(err)
	}
	waitJobs(t, r, 1)

	view := job.Snapshot()
	joined := strings.Join(view.Logs, "\n")
	for _, want := range []string{"job started", "hello from the operation", "synthetic failure", "job finished"} {
		if !strings.Contains(joined, want) {
			t.Errorf("logs missing %q:\n%s", want, joined)
		}
	}
	data, err := os.ReadFile(filepath.Join(logDir, job.ID()+".log"))
	if err != nil {
		t.Fatalf("log file: %v", err)
	}
	if !strings.Contains(string(data), "hello from the operation") {
		t.Errorf("persisted log = %q", data)
	}
}

func TestRunnerHonorsSuggestedName(t *testing.T) {
	named := testOp{id: "test.named", run: func(ctx context.Context, req core.Request, rep core.Reporter) (core.Result, error) {
		path := filepath.Join(req.Target.Dir, "raw.tmp")
		if err := os.WriteFile(path, []byte("named"), 0o644); err != nil {
			return core.Result{}, err
		}
		return core.Result{Artifacts: []core.Artifact{{Path: path, Kind: core.Video, Format: "mp4", SuggestedName: "Final Clip [abc].mp4"}}}, nil
	}}
	inDir := t.TempDir()
	outDir := t.TempDir()
	r := testRunner(t, 1, named)
	job, err := r.Start(
		core.Pipeline{Steps: []core.Step{{Op: "test.named"}}},
		[]core.Artifact{writeInput(t, inDir, "input.txt", "x")},
		core.Target{Dir: outDir},
	)
	if err != nil {
		t.Fatal(err)
	}
	done, _ := waitJobs(t, r, 1)
	if got := done[job.ID()].Status; got != core.StatusDone {
		t.Fatalf("status = %q (err: %v)", got, done[job.ID()].Err)
	}
	if _, err := os.Stat(filepath.Join(outDir, "Final Clip [abc].mp4")); err != nil {
		t.Errorf("suggested name not used: %v", err)
	}
}

func TestRetryFailedAndRerun(t *testing.T) {
	flaky := testOp{id: "test.flaky2", run: func(ctx context.Context, req core.Request, rep core.Reporter) (core.Result, error) {
		in := req.Items[0]
		if strings.Contains(in.Path, "bad") {
			return core.Result{}, core.NewError(core.CodeInput, "test.flaky2", in.Path, "bad file", "")
		}
		return core.Result{Artifacts: []core.Artifact{produce(req, ".out", "ok")}}, nil
	}}
	inDir := t.TempDir()
	target := core.Target{Dir: t.TempDir()}
	r := testRunner(t, 2, flaky)
	job, err := r.Start(
		core.Pipeline{Name: "flaky", Steps: []core.Step{{Op: "test.flaky2"}}},
		[]core.Artifact{writeInput(t, inDir, "good.txt", "a"), writeInput(t, inDir, "bad.txt", "b")},
		target,
	)
	if err != nil {
		t.Fatal(err)
	}
	done, _ := waitJobs(t, r, 1)
	if done[job.ID()].Status != core.StatusPartial {
		t.Fatalf("status = %q", done[job.ID()].Status)
	}

	retry, err := r.RetryFailed(job.ID())
	if err != nil {
		t.Fatal(err)
	}
	if got := len(retry.Snapshot().Items); got != 1 {
		t.Errorf("retry items = %d, want 1", got)
	}
	if !strings.Contains(retry.Name(), "retry") {
		t.Errorf("retry name = %q", retry.Name())
	}
	waitJobs(t, r, 1)

	rerun, err := r.Rerun(job.ID())
	if err != nil {
		t.Fatal(err)
	}
	if got := len(rerun.Snapshot().Items); got != 2 {
		t.Errorf("rerun items = %d, want 2", got)
	}
	waitJobs(t, r, 1)
}

func TestRunnerCarriesSuggestedNameThroughChain(t *testing.T) {
	download := testOp{id: "test.download", run: func(ctx context.Context, req core.Request, rep core.Reporter) (core.Result, error) {
		path := filepath.Join(req.Target.Dir, "raw.tmp")
		os.WriteFile(path, []byte("video"), 0o644)
		return core.Result{Artifacts: []core.Artifact{{Path: path, Kind: core.Video, Format: "mp4", SuggestedName: "My Clip [id].mp4"}}}, nil
	}}
	convert := testOp{id: "test.convert", run: func(ctx context.Context, req core.Request, rep core.Reporter) (core.Result, error) {
		path := filepath.Join(req.Target.Dir, "stage.tmp")
		os.WriteFile(path, []byte("mkv"), 0o644)
		return core.Result{Artifacts: []core.Artifact{{Path: path, Kind: core.Video, Format: "mkv"}}}, nil
	}}
	outDir := t.TempDir()
	r := testRunner(t, 1, download, convert)
	job, err := r.Start(
		core.Pipeline{Steps: []core.Step{{Op: "test.download"}, {Op: "test.convert", Input: core.StepInput{From: "previous"}}}},
		[]core.Artifact{{Path: "https://example.com/v", Kind: core.Video, Format: "url", Label: "https://example.com/v"}},
		core.Target{Dir: outDir},
	)
	if err != nil {
		t.Fatal(err)
	}
	done, _ := waitJobs(t, r, 1)
	if got := done[job.ID()].Status; got != core.StatusDone {
		t.Fatalf("status = %q (err: %v)", got, done[job.ID()].Err)
	}
	if _, err := os.Stat(filepath.Join(outDir, "My Clip [id].mkv")); err != nil {
		t.Errorf("chained suggested name missing: %v", err)
	}
}

type recordingHistory struct {
	records []Record
}

func (h *recordingHistory) Append(rec Record) error {
	h.records = append(h.records, rec)
	return nil
}

func TestRunnerRecordsHistory(t *testing.T) {
	history := &recordingHistory{}
	op := testOp{id: "test.copy", run: func(ctx context.Context, req core.Request, rep core.Reporter) (core.Result, error) {
		return core.Result{Artifacts: []core.Artifact{produce(req, ".txt", "copied")}}, nil
	}}
	inDir := t.TempDir()
	r := New(Options{
		Operations: core.NewRegistry(op),
		CacheDir:   t.TempDir(),
		LogDir:     t.TempDir(),
		MaxJobs:    1,
		History:    history,
	})
	defer r.Close()

	job, err := r.Start(
		core.Pipeline{Name: "recorded", Steps: []core.Step{{ID: "step-1", Op: "test.copy"}}},
		[]core.Artifact{writeInput(t, inDir, "a.txt", "a")},
		core.Target{Dir: t.TempDir()},
	)
	if err != nil {
		t.Fatal(err)
	}
	waitJobs(t, r, 1)

	if len(history.records) != 1 {
		t.Fatalf("records = %d, want 1", len(history.records))
	}
	rec := history.records[0]
	if rec.View.ID != job.ID() || rec.View.Status != core.StatusDone {
		t.Errorf("record = %+v", rec.View)
	}
	if len(rec.View.Pipeline.Steps) != 1 {
		t.Error("record lost the pipeline")
	}
	if rec.View.LogPath == "" {
		t.Error("record lost the log path")
	}
}

type aggregateTestOp struct{}

func (aggregateTestOp) Spec() core.Spec {
	return core.Spec{ID: "test.aggregate", Title: "Aggregate", Engines: []core.EngineID{"internal"}, Mode: core.ModeAggregate, Batchable: false}
}
func (aggregateTestOp) Validate(ctx context.Context, req core.Request) error {
	if len(req.Items) < 2 {
		return core.NewError(core.CodeUsage, "test.aggregate", "", "need two", "")
	}
	return nil
}
func (aggregateTestOp) Run(ctx context.Context, req core.Request, rep core.Reporter) (core.Result, error) {
	var b strings.Builder
	for _, item := range req.Items {
		b.WriteString(readFileStatic(item.Path))
	}
	path := filepath.Join(req.Target.Dir, "all.txt")
	if err := os.WriteFile(path, []byte(b.String()), 0o644); err != nil {
		return core.Result{}, err
	}
	info, _ := os.Stat(path)
	return core.Result{
		Artifacts: []core.Artifact{{Path: path, Kind: core.File, Format: "txt", Size: info.Size(), SuggestedName: "all.txt"}},
		Metrics:   core.Metrics{BytesIn: info.Size(), BytesOut: info.Size(), Engine: "internal"},
	}, nil
}

func readFileStatic(path string) string {
	data, _ := os.ReadFile(path)
	return string(data)
}

func TestAggregateOperationCommitsOneOutput(t *testing.T) {
	inDir := t.TempDir()
	outDir := t.TempDir()
	inputs := []core.Artifact{
		writeInput(t, inDir, "a.txt", "alpha"),
		writeInput(t, inDir, "b.txt", "beta"),
	}
	r := testRunner(t, 1, aggregateTestOp{})
	job, err := r.Start(core.Pipeline{
		Name: "aggregate", Version: core.PipelineVersion,
		Steps: []core.Step{{Op: "test.aggregate"}},
	}, inputs, core.Target{Dir: outDir})
	if err != nil {
		t.Fatal(err)
	}
	done, _ := waitJobs(t, r, 1)
	if done[job.ID()].Status != core.StatusDone {
		t.Fatalf("status = %v", done[job.ID()].Status)
	}
	view := job.Snapshot()
	if len(view.Aggregate) != 1 {
		t.Fatalf("aggregate = %+v", view.Aggregate)
	}
	if got := readFile(t, view.Aggregate[0].Path); got != "alphabeta" {
		t.Fatalf("output = %q", got)
	}
	for _, item := range view.Items {
		if item.Status != core.ItemStatusDone {
			t.Errorf("item %s status = %s", item.ID, item.Status)
		}
	}
}

type directTestOp struct{}

func (directTestOp) Spec() core.Spec {
	return core.Spec{ID: "test.direct", Title: "Direct", Engines: []core.EngineID{"internal"}, Mode: core.ModeDirect}
}

func (directTestOp) Validate(ctx context.Context, req core.Request) error {
	if req.Target.Dir == "" {
		return core.NewError(core.CodeUsage, "test.direct", "", "no target", "")
	}
	return nil
}

func (directTestOp) Run(ctx context.Context, req core.Request, rep core.Reporter) (core.Result, error) {
	in := req.Items[0]
	out := filepath.Join(req.Target.Dir, "moved-"+in.Name())
	if err := os.Rename(in.Path, out); err != nil {
		return core.Result{}, err
	}
	info, err := os.Stat(out)
	if err != nil {
		return core.Result{}, err
	}
	return core.Result{
		Artifacts: []core.Artifact{{Path: out, Kind: in.Kind, Format: in.Format, Size: info.Size()}},
		Metrics:   core.Metrics{BytesIn: in.Size, BytesOut: info.Size(), Engine: "internal"},
	}, nil
}

func TestDirectOperationWritesFinalTargetWithoutCommit(t *testing.T) {
	inDir := t.TempDir()
	outDir := t.TempDir()
	input := writeInput(t, inDir, "a.txt", "hello")
	r := testRunner(t, 1, directTestOp{})
	job, err := r.Start(core.Pipeline{
		Name: "direct", Version: core.PipelineVersion,
		Steps: []core.Step{{Op: "test.direct"}},
	}, []core.Artifact{input}, core.Target{Dir: outDir})
	if err != nil {
		t.Fatal(err)
	}
	done, _ := waitJobs(t, r, 1)
	if done[job.ID()].Status != core.StatusDone {
		t.Fatalf("status = %v (err: %v)", done[job.ID()].Status, done[job.ID()].Err)
	}
	if _, err := os.Stat(input.Path); !os.IsNotExist(err) {
		t.Fatalf("input still exists after direct move: %v", err)
	}
	final := filepath.Join(outDir, "moved-a.txt")
	if data := readFile(t, final); data != "hello" {
		t.Fatalf("output = %q", data)
	}
	view := job.Snapshot()
	if len(view.Items) != 1 || view.Items[0].Status != core.ItemStatusDone {
		t.Fatalf("items = %+v", view.Items)
	}
	if len(view.Items[0].Outputs) != 1 || view.Items[0].Outputs[0].Path != final {
		t.Fatalf("outputs = %+v", view.Items[0].Outputs)
	}
	entries, err := os.ReadDir(outDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != "moved-a.txt" {
		t.Fatalf("unexpected commit artifacts in target: %v", names(entries))
	}
}

func TestUndoRestoresOverwrittenOutput(t *testing.T) {
	inDir := t.TempDir()
	outDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(outDir, "a.txt"), []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	op := testOp{id: "test.overwrite", run: func(ctx context.Context, req core.Request, rep core.Reporter) (core.Result, error) {
		path := filepath.Join(req.Target.Dir, "raw.tmp")
		if err := os.WriteFile(path, []byte("new"), 0o644); err != nil {
			return core.Result{}, err
		}
		return core.Result{Artifacts: []core.Artifact{{Path: path, Kind: core.File, Format: "txt", SuggestedName: "a.txt"}}}, nil
	}}
	r := testRunner(t, 1, op)
	job, err := r.Start(core.Pipeline{
		Name: "undo", Version: core.PipelineVersion,
		Steps: []core.Step{{Op: "test.overwrite"}},
	}, []core.Artifact{writeInput(t, inDir, "input.txt", "input")}, core.Target{Dir: outDir, Collision: core.CollisionOverwrite})
	if err != nil {
		t.Fatal(err)
	}
	done, _ := waitJobs(t, r, 1)
	if done[job.ID()].Status != core.StatusDone {
		t.Fatalf("status = %v (%v)", done[job.ID()].Status, done[job.ID()].Err)
	}
	view := job.Snapshot()
	if len(view.Items) != 1 || len(view.Items[0].Outputs) != 1 {
		t.Fatalf("view = %+v", view)
	}
	backup, _ := view.Items[0].Outputs[0].Meta["backup"].(string)
	if backup == "" {
		t.Fatal("overwritten output has no backup")
	}
	if data := readFile(t, backup); data != "old" {
		t.Fatalf("backup = %q", data)
	}
	if data := readFile(t, filepath.Join(outDir, "a.txt")); data != "new" {
		t.Fatalf("output = %q", data)
	}

	result, err := UndoJob(view, r.undoDir)
	if err != nil {
		t.Fatal(err)
	}
	if result.Files != 1 || result.Restored != 1 {
		t.Fatalf("undo = %+v", result)
	}
	if data := readFile(t, filepath.Join(outDir, "a.txt")); data != "old" {
		t.Fatalf("restored output = %q", data)
	}
	if _, err := os.Stat(backup); !os.IsNotExist(err) {
		t.Fatalf("backup still exists: %v", err)
	}
	entries, err := os.ReadDir(filepath.Join(r.undoDir, "trash", job.ID()))
	if err != nil {
		t.Fatalf("undo trash: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("undo trash = %v", names(entries))
	}
}

type chainDirectOp struct{}

func (chainDirectOp) Spec() core.Spec {
	return core.Spec{ID: "test.chain-direct", Title: "Chain direct", Engines: []core.EngineID{"internal"}, Mode: core.ModeDirect, Chainable: true}
}
func (chainDirectOp) Validate(context.Context, core.Request) error { return nil }
func (chainDirectOp) Run(ctx context.Context, req core.Request, rep core.Reporter) (core.Result, error) {
	in := req.Items[0]
	out := filepath.Join(req.Target.Dir, in.Name())
	if err := os.Rename(in.Path, out); err != nil {
		return core.Result{}, err
	}
	info, err := os.Stat(out)
	if err != nil {
		return core.Result{}, err
	}
	return core.Result{Artifacts: []core.Artifact{{Path: out, Kind: in.Kind, Format: in.Format, Size: info.Size()}}}, nil
}

func TestChainableDirectOperationFeedsNextStep(t *testing.T) {
	inDir := t.TempDir()
	outDir := t.TempDir()
	input := writeInput(t, inDir, "a.txt", "hello")
	copyOp := testOp{id: "test.copy-after-direct", run: func(ctx context.Context, req core.Request, rep core.Reporter) (core.Result, error) {
		data, err := os.ReadFile(req.Items[0].Path)
		if err != nil {
			return core.Result{}, err
		}
		path := filepath.Join(req.Target.Dir, "final.txt")
		if err := os.WriteFile(path, data, 0o644); err != nil {
			return core.Result{}, err
		}
		info, _ := os.Stat(path)
		return core.Result{Artifacts: []core.Artifact{{Path: path, Kind: core.File, Format: "txt", Size: info.Size(), SuggestedName: "final.txt"}}}, nil
	}}
	r := testRunner(t, 1, chainDirectOp{}, copyOp)
	job, err := r.Start(core.Pipeline{
		Name: "chain", Version: core.PipelineVersion,
		Steps: []core.Step{{Op: "test.chain-direct"}, {Op: "test.copy-after-direct", Input: core.StepInput{From: "previous"}}},
	}, []core.Artifact{input}, core.Target{Dir: outDir})
	if err != nil {
		t.Fatal(err)
	}
	done, _ := waitJobs(t, r, 1)
	if done[job.ID()].Status != core.StatusDone {
		t.Fatalf("status = %v (%v)", done[job.ID()].Status, done[job.ID()].Err)
	}
	if _, err := os.Stat(input.Path); !os.IsNotExist(err) {
		t.Fatalf("input still exists: %v", err)
	}
	if data := readFile(t, filepath.Join(outDir, "final.txt")); data != "hello" {
		t.Fatalf("final = %q", data)
	}
}
