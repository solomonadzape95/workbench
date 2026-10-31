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

var normalizeSpec = core.Spec{
	ID:      "audio.normalize",
	Title:   "Normalize loudness",
	Summary: "Even out volume with EBU R128 loudness normalization",
	Domain:  core.Audio,
	Accepts: []core.Accept{{Kind: core.Audio}},
	Params: []core.ParamSpec{
		{Key: "preset", Label: "Preset", Type: core.ParamEnum, Default: "podcast",
			Options: []core.Option{
				{Value: "podcast", Label: "Podcast (-16 LUFS)"},
				{Value: "music", Label: "Music (-14 LUFS)"},
				{Value: "custom", Label: "Custom targets"},
			},
			Help: "Keeps the source container and codec family."},
		{Key: "lufs", Label: "Integrated loudness", Type: core.ParamFloat, Default: -16.0,
			Min: floatPtr(-70), Max: floatPtr(-5),
			Help: "Target loudness in LUFS; used by the custom preset."},
		{Key: "true_peak", Label: "True peak", Type: core.ParamFloat, Default: -1.5,
			Min: floatPtr(-9), Max: floatPtr(0),
			Help: "Maximum true peak in dBTP; used by the custom preset."},
		{Key: "lra", Label: "Loudness range", Type: core.ParamFloat, Default: 11.0,
			Min: floatPtr(1), Max: floatPtr(50), Advanced: true,
			Help: "Target loudness range in LU; used by the custom preset."},
	},
	Engines:   []core.EngineID{engines.FFmpeg},
	Batchable: true,
	Cost:      core.CostMedium,
}

type Normalize struct {
	Client *ffmpeg.Client
}

func (n Normalize) Spec() core.Spec { return normalizeSpec }

// loudnessTargets resolves the preset into concrete loudnorm arguments.
func loudnessTargets(preset string, lufs, truePeak, lra float64) (float64, float64, float64) {
	switch preset {
	case "podcast":
		return -16, -1.5, 11
	case "music":
		return -14, -1.5, 11
	default:
		return lufs, truePeak, lra
	}
}

func loudnormFilter(lufs, truePeak, lra float64) string {
	return fmt.Sprintf("loudnorm=I=%s:TP=%s:LRA=%s",
		audioDecimal(lufs), audioDecimal(truePeak), audioDecimal(lra))
}

func (n Normalize) Validate(ctx context.Context, req core.Request) error {
	if err := checkAudioInputs(normalizeSpec, req, n.Client); err != nil {
		return err
	}
	return validateAudioSources(normalizeSpec, req)
}

func (n Normalize) Run(ctx context.Context, req core.Request, rep core.Reporter) (core.Result, error) {
	start := time.Now()
	preset := req.Params.String("preset")
	if preset == "" {
		preset = "podcast"
	}
	lufs := req.Params.Float("lufs")
	if _, ok := req.Params["lufs"]; !ok {
		lufs = -16
	}
	truePeak := req.Params.Float("true_peak")
	if _, ok := req.Params["true_peak"]; !ok {
		truePeak = -1.5
	}
	lra := req.Params.Float("lra")
	if _, ok := req.Params["lra"]; !ok {
		lra = 11
	}
	lufs, truePeak, lra = loudnessTargets(preset, lufs, truePeak, lra)
	filter := loudnormFilter(lufs, truePeak, lra)

	var out []core.Artifact
	var bytesIn, bytesOut int64
	for _, item := range req.Items {
		if err := ctx.Err(); err != nil {
			return core.Result{}, err
		}
		m, err := media.Probe(ctx, n.Client, normalizeSpec.ID, item, rep)
		if err != nil {
			return core.Result{}, err
		}
		if !m.HasAudio {
			return core.Result{}, core.NewError(core.CodeInput, normalizeSpec.ID, item.Path,
				"no audio stream found", "choose an audio file")
		}

		format, encoder, err := resolveAudioSource(normalizeSpec, item)
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

		rep.Stage("normalizing")
		if err := n.Client.Run(ctx, args, m.Duration, rep); err != nil {
			if ctx.Err() != nil {
				return core.Result{}, ctx.Err()
			}
			return core.Result{}, core.Wrap(core.CodeEngineFailed, normalizeSpec.ID, item.Path, err, core.HintOf(err))
		}
		art, err := media.Finish(req, item, output, core.Audio, format, m, m.Duration,
			map[string]any{
				"preset":    preset,
				"lufs":      lufs,
				"true_peak": truePeak,
				"lra":       lra,
			})
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
