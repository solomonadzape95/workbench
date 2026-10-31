package core

import "fmt"

// StepInput binds a step to its inputs. The MVP supports the implicit previous
// step; the field is already shaped for explicit DAG edges.
type StepInput struct {
	From string `toml:"from,omitempty"` // "" or "previous"; v1.1: another step ID
}

type OnError string

const (
	OnErrorContinue OnError = "continue"
	OnErrorStop     OnError = "stop"
)

type Step struct {
	ID      string    `toml:"id,omitempty"`
	Op      string    `toml:"op"`
	Params  Values    `toml:"params,omitempty"`
	Input   StepInput `toml:"input,omitempty"`
	OnError OnError   `toml:"on_error,omitempty"`
}

type Pipeline struct {
	Name        string `toml:"name"`
	Version     int    `toml:"version"`
	Description string `toml:"description,omitempty"`
	Steps       []Step `toml:"step"`
}

// PipelineVersion is the highest pipeline file version this build understands.
const PipelineVersion = 1

// Validate checks that the pipeline can run against reg. It reports the first
// problem with enough context to fix it.
func (p Pipeline) Validate(reg *Registry) error {
	if len(p.Steps) == 0 {
		return NewError(CodeUsage, "", "", "pipeline has no steps", "add at least one operation")
	}
	for i, step := range p.Steps {
		op, ok := reg.Get(step.Op)
		if !ok {
			return NewError(CodeUsage, step.Op, "", fmt.Sprintf("step %d uses unknown operation %q", i+1, step.Op), "")
		}
		if i == 0 {
			if step.Input.From != "" && step.Input.From != "previous" {
				return NewError(CodeUsage, step.Op, "", "the first step cannot read from another step", "")
			}
		} else if step.Input.From != "" && step.Input.From != "previous" {
			return NewError(CodeUsage, step.Op, "",
				fmt.Sprintf("step %d reads from %q; only the previous step is supported in v1", i+1, step.Input.From), "")
		}
		params := op.Spec().DefaultValues()
		for key, value := range step.Params {
			params[key] = value
		}
		if err := Validate(op.Spec().Params, params); err != nil {
			return err
		}
	}
	return nil
}
