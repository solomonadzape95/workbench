package audio

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/solomonadzape95/workbench/internal/core"
	"github.com/solomonadzape95/workbench/internal/engines"
	"github.com/solomonadzape95/workbench/internal/engines/ffmpeg"
	"github.com/solomonadzape95/workbench/internal/ops/media"
)

var silenceTrimSpec = core.Spec{
	ID:      "audio.silence_trim",
	Title:   "Trim silence",
	Summary: "Remove leading and trailing silence, including long gaps",
	Domain:  core.Audio,
	Accepts: []core.Accept{{Kind: core.Audio}},
	Params: []core.ParamSpec{
		{Key: "threshold", Label: "Silence threshold", Type: core.ParamString, Default: "-50dB",
			Help: "Audio quieter than this counts as silence, for example -50dB."},
		{Key: "min_silence", Label: "Minimum silence", Type: core.ParamDuration, Default: "0.5s",
			Help: "Silence shorter than this is kept."},
	},
	Engines:   []core.EngineID{engines.FFmpeg},
	Batchable: true,
	Cost:      core.CostMedium,
}

type SilenceTrim struct {
	Client *ffmpeg.Client
}

func (s SilenceTrim) Spec() core.Spec { return silenceTrimSpec }

var silenceThresholdRE = regexp.MustCompile(`(?i)^[+-]?(?:\d+(?:\.\d*)?|\.\d+)(?:dBFS|dB)?$`)

func validSilenceThreshold(value string) bool {
	value = strings.TrimSpace(value)
	return value != "" && silenceThresholdRE.MatchString(value)
}

func silenceFilter(threshold string, minSilence time.Duration) string {
	period := audioSeconds(minSilence)
	return fmt.Sprintf(
		"silenceremove=start_periods=1:start_duration=%s:start_threshold=%s:stop_periods=-1:stop_duration=%s:stop_threshold=%s",
		period, strings.TrimSpace(threshold), period, strings.TrimSpace(threshold))
}

func (s SilenceTrim) Validate(ctx context.Context, req core.Request) error {
	threshold := req.Params.String("threshold")
	if !validSilenceThreshold(threshold) {
		return core.NewError(core.CodeUsage, silenceTrimSpec.ID, "",
			fmt.Sprintf("silence threshold %q is not a level", threshold),
			"use a value such as -50dB")
	}
	if req.Params.Duration("min_silence") <= 0 {
		return core.NewError(core.CodeUsage, silenceTrimSpec.ID, "",
			"minimum silence must be greater than zero", "")
	}
	if err := checkAudioInputs(silenceTrimSpec, req, s.Client); err != nil {
		return err
	}
	return validateAudioSources(silenceTrimSpec, req)
}

func (s SilenceTrim) Run(ctx context.Context, req core.Request, rep core.Reporter) (core.Result, error) {
	start := time.Now()
	threshold := req.Params.String("threshold")
	if threshold == "" {
		threshold = "-50dB"
	}
	minSilence := req.Params.Duration("min_silence")
	if _, ok := req.Params["min_silence"]; !ok {
		minSilence = 500 * time.Millisecond
	}
	if !validSilenceThreshold(threshold) || minSilence <= 0 {
		return core.Result{}, core.NewError(core.CodeUsage, silenceTrimSpec.ID, "",
			"invalid silence settings", "use a threshold such as -50dB and a positive minimum silence")
	}
	filter := silenceFilter(threshold, minSilence)

	var out []core.Artifact
	var bytesIn, bytesOut int64
	for _, item := range req.Items {
		if err := ctx.Err(); err != nil {
			return core.Result{}, err
		}
		m, err := media.Probe(ctx, s.Client, silenceTrimSpec.ID, item, rep)
		if err != nil {
			return core.Result{}, err
		}
		if !m.HasAudio {
			return core.Result{}, core.NewError(core.CodeInput, silenceTrimSpec.ID, item.Path,
				"no audio stream found", "choose an audio file")
		}

		format, encoder, err := resolveAudioSource(silenceTrimSpec, item)
		if err != nil {
			return core.Result{}, err
		}

		output := media.OutputPath(req, item, format)
		args := []string{"-i", item.Path, "-vn", "-map_metadata", "0", "-filter:a", filter, "-c:a", encoder}
		args = qualityArgs(args, encoder, "192k")
		args = append(args, output)

		rep.Stage("removing silence")
		if err := s.Client.Run(ctx, args, m.Duration, rep); err != nil {
			if ctx.Err() != nil {
				return core.Result{}, ctx.Err()
			}
			return core.Result{}, core.Wrap(core.CodeEngineFailed, silenceTrimSpec.ID, item.Path, err, core.HintOf(err))
		}
		art, err := media.Finish(req, item, output, core.Audio, format, m, m.Duration,
			map[string]any{
				"threshold":      strings.TrimSpace(threshold),
				"min_silence_ms": minSilence.Milliseconds(),
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
