package video

import (
	"context"
	"os"
	"path/filepath"
	"time"

	"github.com/solomonadzape95/workbench/internal/core"
	"github.com/solomonadzape95/workbench/internal/engines"
	"github.com/solomonadzape95/workbench/internal/engines/ytdlp"
)

var transcriptSpec = core.Spec{
	ID:      "video.transcript",
	Title:   "Get transcript",
	Summary: "Fetch YouTube captions from a URL and save a plain-text transcript",
	Domain:  core.Video,
	Params: []core.ParamSpec{
		{Key: "language", Label: "Language", Type: core.ParamString, Default: "en",
			Help: "Language code such as en, es, or de."},
		{Key: "auto", Label: "Auto captions", Type: core.ParamBool, Default: true,
			Help: "Use generated captions when the creator did not upload subtitles."},
		{Key: "timestamps", Label: "Timestamps", Type: core.ParamBool, Default: false,
			Help: "Prefix each caption with its start time."},
	},
	Output:         core.OutputSpec{Kind: core.Document, StaticFormat: "txt"},
	Engines:        []core.EngineID{engines.YtDlp},
	RequireEngines: true,
	Source:         core.SourceURLs,
	Batchable:      true,
	Cost:           core.CostMedium,
}

type Transcript struct {
	YtDlp *ytdlp.Client
}

func (t Transcript) Spec() core.Spec { return transcriptSpec }

func (t Transcript) Validate(ctx context.Context, req core.Request) error {
	if err := core.Validate(transcriptSpec.Params, req.Params); err != nil {
		return err
	}
	if len(req.Items) == 0 {
		return core.NewError(core.CodeUsage, transcriptSpec.ID, "", "no URLs", "enter at least one http(s) URL")
	}
	if t.YtDlp == nil || t.YtDlp.Check() != nil {
		return core.NewError(core.CodeEngineMissing, transcriptSpec.ID, "",
			"yt-dlp is not installed", ytdlp.InstallHint())
	}
	for _, item := range req.Items {
		if err := validateURL(item.Path); err != nil {
			return core.NewError(core.CodeInput, transcriptSpec.ID, item.Path, err.Error(), "enter a full http:// or https:// URL")
		}
	}
	return nil
}

func (t Transcript) Run(ctx context.Context, req core.Request, rep core.Reporter) (core.Result, error) {
	start := time.Now()
	opts := ytdlp.TranscriptOptions{
		Language:   req.Params.String("language"),
		Auto:       req.Params.Bool("auto"),
		Timestamps: req.Params.Bool("timestamps"),
	}

	var out []core.Artifact
	var bytesOut int64
	for _, item := range req.Items {
		if err := ctx.Err(); err != nil {
			return core.Result{}, err
		}
		result, err := t.YtDlp.Transcript(ctx, item.Path, req.Target.Dir, opts, rep)
		if err != nil {
			if ctx.Err() != nil {
				return core.Result{}, ctx.Err()
			}
			return core.Result{}, core.Wrap(core.CodeEngineFailed, transcriptSpec.ID, item.Path, err, core.HintOf(err))
		}
		info, err := os.Stat(result.Path)
		if err != nil {
			return core.Result{}, core.Wrap(core.CodeOutput, transcriptSpec.ID, result.Path, err, "")
		}
		out = append(out, core.Artifact{
			Path:          result.Path,
			Kind:          core.Document,
			Format:        "txt",
			Size:          info.Size(),
			Label:         result.Title,
			SuggestedName: filepath.Base(result.Path),
			Meta: map[string]any{
				"source":   item.Path,
				"url":      item.Path,
				"language": result.Language,
			},
		})
		bytesOut += info.Size()
		rep.Progress(1, 1)
	}

	return core.Result{
		Artifacts: out,
		Metrics: core.Metrics{
			BytesOut: bytesOut,
			Duration: time.Since(start),
			Engine:   engines.YtDlp,
		},
	}, nil
}
