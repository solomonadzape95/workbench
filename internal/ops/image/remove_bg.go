package image

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/solomonadzape95/workbench/internal/core"
	"github.com/solomonadzape95/workbench/internal/engines"
	"github.com/solomonadzape95/workbench/internal/engines/rembg"
)

var removeBGSpec = core.Spec{
	ID:      "image.remove_bg",
	Title:   "Remove background",
	Summary: "Cut the subject out of an image and write a transparent PNG",
	Domain:  core.Image,
	Accepts: []core.Accept{{Kind: core.Image, Formats: []string{"png", "jpg", "jpeg", "gif"}}},
	Params: []core.ParamSpec{
		{
			Key: "model", Label: "Model", Type: core.ParamEnum, Default: "bria-rmbg",
			Options: []core.Option{
				{Value: "u2net", Label: "U2Net (general)"},
				{Value: "u2netp", Label: "U2Net lightweight"},
				{Value: "isnet-general-use", Label: "ISNet general"},
				{Value: "bria-rmbg", Label: "BRIA RMBG (default)"},
			},
			Help: "Models download on first use; BRIA is the default.",
		},
		{
			Key: "alpha_matting", Label: "Refine edges", Type: core.ParamBool, Default: false, Advanced: true,
			Help: "Slower alpha matting for hair and soft edges.",
		},
	},
	Output:         core.OutputSpec{StaticFormat: "png", Kind: core.Image},
	Engines:        []core.EngineID{engines.Rembg},
	RequireEngines: true,
	Batchable:      true,
	Cost:           core.CostSlow,
}

type RemoveBG struct {
	Rembg *rembg.Client
}

func (RemoveBG) Spec() core.Spec { return removeBGSpec }

func (c RemoveBG) Validate(ctx context.Context, req core.Request) error {
	if err := core.Validate(removeBGSpec.Params, req.Params); err != nil {
		return err
	}
	if len(req.Items) == 0 {
		return core.NewError(core.CodeUsage, removeBGSpec.ID, "", "no input files", "")
	}
	if c.Rembg == nil {
		return core.NewError(core.CodeEngineMissing, removeBGSpec.ID, "",
			"rembg is not installed", rembg.InstallHint())
	}
	if err := c.Rembg.Check(); err != nil {
		return err
	}
	for _, item := range req.Items {
		if !removeBGSpec.AcceptsArtifact(item) {
			return core.NewError(core.CodeInput, removeBGSpec.ID, item.Path,
				fmt.Sprintf("%s is not a supported image format", item.Format),
				"supported inputs: PNG, JPEG, GIF")
		}
	}
	return nil
}

func (c RemoveBG) Run(ctx context.Context, req core.Request, rep core.Reporter) (core.Result, error) {
	start := time.Now()
	model := req.Params.String("model")
	if model == "" {
		model = "bria-rmbg"
	}
	opts := rembg.Options{Model: model, AlphaMatting: req.Params.Bool("alpha_matting")}

	var out []core.Artifact
	var bytesIn, bytesOut int64
	for _, item := range req.Items {
		if err := ctx.Err(); err != nil {
			return core.Result{}, err
		}
		output := rembg.OutputName(req.Target.Dir, item.Stem())
		rep.Progress(0, 1)
		rep.Stage("removing background")
		if err := c.Rembg.Remove(ctx, item.Path, output, opts, rep); err != nil {
			_ = os.Remove(output)
			if ctx.Err() != nil {
				return core.Result{}, ctx.Err()
			}
			return core.Result{}, core.Wrap(core.CodeEngineFailed, removeBGSpec.ID, item.Path, err,
				core.HintOf(err))
		}
		info, err := os.Stat(output)
		if err != nil {
			return core.Result{}, core.Wrap(core.CodeOutput, removeBGSpec.ID, output, err, "")
		}
		out = append(out, core.Artifact{
			Path:   output,
			Kind:   core.Image,
			Format: "png",
			Size:   info.Size(),
			Meta: map[string]any{
				"source": item.Path,
				"model":  model,
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
			Engine:   engines.Rembg,
			Model:    model,
		},
	}, nil
}
