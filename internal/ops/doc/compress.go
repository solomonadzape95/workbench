package doc

import (
	"context"
	"os"
	"path/filepath"
	"time"

	"github.com/solomonadzape95/workbench/internal/core"
	"github.com/solomonadzape95/workbench/internal/engines"
	"github.com/solomonadzape95/workbench/internal/engines/ghostscript"
)

var compressSpec = core.Spec{
	ID:      "doc.compress",
	Title:   "Compress PDF",
	Summary: "Shrink a PDF with Ghostscript presets",
	Domain:  core.Document,
	Accepts: []core.Accept{{Kind: core.Document, Formats: []string{"pdf"}}},
	Params: []core.ParamSpec{
		{
			Key: "preset", Label: "Preset", Type: core.ParamEnum, Default: "ebook",
			Options: []core.Option{
				{Value: "screen", Label: "Screen (smallest, 72 dpi)"},
				{Value: "ebook", Label: "Ebook (150 dpi)"},
				{Value: "prepress", Label: "Prepress (300 dpi)"},
			},
			Help: "Ghostscript's PDFSETTINGS presets. Ebook is the usual balance.",
		},
	},
	Output:         core.OutputSpec{StaticFormat: "pdf", Kind: core.Document},
	Engines:        []core.EngineID{engines.Ghostscript},
	RequireEngines: true,
	Batchable:      true,
	Cost:           core.CostMedium,
}

type Compress struct {
	Ghostscript *ghostscript.Client
}

func (c Compress) Spec() core.Spec { return compressSpec }

func (c Compress) Validate(ctx context.Context, req core.Request) error {
	if err := core.Validate(compressSpec.Params, req.Params); err != nil {
		return err
	}
	if len(req.Items) == 0 {
		return core.NewError(core.CodeUsage, compressSpec.ID, "", "no PDF files", "")
	}
	if c.Ghostscript == nil {
		return core.NewError(core.CodeEngineMissing, compressSpec.ID, "",
			"ghostscript is not configured", ghostscript.InstallHint())
	}
	if err := c.Ghostscript.Check(); err != nil {
		return core.Wrap(core.CodeEngineMissing, compressSpec.ID, "", err, ghostscript.InstallHint())
	}
	for _, item := range req.Items {
		if !compressSpec.AcceptsArtifact(item) {
			return core.NewError(core.CodeInput, compressSpec.ID, item.Path,
				"only PDF files can be compressed", "choose a PDF")
		}
	}
	return nil
}

func (c Compress) Run(ctx context.Context, req core.Request, rep core.Reporter) (core.Result, error) {
	start := time.Now()
	preset := req.Params.String("preset")
	if preset == "" {
		preset = "ebook"
	}
	var out []core.Artifact
	var bytesIn, bytesOut int64
	for _, item := range req.Items {
		if err := ctx.Err(); err != nil {
			return core.Result{}, err
		}
		output := filepath.Join(req.Target.Dir, core.SanitizeStem(item.Stem())+"."+core.ExtFor("pdf", item))
		rep.Stage("compressing PDF")
		if err := c.Ghostscript.Compress(ctx, item.Path, output, preset, rep); err != nil {
			if ctx.Err() != nil {
				return core.Result{}, ctx.Err()
			}
			return core.Result{}, core.Wrap(core.CodeEngineFailed, compressSpec.ID, item.Path, err, core.HintOf(err))
		}
		info, err := os.Stat(output)
		if err != nil {
			return core.Result{}, core.Wrap(core.CodeOutput, compressSpec.ID, output, err,
				"ghostscript reported success but produced no output")
		}
		out = append(out, core.Artifact{
			Path:   output,
			Kind:   core.Document,
			Format: "pdf",
			Size:   info.Size(),
			Meta: map[string]any{
				"source": item.Path,
				"engine": string(engines.Ghostscript),
				"preset": preset,
			},
		})
		bytesIn += item.Size
		bytesOut += info.Size()
		rep.Progress(1, 1)
	}
	return core.Result{
		Artifacts: out,
		Metrics: core.Metrics{
			BytesIn:  bytesIn,
			BytesOut: bytesOut,
			Duration: time.Since(start),
			Engine:   engines.Ghostscript,
		},
	}, nil
}
