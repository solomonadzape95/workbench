package audio

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/solomonadzape95/workbench/internal/core"
	"github.com/solomonadzape95/workbench/internal/engines"
	"github.com/solomonadzape95/workbench/internal/engines/ffmpeg"
	"github.com/solomonadzape95/workbench/internal/ops/media"
)

var joinSpec = core.Spec{
	ID:      "audio.join",
	Title:   "Join audio",
	Summary: "Concatenate audio files into one track",
	Domain:  core.Audio,
	Accepts: []core.Accept{{Kind: core.Audio}},
	Params: []core.ParamSpec{
		{Key: "format", Label: "Format", Type: core.ParamEnum, Default: "mp3", Options: formatOptions()},
		{Key: "bitrate", Label: "Bitrate", Type: core.ParamEnum, Default: "192k", Options: bitrateOptions(), Advanced: true},
		{Key: "crossfade", Label: "Crossfade", Type: core.ParamDuration, Default: "0s",
			Help: "Optional overlap between tracks. 0 joins them end to end."},
	},
	Output:  core.OutputSpec{FormatParam: "format", Kind: core.Audio},
	Engines: []core.EngineID{engines.FFmpeg},
	Mode:    core.ModeAggregate,
	Cost:    core.CostMedium,
}

type Join struct {
	Client *ffmpeg.Client
}

func (j Join) Spec() core.Spec { return joinSpec }

func (j Join) Validate(ctx context.Context, req core.Request) error {
	if err := core.Validate(joinSpec.Params, req.Params); err != nil {
		return err
	}
	if len(req.Items) < 2 {
		return core.NewError(core.CodeUsage, joinSpec.ID, "", "join needs at least two audio files", "add more inputs")
	}
	if err := media.Check(j.Client, joinSpec.ID); err != nil {
		return err
	}
	for _, item := range req.Items {
		if !joinSpec.AcceptsArtifact(item) {
			return core.NewError(core.CodeInput, joinSpec.ID, item.Path, "not an audio file", "choose audio files")
		}
	}
	if _, _, ok := media.AudioEncoder(req.Params.String("format")); !ok {
		return core.NewError(core.CodeUsage, joinSpec.ID, "", "unsupported output format", "")
	}
	return nil
}

func (j Join) Run(ctx context.Context, req core.Request, rep core.Reporter) (core.Result, error) {
	start := time.Now()
	format := req.Params.String("format")
	encoder, _, ok := media.AudioEncoder(format)
	if !ok {
		return core.Result{}, core.NewError(core.CodeUsage, joinSpec.ID, "", "unsupported output format", "")
	}
	bitrate := req.Params.String("bitrate")
	if bitrate == "" {
		bitrate = "192k"
	}
	crossfade := req.Params.Duration("crossfade")

	args := []string{}
	var filterParts []string
	var totalDuration time.Duration
	for i, item := range req.Items {
		m, err := media.Probe(ctx, j.Client, joinSpec.ID, item, rep)
		if err != nil {
			return core.Result{}, err
		}
		if !m.HasAudio {
			return core.Result{}, core.NewError(core.CodeInput, joinSpec.ID, item.Path, "no audio stream", "")
		}
		totalDuration += m.Duration
		args = append(args, "-i", item.Path)
		_ = i
	}
	inputs := len(req.Items)
	if crossfade > 0 {
		parts := make([]string, 0, inputs*2)
		last := "[0:a]"
		for i := 1; i < inputs; i++ {
			out := fmt.Sprintf("[a%d]", i)
			parts = append(parts, fmt.Sprintf("%s[%d:a]acrossfade=d=%.3f:c1=tri:c2=tri%s", last, i, crossfade.Seconds(), out))
			last = out
		}
		filterParts = append(filterParts, strings.Join(parts, ";"))
		args = append(args, "-filter_complex", strings.Join(filterParts, ";"), "-map", last)
	} else {
		args = append(args, "-filter_complex", fmt.Sprintf("concat=n=%d:v=0:a=1[out]", inputs), "-map", "[out]")
	}
	ext := core.ExtFor(format, req.Items[0])
	name := core.SanitizeStem(req.Items[0].Stem()) + "-joined." + ext
	output := filepath.Join(req.Target.Dir, name)
	args = append(args, "-c:a", encoder)
	args = qualityArgs(args, encoder, bitrate)
	args = append(args, output)

	rep.Stage("joining audio")
	if err := j.Client.Run(ctx, args, totalDuration, rep); err != nil {
		if ctx.Err() != nil {
			return core.Result{}, ctx.Err()
		}
		return core.Result{}, core.Wrap(core.CodeEngineFailed, joinSpec.ID, "", err, core.HintOf(err))
	}
	info, err := os.Stat(output)
	if err != nil {
		return core.Result{}, core.Wrap(core.CodeOutput, joinSpec.ID, output, err, "")
	}
	var bytesIn int64
	for _, item := range req.Items {
		bytesIn += item.Size
	}
	artifact := core.Artifact{
		Path: output, Kind: core.Audio, Format: format, Size: info.Size(),
		SuggestedName: name,
		Meta: map[string]any{
			"source":       fmt.Sprintf("%d tracks", len(req.Items)),
			"duration_ms":  totalDuration.Milliseconds(),
			"crossfade_ms": crossfade.Milliseconds(),
		},
	}
	return core.Result{
		Artifacts: []core.Artifact{artifact},
		Metrics: core.Metrics{
			BytesIn: bytesIn, BytesOut: info.Size(), Duration: time.Since(start), Engine: engines.FFmpeg,
		},
	}, nil
}
