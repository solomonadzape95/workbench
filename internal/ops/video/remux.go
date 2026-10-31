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

var remuxSpec = core.Spec{
	ID:      "video.remux",
	Title:   "Remux video",
	Summary: "Move video and audio into another container without re-encoding",
	Domain:  core.Video,
	Accepts: []core.Accept{{Kind: core.Video}},
	Params: []core.ParamSpec{
		{Key: "container", Label: "Container", Type: core.ParamEnum, Default: "mp4", Options: containerOptions()},
	},
	Output:    core.OutputSpec{FormatParam: "container", Kind: core.Video},
	Engines:   []core.EngineID{engines.FFmpeg},
	Batchable: true,
	Cost:      core.CostFast,
}

type Remux struct {
	Client *ffmpeg.Client
}

func (r Remux) Spec() core.Spec { return remuxSpec }

func (r Remux) Validate(ctx context.Context, req core.Request) error {
	if err := checkVideoInputs(remuxSpec, req, r.Client); err != nil {
		return err
	}
	for _, item := range req.Items {
		m, err := r.Client.Probe(ctx, item.Path)
		if err != nil {
			return core.Wrap(core.CodeInput, remuxSpec.ID, item.Path, err, core.HintOf(err))
		}
		if !m.HasVideo {
			return core.NewError(core.CodeInput, remuxSpec.ID, item.Path,
				"no video stream found", "choose a video file")
		}
	}
	return nil
}

func (r Remux) Run(ctx context.Context, req core.Request, rep core.Reporter) (core.Result, error) {
	started := time.Now()
	container := req.Params.String("container")
	if container == "" {
		container = "mp4"
	}

	var out []core.Artifact
	var bytesIn, bytesOut int64
	for _, item := range req.Items {
		if err := ctx.Err(); err != nil {
			return core.Result{}, err
		}
		m, err := media.Probe(ctx, r.Client, remuxSpec.ID, item, rep)
		if err != nil {
			return core.Result{}, err
		}
		if !m.HasVideo {
			return core.Result{}, core.NewError(core.CodeInput, remuxSpec.ID, item.Path,
				"no video stream found", "choose a video file")
		}

		output := media.OutputPath(req, item, container)
		args := []string{
			"-i", item.Path,
			"-map", "0:v:0",
			"-map", "0:a?",
			"-map_metadata", "0",
			"-c", "copy",
		}
		if container == "mp4" || container == "mov" {
			args = append(args, "-movflags", "+faststart")
		}
		args = append(args, output)

		rep.Stage("remuxing")
		if err := r.Client.Run(ctx, args, m.Duration, rep); err != nil {
			if ctx.Err() != nil {
				return core.Result{}, ctx.Err()
			}
			hint := fmt.Sprintf("the source codec may not be supported by %s; use video.convert to re-encode", container)
			return core.Result{}, core.Wrap(core.CodeEngineFailed, remuxSpec.ID, item.Path, err, hint)
		}
		art, err := media.Finish(req, item, output, core.Video, container, m, m.Duration,
			map[string]any{"stream_copy": true})
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
