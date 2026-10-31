package video

import (
	"context"
	"strconv"
	"time"

	"github.com/solomonadzape95/workbench/internal/core"
	"github.com/solomonadzape95/workbench/internal/engines"
	"github.com/solomonadzape95/workbench/internal/engines/ffmpeg"
	"github.com/solomonadzape95/workbench/internal/ops/media"
)

var speedSpec = core.Spec{
	ID:      "video.speed",
	Title:   "Change video speed",
	Summary: "Speed a video up or down, with audio kept in sync",
	Domain:  core.Video,
	Accepts: []core.Accept{{Kind: core.Video}},
	Params: []core.ParamSpec{
		{Key: "factor", Label: "Speed factor", Type: core.ParamFloat, Default: 1.5,
			Min: floatPtr(0.25), Max: floatPtr(4.0),
			Help: "0.25–4.0. 2.0 is twice as fast."},
		{Key: "with_audio", Label: "Keep audio", Type: core.ParamBool, Default: true,
			Help: "Audio is time-stretched to stay in sync."},
	},
	Engines:   []core.EngineID{engines.FFmpeg},
	Batchable: true,
	Cost:      core.CostSlow,
}

type Speed struct {
	Client *ffmpeg.Client
}

func (s Speed) Spec() core.Spec { return speedSpec }

func (s Speed) Validate(ctx context.Context, req core.Request) error {
	return checkVideoInputs(speedSpec, req, s.Client)
}

func (s Speed) Run(ctx context.Context, req core.Request, rep core.Reporter) (core.Result, error) {
	start := time.Now()
	factor := req.Params.Float("factor")
	if factor == 0 {
		factor = 1.5
	}
	withAudio := req.Params.Bool("with_audio")
	factorText := strconv.FormatFloat(factor, 'f', -1, 64)

	var out []core.Artifact
	var bytesIn, bytesOut int64
	for _, item := range req.Items {
		if err := ctx.Err(); err != nil {
			return core.Result{}, err
		}
		m, err := media.Probe(ctx, s.Client, speedSpec.ID, item, rep)
		if err != nil {
			return core.Result{}, err
		}
		if !m.HasVideo {
			return core.Result{}, core.NewError(core.CodeInput, speedSpec.ID, item.Path,
				"no video stream found", "choose a video file")
		}

		container, err := videoContainer(item)
		if err != nil {
			return core.Result{}, core.NewError(core.CodeInput, speedSpec.ID, item.Path, err.Error(), "")
		}
		codecArgs, err := media.VideoArgs(container, "auto", "23", "medium")
		if err != nil {
			return core.Result{}, core.NewError(core.CodeInput, speedSpec.ID, item.Path, err.Error(), "")
		}

		output := media.OutputPath(req, item, container)
		args := []string{
			"-i", item.Path,
			"-map_metadata", "0",
			"-vf", "setpts=PTS/" + factorText,
		}
		if withAudio && m.HasAudio {
			args = append(args, "-af", media.AtempoChain(factor))
		} else {
			args = append(args, "-an")
		}
		args = append(args, codecArgs...)
		if container == "mp4" || container == "mov" {
			args = append(args, "-movflags", "+faststart")
		}
		args = append(args, output)

		duration := time.Duration(0)
		if m.Duration > 0 {
			duration = time.Duration(float64(m.Duration) / factor)
		}

		rep.Stage("transcoding")
		if err := s.Client.Run(ctx, args, duration, rep); err != nil {
			if ctx.Err() != nil {
				return core.Result{}, ctx.Err()
			}
			return core.Result{}, core.Wrap(core.CodeEngineFailed, speedSpec.ID, item.Path, err, core.HintOf(err))
		}
		art, err := media.Finish(req, item, output, core.Video, container, m, duration,
			map[string]any{"speed": factor, "with_audio": withAudio})
		if err != nil {
			return core.Result{}, err
		}
		out = append(out, art)
		bytesIn += item.Size
		bytesOut += art.Size
	}

	return core.Result{
		Artifacts: out,
		Metrics: core.Metrics{
			BytesIn:  bytesIn,
			BytesOut: bytesOut,
			Duration: time.Since(start),
			Engine:   engines.FFmpeg,
		},
	}, nil
}
