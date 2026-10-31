// Package mcp exposes Workbench operations over the Model Context Protocol
// stdio transport. It is intentionally small: initialize, tools/list, and
// tools/call are the only methods agents need.
package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/solomonadzape95/workbench/internal/alias"
	"github.com/solomonadzape95/workbench/internal/app"
	"github.com/solomonadzape95/workbench/internal/config"
	"github.com/solomonadzape95/workbench/internal/core"
	"github.com/solomonadzape95/workbench/internal/runner"
)

type Server struct {
	App    *app.App
	Runner *runner.Runner
	In     io.Reader
	Out    io.Writer
}

func New(a *app.App, r *runner.Runner, in io.Reader, out io.Writer) *Server {
	return &Server{App: a, Runner: r, In: in, Out: out}
}

type request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (s *Server) Serve(ctx context.Context) error {
	scanner := bufio.NewScanner(s.In)
	scanner.Buffer(make([]byte, 64*1024), 8<<20)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var req request
		if err := json.Unmarshal([]byte(line), &req); err != nil {
			s.write(response{JSONRPC: "2.0", Error: &rpcError{Code: -32700, Message: err.Error()}})
			continue
		}
		if len(req.ID) == 0 {
			// Notification; no response is allowed.
			continue
		}
		s.write(s.handle(ctx, req))
	}
	return scanner.Err()
}

func (s *Server) handle(ctx context.Context, req request) response {
	switch req.Method {
	case "initialize":
		return response{JSONRPC: "2.0", ID: req.ID, Result: map[string]any{
			"protocolVersion": "2024-11-05",
			"capabilities":    map[string]any{"tools": map[string]any{}},
			"serverInfo":      map[string]any{"name": "workbench", "version": "1"},
		}}
	case "ping":
		return response{JSONRPC: "2.0", ID: req.ID, Result: map[string]any{}}
	case "tools/list":
		return response{JSONRPC: "2.0", ID: req.ID, Result: map[string]any{"tools": s.tools()}}
	case "tools/call":
		result, err := s.call(ctx, req.Params)
		if err != nil {
			return response{JSONRPC: "2.0", ID: req.ID, Error: &rpcError{Code: -32602, Message: err.Error()}}
		}
		return response{JSONRPC: "2.0", ID: req.ID, Result: result}
	default:
		return response{JSONRPC: "2.0", ID: req.ID, Error: &rpcError{Code: -32601, Message: "unknown method " + req.Method}}
	}
}

func (s *Server) write(resp response) {
	data, err := json.Marshal(resp)
	if err != nil {
		return
	}
	_, _ = s.Out.Write(append(data, '\n'))
}

func (s *Server) tools() []map[string]any {
	ops := s.App.Operations.All()
	out := make([]map[string]any, 0, len(ops))
	for _, op := range ops {
		spec := op.Spec()
		out = append(out, map[string]any{
			"name":        toolName(spec.ID),
			"description": spec.Summary,
			"inputSchema": inputSchema(spec),
		})
	}
	return out
}

func toolName(opID string) string {
	return "wb_" + strings.NewReplacer(".", "_", "/", "_").Replace(opID)
}

func inputSchema(spec core.Spec) map[string]any {
	props := map[string]any{
		"inputs": map[string]any{
			"type": "array", "items": map[string]any{"type": "string"},
			"description": "File paths, globs, or @aliases.",
		},
		"output_dir": map[string]any{
			"type": "string", "description": "Output directory; defaults to the configured output folder.",
		},
	}
	required := []string{"inputs"}
	for _, p := range spec.Params {
		prop := map[string]any{"description": p.Help}
		switch p.Type {
		case core.ParamInt:
			prop["type"] = "integer"
		case core.ParamFloat:
			prop["type"] = "number"
		case core.ParamBool:
			prop["type"] = "boolean"
		case core.ParamEnum:
			prop["type"] = "string"
			values := make([]string, 0, len(p.Options))
			for _, option := range p.Options {
				values = append(values, option.Value)
			}
			prop["enum"] = values
		default:
			prop["type"] = "string"
		}
		if p.Default != nil {
			prop["default"] = p.Default
		}
		if p.Required {
			required = append(required, p.Key)
		}
		props[p.Key] = prop
	}
	return map[string]any{"type": "object", "properties": props, "required": required}
}

func (s *Server) call(ctx context.Context, raw json.RawMessage) (map[string]any, error) {
	var call struct {
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
	}
	if err := json.Unmarshal(raw, &call); err != nil {
		return nil, err
	}
	var op core.Operation
	for _, candidate := range s.App.Operations.All() {
		if toolName(candidate.Spec().ID) == call.Name {
			op = candidate
			break
		}
	}
	if op == nil {
		return nil, fmt.Errorf("unknown tool %q", call.Name)
	}
	spec := op.Spec()
	var args map[string]json.RawMessage
	if len(call.Arguments) > 0 && string(call.Arguments) != "null" {
		if err := json.Unmarshal(call.Arguments, &args); err != nil {
			return nil, fmt.Errorf("arguments: %w", err)
		}
	}
	var inputs []string
	if raw, ok := args["inputs"]; ok {
		if err := json.Unmarshal(raw, &inputs); err != nil {
			return nil, fmt.Errorf("inputs: %w", err)
		}
	}
	if len(inputs) == 0 {
		return nil, errors.New("inputs must contain at least one path")
	}
	outputDir := s.App.Config.Output.Dir
	if raw, ok := args["output_dir"]; ok {
		if err := json.Unmarshal(raw, &outputDir); err != nil {
			return nil, fmt.Errorf("output_dir: %w", err)
		}
	}
	values := spec.DefaultValues()
	for _, param := range spec.Params {
		raw, ok := args[param.Key]
		if !ok {
			continue
		}
		value, err := decodeParam(param, raw)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", param.Key, err)
		}
		values[param.Key] = value
	}
	artifacts, err := resolveInputs(inputs, s.App.Config.Aliases)
	if err != nil {
		return nil, err
	}
	target := core.Target{
		Dir:       config.ExpandHome(outputDir),
		Template:  s.App.Config.Output.Template,
		Collision: core.Collision(s.App.Config.Output.Collision),
	}
	pipeline := core.Pipeline{
		Name: spec.ID, Version: core.PipelineVersion,
		Steps: []core.Step{{Op: spec.ID, Params: values}},
	}
	job, err := s.Runner.Start(pipeline, artifacts, target)
	if err != nil {
		return nil, err
	}
	if err := waitJob(ctx, job); err != nil {
		return nil, err
	}
	view := job.Snapshot()
	summary := map[string]any{
		"job_id": view.ID, "status": view.Status, "name": view.Name,
		"output_dir": view.Target.Dir, "metrics": view.Metrics,
	}
	if view.Err != nil {
		summary["error"] = view.Err.Error()
	}
	var outputs []core.Artifact
	if len(view.Aggregate) > 0 {
		outputs = view.Aggregate
	} else {
		for _, item := range view.Items {
			outputs = append(outputs, item.Outputs...)
			if item.Err != nil && view.Err == nil {
				summary["error"] = item.Err.Error()
			}
		}
	}
	summary["outputs"] = outputs
	text, err := json.MarshalIndent(summary, "", "  ")
	if err != nil {
		return nil, err
	}
	failed := view.Status == core.StatusFailed || view.Status == core.StatusCanceled
	return map[string]any{
		"content": []map[string]any{{"type": "text", "text": string(text)}},
		"isError": failed,
	}, nil
}

func decodeParam(param core.ParamSpec, raw json.RawMessage) (any, error) {
	switch param.Type {
	case core.ParamInt:
		var n int
		if err := json.Unmarshal(raw, &n); err == nil {
			return n, nil
		}
		var f float64
		if err := json.Unmarshal(raw, &f); err != nil {
			return nil, fmt.Errorf("expected a whole number")
		}
		return int(f), nil
	case core.ParamFloat:
		var f float64
		if err := json.Unmarshal(raw, &f); err != nil {
			return nil, fmt.Errorf("expected a number")
		}
		return f, nil
	case core.ParamBool:
		var b bool
		if err := json.Unmarshal(raw, &b); err != nil {
			return nil, fmt.Errorf("expected true or false")
		}
		return b, nil
	case core.ParamDuration:
		var text string
		if err := json.Unmarshal(raw, &text); err != nil {
			return nil, fmt.Errorf("expected a duration string such as 30s")
		}
		d, err := time.ParseDuration(text)
		if err != nil {
			return nil, err
		}
		return d, nil
	default:
		var text string
		if err := json.Unmarshal(raw, &text); err != nil {
			return nil, fmt.Errorf("expected a string")
		}
		return text, nil
	}
}

func resolveInputs(inputs []string, aliases []config.Alias) ([]core.Artifact, error) {
	var out []core.Artifact
	for _, raw := range inputs {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		arts, err := alias.Resolve(raw, aliases)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", raw, err)
		}
		out = append(out, arts...)
	}
	if len(out) == 0 {
		return nil, errors.New("inputs matched no files")
	}
	return out, nil
}

func waitJob(ctx context.Context, job *runner.Job) error {
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	for {
		status := job.Snapshot().Status
		switch status {
		case core.StatusDone, core.StatusPartial, core.StatusFailed, core.StatusCanceled:
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}
