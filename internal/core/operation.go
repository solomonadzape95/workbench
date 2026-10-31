package core

import (
	"context"
	"time"
)

// EngineID identifies an execution backend. The engines package aliases this
// type so core stays dependency-free.
type EngineID string

// SourceKind describes what the input field accepts for a pipeline.
type SourceKind string

const (
	SourceFiles SourceKind = "files"
	SourceURLs  SourceKind = "urls"
)

// Mode controls how a step fans out across inputs and whether it may chain.
type Mode string

const (
	ModePerItem   Mode = "per_item"
	ModeAggregate Mode = "aggregate"
	ModeDirect    Mode = "direct"
)

type Cost string

const (
	CostFast   Cost = "fast"
	CostMedium Cost = "medium"
	CostSlow   Cost = "slow"
)

type Accept struct {
	Kind    Kind
	Formats []string // empty means any format of Kind
}

type OutputSpec struct {
	FormatParam  string // parameter key that chooses the output format
	StaticFormat string // used when the format never changes
	Kind         Kind   // empty means the operation's Domain; use for cross-domain ops
}

type Spec struct {
	ID             string
	Title          string
	Summary        string
	Domain         Kind
	Accepts        []Accept
	Params         []ParamSpec
	Output         OutputSpec
	Engines        []EngineID
	RequireEngines bool       // every listed engine must be available
	Source         SourceKind // empty means SourceFiles
	Batchable      bool
	Chainable      bool // direct operations may be followed by another step
	Mode           Mode
	Cost           Cost
}

func (s Spec) Param(key string) (ParamSpec, bool) {
	for _, p := range s.Params {
		if p.Key == key {
			return p, true
		}
	}
	return ParamSpec{}, false
}

func (s Spec) DefaultValues() Values {
	return Defaults(s.Params)
}

// Aggregate reports whether the step consumes every active item at once.
func (s Spec) Aggregate() bool { return s.Mode == ModeAggregate }

// Direct reports whether the step writes to the final target without a commit.
func (s Spec) Direct() bool { return s.Mode == ModeDirect }

func (s Spec) SourceKind() SourceKind {
	if s.Source == "" {
		return SourceFiles
	}
	return s.Source
}

// OutputKind reports the kind of artifact this operation produces.
func (s Spec) OutputKind() Kind {
	if s.Output.Kind != "" {
		return s.Output.Kind
	}
	if s.Domain == File {
		return Unknown // generic file ops inherit their input kind
	}
	return s.Domain
}

func (s Spec) AcceptsArtifact(a Artifact) bool {
	if len(s.Accepts) == 0 {
		return true
	}
	for _, accept := range s.Accepts {
		if accept.Kind != a.Kind {
			continue
		}
		if len(accept.Formats) == 0 {
			return true
		}
		for _, format := range accept.Formats {
			if format == a.Format || format == a.Ext() {
				return true
			}
		}
	}
	return false
}

// Request is one operation invocation. The runner supplies a fresh Target
// directory per item; operations write only there.
type Request struct {
	Items  []Artifact
	Params Values
	Target Target
	DryRun bool
}

type Skip struct {
	Artifact Artifact
	Reason   string
}

type Metrics struct {
	BytesIn  int64
	BytesOut int64
	Duration time.Duration
	Engine   EngineID
	Model    string
}

type Level string

const (
	LevelDebug Level = "debug"
	LevelInfo  Level = "info"
	LevelWarn  Level = "warn"
	LevelError Level = "error"
)

// Reporter is how an operation tells the runner what is happening. It must be
// safe to call from the goroutine running the operation.
type Reporter interface {
	Stage(name string)
	Progress(done, total int)
	Detail(item Artifact, msg string)
	Log(level Level, format string, args ...any)
}

// Operation is a reusable capability with a typed parameter schema.
type Operation interface {
	Spec() Spec
	Validate(ctx context.Context, req Request) error
	Run(ctx context.Context, req Request, rep Reporter) (Result, error)
}

type Result struct {
	Artifacts []Artifact
	Skipped   []Skip
	Metrics   Metrics
}
