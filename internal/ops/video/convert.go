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

var convertSpec = core.Spec{
	ID:      "video.convert",
	Title:   "Convert video",
	Summary: "Re-encode video into another container",
	Domain:  core.Video,
	Accepts: []core.Accept{{Kind: core.Video}},
	Params: []core.ParamSpec{
		{Key: "format", Label: "Format", Type: core.ParamEnum, Required: true, Default: "mp4", Options: containerOptions()},
		{Key: "codec", Label: "Video codec", Type: core.ParamEnum, Default: "auto", Options: codecOptions(),
			Help: "Auto picks H.264, or VP9 for WebM."},
		{Key: "crf", Label: "Quality (CRF)", Type: core.ParamInt, Default: 23,
			Min: floatPtr(0), Max: floatPtr(51), Advanced: true,
			Help: "Lower is better quality and larger files."},
		{Key: "preset", Label: "Preset", Type: core.ParamEnum, Default: "medium", Options: presetOptions(), Advanced: true},
	},
	Output:    core.OutputSpec{FormatParam: "format"},
	Engines:   []core.EngineID{engines.FFmpeg},
	Batchable: true,
	Cost:      core.CostSlow,
}

type Convert struct {
	Client *ffmpeg.Client
}

func (c Convert) Spec() core.Spec { return convertSpec }

func (c Convert) Validate(ctx context.Context, req core.Request) error {
	if err := checkVideoInputs(convertSpec, req, c.Client); err != nil {
		return err
	}
	if _, err := media.VideoEncoder(req.Params.String("format"), req.Params.String("codec")); err != nil {
		return core.NewError(core.CodeUsage, convertSpec.ID, "", err.Error(), "")
	}
	return nil
}

func (c Convert) Run(ctx context.Context, req core.Request, rep core.Reporter) (core.Result, error) {
	start := time.Now()
	container := req.Params.String("format")
	codec := req.Params.String("codec")
	crf := strconv.Itoa(req.Params.Int("crf"))
	if req.Params.Int("crf") == 0 {
		// CRF 0 is valid (lossless), so only replace an unset value.
		if _, present := req.Params["crf"]; !present {
			crf = "23"
		}
	}
	preset := req.Params.String("preset")

	codecArgs, err := media.VideoArgs(container, codec, crf, preset)
	if err != nil {
		return core.Result{}, core.NewError(core.CodeUsage, convertSpec.ID, "", err.Error(), "")
	}

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
		if !m.HasVideo {
			return core.Result{}, core.NewError(core.CodeInput, convertSpec.ID, item.Path,
				"no video stream found", "choose a video file")
		}

		output := media.OutputPath(req, item, container)
		args := []string{"-i", item.Path, "-map_metadata", "0"}
		args = append(args, codecArgs...)
		if container == "mp4" || container == "mov" {
			args = append(args, "-movflags", "+faststart")
		}
		args = append(args, output)

		rep.Stage("transcoding")
		if err := c.Client.Run(ctx, args, m.Duration, rep); err != nil {
			if ctx.Err() != nil {
				return core.Result{}, ctx.Err()
			}
			return core.Result{}, core.Wrap(core.CodeEngineFailed, convertSpec.ID, item.Path, err, core.HintOf(err))
		}
		art, err := media.Finish(req, item, output, core.Video, container, m, m.Duration, nil)
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
