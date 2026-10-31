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

var burnSubsSpec = core.Spec{
	ID:      "video.burn_subs",
	Title:   "Burn in subtitles",
	Summary: "Render a subtitle file permanently onto every frame",
	Domain:  core.Video,
	Accepts: []core.Accept{{Kind: core.Video}},
	Params: []core.ParamSpec{
		{Key: "subtitle", Label: "Subtitle file", Type: core.ParamPath, Required: true,
			Help: "SRT, ASS, or VTT file to burn in."},
		{Key: "font_size", Label: "Font size", Type: core.ParamInt, Default: 24,
			Min: floatPtr(1)},
	},
	Engines:   []core.EngineID{engines.FFmpeg},
	Batchable: true,
	Cost:      core.CostSlow,
}

type BurnSubs struct {
	Client *ffmpeg.Client
}

func (b BurnSubs) Spec() core.Spec { return burnSubsSpec }

func (b BurnSubs) Validate(ctx context.Context, req core.Request) error {
	if err := checkVideoInputs(burnSubsSpec, req, b.Client); err != nil {
		return err
	}
	subtitle := req.Params.String("subtitle")
	if err := requireExistingFile(subtitle, "subtitle file"); err != nil {
		return core.NewError(core.CodeInput, burnSubsSpec.ID, subtitle, err.Error(),
			"choose an SRT, ASS, or VTT file")
	}
	return nil
}

func (b BurnSubs) Run(ctx context.Context, req core.Request, rep core.Reporter) (core.Result, error) {
	started := time.Now()
	subtitle := req.Params.String("subtitle")
	fontSize := req.Params.Int("font_size")
	if fontSize <= 0 {
		fontSize = 24
	}

	var out []core.Artifact
	var bytesIn, bytesOut int64
	for _, item := range req.Items {
		if err := ctx.Err(); err != nil {
			return core.Result{}, err
		}
		m, err := media.Probe(ctx, b.Client, burnSubsSpec.ID, item, rep)
		if err != nil {
			return core.Result{}, err
		}
		if !m.HasVideo {
			return core.Result{}, core.NewError(core.CodeInput, burnSubsSpec.ID, item.Path,
				"no video stream found", "choose a video file")
		}

		sourceContainer, err := videoContainer(item)
		if err != nil {
			return core.Result{}, core.NewError(core.CodeInput, burnSubsSpec.ID, item.Path, err.Error(), "")
		}
		container := subtitleOutputContainer(sourceContainer)
		codecArgs, err := media.VideoArgs(container, "auto", "20", "medium")
		if err != nil {
			return core.Result{}, core.NewError(core.CodeInput, burnSubsSpec.ID, item.Path, err.Error(), "")
		}

		output := media.OutputPath(req, item, container)
		filter := subtitlesFilter(subtitle, fontSize)
		args := []string{"-i", item.Path, "-vf", filter, "-map_metadata", "0"}
		args = append(args, codecArgs...)
		if container == "mp4" || container == "mov" {
			args = append(args, "-movflags", "+faststart")
		}
		args = append(args, output)

		rep.Stage("burning subtitles")
		if err := b.Client.Run(ctx, args, m.Duration, rep); err != nil {
			if ctx.Err() != nil {
				return core.Result{}, ctx.Err()
			}
			hint := core.HintOf(err)
			if hint == "" {
				hint = "check that the subtitle file is valid and that ffmpeg was built with libass"
			}
			return core.Result{}, core.Wrap(core.CodeEngineFailed, burnSubsSpec.ID, item.Path, err, hint)
		}
		art, err := media.Finish(req, item, output, core.Video, container, m, m.Duration,
			map[string]any{"font_size": fontSize, "subtitles": subtitle})
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

// subtitleOutputContainer keeps containers that commonly carry burned-in
// subtitles and falls back to MP4 for everything else.
func subtitleOutputContainer(container string) string {
	switch container {
	case "mp4", "mkv", "mov":
		return container
	default:
		return "mp4"
	}
}

// subtitlesFilter builds the libass filter, quoting and escaping the path so
// spaces and filtergraph separators survive the trip through ffmpeg.
func subtitlesFilter(subtitle string, fontSize int) string {
	return fmt.Sprintf("subtitles=filename='%s':force_style='FontSize=%d'",
		escapeFilterPath(subtitle), fontSize)
}
