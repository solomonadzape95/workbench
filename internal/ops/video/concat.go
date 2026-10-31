package video

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/solomonadzape95/workbench/internal/core"
	"github.com/solomonadzape95/workbench/internal/engines"
	"github.com/solomonadzape95/workbench/internal/engines/ffmpeg"
	"github.com/solomonadzape95/workbench/internal/ops/media"
)

var concatSpec = core.Spec{
	ID:      "video.concat",
	Title:   "Join videos",
	Summary: "Concatenate video clips into one file",
	Domain:  core.Video,
	Accepts: []core.Accept{{Kind: core.Video}},
	Params: []core.ParamSpec{
		{Key: "container", Label: "Container", Type: core.ParamEnum, Default: "mp4", Options: containerOptions()},
		{Key: "codec", Label: "Codec", Type: core.ParamEnum, Default: "auto", Options: codecOptions(), Advanced: true},
		{Key: "crf", Label: "Quality (CRF)", Type: core.ParamInt, Default: 20, Min: floatPtr(0), Max: floatPtr(51)},
		{Key: "preset", Label: "Preset", Type: core.ParamEnum, Default: "medium", Options: presetOptions(), Advanced: true},
	},
	Output:  core.OutputSpec{FormatParam: "container", Kind: core.Video},
	Engines: []core.EngineID{engines.FFmpeg},
	Mode:    core.ModeAggregate,
	Cost:    core.CostSlow,
}

type Concat struct {
	Client *ffmpeg.Client
}

func (c Concat) Spec() core.Spec { return concatSpec }

func (c Concat) Validate(ctx context.Context, req core.Request) error {
	if err := core.Validate(concatSpec.Params, req.Params); err != nil {
		return err
	}
	if len(req.Items) < 2 {
		return core.NewError(core.CodeUsage, concatSpec.ID, "", "join needs at least two videos", "add more inputs")
	}
	if err := media.Check(c.Client, concatSpec.ID); err != nil {
		return err
	}
	for _, item := range req.Items {
		if !concatSpec.AcceptsArtifact(item) {
			return core.NewError(core.CodeInput, concatSpec.ID, item.Path, "not a video file", "choose video files")
		}
	}
	if _, err := media.VideoEncoder(req.Params.String("container"), req.Params.String("codec")); err != nil {
		return core.NewError(core.CodeUsage, concatSpec.ID, "", err.Error(), "")
	}
	return nil
}

func (c Concat) Run(ctx context.Context, req core.Request, rep core.Reporter) (core.Result, error) {
	start := time.Now()
	container := req.Params.String("container")
	codec := req.Params.String("codec")
	crf := req.Params.Int("crf")
	if crf == 0 {
		crf = 20
	}
	preset := req.Params.String("preset")
	if preset == "" {
		preset = "medium"
	}

	args := make([]string, 0, len(req.Items)*2+12)
	var total time.Duration
	for _, item := range req.Items {
		m, err := media.Probe(ctx, c.Client, concatSpec.ID, item, rep)
		if err != nil {
			return core.Result{}, err
		}
		if !m.HasVideo {
			return core.Result{}, core.NewError(core.CodeInput, concatSpec.ID, item.Path, "no video stream", "")
		}
		total += m.Duration
		args = append(args, "-i", item.Path)
	}
	filter := fmt.Sprintf("concat=n=%d:v=1:a=1[outv][outa]", len(req.Items))
	codecArgs, err := media.VideoArgs(container, codec, strconv.Itoa(crf), preset)
	if err != nil {
		return core.Result{}, core.NewError(core.CodeUsage, concatSpec.ID, "", err.Error(), "")
	}
	ext := core.ExtFor(container, req.Items[0])
	name := core.SanitizeStem(req.Items[0].Stem()) + "-joined." + ext
	output := filepath.Join(req.Target.Dir, name)
	args = append(args, "-filter_complex", filter, "-map", "[outv]", "-map", "[outa]")
	args = append(args, codecArgs...)
	if container == "mp4" || container == "mov" {
		args = append(args, "-movflags", "+faststart")
	}
	args = append(args, output)

	rep.Stage("joining video")
	if err := c.Client.Run(ctx, args, total, rep); err != nil {
		if ctx.Err() != nil {
			return core.Result{}, ctx.Err()
		}
		return core.Result{}, core.Wrap(core.CodeEngineFailed, concatSpec.ID, "", err, core.HintOf(err))
	}
	info, err := os.Stat(output)
	if err != nil {
		return core.Result{}, core.Wrap(core.CodeOutput, concatSpec.ID, output, err, "")
	}
	var bytesIn int64
	for _, item := range req.Items {
		bytesIn += item.Size
	}
	artifact := core.Artifact{
		Path: output, Kind: core.Video, Format: container, Size: info.Size(),
		SuggestedName: name,
		Meta: map[string]any{
			"source":      fmt.Sprintf("%d clips", len(req.Items)),
			"duration_ms": total.Milliseconds(),
		},
	}
	return core.Result{
		Artifacts: []core.Artifact{artifact},
		Metrics: core.Metrics{
			BytesIn: bytesIn, BytesOut: info.Size(), Duration: time.Since(start), Engine: engines.FFmpeg,
		},
	}, nil
}
