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

var compressSpec = core.Spec{
	ID:      "video.compress",
	Title:   "Compress video",
	Summary: "Shrink a video by raising CRF and optionally capping resolution",
	Domain:  core.Video,
	Accepts: []core.Accept{{Kind: core.Video}},
	Params: []core.ParamSpec{
		{Key: "crf", Label: "Quality (CRF)", Type: core.ParamInt, Default: 28,
			Min: floatPtr(0), Max: floatPtr(51),
			Help: "Higher is smaller and softer. 23–30 is the useful range."},
		{Key: "preset", Label: "Preset", Type: core.ParamEnum, Default: "medium", Options: presetOptions(), Advanced: true},
		{Key: "resolution", Label: "Resolution", Type: core.ParamEnum, Default: "source", Options: resolutionOptions()},
		{Key: "max_bitrate", Label: "Bitrate cap", Type: core.ParamEnum, Default: "none", Options: maxBitrateOptions(), Advanced: true},
	},
	Engines:   []core.EngineID{engines.FFmpeg},
	Batchable: true,
	Cost:      core.CostSlow,
}

type Compress struct {
	Client *ffmpeg.Client
}

func (c Compress) Spec() core.Spec { return compressSpec }

func (c Compress) Validate(ctx context.Context, req core.Request) error {
	if err := checkVideoInputs(compressSpec, req, c.Client); err != nil {
		return err
	}
	for _, item := range req.Items {
		container, err := videoContainer(item)
		if err != nil {
			return core.NewError(core.CodeInput, compressSpec.ID, item.Path, err.Error(), "")
		}
		if _, err := media.VideoEncoder(container, "auto"); err != nil {
			return core.NewError(core.CodeInput, compressSpec.ID, item.Path, err.Error(), "")
		}
	}
	return nil
}

func (c Compress) Run(ctx context.Context, req core.Request, rep core.Reporter) (core.Result, error) {
	start := time.Now()
	crf := req.Params.Int("crf")
	if crf == 0 {
		if _, present := req.Params["crf"]; !present {
			crf = 28
		}
	}
	preset := req.Params.String("preset")
	resolution := req.Params.String("resolution")
	bitrateCap := req.Params.String("max_bitrate")

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
		if !m.HasVideo {
			return core.Result{}, core.NewError(core.CodeInput, compressSpec.ID, item.Path,
				"no video stream found", "choose a video file")
		}

		container, err := videoContainer(item)
		if err != nil {
			return core.Result{}, core.NewError(core.CodeInput, compressSpec.ID, item.Path, err.Error(), "")
		}
		codecArgs, err := media.VideoArgs(container, "auto", strconv.Itoa(crf), preset)
		if err != nil {
			return core.Result{}, core.NewError(core.CodeInput, compressSpec.ID, item.Path, err.Error(), "")
		}

		output := media.OutputPath(req, item, container)
		args := []string{"-i", item.Path, "-map_metadata", "0"}
		if filter := resolutionFilter(resolution, m.Height); filter != "" {
			args = append(args, "-vf", filter)
		}
		args = append(args, codecArgs...)
		args = append(args, maxRateArgs(bitrateCap)...)
		if container == "mp4" || container == "mov" {
			args = append(args, "-movflags", "+faststart")
		}
		args = append(args, output)

		duration := m.Duration
		rep.Stage("transcoding")
		if err := c.Client.Run(ctx, args, duration, rep); err != nil {
			if ctx.Err() != nil {
				return core.Result{}, ctx.Err()
			}
			return core.Result{}, core.Wrap(core.CodeEngineFailed, compressSpec.ID, item.Path, err, core.HintOf(err))
		}
		art, err := media.Finish(req, item, output, core.Video, container, m, duration,
			map[string]any{"crf": crf, "resolution": resolution})
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
