package doc

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/solomonadzape95/workbench/internal/core"
	"github.com/solomonadzape95/workbench/internal/engines"
	"github.com/solomonadzape95/workbench/internal/engines/ghostscript"
)

var mergeSpec = core.Spec{
	ID:      "doc.merge",
	Title:   "Merge PDFs",
	Summary: "Combine several PDFs into one file",
	Domain:  core.Document,
	Accepts: []core.Accept{{Kind: core.Document, Formats: []string{"pdf"}}},
	Params: []core.ParamSpec{
		{
			Key: "name", Label: "Output name", Type: core.ParamString,
			Help: "Optional filename; defaults to the first PDF's name plus -merged.pdf.",
		},
		{
			Key: "preset", Label: "Quality", Type: core.ParamEnum, Default: "prepress",
			Options: []core.Option{
				{Value: "screen", Label: "Screen (smallest)"},
				{Value: "ebook", Label: "Ebook"},
				{Value: "prepress", Label: "Prepress (best)"},
			},
		},
	},
	Output:         core.OutputSpec{StaticFormat: "pdf", Kind: core.Document},
	Engines:        []core.EngineID{engines.Ghostscript},
	RequireEngines: true,
	Mode:           core.ModeAggregate,
	Cost:           core.CostMedium,
}

type Merge struct {
	Ghostscript *ghostscript.Client
}

func (m Merge) Spec() core.Spec { return mergeSpec }

func (m Merge) Validate(ctx context.Context, req core.Request) error {
	if err := core.Validate(mergeSpec.Params, req.Params); err != nil {
		return err
	}
	if len(req.Items) < 2 {
		return core.NewError(core.CodeUsage, mergeSpec.ID, "", "merge needs at least two PDFs", "add more inputs")
	}
	if m.Ghostscript == nil {
		return core.NewError(core.CodeEngineMissing, mergeSpec.ID, "", "ghostscript is not configured", ghostscript.InstallHint())
	}
	if err := m.Ghostscript.Check(); err != nil {
		return core.Wrap(core.CodeEngineMissing, mergeSpec.ID, "", err, ghostscript.InstallHint())
	}
	for _, item := range req.Items {
		if !mergeSpec.AcceptsArtifact(item) {
			return core.NewError(core.CodeInput, mergeSpec.ID, item.Path, "merge accepts PDFs only", "choose PDF files")
		}
	}
	return nil
}

func (m Merge) Run(ctx context.Context, req core.Request, rep core.Reporter) (core.Result, error) {
	start := time.Now()
	name := strings.TrimSpace(req.Params.String("name"))
	if name == "" {
		name = core.SanitizeStem(req.Items[0].Stem()) + "-merged.pdf"
	}
	name = core.SanitizeFilename(name)
	if !strings.HasSuffix(strings.ToLower(name), ".pdf") {
		name += ".pdf"
	}
	output := filepath.Join(req.Target.Dir, name)
	inputs := make([]string, 0, len(req.Items))
	for _, item := range req.Items {
		inputs = append(inputs, item.Path)
	}
	preset := req.Params.String("preset")
	if preset == "" {
		preset = "prepress"
	}
	if err := m.Ghostscript.Merge(ctx, inputs, output, preset, rep); err != nil {
		if ctx.Err() != nil {
			return core.Result{}, ctx.Err()
		}
		return core.Result{}, core.Wrap(core.CodeEngineFailed, mergeSpec.ID, "", err, core.HintOf(err))
	}
	info, err := os.Stat(output)
	if err != nil {
		return core.Result{}, core.Wrap(core.CodeOutput, mergeSpec.ID, output, err,
			"ghostscript reported success but produced no output")
	}
	var bytesIn int64
	for _, item := range req.Items {
		bytesIn += item.Size
	}
	artifact := core.Artifact{
		Path: output, Kind: core.Document, Format: "pdf", Size: info.Size(),
		SuggestedName: name,
		Meta: map[string]any{
			"source": fmt.Sprintf("%d PDFs", len(req.Items)),
			"engine": string(engines.Ghostscript),
			"preset": preset,
			"merged": true,
		},
	}
	return core.Result{
		Artifacts: []core.Artifact{artifact},
		Metrics: core.Metrics{
			BytesIn: bytesIn, BytesOut: info.Size(), Duration: time.Since(start), Engine: engines.Ghostscript,
		},
	}, nil
}
