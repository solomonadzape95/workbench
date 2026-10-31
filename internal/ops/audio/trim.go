package audio

import (
	"context"
	"time"

	"github.com/solomonadzape95/workbench/internal/core"
	"github.com/solomonadzape95/workbench/internal/engines"
	"github.com/solomonadzape95/workbench/internal/engines/ffmpeg"
	"github.com/solomonadzape95/workbench/internal/ops/media"
)

var trimSpec = core.Spec{
	ID:      "audio.trim",
	Title:   "Trim audio",
	Summary: "Cut a section out of an audio file",
	Domain:  core.Audio,
	Accepts: []core.Accept{{Kind: core.Audio}},
	Params: []core.ParamSpec{
		{Key: "start", Label: "Start", Type: core.ParamDuration, Default: "0s",
			Help: "Where the clip begins, for example 5s or 1m30s."},
		{Key: "end", Label: "End", Type: core.ParamDuration, Default: "0s",
			Help: "Where the clip stops; 0s means the end of the file."},
	},
	Engines:   []core.EngineID{engines.FFmpeg},
	Batchable: true,
	Cost:      core.CostFast,
}

type Trim struct {
	Client *ffmpeg.Client
}

func (t Trim) Spec() core.Spec { return trimSpec }

func validateTrimWindow(start, end time.Duration) error {
	if start < 0 || end < 0 {
		return core.NewError(core.CodeUsage, trimSpec.ID, "", "start and end cannot be negative", "")
	}
	if start > 0 && end > 0 && end <= start {
		return core.NewError(core.CodeUsage, trimSpec.ID, "",
			"end must be after start", "set end to 0s to keep the rest of the file")
	}
	return nil
}

func (t Trim) Validate(ctx context.Context, req core.Request) error {
	if err := validateTrimWindow(req.Params.Duration("start"), req.Params.Duration("end")); err != nil {
		return err
	}
	if err := checkAudioInputs(trimSpec, req, t.Client); err != nil {
		return err
	}
	return validateAudioSources(trimSpec, req)
}

// trimWindow computes the output length once the source duration is known.
// end == 0 means "through the end of the file".
func trimWindow(start, end, total time.Duration) (time.Duration, error) {
	if total > 0 && start >= total {
		return 0, core.NewError(core.CodeUsage, trimSpec.ID, "",
			"start is at or past the end of the file", "choose an earlier start")
	}
	stop := total
	if end > 0 {
		stop = end
		if total > 0 && stop > total {
			stop = total
		}
	}
	if stop <= start {
		return 0, core.NewError(core.CodeUsage, trimSpec.ID, "",
			"trim window is empty", "choose an end after the start")
	}
	return stop - start, nil
}

func (t Trim) Run(ctx context.Context, req core.Request, rep core.Reporter) (core.Result, error) {
	start := time.Now()
	from := req.Params.Duration("start")
	to := req.Params.Duration("end")
	if err := validateTrimWindow(from, to); err != nil {
		return core.Result{}, err
	}

	var out []core.Artifact
	var bytesIn, bytesOut int64
	for _, item := range req.Items {
		if err := ctx.Err(); err != nil {
			return core.Result{}, err
		}
		m, err := media.Probe(ctx, t.Client, trimSpec.ID, item, rep)
		if err != nil {
			return core.Result{}, err
		}
		if !m.HasAudio {
			return core.Result{}, core.NewError(core.CodeInput, trimSpec.ID, item.Path,
				"no audio stream found", "choose an audio file")
		}

		format, encoder, err := resolveAudioSource(trimSpec, item)
		if err != nil {
			return core.Result{}, err
		}

		length := time.Duration(0)
		if m.Duration > 0 || to > 0 {
			length, err = trimWindow(from, to, m.Duration)
			if err != nil {
				return core.Result{}, err
			}
		}

		output := media.OutputPath(req, item, format)
		args := []string{"-i", item.Path}
		if from > 0 {
			// -ss after -i decodes from the start, so the cut lands on the
			// requested frame instead of the nearest keyframe.
			args = append(args, "-ss", audioSeconds(from))
		}
		if length > 0 {
			args = append(args, "-t", audioSeconds(length))
		}
		args = append(args, "-vn", "-map_metadata", "0", "-c:a", encoder)
		args = qualityArgs(args, encoder, "192k")
		args = append(args, output)

		rep.Stage("trimming")
		if err := t.Client.Run(ctx, args, length, rep); err != nil {
			if ctx.Err() != nil {
				return core.Result{}, ctx.Err()
			}
			return core.Result{}, core.Wrap(core.CodeEngineFailed, trimSpec.ID, item.Path, err, core.HintOf(err))
		}
		art, err := media.Finish(req, item, output, core.Audio, format, m, length,
			map[string]any{
				"start_ms": from.Milliseconds(),
				"end_ms":   to.Milliseconds(),
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
