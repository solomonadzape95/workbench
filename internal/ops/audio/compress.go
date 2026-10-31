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

var compressSpec = core.Spec{
	ID:      "audio.compress",
	Title:   "Compress audio",
	Summary: "Re-encode audio at a lower bitrate",
	Domain:  core.Audio,
	Accepts: []core.Accept{{Kind: core.Audio}},
	Params: []core.ParamSpec{
		{Key: "bitrate", Label: "Bitrate", Type: core.ParamEnum, Default: "128k", Options: bitrateOptions(),
			Help: "Keeps the container; use Convert to change format. WAV and FLAC ignore bitrate."},
	},
	Engines:   []core.EngineID{engines.FFmpeg},
	Batchable: true,
	Cost:      core.CostMedium,
}

type Compress struct {
	Client *ffmpeg.Client
}

func (c Compress) Spec() core.Spec { return compressSpec }

func (c Compress) Validate(ctx context.Context, req core.Request) error {
	if err := checkAudioInputs(compressSpec, req, c.Client); err != nil {
		return err
	}
	for _, item := range req.Items {
		format, err := audioFormat(item)
		if err != nil {
			return core.NewError(core.CodeInput, compressSpec.ID, item.Path, err.Error(), "")
		}
		if _, _, ok := media.AudioEncoder(format); !ok {
			return core.NewError(core.CodeInput, compressSpec.ID, item.Path,
				fmt.Sprintf("unsupported audio format %q", format), "")
		}
	}
	return nil
}

func (c Compress) Run(ctx context.Context, req core.Request, rep core.Reporter) (core.Result, error) {
	start := time.Now()
	bitrate := req.Params.String("bitrate")

	var out []core.Artifact
	var bytesIn, bytesOut int64
	for _, item := range req.Items {
		if err := ctx.Err(); err != nil {
			return core.Result{}, err
		}
		m, err := media.Probe(ctx, c.Client, compressSpec.ID, item, rep)
		if err != nil {
			return core.Result{}, err
		}
		if !m.HasAudio {
			return core.Result{}, core.NewError(core.CodeInput, compressSpec.ID, item.Path,
				"no audio stream found", "choose an audio file")
		}

		format, err := audioFormat(item)
		if err != nil {
			return core.Result{}, core.NewError(core.CodeInput, compressSpec.ID, item.Path, err.Error(), "")
		}
		encoder, _, ok := media.AudioEncoder(format)
		if !ok {
			return core.Result{}, core.NewError(core.CodeInput, compressSpec.ID, item.Path,
				fmt.Sprintf("unsupported audio format %q", format), "")
		}

		output := media.OutputPath(req, item, format)
		args := []string{"-i", item.Path, "-vn", "-map_metadata", "0", "-c:a", encoder}
		args = qualityArgs(args, encoder, bitrate)
		args = append(args, output)

		rep.Stage("transcoding")
		if err := c.Client.Run(ctx, args, m.Duration, rep); err != nil {
			if ctx.Err() != nil {
				return core.Result{}, ctx.Err()
			}
			return core.Result{}, core.Wrap(core.CodeEngineFailed, compressSpec.ID, item.Path, err, core.HintOf(err))
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
