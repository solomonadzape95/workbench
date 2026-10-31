package video

import (
	"context"
	"fmt"
	"time"

	"github.com/solomonadzape95/workbench/internal/core"
	"github.com/solomonadzape95/workbench/internal/engines"
	"github.com/solomonadzape95/workbench/internal/engines/ffmpeg"
	"github.com/solomonadzape95/workbench/internal/ops/media"
)

var thumbnailSpec = core.Spec{
	ID:      "video.thumbnail",
	Title:   "Extract thumbnail",
	Summary: "Save one frame from a video as a PNG or JPEG image",
	Domain:  core.Video,
	Accepts: []core.Accept{{Kind: core.Video}},
	Params: []core.ParamSpec{
		{Key: "at", Label: "At", Type: core.ParamDuration, Default: "1s",
			Help: "Where to grab the frame."},
		{Key: "format", Label: "Format", Type: core.ParamEnum, Default: "png",
			Options: []core.Option{{Value: "png", Label: "PNG"}, {Value: "jpeg", Label: "JPEG"}}},
		{Key: "width", Label: "Size", Type: core.ParamInt, Default: 0,
			Min:  floatPtr(0),
			Help: "0 keeps the source size. Otherwise this sets the scaled width; height follows the aspect ratio."},
	},
	Output:    core.OutputSpec{FormatParam: "format", Kind: core.Image},
	Engines:   []core.EngineID{engines.FFmpeg},
	Batchable: true,
	Cost:      core.CostFast,
}

type Thumbnail struct {
	Client *ffmpeg.Client
}

func (t Thumbnail) Spec() core.Spec { return thumbnailSpec }

func (t Thumbnail) Validate(ctx context.Context, req core.Request) error {
	return checkVideoInputs(thumbnailSpec, req, t.Client)
}

func (t Thumbnail) Run(ctx context.Context, req core.Request, rep core.Reporter) (core.Result, error) {
	started := time.Now()
	at := req.Params.Duration("at")
	if _, present := req.Params["at"]; !present {
		at = time.Second
	}
	format := req.Params.String("format")
	if format == "" {
		format = "png"
	}
	width := req.Params.Int("width")

	var out []core.Artifact
	var bytesIn, bytesOut int64
	for _, item := range req.Items {
		if err := ctx.Err(); err != nil {
			return core.Result{}, err
		}
		m, err := media.Probe(ctx, t.Client, thumbnailSpec.ID, item, rep)
		if err != nil {
			return core.Result{}, err
		}
		if !m.HasVideo {
			return core.Result{}, core.NewError(core.CodeInput, thumbnailSpec.ID, item.Path,
				"no video stream found", "choose a video file")
		}

		output := media.OutputPath(req, item, format)
		args := make([]string, 0, 12)
		if at > 0 {
			args = append(args, "-ss", formatSeconds(at))
		}
		args = append(args, "-i", item.Path, "-frames:v", "1", "-an", "-update", "1")
		if width > 0 {
			args = append(args, "-vf", fmt.Sprintf("scale=%d:-2", width))
		}
		if format == "jpeg" {
			args = append(args, "-q:v", "2")
		}
		args = append(args, output)

		rep.Stage("extracting frame")
		if err := t.Client.Run(ctx, args, 0, rep); err != nil {
			if ctx.Err() != nil {
				return core.Result{}, ctx.Err()
			}
			return core.Result{}, core.Wrap(core.CodeEngineFailed, thumbnailSpec.ID, item.Path, err, core.HintOf(err))
		}
		extra := map[string]any{"at_ms": at.Milliseconds()}
		if width > 0 {
			extra["width"] = width
			if m.Width > 0 && m.Height > 0 {
				scaledHeight := (m.Height*width + m.Width/2) / m.Width
				extra["height"] = scaledHeight &^ 1
			}
		}
		art, err := media.Finish(req, item, output, core.Image, format, m, 0, extra)
		if err != nil {
			return core.Result{}, err
		}
		// A still has no duration, even though the source does.
		art.Meta["duration_ms"] = int64(0)
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
