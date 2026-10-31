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

var convertSpec = core.Spec{
	ID:      "audio.convert",
	Title:   "Convert audio",
	Summary: "Re-encode audio into another format",
	Domain:  core.Audio,
	Accepts: []core.Accept{{Kind: core.Audio}},
	Params: []core.ParamSpec{
		{Key: "format", Label: "Format", Type: core.ParamEnum, Required: true, Default: "mp3", Options: formatOptions()},
		{Key: "bitrate", Label: "Bitrate", Type: core.ParamEnum, Default: "192k", Options: bitrateOptions(),
			Help: "Ignored for WAV and FLAC."},
		{Key: "sample_rate", Label: "Sample rate", Type: core.ParamEnum, Default: "auto", Options: sampleRateOptions(), Advanced: true},
		{Key: "channels", Label: "Channels", Type: core.ParamEnum, Default: "auto", Options: channelOptions(), Advanced: true},
	},
	Output:    core.OutputSpec{FormatParam: "format"},
	Engines:   []core.EngineID{engines.FFmpeg},
	Batchable: true,
	Cost:      core.CostMedium,
}

type Convert struct {
	Client *ffmpeg.Client
}

func (c Convert) Spec() core.Spec { return convertSpec }

func (c Convert) Validate(ctx context.Context, req core.Request) error {
	if err := checkAudioInputs(convertSpec, req, c.Client); err != nil {
		return err
	}
	if _, _, ok := media.AudioEncoder(req.Params.String("format")); !ok {
		return core.NewError(core.CodeUsage, convertSpec.ID, "",
			fmt.Sprintf("unsupported audio format %q", req.Params.String("format")), "")
	}
	return nil
}

func (c Convert) Run(ctx context.Context, req core.Request, rep core.Reporter) (core.Result, error) {
	start := time.Now()
	format := req.Params.String("format")
	encoder, _, ok := media.AudioEncoder(format)
	if !ok {
		return core.Result{}, core.NewError(core.CodeUsage, convertSpec.ID, "", "unsupported audio format", "")
	}
	bitrate := req.Params.String("bitrate")
	sampleRate := req.Params.String("sample_rate")
	channels := req.Params.String("channels")

	var out []core.Artifact
	var bytesIn, bytesOut int64
	for _, item := range req.Items {
		if err := ctx.Err(); err != nil {
			return core.Result{}, err
		}
		m, err := media.Probe(ctx, c.Client, convertSpec.ID, item, rep)
		if err != nil {
			return core.Result{}, err
		}
		if !m.HasAudio {
			return core.Result{}, core.NewError(core.CodeInput, convertSpec.ID, item.Path,
				"no audio stream found", "choose an audio file")
		}

		output := media.OutputPath(req, item, format)
		args := []string{"-i", item.Path, "-vn", "-map_metadata", "0", "-c:a", encoder}
		args = qualityArgs(args, encoder, bitrate)
		args = applyChannels(args, sampleRate, channels)
		args = append(args, output)

		rep.Stage("transcoding")
		if err := c.Client.Run(ctx, args, m.Duration, rep); err != nil {
			if ctx.Err() != nil {
				return core.Result{}, ctx.Err()
			}
			return core.Result{}, core.Wrap(core.CodeEngineFailed, convertSpec.ID, item.Path, err, core.HintOf(err))
		}
		art, err := media.Finish(req, item, output, core.Audio, format, m, m.Duration, nil)
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
