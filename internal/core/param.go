package core

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// ParamType drives both form rendering and validation.
type ParamType string

const (
	ParamString   ParamType = "string"
	ParamInt      ParamType = "int"
	ParamFloat    ParamType = "float"
	ParamBool     ParamType = "bool"
	ParamEnum     ParamType = "enum"
	ParamDuration ParamType = "duration"
	ParamPath     ParamType = "path"
)

// Option is one choice in an enum parameter.
type Option struct {
	Value string
	Label string
}

// ParamSpec describes one input to an operation. One spec feeds the compose
// form, validation, pipeline serialization, and palette search.
type ParamSpec struct {
	Key      string
	Label    string
	Type     ParamType
	Default  any
	Options  []Option
	Min, Max *float64
	Required bool
	Advanced bool
	Help     string
}

// Values is a parameter set. It stores TOML-friendly primitives: string, int,
// float64, bool, and time.Duration.
type Values map[string]any

// Defaults returns every parameter's default value.
func Defaults(specs []ParamSpec) Values {
	v := make(Values, len(specs))
	for _, p := range specs {
		if p.Default != nil {
			v[p.Key] = p.Default
		}
	}
	return v
}

func (v Values) String(key string) string {
	switch x := v[key].(type) {
	case string:
		return x
	case fmt.Stringer:
		return x.String()
	case nil:
		return ""
	default:
		return fmt.Sprint(x)
	}
}

func (v Values) Int(key string) int {
	switch x := v[key].(type) {
	case int:
		return x
	case int64:
		return int(x)
	case float64:
		return int(x)
	case string:
		n, _ := strconv.Atoi(strings.TrimSpace(x))
		return n
	default:
		return 0
	}
}

func (v Values) Float(key string) float64 {
	switch x := v[key].(type) {
	case float64:
		return x
	case float32:
		return float64(x)
	case int:
		return float64(x)
	case int64:
		return float64(x)
	case string:
		f, _ := strconv.ParseFloat(strings.TrimSpace(x), 64)
		return f
	default:
		return 0
	}
}

func (v Values) Bool(key string) bool {
	switch x := v[key].(type) {
	case bool:
		return x
	case string:
		b, _ := strconv.ParseBool(strings.TrimSpace(x))
		return b
	default:
		return false
	}
}

func (v Values) Duration(key string) time.Duration {
	switch x := v[key].(type) {
	case time.Duration:
		return x
	case string:
		d, _ := time.ParseDuration(strings.TrimSpace(x))
		return d
	case int:
		return time.Duration(x) * time.Millisecond
	case int64:
		return time.Duration(x) * time.Millisecond
	case float64:
		return time.Duration(x * float64(time.Millisecond))
	default:
		return 0
	}
}

// Validate checks values against their specs and returns a usage error naming
// the offending field. It never mutates the values.
func Validate(specs []ParamSpec, values Values) error {
	for _, spec := range specs {
		raw, present := values[spec.Key]
		if spec.Required && missing(raw, present) {
			return NewError(CodeUsage, "", "", fmt.Sprintf("%s is required", label(spec)), "")
		}
		if !present || raw == nil {
			continue
		}

		switch spec.Type {
		case ParamEnum:
			value := values.String(spec.Key)
			if !hasOption(spec.Options, value) {
				return NewError(CodeUsage, "", "",
					fmt.Sprintf("%s must be one of %s", label(spec), optionList(spec.Options)), "")
			}
		case ParamInt:
			n := values.Int(spec.Key)
			if spec.Min != nil && float64(n) < *spec.Min {
				return NewError(CodeUsage, "", "", fmt.Sprintf("%s must be at least %g", label(spec), *spec.Min), "")
			}
			if spec.Max != nil && float64(n) > *spec.Max {
				return NewError(CodeUsage, "", "", fmt.Sprintf("%s must be at most %g", label(spec), *spec.Max), "")
			}
		case ParamFloat:
			n := values.Float(spec.Key)
			if spec.Min != nil && n < *spec.Min {
				return NewError(CodeUsage, "", "", fmt.Sprintf("%s must be at least %g", label(spec), *spec.Min), "")
			}
			if spec.Max != nil && n > *spec.Max {
				return NewError(CodeUsage, "", "", fmt.Sprintf("%s must be at most %g", label(spec), *spec.Max), "")
			}
		case ParamDuration:
			if values.Duration(spec.Key) < 0 {
				return NewError(CodeUsage, "", "", fmt.Sprintf("%s cannot be negative", label(spec)), "")
			}
		case ParamString, ParamPath:
			if spec.Required && strings.TrimSpace(values.String(spec.Key)) == "" {
				return NewError(CodeUsage, "", "", fmt.Sprintf("%s cannot be empty", label(spec)), "")
			}
		}
	}
	return nil
}

func missing(raw any, present bool) bool {
	if !present || raw == nil {
		return true
	}
	if s, ok := raw.(string); ok {
		return strings.TrimSpace(s) == ""
	}
	return false
}

func hasOption(options []Option, value string) bool {
	for _, o := range options {
		if o.Value == value {
			return true
		}
	}
	return false
}

func optionList(options []Option) string {
	names := make([]string, 0, len(options))
	for _, o := range options {
		names = append(names, o.Value)
	}
	return strings.Join(names, ", ")
}

func label(spec ParamSpec) string {
	if spec.Label != "" {
		return spec.Label
	}
	return spec.Key
}
