package image

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/solomonadzape95/workbench/internal/core"
	"github.com/solomonadzape95/workbench/internal/engines"
	"github.com/solomonadzape95/workbench/internal/engines/executil"
	"github.com/solomonadzape95/workbench/internal/engines/realesrgan"
)

var upscaleSpec = core.Spec{
	ID:      "image.upscale",
	Title:   "Upscale image",
	Summary: "Enlarge an image 2×, 3×, or 4× with Real-ESRGAN",
	Domain:  core.Image,
	Accepts: []core.Accept{{Kind: core.Image, Formats: []string{"png", "jpg", "jpeg", "webp"}}},
	Params: []core.ParamSpec{
		{
			Key: "scale", Label: "Scale", Type: core.ParamEnum, Default: "2",
			Options: []core.Option{
				{Value: "2", Label: "2×"},
				{Value: "3", Label: "3×"},
				{Value: "4", Label: "4×"},
			},
		},
		{
			Key: "model", Label: "Model", Type: core.ParamEnum, Default: "realesrgan-x4plus",
			Options: []core.Option{
				{Value: "realesrgan-x4plus", Label: "General photo"},
				{Value: "realesrgan-x4plus-anime", Label: "Anime / illustration"},
				{Value: "realesr-animevideov3", Label: "Anime video"},
			},
		},
		{
			Key: "format", Label: "Output", Type: core.ParamEnum, Default: "png",
			Options: []core.Option{
				{Value: "png", Label: "PNG"},
				{Value: "jpg", Label: "JPEG"},
				{Value: "webp", Label: "WebP"},
			},
		},
		{
			Key: "tile", Label: "Tile size", Type: core.ParamInt, Default: 0, Min: floatPtr(0),
			Advanced: true,
			Help:     "0 lets Real-ESRGAN choose; lower values use less VRAM.",
		},
	},
	Output:         core.OutputSpec{FormatParam: "format", Kind: core.Image},
	Engines:        []core.EngineID{engines.RealESRGAN},
	RequireEngines: true,
	Batchable:      true,
	Cost:           core.CostSlow,
}

type Upscale struct {
	Client *realesrgan.Client
}

func (Upscale) Spec() core.Spec { return upscaleSpec }

func (c Upscale) Validate(ctx context.Context, req core.Request) error {
	if err := core.Validate(upscaleSpec.Params, req.Params); err != nil {
		return err
	}
	if len(req.Items) == 0 {
		return core.NewError(core.CodeUsage, upscaleSpec.ID, "", "no input images", "")
	}
	if c.Client == nil {
		return core.NewError(core.CodeEngineMissing, upscaleSpec.ID, "",
			"Real-ESRGAN is not installed", realesrgan.InstallHint())
	}
	if err := c.Client.Check(); err != nil {
		return err
	}
	for _, item := range req.Items {
		if !upscaleSpec.AcceptsArtifact(item) {
			return core.NewError(core.CodeInput, upscaleSpec.ID, item.Path,
				fmt.Sprintf("%s is not a supported image format", item.Format),
				"supported inputs: PNG, JPEG, WebP")
		}
	}
	return nil
}

func (c Upscale) Run(ctx context.Context, req core.Request, rep core.Reporter) (core.Result, error) {
	start := time.Now()
	binary, err := c.Client.Binary()
	if err != nil {
		return core.Result{}, err
	}
	scale := req.Params.String("scale")
	if scale == "" {
		scale = "2"
	}
	model := req.Params.String("model")
	if model == "" {
		model = "realesrgan-x4plus"
	}
	format := normalizeFormat(req.Params.String("format"))
	if format == "" {
		format = "png"
	}
	tile := req.Params.Int("tile")

	var out []core.Artifact
	var bytesIn, bytesOut int64
	for _, item := range req.Items {
		if err := ctx.Err(); err != nil {
			return core.Result{}, err
		}
		ext := format
		if ext == "jpeg" {
			ext = "jpg"
		}
		output := filepath.Join(req.Target.Dir, core.SanitizeStem(item.Stem())+"."+ext)
		args := []string{"-i", item.Path, "-o", output, "-s", scale, "-n", model, "-f", ext}
		if tile > 0 {
			args = append(args, "-t", fmt.Sprint(tile))
		}
		cmd := exec.CommandContext(ctx, binary, args...)
		executil.Configure(cmd)
		cmd.WaitDelay = 10 * time.Second
		var stderr, stdout bytes.Buffer
		cmd.Stderr = &stderr
		cmd.Stdout = &stdout
		rep.Stage("upscaling " + item.Name())
		rep.Progress(0, 1)
		if err := cmd.Run(); err != nil {
			_ = os.Remove(output)
			if ctx.Err() != nil {
				return core.Result{}, ctx.Err()
			}
			detail := lastCommandLine(stderr.String())
			if detail == "" {
				detail = lastCommandLine(stdout.String())
			}
			if detail != "" {
				rep.Log(core.LevelError, "realesrgan: %s", detail)
			}
			return core.Result{}, core.NewError(core.CodeEngineFailed, upscaleSpec.ID, item.Path,
				"Real-ESRGAN failed: "+detail,
				"check that the Vulkan driver works and lower the tile size if VRAM is limited")
		}
		info, err := os.Stat(output)
		if err != nil {
			return core.Result{}, core.Wrap(core.CodeOutput, upscaleSpec.ID, output, err,
				"Real-ESRGAN reported success but produced no image")
		}
		out = append(out, core.Artifact{
			Path: output, Kind: core.Image, Format: format, Size: info.Size(),
			SuggestedName: filepath.Base(output),
			Meta: map[string]any{
				"source": item.Path,
				"scale":  scale,
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
			BytesIn: bytesIn, BytesOut: bytesOut, Duration: time.Since(start), Engine: engines.RealESRGAN,
		},
	}, nil
}

func lastCommandLine(text string) string {
	lines := strings.Split(strings.TrimSpace(text), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		if line := strings.TrimSpace(lines[i]); line != "" {
			if len(line) > 300 {
				line = line[:300] + "…"
			}
			return line
		}
	}
	return ""
}
