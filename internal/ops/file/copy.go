// Package file implements generic file operations.
package file

import (
	"context"
	"os"
	"path/filepath"
	"time"

	"github.com/solomonadzape95/workbench/internal/core"
	"github.com/solomonadzape95/workbench/internal/engines"
	"github.com/solomonadzape95/workbench/internal/ops/oputil"
)

var copySpec = core.Spec{
	ID:        "file.copy",
	Title:     "Copy file",
	Summary:   "Copy files without changing their contents",
	Domain:    core.File,
	Accepts:   nil, // any file kind
	Params:    nil,
	Engines:   []core.EngineID{engines.Internal},
	Batchable: true,
	Cost:      core.CostFast,
}

// Copy is useful on its own, and it is the simplest way to exercise the runner
// end to end.
type Copy struct{}

func (Copy) Spec() core.Spec { return copySpec }

func (Copy) Validate(ctx context.Context, req core.Request) error {
	if len(req.Items) == 0 {
		return core.NewError(core.CodeUsage, copySpec.ID, "", "no input files", "")
	}
	return core.Validate(copySpec.Params, req.Params)
}

func (Copy) Run(ctx context.Context, req core.Request, rep core.Reporter) (core.Result, error) {
	start := time.Now()
	var out []core.Artifact
	var bytesIn, bytesOut int64

	for _, item := range req.Items {
		if err := ctx.Err(); err != nil {
			return core.Result{}, err
		}

		ext := item.Ext()
		name := core.SanitizeStem(item.Stem())
		if ext != "" {
			name += "." + ext
		}
		outPath := filepath.Join(req.Target.Dir, name)

		rep.Stage("copying")
		rep.Progress(0, int(item.Size))
		_, err := copyOne(ctx, item.Path, outPath, item.Size, rep)
		if err != nil {
			return core.Result{}, core.Wrap(core.CodeInput, copySpec.ID, item.Path, err, "")
		}

		info, statErr := os.Stat(outPath)
		if statErr != nil {
			return core.Result{}, core.Wrap(core.CodeOutput, copySpec.ID, outPath, statErr, "")
		}
		out = append(out, core.Artifact{
			Path:   outPath,
			Kind:   item.Kind,
			Format: item.Format,
			Size:   info.Size(),
			Meta:   item.Meta,
		})
		bytesIn += item.Size
		bytesOut += info.Size()
		rep.Progress(int(item.Size), int(item.Size))
	}

	return core.Result{
		Artifacts: out,
		Metrics: core.Metrics{
			BytesIn:  bytesIn,
			BytesOut: bytesOut,
			Duration: time.Since(start),
			Engine:   engines.Internal,
		},
	}, nil
}

func copyOne(ctx context.Context, src, dst string, size int64, rep core.Reporter) (int64, error) {
	in, err := os.Open(src)
	if err != nil {
		return 0, err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return 0, err
	}

	var lastReport time.Time
	report := func(done int64) {
		if time.Since(lastReport) < 100*time.Millisecond && done < size {
			return
		}
		lastReport = time.Now()
		rep.Progress(int(done), int(size))
	}

	written, err := oputil.Copy(ctx, out, in, report)
	closeErr := out.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		_ = os.Remove(dst)
		return written, err
	}
	if info, statErr := os.Stat(src); statErr == nil {
		_ = os.Chtimes(dst, time.Now(), info.ModTime())
	}
	return written, nil
}
