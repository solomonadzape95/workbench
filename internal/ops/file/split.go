package file

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/solomonadzape95/workbench/internal/core"
	"github.com/solomonadzape95/workbench/internal/engines"
)

var splitSpec = core.Spec{
	ID:      "file.split",
	Title:   "Split file",
	Summary: "Split a large file into numbered parts",
	Domain:  core.File,
	Accepts: nil, // any file kind
	Params: []core.ParamSpec{
		{Key: "size", Label: "Part size", Type: core.ParamString, Default: "100m", Required: true,
			Help: "Bytes per part: 10m, 500k, or a raw byte count."},
	},
	Output:    core.OutputSpec{StaticFormat: "part", Kind: core.File},
	Engines:   []core.EngineID{engines.Internal},
	Batchable: true,
	Cost:      core.CostFast,
}

type Split struct{}

func (Split) Spec() core.Spec { return splitSpec }

func (Split) Validate(ctx context.Context, req core.Request) error {
	if err := core.Validate(splitSpec.Params, req.Params); err != nil {
		return err
	}
	if _, err := parsePartSize(req.Params.String("size")); err != nil {
		return core.NewError(core.CodeUsage, splitSpec.ID, "", err.Error(), "use a value such as 100m or 500k")
	}
	if len(req.Items) == 0 {
		return core.NewError(core.CodeUsage, splitSpec.ID, "", "no input files", "")
	}
	return nil
}

func (Split) Run(ctx context.Context, req core.Request, rep core.Reporter) (core.Result, error) {
	start := time.Now()
	size, err := parsePartSize(req.Params.String("size"))
	if err != nil {
		return core.Result{}, core.NewError(core.CodeUsage, splitSpec.ID, "", err.Error(), "")
	}

	var out []core.Artifact
	var bytesIn, bytesOut int64
	for _, item := range req.Items {
		if err := ctx.Err(); err != nil {
			return core.Result{}, err
		}
		parts, written, err := splitOne(ctx, req, item, size, rep)
		if err != nil {
			return core.Result{}, core.Wrap(core.CodeOutput, splitSpec.ID, item.Path, err,
				"check permissions on the input and output directories")
		}
		out = append(out, parts...)
		bytesIn += item.Size
		bytesOut += written
		rep.Progress(1, 1)
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

func splitOne(ctx context.Context, req core.Request, item core.Artifact, size int64, rep core.Reporter) ([]core.Artifact, int64, error) {
	in, err := os.Open(item.Path)
	if err != nil {
		return nil, 0, err
	}
	defer in.Close()

	stem := core.SanitizeStem(item.Stem())
	ext := item.Ext()
	reader := bufio.NewReaderSize(in, 128*1024)
	var parts []core.Artifact
	var total int64
	for index := 1; ; index++ {
		if err := ctx.Err(); err != nil {
			return nil, total, err
		}
		name := fmt.Sprintf("%s.part%03d", stem, index)
		if ext != "" {
			name += "." + ext
		}
		path := filepath.Join(req.Target.Dir, name)
		f, err := os.Create(path)
		if err != nil {
			return nil, total, err
		}
		n, copyErr := io.CopyN(f, reader, size)
		closeErr := f.Close()
		if copyErr != nil && copyErr != io.EOF {
			return nil, total, copyErr
		}
		if closeErr != nil {
			return nil, total, closeErr
		}
		if n == 0 {
			_ = os.Remove(path)
			break
		}
		total += n
		parts = append(parts, core.Artifact{
			Path:          path,
			Kind:          core.File,
			Format:        "part",
			Size:          n,
			SuggestedName: name,
			Meta: map[string]any{
				"source": item.Path,
				"part":   index,
				"bytes":  n,
			},
		})
		if n < size {
			break
		}
		rep.Stage(fmt.Sprintf("writing part %d", index))
	}
	return parts, total, nil
}

func parsePartSize(raw string) (int64, error) {
	raw = strings.ToLower(strings.TrimSpace(raw))
	if raw == "" {
		return 0, fmt.Errorf("part size is required")
	}
	multiplier := float64(1)
	for _, suffix := range []struct {
		unit string
		mult float64
	}{
		{"gb", 1 << 30}, {"g", 1 << 30},
		{"mb", 1 << 20}, {"m", 1 << 20},
		{"kb", 1 << 10}, {"k", 1 << 10},
		{"b", 1},
	} {
		if strings.HasSuffix(raw, suffix.unit) {
			multiplier = suffix.mult
			raw = strings.TrimSpace(strings.TrimSuffix(raw, suffix.unit))
			break
		}
	}
	value, err := strconv.ParseFloat(raw, 64)
	if err != nil || value <= 0 {
		return 0, fmt.Errorf("want a positive size such as 100m or 500k")
	}
	return int64(value * multiplier), nil
}
