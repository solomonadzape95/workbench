package audio

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"time"

	"github.com/solomonadzape95/workbench/internal/core"
	"github.com/solomonadzape95/workbench/internal/engines"
	"github.com/solomonadzape95/workbench/internal/engines/ffmpeg"
	"github.com/solomonadzape95/workbench/internal/ops/media"
)

var splitSpec = core.Spec{
	ID:      "audio.split",
	Title:   "Split audio",
	Summary: "Cut audio into fixed-length segments",
	Domain:  core.Audio,
	Accepts: []core.Accept{{Kind: core.Audio}},
	Params: []core.ParamSpec{
		{Key: "interval", Label: "Segment length", Type: core.ParamDuration, Default: "10m",
			Help: "Each segment is about this long; the last one is shorter."},
	},
	Engines:   []core.EngineID{engines.FFmpeg},
	Batchable: true,
	Cost:      core.CostMedium,
}

type Split struct {
	Client *ffmpeg.Client
}

func (s Split) Spec() core.Spec { return splitSpec }

// segmentMuxer names the container the segment muxer should write. The output
// extension still comes from the source format so names look natural.
func segmentMuxer(format string) (string, bool) {
	switch format {
	case "mp3":
		return "mp3", true
	case "m4a", "aac":
		return "mp4", true
	case "ogg", "oga":
		return "ogg", true
	case "opus":
		return "opus", true
	case "wav":
		return "wav", true
	case "flac":
		return "flac", true
	default:
		return "", false
	}
}

func (s Split) Validate(ctx context.Context, req core.Request) error {
	if req.Params.Duration("interval") <= 0 {
		return core.NewError(core.CodeUsage, splitSpec.ID, "",
			"segment length must be greater than zero", "")
	}
	if err := checkAudioInputs(splitSpec, req, s.Client); err != nil {
		return err
	}
	if err := validateAudioSources(splitSpec, req); err != nil {
		return err
	}
	for _, item := range req.Items {
		if _, ok := segmentMuxer(item.Ext()); !ok {
			return core.NewError(core.CodeInput, splitSpec.ID, item.Path,
				fmt.Sprintf("cannot split %q containers", item.Ext()),
				"supported inputs: MP3, M4A, WAV, FLAC, OGG, OPUS")
		}
	}
	return nil
}

func (s Split) Run(ctx context.Context, req core.Request, rep core.Reporter) (core.Result, error) {
	start := time.Now()
	interval := req.Params.Duration("interval")
	if interval <= 0 {
		return core.Result{}, core.NewError(core.CodeUsage, splitSpec.ID, "",
			"segment length must be greater than zero", "")
	}

	var out []core.Artifact
	var bytesIn, bytesOut int64
	for _, item := range req.Items {
		if err := ctx.Err(); err != nil {
			return core.Result{}, err
		}
		m, err := media.Probe(ctx, s.Client, splitSpec.ID, item, rep)
		if err != nil {
			return core.Result{}, err
		}
		if !m.HasAudio {
			return core.Result{}, core.NewError(core.CodeInput, splitSpec.ID, item.Path,
				"no audio stream found", "choose an audio file")
		}

		format, encoder, err := resolveAudioSource(splitSpec, item)
		if err != nil {
			return core.Result{}, err
		}
		muxer, ok := segmentMuxer(format)
		if !ok {
			return core.Result{}, core.NewError(core.CodeInput, splitSpec.ID, item.Path,
				fmt.Sprintf("cannot split %q containers", format), "")
		}

		stem := core.SanitizeStem(item.Stem())
		ext := core.ExtFor(format, item)
		pattern := filepath.Join(req.Target.Dir, stem+"-%03d."+ext)
		args := []string{"-i", item.Path, "-vn", "-map_metadata", "0", "-c:a", encoder}
		args = qualityArgs(args, encoder, "192k")
		args = append(args,
			"-f", "segment",
			"-segment_format", muxer,
			"-segment_time", audioSeconds(interval),
			"-reset_timestamps", "1",
			"-segment_start_number", "1",
			pattern,
		)

		rep.Stage("splitting")
		if err := s.Client.Run(ctx, args, m.Duration, rep); err != nil {
			if ctx.Err() != nil {
				return core.Result{}, ctx.Err()
			}
			return core.Result{}, core.Wrap(core.CodeEngineFailed, splitSpec.ID, item.Path, err, core.HintOf(err))
		}

		files, err := filepath.Glob(filepath.Join(req.Target.Dir, stem+"-*."+ext))
		if err != nil {
			return core.Result{}, core.Wrap(core.CodeOutput, splitSpec.ID, item.Path, err, "")
		}
		sort.Strings(files)
		if len(files) == 0 {
			return core.Result{}, core.NewError(core.CodeOutput, splitSpec.ID, item.Path,
				"ffmpeg produced no segments", "check that the source contains audio")
		}

		var offset time.Duration
		for i, path := range files {
			if err := ctx.Err(); err != nil {
				return core.Result{}, err
			}
			// Trust the real segment duration when ffprobe can read it; fall
			// back to the requested interval for the metadata otherwise.
			segDuration := interval
			if m.Duration > 0 {
				if remaining := m.Duration - offset; remaining < segDuration {
					segDuration = remaining
				}
			}
			if probed, err := s.Client.Probe(ctx, path); err == nil && probed.Duration > 0 {
				segDuration = probed.Duration
			}
			if segDuration <= 0 {
				segDuration = interval
			}
			art, err := media.Finish(req, item, path, core.Audio, format, m, segDuration,
				map[string]any{
					"segment":     i + 1,
					"segments":    len(files),
					"start_ms":    offset.Milliseconds(),
					"interval_ms": interval.Milliseconds(),
				})
			if err != nil {
				return core.Result{}, err
			}
			art.SuggestedName = filepath.Base(path)
			out = append(out, art)
			bytesOut += art.Size
			offset += segDuration
		}
		bytesIn += item.Size
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
