package video

import (
	"context"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/solomonadzape95/workbench/internal/core"
	"github.com/solomonadzape95/workbench/internal/engines"
	"github.com/solomonadzape95/workbench/internal/engines/ffmpeg"
	"github.com/solomonadzape95/workbench/internal/ops/media"
)

var watermarkSpec = core.Spec{
	ID:      "video.watermark",
	Title:   "Watermark video",
	Summary: "Overlay a PNG or JPEG image on top of a video",
	Domain:  core.Video,
	Accepts: []core.Accept{{Kind: core.Video}},
	Params: []core.ParamSpec{
		{Key: "watermark", Label: "Watermark image", Type: core.ParamPath, Required: true,
			Help: "PNG or JPEG image to overlay."},
		{Key: "position", Label: "Position", Type: core.ParamEnum, Default: "bottom-right",
			Options: watermarkPositionOptions()},
		{Key: "opacity", Label: "Opacity", Type: core.ParamFloat, Default: 1.0,
			Min: floatPtr(0.1), Max: floatPtr(1.0)},
		{Key: "margin", Label: "Margin", Type: core.ParamInt, Default: 16,
			Min:  floatPtr(0),
			Help: "Distance from the nearest edges, in pixels."},
	},
	Engines:   []core.EngineID{engines.FFmpeg},
	Batchable: true,
	Cost:      core.CostSlow,
}

type Watermark struct {
	Client *ffmpeg.Client
}

func (w Watermark) Spec() core.Spec { return watermarkSpec }

func (w Watermark) Validate(ctx context.Context, req core.Request) error {
	if err := checkVideoInputs(watermarkSpec, req, w.Client); err != nil {
		return err
	}
	image := req.Params.String("watermark")
	if err := requireExistingFile(image, "watermark image"); err != nil {
		return core.NewError(core.CodeInput, watermarkSpec.ID, image, err.Error(),
			"choose a PNG or JPEG image")
	}
	ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(image), "."))
	switch ext {
	case "png", "jpg", "jpeg":
	default:
		return core.NewError(core.CodeInput, watermarkSpec.ID, image,
			"watermark must be a PNG or JPEG image", "choose a .png, .jpg, or .jpeg file")
	}
	return nil
}

func (w Watermark) Run(ctx context.Context, req core.Request, rep core.Reporter) (core.Result, error) {
	started := time.Now()
	image := req.Params.String("watermark")
	position := req.Params.String("position")
	if position == "" {
		position = "bottom-right"
	}
	opacity := req.Params.Float("opacity")
	if _, present := req.Params["opacity"]; !present || opacity <= 0 || opacity > 1 {
		opacity = 1.0
	}
	margin := req.Params.Int("margin")
	if _, present := req.Params["margin"]; !present {
		margin = 16
	}
	if margin < 0 {
		margin = 0
	}

	var out []core.Artifact
	var bytesIn, bytesOut int64
	for _, item := range req.Items {
		if err := ctx.Err(); err != nil {
			return core.Result{}, err
		}
		m, err := media.Probe(ctx, w.Client, watermarkSpec.ID, item, rep)
		if err != nil {
			return core.Result{}, err
		}
		if !m.HasVideo {
			return core.Result{}, core.NewError(core.CodeInput, watermarkSpec.ID, item.Path,
				"no video stream found", "choose a video file")
		}

		container, err := videoContainer(item)
		if err != nil {
			return core.Result{}, core.NewError(core.CodeInput, watermarkSpec.ID, item.Path, err.Error(), "")
		}
		codecArgs, err := media.VideoArgs(container, "auto", "20", "medium")
		if err != nil {
			return core.Result{}, core.NewError(core.CodeInput, watermarkSpec.ID, item.Path, err.Error(), "")
		}

		filter := overlayFilter(position, opacity, margin)

		output := media.OutputPath(req, item, container)
		args := []string{
			"-i", item.Path,
			"-i", image,
			"-filter_complex", filter,
			"-map", "[out]",
			"-map", "0:a?",
			"-map_metadata", "0",
		}
		args = append(args, codecArgs...)
		if container == "mp4" || container == "mov" {
			args = append(args, "-movflags", "+faststart")
		}
		args = append(args, output)

		rep.Stage("watermarking")
		if err := w.Client.Run(ctx, args, m.Duration, rep); err != nil {
			if ctx.Err() != nil {
				return core.Result{}, ctx.Err()
			}
			return core.Result{}, core.Wrap(core.CodeEngineFailed, watermarkSpec.ID, item.Path, err, core.HintOf(err))
		}
		art, err := media.Finish(req, item, output, core.Video, container, m, m.Duration,
			map[string]any{
				"position":  position,
				"opacity":   opacity,
				"margin":    margin,
				"watermark": image,
			})
		if err != nil {
			return core.Result{}, err
		}
		art.SuggestedName = outputBase(output)
		out = append(out, art)
		bytesIn += item.Size
		bytesOut += art.Size
	}

	return core.Result{
		Artifacts: out,
		Metrics: core.Metrics{
			BytesIn:  bytesIn,
			BytesOut: bytesOut,
			Duration: time.Since(started),
			Engine:   engines.FFmpeg,
		},
	}, nil
}

func watermarkPositionOptions() []core.Option {
	return []core.Option{
		{Value: "top-left", Label: "Top left"},
		{Value: "top-right", Label: "Top right"},
		{Value: "bottom-left", Label: "Bottom left"},
		{Value: "bottom-right", Label: "Bottom right"},
		{Value: "center", Label: "Center"},
	}
}

// overlayFilter builds the filter_complex graph that scales the watermark to
// RGBA (optionally with reduced alpha) and overlays it on the video.
func overlayFilter(position string, opacity float64, margin int) string {
	x, y := overlayPosition(position, margin)
	watermark := "[1:v]format=rgba"
	if opacity < 1 {
		watermark += ",colorchannelmixer=aa=" + strconv.FormatFloat(opacity, 'f', -1, 64)
	}
	return fmt.Sprintf("%s[wm];[0:v][wm]overlay=%s:%s[out]", watermark, x, y)
}

// overlayPosition returns ffmpeg overlay x/y expressions for a placement.
func overlayPosition(position string, margin int) (string, string) {
	m := strconv.Itoa(margin)
	switch position {
	case "top-left":
		return m, m
	case "top-right":
		return "W-w-" + m, m
	case "bottom-left":
		return m, "H-h-" + m
	case "center":
		return "(W-w)/2", "(H-h)/2"
	default:
		return "W-w-" + m, "H-h-" + m
	}
}
