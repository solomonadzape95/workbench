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

var trimSpec = core.Spec{
	ID:      "video.trim",
	Title:   "Trim video",
	Summary: "Keep one section of a video, either stream-copied or re-encoded",
	Domain:  core.Video,
	Accepts: []core.Accept{{Kind: core.Video}},
	Params: []core.ParamSpec{
		{Key: "start", Label: "Start", Type: core.ParamDuration, Default: "0s",
			Help: "Where the kept section begins."},
		{Key: "end", Label: "End", Type: core.ParamDuration, Default: "0s",
			Help: "Where it stops. Leave at 0 to keep the rest of the clip."},
		{Key: "fast", Label: "Fast (stream copy)", Type: core.ParamBool, Default: false, Advanced: true,
			Help: "Copies streams without re-encoding. Cuts land on keyframes."},
	},
	Engines:   []core.EngineID{engines.FFmpeg},
	Batchable: true,
	Cost:      core.CostMedium,
}

type Trim struct {
	Client *ffmpeg.Client
}

func (t Trim) Spec() core.Spec { return trimSpec }

func (t Trim) Validate(ctx context.Context, req core.Request) error {
	if err := checkVideoInputs(trimSpec, req, t.Client); err != nil {
		return err
	}
	start := req.Params.Duration("start")
	end := req.Params.Duration("end")
	if end > 0 && end <= start {
		return core.NewError(core.CodeUsage, trimSpec.ID, "",
			"end must be after start", "leave end at 0 to keep the rest of the clip")
	}
	return nil
}

func (t Trim) Run(ctx context.Context, req core.Request, rep core.Reporter) (core.Result, error) {
	started := time.Now()
	startAt := req.Params.Duration("start")
	endAt := req.Params.Duration("end")
	fast := req.Params.Bool("fast")

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
		if !m.HasVideo {
			return core.Result{}, core.NewError(core.CodeInput, trimSpec.ID, item.Path,
				"no video stream found", "choose a video file")
		}
		if m.Duration > 0 && startAt >= m.Duration {
			return core.Result{}, core.NewError(core.CodeInput, trimSpec.ID, item.Path,
				"start is at or past the end of the video", "choose a start before the clip ends")
		}

		container, err := videoContainer(item)
		if err != nil {
			return core.Result{}, core.NewError(core.CodeInput, trimSpec.ID, item.Path, err.Error(), "")
		}
		cutStart, cutDuration := trimWindow(m.Duration, startAt, endAt)

		output := media.OutputPath(req, item, container)
		args := make([]string, 0, 16)
		if cutStart > 0 {
			args = append(args, "-ss", formatSeconds(cutStart))
		}
		args = append(args, "-i", item.Path)
		if cutDuration > 0 {
			args = append(args, "-t", formatSeconds(cutDuration))
		}
		if fast {
			args = append(args, "-c", "copy")
		} else {
			codecArgs, err := media.VideoArgs(container, "auto", "20", "medium")
			if err != nil {
				return core.Result{}, core.NewError(core.CodeInput, trimSpec.ID, item.Path, err.Error(), "")
			}
			args = append(args, "-map_metadata", "0")
			args = append(args, codecArgs...)
			if container == "mp4" || container == "mov" {
				args = append(args, "-movflags", "+faststart")
			}
		}
		args = append(args, output)

		rep.Stage("trimming")
		if err := t.Client.Run(ctx, args, cutDuration, rep); err != nil {
			if ctx.Err() != nil {
				return core.Result{}, ctx.Err()
			}
			return core.Result{}, core.Wrap(core.CodeEngineFailed, trimSpec.ID, item.Path, err, core.HintOf(err))
		}
		art, err := media.Finish(req, item, output, core.Video, container, m, cutDuration,
			map[string]any{
				"start_ms": cutStart.Milliseconds(),
				"end_ms":   (cutStart + cutDuration).Milliseconds(),
				"fast":     fast,
			})
		if err != nil {
			return core.Result{}, err
		}
		art.SuggestedName = outputBase(output)
		out = append(out, art)
		bytesIn += item.Size
		bytesOut += art.Size
	}

	return core.Result{
		Artifacts: out,
		Metrics: core.Metrics{
			BytesIn:  bytesIn,
			BytesOut: bytesOut,
			Duration: time.Since(started),
			Engine:   engines.FFmpeg,
		},
	}, nil
}

// trimWindow resolves a start/end pair into a seek offset and a length. An end
// of zero means "to the end of the source".
func trimWindow(source, start, end time.Duration) (time.Duration, time.Duration) {
	if start < 0 {
		start = 0
	}
	if source > 0 && start > source {
		start = source
	}
	if end == 0 {
		if source > start {
			return start, source - start
		}
		return start, 0
	}
	if source > 0 && end > source {
		end = source
	}
	if end <= start {
		return start, 0
	}
	return start, end - start
}

// formatSeconds renders a duration in the decimal-seconds form ffmpeg accepts.
func formatSeconds(d time.Duration) string {
	if d <= 0 {
		return "0"
	}
	return strconv.FormatFloat(d.Seconds(), 'f', 3, 64)
}
