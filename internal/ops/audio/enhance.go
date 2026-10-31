package audio

import (
	"context"
	"fmt"
	"time"

	"github.com/solomonadzape95/workbench/internal/core"
	"github.com/solomonadzape95/workbench/internal/engines"
	"github.com/solomonadzape95/workbench/internal/engines/ffmpeg"
	"github.com/solomonadzape95/workbench/internal/ops/media"
)

var enhanceSpec = core.Spec{
	ID:      "audio.enhance",
	Title:   "Enhance audio",
	Summary: "Clean up speech or music and bring it to a consistent level",
	Domain:  core.Audio,
	Accepts: []core.Accept{{Kind: core.Audio}},
	Params: []core.ParamSpec{
		{Key: "preset", Label: "Preset", Type: core.ParamEnum, Default: "voice",
			Options: []core.Option{
				{Value: "voice", Label: "Voice (noise reduction + loudness)"},
				{Value: "podcast", Label: "Podcast (noise reduction)"},
				{Value: "music", Label: "Music (loudness only)"},
			},
			Help: "Keeps the source container and codec family."},
	},
	Engines:   []core.EngineID{engines.FFmpeg},
	Batchable: true,
	Cost:      core.CostSlow,
}

type Enhance struct {
	Client *ffmpeg.Client
}

func (e Enhance) Spec() core.Spec { return enhanceSpec }

func enhanceFilter(preset string) (string, error) {
	switch preset {
	case "voice":
		return "highpass=f=90,afftdn=nf=-25,loudnorm=I=-16:TP=-1.5:LRA=11", nil
	case "podcast":
		return "afftdn=nf=-25,loudnorm=I=-16", nil
	case "music":
		return "loudnorm=I=-14:TP=-1.5:LRA=11", nil
	default:
		return "", fmt.Errorf("unknown enhance preset %q", preset)
	}
}

func (e Enhance) Validate(ctx context.Context, req core.Request) error {
	preset := req.Params.String("preset")
	if preset == "" {
		preset = "voice"
	}
	if _, err := enhanceFilter(preset); err != nil {
		return core.NewError(core.CodeUsage, enhanceSpec.ID, "", err.Error(), "")
	}
	if err := checkAudioInputs(enhanceSpec, req, e.Client); err != nil {
		return err
	}
	return validateAudioSources(enhanceSpec, req)
}

func (e Enhance) Run(ctx context.Context, req core.Request, rep core.Reporter) (core.Result, error) {
	start := time.Now()
	preset := req.Params.String("preset")
	if preset == "" {
		preset = "voice"
	}
	filter, err := enhanceFilter(preset)
	if err != nil {
		return core.Result{}, core.NewError(core.CodeUsage, enhanceSpec.ID, "", err.Error(), "")
	}

	var out []core.Artifact
	var bytesIn, bytesOut int64
	for _, item := range req.Items {
		if err := ctx.Err(); err != nil {
			return core.Result{}, err
		}
		m, err := media.Probe(ctx, e.Client, enhanceSpec.ID, item, rep)
		if err != nil {
			return core.Result{}, err
		}
		if !m.HasAudio {
			return core.Result{}, core.NewError(core.CodeInput, enhanceSpec.ID, item.Path,
				"no audio stream found", "choose an audio file")
		}

		format, encoder, err := resolveAudioSource(enhanceSpec, item)
		if err != nil {
			return core.Result{}, err
		}

		output := media.OutputPath(req, item, format)
		args := []string{"-i", item.Path, "-vn", "-map_metadata", "0", "-filter:a", filter, "-c:a", encoder}
		if m.SampleRate > 0 {
			args = append(args, "-ar", fmt.Sprint(m.SampleRate))
		}
		args = qualityArgs(args, encoder, "192k")
		args = append(args, output)

		rep.Stage("enhancing")
		if err := e.Client.Run(ctx, args, m.Duration, rep); err != nil {
			if ctx.Err() != nil {
				return core.Result{}, ctx.Err()
			}
			return core.Result{}, core.Wrap(core.CodeEngineFailed, enhanceSpec.ID, item.Path, err, core.HintOf(err))
		}
		art, err := media.Finish(req, item, output, core.Audio, format, m, m.Duration,
			map[string]any{"preset": preset})
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
