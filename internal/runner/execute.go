package runner

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/solomonadzape95/workbench/internal/core"
)

func (r *Runner) execute(job *Job) {
	started := time.Now()
	job.setRunning(started)
	r.log(job, core.LevelInfo, "job started: %s", job.name)
	r.emit(core.JobStarted{JobID: job.id, At: started})

	tempRoot := r.tempRoot(job)
	job.setTempDir(tempRoot)

	defer func() {
		if p := recover(); p != nil {
			job.fail(core.NewError(core.CodeInternal, "", "",
				fmt.Sprintf("job panic: %v", p), "this is a Workbench bug; the job was stopped cleanly"))
			r.log(job, core.LevelError, "job panic: %v", p)
		}
		status, err, duration, metrics := job.finalize(job.ctx)
		level := core.LevelInfo
		if status == core.StatusFailed {
			level = core.LevelError
		} else if status == core.StatusPartial || status == core.StatusCanceled || status == core.StatusCancelling {
			level = core.LevelWarn
		}
		r.log(job, level, "job finished: %s in %s", status, duration.Round(time.Millisecond))
		r.persistLogs(job)
		if r.history != nil {
			_ = r.history.Append(Record{Version: 1, View: job.Snapshot()})
		}
		_ = os.RemoveAll(tempRoot)
		r.emit(core.JobDone{
			JobID:    job.id,
			Status:   status,
			Err:      err,
			Duration: duration,
			Metrics:  metrics,
		})
	}()

	if err := r.ensureCacheDir(); err != nil {
		job.fail(core.Wrap(core.CodeInternal, "", "", err, "cannot create the Workbench cache directory"))
		return
	}
	if err := os.MkdirAll(tempRoot, 0o755); err != nil {
		job.fail(core.Wrap(core.CodeInternal, "", "", err, "cannot create the job temp directory"))
		return
	}
	if job.ctx.Err() != nil {
		return
	}

	for i, step := range job.pipeline.Steps {
		if job.ctx.Err() != nil {
			break
		}
		op, ok := r.ops.Get(step.Op)
		if !ok {
			job.fail(core.NewError(core.CodeInternal, step.Op, "",
				"operation disappeared from the registry", "this is a Workbench bug"))
			break
		}

		params := mergeParams(op.Spec().DefaultValues(), step.Params)
		stepID := step.ID
		if stepID == "" {
			stepID = fmt.Sprintf("step-%d", i+1)
		}
		job.startStep(i, stepID, step.Op, op.Spec().Title)
		r.log(job, core.LevelInfo, "step %d/%d: %s", i+1, len(job.pipeline.Steps), op.Spec().Title)
		r.emit(core.StepStarted{JobID: job.id, StepID: stepID, Index: i, Op: step.Op})

		stepDir := filepath.Join(tempRoot, fmt.Sprintf("step-%02d", i+1))
		terminal := i == len(job.pipeline.Steps)-1
		err := os.MkdirAll(stepDir, 0o755)
		if err == nil {
			err = r.runStep(job.ctx, job, i, op, params, stepDir, terminal, step)
		} else {
			err = core.Wrap(core.CodeInternal, "", "", err, "cannot create the step temp directory")
		}

		job.finishStep(i, err)
		r.emit(core.StepDone{JobID: job.id, StepID: stepID, Index: i, Err: err})
		if err != nil {
			r.log(job, core.LevelError, "step failed: %v", err)
			job.fail(err)
			break
		}
	}
}

func (r *Runner) runStep(
	ctx context.Context,
	job *Job,
	stepIndex int,
	op core.Operation,
	params core.Values,
	stepDir string,
	terminal bool,
	step core.Step,
) error {
	active := job.activeItems()
	if len(active) == 0 {
		return core.NewError(core.CodeInternal, op.Spec().ID, "",
			"no items left to process", "this is a Workbench bug")
	}

	if op.Spec().Aggregate() {
		if !terminal {
			return core.NewError(core.CodeUsage, op.Spec().ID, "",
				"aggregate operations must be the final step", "move this operation to the end of the chain")
		}
		return r.runAggregate(ctx, job, stepIndex, op, params, stepDir)
	}
	if op.Spec().Direct() {
		if !terminal && !op.Spec().Chainable {
			return core.NewError(core.CodeUsage, op.Spec().ID, "",
				"this direct operation must be the final step", "move this operation to the end of the chain")
		}
		return r.runDirectStep(ctx, job, stepIndex, op, params, terminal)
	}

	limit := r.engineLimit(primaryEngine(op.Spec()))
	if limit < 1 {
		limit = 1
	}

	stepCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	var (
		g        errgroup.Group
		mu       sync.Mutex
		firstErr error
	)
	g.SetLimit(limit)

	for _, it := range active {
		it := it
		g.Go(func() error {
			err := r.runItem(stepCtx, job, it, stepIndex, op, params, stepDir, terminal)
			if err != nil {
				mu.Lock()
				if firstErr == nil {
					firstErr = err
				}
				mu.Unlock()
				if step.OnError == core.OnErrorStop {
					cancel()
				}
			}
			return nil
		})
	}
	_ = g.Wait()
	return firstErr
}

func (r *Runner) runDirectStep(
	ctx context.Context,
	job *Job,
	stepIndex int,
	op core.Operation,
	params core.Values,
	terminal bool,
) error {
	active := job.activeItems()
	if len(active) == 0 {
		return core.NewError(core.CodeInternal, op.Spec().ID, "",
			"no items left to process", "this is a Workbench bug")
	}
	limit := r.engineLimit(primaryEngine(op.Spec()))
	if limit < 1 {
		limit = 1
	}
	stepCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	var (
		g        errgroup.Group
		mu       sync.Mutex
		firstErr error
	)
	g.SetLimit(limit)
	for _, it := range active {
		it := it
		g.Go(func() error {
			err := r.runDirectItem(stepCtx, job, it, stepIndex, op, params, terminal)
			if err != nil {
				mu.Lock()
				if firstErr == nil {
					firstErr = err
				}
				mu.Unlock()
				cancel()
			}
			return nil
		})
	}
	_ = g.Wait()
	return firstErr
}

func (r *Runner) runDirectItem(
	ctx context.Context,
	job *Job,
	it *item,
	stepIndex int,
	op core.Operation,
	params core.Values,
	terminal bool,
) error {
	if err := ctx.Err(); err != nil {
		return r.cancelItem(job, it, err)
	}
	rep := &reporter{runner: r, job: job, it: it}
	r.emit(core.ItemStarted{JobID: job.id, ItemID: it.id, Artifact: it.current})
	req := core.Request{
		Items:  []core.Artifact{it.current},
		Params: params,
		Target: job.target,
	}
	if err := op.Validate(ctx, req); err != nil {
		return r.failItem(job, it, err)
	}
	slot := r.engineSlot(primaryEngine(op.Spec()))
	if err := slot.acquire(ctx); err != nil {
		return r.cancelItem(job, it, err)
	}
	result, runErr := safeRun(ctx, op, req, rep)
	slot.release()
	if runErr != nil {
		if ctx.Err() != nil || core.IsCanceled(runErr) {
			return r.cancelItem(job, it, runErr)
		}
		return r.failItem(job, it, runErr)
	}
	if len(result.Artifacts) == 0 {
		return r.failItem(job, it, core.NewError(core.CodeInternal, op.Spec().ID, it.current.Path,
			"operation produced no output", "this is a Workbench bug"))
	}
	for _, art := range result.Artifacts {
		if _, err := os.Stat(art.Path); err != nil {
			return r.failItem(job, it, core.Wrap(core.CodeOutput, op.Spec().ID, art.Path, err,
				"the operation reported success but the output is missing"))
		}
	}
	job.setMetrics(core.Metrics{
		BytesIn:  sumBytesIn(result, stepIndex),
		BytesOut: sumBytesOut(result, terminal),
		Engine:   result.Metrics.Engine,
		Model:    result.Metrics.Model,
	})
	if !terminal {
		if len(result.Artifacts) != 1 {
			return r.failItem(job, it, core.NewError(core.CodeInternal, op.Spec().ID, it.current.Path,
				fmt.Sprintf("operation produced %d outputs; chaining requires exactly one", len(result.Artifacts)),
				"this operation cannot be chained yet"))
		}
		next := result.Artifacts[0]
		next.SuggestedName = carrySuggestedName(it.current, next)
		job.setItemCurrent(it, next)
		job.setItemOutputs(it, result.Artifacts)
		job.setItemStatus(it, core.ItemStatusRunning, nil)
		r.emit(core.ItemDone{JobID: job.id, ItemID: it.id, Outputs: result.Artifacts})
		return nil
	}
	job.setItemOutputs(it, result.Artifacts)
	job.setItemCurrent(it, result.Artifacts[0])
	job.setItemStatus(it, core.ItemStatusDone, nil)
	r.emit(core.ItemDone{JobID: job.id, ItemID: it.id, Outputs: result.Artifacts})
	return nil
}

func (r *Runner) runAggregate(
	ctx context.Context,
	job *Job,
	stepIndex int,
	op core.Operation,
	params core.Values,
	stepDir string,
) error {
	active := job.activeItems()
	if len(active) == 0 {
		return core.NewError(core.CodeInternal, op.Spec().ID, "",
			"no items left to aggregate", "this is a Workbench bug")
	}
	workDir := filepath.Join(stepDir, "aggregate")
	if err := os.MkdirAll(workDir, 0o755); err != nil {
		return r.failAggregate(job, active, core.Wrap(core.CodeOutput, op.Spec().ID, workDir, err,
			"check permissions on the Workbench cache directory"))
	}
	inputs := make([]core.Artifact, 0, len(active))
	for _, it := range active {
		inputs = append(inputs, it.current)
		r.emit(core.ItemStarted{JobID: job.id, ItemID: it.id, Artifact: it.current})
	}
	rep := &reporter{runner: r, job: job, it: active[0]}
	req := core.Request{
		Items:  inputs,
		Params: params,
		Target: core.Target{Dir: workDir, Template: "{stem}.{ext}", Collision: core.CollisionOverwrite},
	}
	if err := op.Validate(ctx, req); err != nil {
		return r.failAggregate(job, active, err)
	}
	slot := r.engineSlot(primaryEngine(op.Spec()))
	if err := slot.acquire(ctx); err != nil {
		return r.failAggregate(job, active, err)
	}
	result, runErr := safeRun(ctx, op, req, rep)
	slot.release()
	if runErr != nil {
		if ctx.Err() != nil || core.IsCanceled(runErr) {
			return r.failAggregate(job, active, runErr)
		}
		return r.failAggregate(job, active, runErr)
	}
	if len(result.Artifacts) == 0 {
		return r.failAggregate(job, active, core.NewError(core.CodeInternal, op.Spec().ID, "",
			"aggregate operation produced no output", "this is a Workbench bug"))
	}
	for _, art := range result.Artifacts {
		if !pathWithin(workDir, art.Path) {
			return r.failAggregate(job, active, core.NewError(core.CodeInternal, op.Spec().ID, art.Path,
				"aggregate operation wrote outside its work directory", "this is a Workbench bug"))
		}
	}
	var finals []core.Artifact
	for _, art := range result.Artifacts {
		final, err := r.commit(job, active[0], art, op.Spec())
		if errors.Is(err, core.ErrSkip) {
			continue
		}
		if err != nil {
			return r.failAggregate(job, active, err)
		}
		finals = append(finals, final)
	}
	if len(finals) == 0 {
		return r.failAggregate(job, active, core.NewError(core.CodeOutput, op.Spec().ID, "",
			"all aggregate outputs were skipped", "choose the overwrite collision policy"))
	}
	job.setAggregateOutputs(finals)
	job.setMetrics(core.Metrics{
		BytesIn:  sumBytesIn(result, stepIndex),
		BytesOut: sumBytesOut(result, true),
		Engine:   result.Metrics.Engine,
		Model:    result.Metrics.Model,
	})
	for _, it := range active {
		job.setItemOutputs(it, nil)
		job.setItemStatus(it, core.ItemStatusDone, nil)
		r.emit(core.ItemDone{JobID: job.id, ItemID: it.id})
	}
	return nil
}

func (r *Runner) failAggregate(job *Job, active []*item, err error) error {
	for _, it := range active {
		job.setItemStatus(it, core.ItemStatusFailed, err)
		r.emit(core.ItemDone{JobID: job.id, ItemID: it.id, Err: err})
	}
	return err
}

func (r *Runner) runItem(
	ctx context.Context,
	job *Job,
	it *item,
	stepIndex int,
	op core.Operation,
	params core.Values,
	stepDir string,
	terminal bool,
) error {
	if err := ctx.Err(); err != nil {
		return r.cancelItem(job, it, err)
	}

	workDir := filepath.Join(stepDir, it.id)
	if err := os.MkdirAll(workDir, 0o755); err != nil {
		return r.failItem(job, it, core.Wrap(core.CodeOutput, op.Spec().ID, it.input.Path, err,
			"check permissions on the Workbench cache directory"))
	}

	rep := &reporter{runner: r, job: job, it: it}
	r.emit(core.ItemStarted{JobID: job.id, ItemID: it.id, Artifact: it.current})
	req := core.Request{
		Items:  []core.Artifact{it.current},
		Params: params,
		Target: core.Target{
			Dir:       workDir,
			Template:  "{stem}.{ext}",
			Collision: core.CollisionOverwrite,
		},
	}

	if err := op.Validate(ctx, req); err != nil {
		return r.failItem(job, it, err)
	}

	slot := r.engineSlot(primaryEngine(op.Spec()))
	if err := slot.acquire(ctx); err != nil {
		return r.cancelItem(job, it, err)
	}
	result, runErr := safeRun(ctx, op, req, rep)
	slot.release()

	if runErr != nil {
		if ctx.Err() != nil || core.IsCanceled(runErr) {
			return r.cancelItem(job, it, runErr)
		}
		return r.failItem(job, it, runErr)
	}
	if len(result.Artifacts) == 0 {
		return r.failItem(job, it, core.NewError(core.CodeInternal, op.Spec().ID, it.current.Path,
			"operation produced no output", "this is a Workbench bug"))
	}
	for _, art := range result.Artifacts {
		if !pathWithin(workDir, art.Path) {
			return r.failItem(job, it, core.NewError(core.CodeInternal, op.Spec().ID, art.Path,
				"operation wrote outside its work directory", "this is a Workbench bug"))
		}
	}

	metrics := core.Metrics{
		BytesIn:  sumBytesIn(result, stepIndex),
		BytesOut: sumBytesOut(result, terminal),
		Engine:   result.Metrics.Engine,
		Model:    result.Metrics.Model,
	}
	job.setMetrics(metrics)

	if !terminal {
		if len(result.Artifacts) != 1 {
			return r.failItem(job, it, core.NewError(core.CodeInternal, op.Spec().ID, it.current.Path,
				fmt.Sprintf("operation produced %d outputs; chaining requires exactly one", len(result.Artifacts)),
				"this operation cannot be chained yet"))
		}
		next := result.Artifacts[0]
		next.SuggestedName = carrySuggestedName(it.current, next)
		job.setItemCurrent(it, next)
		job.setItemOutputs(it, result.Artifacts)
		job.setItemStatus(it, core.ItemStatusRunning, nil)
		r.emit(core.ItemDone{JobID: job.id, ItemID: it.id, Outputs: result.Artifacts})
		return nil
	}

	var finals []core.Artifact
	for _, art := range result.Artifacts {
		art.SuggestedName = carrySuggestedName(it.current, art)
		final, err := r.commit(job, it, art, op.Spec())
		if errors.Is(err, core.ErrSkip) {
			continue
		}
		if err != nil {
			return r.failItem(job, it, err)
		}
		finals = append(finals, final)
	}

	if len(finals) == 0 {
		job.setItemOutputs(it, nil)
		job.setItemStatus(it, core.ItemStatusSkipped, nil)
		r.emit(core.ItemDone{JobID: job.id, ItemID: it.id})
		return nil
	}

	job.setItemOutputs(it, finals)
	job.setItemStatus(it, core.ItemStatusDone, nil)
	r.emit(core.ItemDone{JobID: job.id, ItemID: it.id, Outputs: finals})
	return nil
}

// carrySuggestedName keeps a download's friendly title through chained steps,
// adjusting the extension to whatever the step actually produced.
func carrySuggestedName(prev, out core.Artifact) string {
	if out.SuggestedName != "" {
		return out.SuggestedName
	}
	name := prev.SuggestedName
	if name == "" {
		return ""
	}
	stem := strings.TrimSuffix(name, filepath.Ext(name))
	return stem + "." + core.ExtFor(out.Format, prev)
}

func (r *Runner) commit(job *Job, it *item, art core.Artifact, spec core.Spec) (core.Artifact, error) {
	r.commitMu.Lock()
	defer r.commitMu.Unlock()

	if err := os.MkdirAll(job.target.Dir, 0o755); err != nil {
		return core.Artifact{}, core.Wrap(core.CodeOutput, spec.ID, job.target.Dir, err,
			"check permissions on the output directory")
	}

	proposed := job.target.OutputPath(it.input, art.Format, spec.ID, it.index, time.Now())
	if art.SuggestedName != "" {
		proposed = filepath.Join(job.target.Dir, core.SanitizeFilename(art.SuggestedName))
	}
	final, err := core.UniquePath(proposed, job.target.Collision)
	if err != nil {
		return core.Artifact{}, err
	}
	if samePath(final, it.input.Path) {
		return core.Artifact{}, core.NewError(core.CodeOutput, spec.ID, final,
			"refusing to overwrite the input file", "choose a different output directory or the suffix collision policy")
	}
	backup := ""
	if job.target.Collision == core.CollisionOverwrite && r.undoDir != "" {
		if _, statErr := os.Stat(final); statErr == nil {
			backupDir := filepath.Join(r.undoDir, job.id)
			if err := os.MkdirAll(backupDir, 0o755); err != nil {
				return core.Artifact{}, core.Wrap(core.CodeOutput, spec.ID, backupDir, err,
					"check permissions on the undo directory")
			}
			candidate := filepath.Join(backupDir, core.SanitizeFilename(filepath.Base(final)))
			backup, err = core.UniquePath(candidate, core.CollisionSuffix)
			if err != nil {
				return core.Artifact{}, core.Wrap(core.CodeOutput, spec.ID, candidate, err, "")
			}
			if err := moveFile(final, backup); err != nil {
				return core.Artifact{}, core.Wrap(core.CodeOutput, spec.ID, final, err,
					"could not preserve the overwritten file for undo")
			}
		}
	}
	if err := moveFile(art.Path, final); err != nil {
		if backup != "" {
			_ = moveFile(backup, final)
		}
		return core.Artifact{}, core.Wrap(core.CodeOutput, spec.ID, final, err,
			"check permissions on the output directory")
	}

	art.Path = final
	if backup != "" {
		if art.Meta == nil {
			art.Meta = map[string]any{}
		}
		art.Meta["backup"] = backup
		art.Meta["overwritten"] = true
	}
	if info, statErr := os.Stat(final); statErr == nil {
		art.Size = info.Size()
	}
	return art, nil
}

func safeRun(ctx context.Context, op core.Operation, req core.Request, rep core.Reporter) (result core.Result, err error) {
	defer func() {
		if p := recover(); p != nil {
			err = core.NewError(core.CodeInternal, op.Spec().ID, "",
				fmt.Sprintf("operation panic: %v", p), "this is a Workbench bug; the item failed cleanly")
		}
	}()
	return op.Run(ctx, req, rep)
}

func (r *Runner) failItem(job *Job, it *item, err error) error {
	job.setItemStatus(it, core.ItemStatusFailed, err)
	r.log(job, core.LevelError, "%s: %v", it.input.Name(), err)
	r.emit(core.ItemDone{JobID: job.id, ItemID: it.id, Err: err})
	return err
}

func (r *Runner) cancelItem(job *Job, it *item, err error) error {
	job.setItemStatus(it, core.ItemStatusCanceled, err)
	r.log(job, core.LevelWarn, "%s cancelled: %v", it.input.Name(), err)
	r.emit(core.ItemDone{JobID: job.id, ItemID: it.id, Err: err})
	return err
}

func sumBytesIn(result core.Result, stepIndex int) int64 {
	if stepIndex != 0 {
		return 0
	}
	return result.Metrics.BytesIn
}

func sumBytesOut(result core.Result, terminal bool) int64 {
	if !terminal {
		return 0
	}
	return result.Metrics.BytesOut
}

func pathWithin(base, path string) bool {
	rel, err := filepath.Rel(base, path)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel)
}

func samePath(a, b string) bool {
	absA, errA := filepath.Abs(a)
	absB, errB := filepath.Abs(b)
	if errA == nil && errB == nil && filepath.Clean(absA) == filepath.Clean(absB) {
		return true
	}
	infoA, errA := os.Stat(a)
	infoB, errB := os.Stat(b)
	return errA == nil && errB == nil && os.SameFile(infoA, infoB)
}

// moveFile prefers an atomic rename and falls back to copy+rename when the
// cache and output live on different filesystems.
func moveFile(src, dst string) (err error) {
	if err := os.Rename(src, dst); err == nil {
		return nil
	}

	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	mode := os.FileMode(0o644)
	if info, statErr := in.Stat(); statErr == nil {
		mode = info.Mode().Perm()
	}

	part := dst + ".part"
	out, err := os.OpenFile(part, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(out, in)
	closeErr := out.Close()
	if copyErr == nil {
		copyErr = closeErr
	}
	if copyErr != nil {
		_ = os.Remove(part)
		return copyErr
	}
	if err := os.Rename(part, dst); err != nil {
		_ = os.Remove(part)
		return err
	}
	_ = os.Remove(src)
	return nil
}

func (r *Runner) persistLogs(job *Job) {
	if r.logDir == "" {
		return
	}
	text := job.logText()
	if text == "" {
		return
	}
	if err := os.MkdirAll(r.logDir, 0o755); err != nil {
		return
	}
	path := filepath.Join(r.logDir, job.id+".log")
	if err := os.WriteFile(path, []byte(text), 0o644); err == nil {
		job.setLogPath(path)
	}
}
