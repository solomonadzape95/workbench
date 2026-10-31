package audio

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/solomonadzape95/workbench/internal/core"
	"github.com/solomonadzape95/workbench/internal/engines"
	"github.com/solomonadzape95/workbench/internal/engines/ffmpeg"
	"github.com/solomonadzape95/workbench/internal/ops/media"
)

var speedSpec = core.Spec{
	ID:      "audio.speed",
	Title:   "Change audio speed",
	Summary: "Speed audio up or down, with or without pitch correction",
	Domain:  core.Audio,
	Accepts: []core.Accept{{Kind: core.Audio}},
	Params: []core.ParamSpec{
		{Key: "factor", Label: "Speed factor", Type: core.ParamFloat, Default: 1.5,
			Min: floatPtr(0.25), Max: floatPtr(4.0),
			Help: "0.25–4.0. 2.0 is twice as fast."},
		{Key: "preserve_pitch", Label: "Keep pitch", Type: core.ParamBool, Default: true,
			Help: "Off shifts pitch like a tape machine."},
	},
	Engines:   []core.EngineID{engines.FFmpeg},
	Batchable: true,
	Cost:      core.CostMedium,
}

type Speed struct {
	Client *ffmpeg.Client
}

func (s Speed) Spec() core.Spec { return speedSpec }

func (s Speed) Validate(ctx context.Context, req core.Request) error {
	return checkAudioInputs(speedSpec, req, s.Client)
}

func (s Speed) Run(ctx context.Context, req core.Request, rep core.Reporter) (core.Result, error) {
	start := time.Now()
	factor := req.Params.Float("factor")
	if factor == 0 {
		factor = 1.5
	}
	preservePitch := req.Params.Bool("preserve_pitch")

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
		if !m.HasAudio {
			return core.Result{}, core.NewError(core.CodeInput, speedSpec.ID, item.Path,
				"no audio stream found", "choose an audio file")
		}

		filter := media.AtempoChain(factor)
		if !preservePitch {
			if m.SampleRate <= 0 {
				return core.Result{}, core.NewError(core.CodeInput, speedSpec.ID, item.Path,
					"source sample rate is unknown", "turn pitch preservation back on")
			}
			rate := int(math.Round(float64(m.SampleRate) * factor))
			if rate < 1000 || rate > 384000 {
				return core.Result{}, core.NewError(core.CodeUsage, speedSpec.ID, item.Path,
					fmt.Sprintf("pitch-shifted sample rate %d is out of range", rate), "choose a smaller speed factor")
			}
			filter = fmt.Sprintf("asetrate=%d,aresample=%d", rate, m.SampleRate)
		}

		format, err := audioFormat(item)
		if err != nil {
			return core.Result{}, core.NewError(core.CodeInput, speedSpec.ID, item.Path, err.Error(), "")
		}
		encoder, _, ok := media.AudioEncoder(format)
		if !ok {
			return core.Result{}, core.NewError(core.CodeInput, speedSpec.ID, item.Path,
				fmt.Sprintf("unsupported audio format %q", format), "")
		}

		output := media.OutputPath(req, item, format)
		args := []string{"-i", item.Path, "-vn", "-map_metadata", "0", "-filter:a", filter, "-c:a", encoder}
		args = qualityArgs(args, encoder, "192k")
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
		art, err := media.Finish(req, item, output, core.Audio, format, m, duration,
			map[string]any{"speed": factor, "preserve_pitch": preservePitch})
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

func floatPtr(v float64) *float64 { return &v }
