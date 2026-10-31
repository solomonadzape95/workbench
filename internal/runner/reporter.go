package runner

import (
	"sync"
	"time"

	"github.com/solomonadzape95/workbench/internal/core"
)

// reporter translates operation callbacks into job state updates and events.
// Progress is coalesced so a chatty engine cannot flood the UI.
type reporter struct {
	runner *Runner
	job    *Job
	it     *item

	mu    sync.Mutex
	done  int
	total int
	stage string
	last  time.Time
}

func (rep *reporter) Stage(name string) {
	rep.mu.Lock()
	rep.stage = name
	rep.mu.Unlock()
	rep.job.setProgress(rep.it, rep.done, rep.total, name)
	rep.emit()
}

func (rep *reporter) Progress(done, total int) {
	rep.mu.Lock()
	rep.done = done
	rep.total = total
	stage := rep.stage
	shouldEmit := time.Since(rep.last) >= 80*time.Millisecond || (total > 0 && done >= total)
	if shouldEmit {
		rep.last = time.Now()
	}
	rep.mu.Unlock()

	rep.job.setProgress(rep.it, done, total, stage)
	if shouldEmit {
		rep.emit()
	}
}

func (rep *reporter) Detail(item core.Artifact, msg string) {
	rep.job.setProgress(rep.it, rep.done, rep.total, msg)
	rep.emit()
}

func (rep *reporter) Log(level core.Level, format string, args ...any) {
	rep.runner.log(rep.job, level, format, args...)
}

func (rep *reporter) emit() {
	rep.runner.emit(core.ItemProgress{
		JobID:  rep.job.id,
		ItemID: rep.it.id,
		Done:   rep.done,
		Total:  rep.total,
		Stage:  rep.stage,
	})
}
