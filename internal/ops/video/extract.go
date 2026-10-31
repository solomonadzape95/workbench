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

var extractSpec = core.Spec{
	ID:      "video.extract_audio",
	Title:   "Extract audio",
	Summary: "Pull the audio track out of a video",
	Domain:  core.Video,
	Accepts: []core.Accept{{Kind: core.Video}},
	Params: []core.ParamSpec{
		{Key: "format", Label: "Format", Type: core.ParamEnum, Required: true, Default: "mp3", Options: audioFormatOptions()},
		{Key: "bitrate", Label: "Bitrate", Type: core.ParamEnum, Default: "192k", Options: audioBitrateOptions(),
			Help: "Ignored for WAV and FLAC."},
	},
	Output:    core.OutputSpec{FormatParam: "format", Kind: core.Audio},
	Engines:   []core.EngineID{engines.FFmpeg},
	Batchable: true,
	Cost:      core.CostMedium,
}

type Extract struct {
	Client *ffmpeg.Client
}

func (e Extract) Spec() core.Spec { return extractSpec }

func (e Extract) Validate(ctx context.Context, req core.Request) error {
	if err := checkVideoInputs(extractSpec, req, e.Client); err != nil {
		return err
	}
	if _, _, ok := media.AudioEncoder(req.Params.String("format")); !ok {
		return core.NewError(core.CodeUsage, extractSpec.ID, "",
			fmt.Sprintf("unsupported audio format %q", req.Params.String("format")), "")
	}
	return nil
}

func (e Extract) Run(ctx context.Context, req core.Request, rep core.Reporter) (core.Result, error) {
	start := time.Now()
	format := req.Params.String("format")
	encoder, lossy, ok := media.AudioEncoder(format)
	if !ok {
		return core.Result{}, core.NewError(core.CodeUsage, extractSpec.ID, "", "unsupported audio format", "")
	}
	bitrate := req.Params.String("bitrate")

	var out []core.Artifact
	var bytesIn, bytesOut int64
	for _, item := range req.Items {
		if err := ctx.Err(); err != nil {
			return core.Result{}, err
		}
		m, err := media.Probe(ctx, e.Client, extractSpec.ID, item, rep)
		if err != nil {
			return core.Result{}, err
		}
		if !m.HasAudio {
			return core.Result{}, core.NewError(core.CodeInput, extractSpec.ID, item.Path,
				"no audio stream found", "choose a video that has audio")
		}

		output := media.OutputPath(req, item, format)
		args := []string{"-i", item.Path, "-vn", "-map_metadata", "0", "-c:a", encoder}
		if lossy {
			args = append(args, "-b:a", bitrate)
		}
		args = append(args, output)

		rep.Stage("extracting audio")
		if err := e.Client.Run(ctx, args, m.Duration, rep); err != nil {
			if ctx.Err() != nil {
				return core.Result{}, ctx.Err()
			}
			return core.Result{}, core.Wrap(core.CodeEngineFailed, extractSpec.ID, item.Path, err, core.HintOf(err))
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
