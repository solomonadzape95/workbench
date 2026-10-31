package video

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/solomonadzape95/workbench/internal/core"
	"github.com/solomonadzape95/workbench/internal/engines"
	"github.com/solomonadzape95/workbench/internal/engines/ffmpeg"
	"github.com/solomonadzape95/workbench/internal/ops/media"
)

var toGifSpec = core.Spec{
	ID:      "video.to_gif",
	Title:   "Convert to GIF",
	Summary: "Turn a video section into an animated GIF with a generated palette",
	Domain:  core.Video,
	Accepts: []core.Accept{{Kind: core.Video}},
	Params: []core.ParamSpec{
		{Key: "start", Label: "Start", Type: core.ParamDuration, Default: "0s",
			Help: "Where the GIF begins."},
		{Key: "duration", Label: "Duration", Type: core.ParamDuration, Default: "0s",
			Help: "Leave at 0 to use the rest of the video."},
		{Key: "fps", Label: "Frames per second", Type: core.ParamInt, Default: 12,
			Min: floatPtr(1)},
		{Key: "width", Label: "Width", Type: core.ParamInt, Default: 480,
			Min:  floatPtr(0),
			Help: "0 keeps the source width."},
	},
	Output:    core.OutputSpec{StaticFormat: "gif", Kind: core.File},
	Engines:   []core.EngineID{engines.FFmpeg},
	Batchable: true,
	Cost:      core.CostSlow,
}

type ToGif struct {
	Client *ffmpeg.Client
}

func (g ToGif) Spec() core.Spec { return toGifSpec }

func (g ToGif) Validate(ctx context.Context, req core.Request) error {
	return checkVideoInputs(toGifSpec, req, g.Client)
}

func (g ToGif) Run(ctx context.Context, req core.Request, rep core.Reporter) (core.Result, error) {
	started := time.Now()
	startAt := req.Params.Duration("start")
	span := req.Params.Duration("duration")
	fps := req.Params.Int("fps")
	if fps <= 0 {
		fps = 12
	}
	width := req.Params.Int("width")
	if _, present := req.Params["width"]; !present {
		width = 480
	}
	if width < 0 {
		width = 0
	}

	var out []core.Artifact
	var bytesIn, bytesOut int64
	for _, item := range req.Items {
		if err := ctx.Err(); err != nil {
			return core.Result{}, err
		}
		m, err := media.Probe(ctx, g.Client, toGifSpec.ID, item, rep)
		if err != nil {
			return core.Result{}, err
		}
		if !m.HasVideo {
			return core.Result{}, core.NewError(core.CodeInput, toGifSpec.ID, item.Path,
				"no video stream found", "choose a video file")
		}
		if m.Duration > 0 && startAt >= m.Duration {
			return core.Result{}, core.NewError(core.CodeInput, toGifSpec.ID, item.Path,
				"start is at or past the end of the video", "choose a start before the clip ends")
		}

		clipDuration := span
		if clipDuration == 0 && m.Duration > 0 {
			clipDuration = m.Duration - startAt
		}
		if clipDuration < 0 {
			clipDuration = 0
		}

		output := media.OutputPath(req, item, "gif")
		args := make([]string, 0, 16)
		if startAt > 0 {
			args = append(args, "-ss", formatSeconds(startAt))
		}
		args = append(args, "-i", item.Path)
		if clipDuration > 0 {
			args = append(args, "-t", formatSeconds(clipDuration))
		}
		args = append(args,
			"-filter_complex", gifFilter(fps, width),
			"-an", "-loop", "0",
			output,
		)

		rep.Stage("building GIF")
		if err := g.Client.Run(ctx, args, clipDuration, rep); err != nil {
			if ctx.Err() != nil {
				return core.Result{}, ctx.Err()
			}
			return core.Result{}, core.Wrap(core.CodeEngineFailed, toGifSpec.ID, item.Path, err, core.HintOf(err))
		}
		extra := map[string]any{
			"start_ms": startAt.Milliseconds(),
			"fps":      fps,
		}
		if width > 0 {
			extra["width"] = width
			if m.Width > 0 && m.Height > 0 {
				extra["height"] = (m.Height*width + m.Width/2) / m.Width
			}
		}
		art, err := media.Finish(req, item, output, core.File, "gif", m, clipDuration, extra)
		if err != nil {
			return core.Result{}, err
		}
		art.Meta["duration_ms"] = clipDuration.Milliseconds()
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

// gifFilter builds the palettegen/paletteuse chain used for a high-quality GIF.
func gifFilter(fps, width int) string {
	var steps []string
	if fps > 0 {
		steps = append(steps, fmt.Sprintf("fps=%d", fps))
	}
	if width > 0 {
		steps = append(steps, fmt.Sprintf("scale=%d:-1:flags=lanczos", width))
	}
	prefix := ""
	if len(steps) > 0 {
		prefix = strings.Join(steps, ",") + ","
	}
	return prefix + "split[s0][s1];[s0]palettegen[p];[s1][p]paletteuse"
}
