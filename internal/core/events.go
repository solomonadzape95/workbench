package core

import "time"

type Status string

const (
	StatusQueued     Status = "queued"
	StatusRunning    Status = "running"
	StatusCancelling Status = "cancelling"
	StatusDone       Status = "done"
	StatusPartial    Status = "partial"
	StatusFailed     Status = "failed"
	StatusCanceled   Status = "canceled"
)

type ItemStatus string

const (
	ItemStatusPending  ItemStatus = "pending"
	ItemStatusRunning  ItemStatus = "running"
	ItemStatusDone     ItemStatus = "done"
	ItemStatusFailed   ItemStatus = "failed"
	ItemStatusSkipped  ItemStatus = "skipped"
	ItemStatusCanceled ItemStatus = "canceled"
)

// Event is emitted by the runner for every state change worth rendering.
// Events are immutable and safe to send across goroutines.
type Event interface {
	isEvent()
}

type JobQueued struct {
	JobID string
	Name  string
	Items int
}

type JobStarted struct {
	JobID string
	At    time.Time
}

type StepStarted struct {
	JobID  string
	StepID string
	Index  int
	Op     string
}

type ItemStarted struct {
	JobID    string
	ItemID   string
	Artifact Artifact
}

type ItemProgress struct {
	JobID  string
	ItemID string
	Done   int
	Total  int
	Stage  string
}

type ItemDone struct {
	JobID   string
	ItemID  string
	Outputs []Artifact
	Err     error
}

type StepDone struct {
	JobID  string
	StepID string
	Index  int
	Err    error
}

type JobDone struct {
	JobID    string
	Status   Status
	Err      error
	Duration time.Duration
	Metrics  Metrics
}

type JobLog struct {
	JobID string
	Level Level
	Line  string
}

func (JobQueued) isEvent()    {}
func (JobStarted) isEvent()   {}
func (StepStarted) isEvent()  {}
func (ItemStarted) isEvent()  {}
func (ItemProgress) isEvent() {}
func (ItemDone) isEvent()     {}
func (StepDone) isEvent()     {}
func (JobDone) isEvent()      {}
func (JobLog) isEvent()       {}
