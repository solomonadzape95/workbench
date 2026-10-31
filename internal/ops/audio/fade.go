package audio

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

var fadeSpec = core.Spec{
	ID:      "audio.fade",
	Title:   "Fade audio",
	Summary: "Add a fade-in and fade-out to an audio file",
	Domain:  core.Audio,
	Accepts: []core.Accept{{Kind: core.Audio}},
	Params: []core.ParamSpec{
		{Key: "fade_in", Label: "Fade in", Type: core.ParamDuration, Default: "1s",
			Help: "Length of the fade at the start; 0s skips it."},
		{Key: "fade_out", Label: "Fade out", Type: core.ParamDuration, Default: "1s",
			Help: "Length of the fade at the end; 0s skips it."},
	},
	Engines:   []core.EngineID{engines.FFmpeg},
	Batchable: true,
	Cost:      core.CostFast,
}

type Fade struct {
	Client *ffmpeg.Client
}

func (f Fade) Spec() core.Spec { return fadeSpec }

// fadeChain builds afade filters, clamping each fade to the clip length when it
// is known. A non-positive fade duration is skipped entirely.
func fadeChain(total, fadeIn, fadeOut time.Duration) string {
	var parts []string
	if fadeIn > 0 {
		d := fadeIn
		if total > 0 && d > total {
			d = total
		}
		parts = append(parts, fmt.Sprintf("afade=t=in:st=0:d=%s", audioSeconds(d)))
	}
	if fadeOut > 0 {
		d := fadeOut
		if total > 0 && d > total {
			d = total
		}
		st := time.Duration(0)
		if total > 0 && total > d {
			st = total - d
		}
		parts = append(parts, fmt.Sprintf("afade=t=out:st=%s:d=%s", audioSeconds(st), audioSeconds(d)))
	}
	return strings.Join(parts, ",")
}

func (f Fade) Validate(ctx context.Context, req core.Request) error {
	if err := checkAudioInputs(fadeSpec, req, f.Client); err != nil {
		return err
	}
	return validateAudioSources(fadeSpec, req)
}

func (f Fade) Run(ctx context.Context, req core.Request, rep core.Reporter) (core.Result, error) {
	start := time.Now()
	fadeIn := req.Params.Duration("fade_in")
	fadeOut := req.Params.Duration("fade_out")

	var out []core.Artifact
	var bytesIn, bytesOut int64
	for _, item := range req.Items {
		if err := ctx.Err(); err != nil {
			return core.Result{}, err
		}
		m, err := media.Probe(ctx, f.Client, fadeSpec.ID, item, rep)
		if err != nil {
			return core.Result{}, err
		}
		if !m.HasAudio {
			return core.Result{}, core.NewError(core.CodeInput, fadeSpec.ID, item.Path,
				"no audio stream found", "choose an audio file")
		}

		format, encoder, err := resolveAudioSource(fadeSpec, item)
		if err != nil {
			return core.Result{}, err
		}

		filter := fadeChain(m.Duration, fadeIn, fadeOut)
		output := media.OutputPath(req, item, format)
		args := []string{"-i", item.Path, "-vn", "-map_metadata", "0"}
		if filter != "" {
			args = append(args, "-filter:a", filter)
		}
		args = append(args, "-c:a", encoder)
		args = qualityArgs(args, encoder, "192k")
		args = append(args, output)

		rep.Stage("fading")
		if err := f.Client.Run(ctx, args, m.Duration, rep); err != nil {
			if ctx.Err() != nil {
				return core.Result{}, ctx.Err()
			}
			return core.Result{}, core.Wrap(core.CodeEngineFailed, fadeSpec.ID, item.Path, err, core.HintOf(err))
		}
		art, err := media.Finish(req, item, output, core.Audio, format, m, m.Duration,
			map[string]any{
				"fade_in_ms":  fadeIn.Milliseconds(),
				"fade_out_ms": fadeOut.Milliseconds(),
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
