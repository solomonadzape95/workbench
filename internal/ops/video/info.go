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

var infoSpec = core.Spec{
	ID:      "video.info",
	Title:   "Read video info",
	Summary: "Read YouTube metadata from a URL and save it as JSON",
	Domain:  core.Video,
	Params: []core.ParamSpec{
		{Key: "playlist", Label: "Playlist", Type: core.ParamBool, Default: false, Advanced: true,
			Help: "Include every item when the URL is a playlist."},
	},
	Output:         core.OutputSpec{Kind: core.File, StaticFormat: "json"},
	Engines:        []core.EngineID{engines.YtDlp},
	RequireEngines: true,
	Source:         core.SourceURLs,
	Batchable:      true,
	Cost:           core.CostFast,
}

type Info struct {
	YtDlp *ytdlp.Client
}

func (i Info) Spec() core.Spec { return infoSpec }

func (i Info) Validate(ctx context.Context, req core.Request) error {
	if err := core.Validate(infoSpec.Params, req.Params); err != nil {
		return err
	}
	if len(req.Items) == 0 {
		return core.NewError(core.CodeUsage, infoSpec.ID, "", "no URLs", "enter at least one http(s) URL")
	}
	if i.YtDlp == nil || i.YtDlp.Check() != nil {
		return core.NewError(core.CodeEngineMissing, infoSpec.ID, "",
			"yt-dlp is not installed", ytdlp.InstallHint())
	}
	for _, item := range req.Items {
		if err := validateURL(item.Path); err != nil {
			return core.NewError(core.CodeInput, infoSpec.ID, item.Path, err.Error(), "enter a full http:// or https:// URL")
		}
	}
	return nil
}

func (i Info) Run(ctx context.Context, req core.Request, rep core.Reporter) (core.Result, error) {
	start := time.Now()
	opts := ytdlp.InfoOptions{Playlist: req.Params.Bool("playlist")}

	var out []core.Artifact
	var bytesOut int64
	for _, item := range req.Items {
		if err := ctx.Err(); err != nil {
			return core.Result{}, err
		}
		result, err := i.YtDlp.Info(ctx, item.Path, req.Target.Dir, opts, rep)
		if err != nil {
			if ctx.Err() != nil {
				return core.Result{}, ctx.Err()
			}
			return core.Result{}, core.Wrap(core.CodeEngineFailed, infoSpec.ID, item.Path, err, core.HintOf(err))
		}
		fileInfo, err := os.Stat(result.Path)
		if err != nil {
			return core.Result{}, core.Wrap(core.CodeOutput, infoSpec.ID, result.Path, err, "")
		}
		out = append(out, core.Artifact{
			Path:          result.Path,
			Kind:          core.File,
			Format:        "json",
			Size:          fileInfo.Size(),
			Label:         result.Title,
			SuggestedName: filepath.Base(result.Path),
			Meta: map[string]any{
				"source": item.Path,
				"url":    item.Path,
				"id":     result.ID,
			},
		})
		bytesOut += fileInfo.Size()
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
