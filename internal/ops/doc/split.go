package doc

import (
	"context"
	"os"
	"path/filepath"
	"time"

	"github.com/solomonadzape95/workbench/internal/core"
	"github.com/solomonadzape95/workbench/internal/engines"
	"github.com/solomonadzape95/workbench/internal/engines/poppler"
)

var splitSpec = core.Spec{
	ID:      "doc.split",
	Title:   "Split PDF",
	Summary: "Split a PDF into single pages or page groups",
	Domain:  core.Document,
	Accepts: []core.Accept{{Kind: core.Document, Formats: []string{"pdf"}}},
	Params: []core.ParamSpec{
		{Key: "every", Label: "Pages per part", Type: core.ParamInt, Default: 1, Min: floatPtr(1),
			Help: "1 creates one PDF per page; higher values group pages."},
		{Key: "ranges", Label: "Ranges", Type: core.ParamString,
			Help: "Optional comma-separated page groups, for example 1-3,5,7-9. Overrides pages per part."},
	},
	Output:         core.OutputSpec{StaticFormat: "pdf", Kind: core.Document},
	Engines:        []core.EngineID{engines.Poppler},
	RequireEngines: true,
	Batchable:      true,
	Cost:           core.CostFast,
}

type Split struct {
	Poppler *poppler.Client
}

func (s Split) Spec() core.Spec { return splitSpec }

func (s Split) Validate(ctx context.Context, req core.Request) error {
	if err := core.Validate(splitSpec.Params, req.Params); err != nil {
		return err
	}
	if len(req.Items) == 0 {
		return core.NewError(core.CodeUsage, splitSpec.ID, "", "no PDFs", "")
	}
	if s.Poppler == nil {
		return core.NewError(core.CodeEngineMissing, splitSpec.ID, "", "poppler is not installed", poppler.InstallHint())
	}
	if _, err := s.Poppler.ResolvePdfSeparate(); err != nil {
		return err
	}
	for _, item := range req.Items {
		if !splitSpec.AcceptsArtifact(item) {
			return core.NewError(core.CodeInput, splitSpec.ID, item.Path, "only PDFs can be split", "")
		}
	}
	return nil
}

func (s Split) Run(ctx context.Context, req core.Request, rep core.Reporter) (core.Result, error) {
	start := time.Now()
	every := req.Params.Int("every")
	ranges := req.Params.String("ranges")
	var out []core.Artifact
	var bytesIn, bytesOut int64
	for _, item := range req.Items {
		if err := ctx.Err(); err != nil {
			return core.Result{}, err
		}
		prefix := core.SanitizeStem(item.Stem())
		paths, err := s.Poppler.SplitPages(ctx, item.Path, req.Target.Dir, prefix, every, ranges, rep)
		if err != nil {
			if ctx.Err() != nil {
				return core.Result{}, ctx.Err()
			}
			return core.Result{}, core.Wrap(core.CodeEngineFailed, splitSpec.ID, item.Path, err, core.HintOf(err))
		}
		for _, path := range paths {
			info, err := os.Stat(path)
			if err != nil {
				return core.Result{}, core.Wrap(core.CodeOutput, splitSpec.ID, path, err, "")
			}
			out = append(out, core.Artifact{
				Path: path, Kind: core.Document, Format: "pdf", Size: info.Size(),
				SuggestedName: filepath.Base(path),
				Meta: map[string]any{
					"source": item.Path,
					"part":   filepath.Base(path),
				},
			})
			bytesOut += info.Size()
		}
		bytesIn += item.Size
		rep.Progress(1, 1)
	}
	return core.Result{
		Artifacts: out,
		Metrics: core.Metrics{
			BytesIn: bytesIn, BytesOut: bytesOut, Duration: time.Since(start), Engine: engines.Poppler,
		},
	}, nil
}
