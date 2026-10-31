package file

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/solomonadzape95/workbench/internal/core"
	"github.com/solomonadzape95/workbench/internal/engines"
)

var moveSpec = core.Spec{
	ID:      "file.move",
	Title:   "Move file",
	Summary: "Move files into the output directory without copying them",
	Domain:  core.File,
	Accepts: nil,
	Output:  core.OutputSpec{Kind: core.File},
	Engines: []core.EngineID{engines.Internal},
	Mode:    core.ModeDirect,
	Cost:    core.CostFast,
}

type Move struct{}

func (Move) Spec() core.Spec { return moveSpec }

func (Move) Validate(ctx context.Context, req core.Request) error {
	if len(req.Items) == 0 {
		return core.NewError(core.CodeUsage, moveSpec.ID, "", "no files to move", "")
	}
	return nil
}

func (Move) Run(ctx context.Context, req core.Request, rep core.Reporter) (core.Result, error) {
	return moveWithTemplate(ctx, req, rep, moveSpec.ID, req.Target.Template)
}

var renameSpec = core.Spec{
	ID:      "file.rename",
	Title:   "Rename file",
	Summary: "Move files with a pattern-based output name",
	Domain:  core.File,
	Accepts: nil,
	Params: []core.ParamSpec{
		{Key: "template", Label: "Name pattern", Type: core.ParamString, Default: "{stem}.{ext}",
			Help: "Supports {stem}, {ext}, {index}, {counter}, {date}, and {time}."},
	},
	Output:    core.OutputSpec{Kind: core.File},
	Engines:   []core.EngineID{engines.Internal},
	Mode:      core.ModeDirect,
	Chainable: true,
	Cost:      core.CostFast,
}

type Rename struct{}

func (Rename) Spec() core.Spec { return renameSpec }

func (Rename) Validate(ctx context.Context, req core.Request) error {
	if err := core.Validate(renameSpec.Params, req.Params); err != nil {
		return err
	}
	if len(req.Items) == 0 {
		return core.NewError(core.CodeUsage, renameSpec.ID, "", "no files to rename", "")
	}
	if req.Params.String("template") == "" {
		return core.NewError(core.CodeUsage, renameSpec.ID, "", "name pattern is required", "")
	}
	return nil
}

func (Rename) Run(ctx context.Context, req core.Request, rep core.Reporter) (core.Result, error) {
	template := req.Params.String("template")
	if template == "" {
		template = "{stem}.{ext}"
	}
	return moveWithTemplate(ctx, req, rep, renameSpec.ID, template)
}

func moveWithTemplate(ctx context.Context, req core.Request, rep core.Reporter, opID, template string) (core.Result, error) {
	start := time.Now()
	if req.Target.Dir == "" {
		return core.Result{}, core.NewError(core.CodeUsage, opID, "", "no output directory", "")
	}
	if err := os.MkdirAll(req.Target.Dir, 0o755); err != nil {
		return core.Result{}, core.Wrap(core.CodeOutput, opID, req.Target.Dir, err, "")
	}
	var out []core.Artifact
	var bytesIn, bytesOut int64
	for i, item := range req.Items {
		if err := ctx.Err(); err != nil {
			return core.Result{}, err
		}
		target := req.Target
		if template != "" {
			target.Template = template
		}
		proposed := target.OutputPath(item, item.Ext(), opID, i, time.Now())
		final, err := core.UniquePath(proposed, target.Collision)
		if err != nil {
			return core.Result{}, core.Wrap(core.CodeOutput, opID, proposed, err,
				"choose a different output directory or collision policy")
		}
		if sameFilePath(item.Path, final) {
			return core.Result{}, core.NewError(core.CodeOutput, opID, final,
				"source and destination are the same file", "choose a different output directory or name pattern")
		}
		rep.Stage("moving " + item.Name())
		if err := movePath(item.Path, final); err != nil {
			return core.Result{}, core.Wrap(core.CodeOutput, opID, item.Path, err,
				"check permissions on the source and output directories")
		}
		info, err := os.Stat(final)
		if err != nil {
			return core.Result{}, core.Wrap(core.CodeOutput, opID, final, err, "")
		}
		out = append(out, core.Artifact{
			Path: final, Kind: item.Kind, Format: item.Format, Size: info.Size(),
			SuggestedName: filepath.Base(final),
			Meta: map[string]any{
				"source": item.Path,
				"moved":  true,
			},
		})
		bytesIn += item.Size
		bytesOut += info.Size()
		rep.Progress(i+1, len(req.Items))
	}
	return core.Result{
		Artifacts: out,
		Metrics: core.Metrics{
			BytesIn: bytesIn, BytesOut: bytesOut, Duration: time.Since(start), Engine: engines.Internal,
		},
	}, nil
}

func movePath(src, dst string) error {
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
	if copyErr != nil {
		_ = os.Remove(part)
		return copyErr
	}
	if closeErr != nil {
		_ = os.Remove(part)
		return closeErr
	}
	if err := os.Rename(part, dst); err != nil {
		_ = os.Remove(part)
		return err
	}
	return os.Remove(src)
}

func sameFilePath(a, b string) bool {
	absA, errA := filepath.Abs(a)
	absB, errB := filepath.Abs(b)
	if errA == nil && errB == nil && filepath.Clean(absA) == filepath.Clean(absB) {
		return true
	}
	return false
}
