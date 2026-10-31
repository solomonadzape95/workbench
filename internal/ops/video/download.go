package video

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/solomonadzape95/workbench/internal/core"
	"github.com/solomonadzape95/workbench/internal/engines"
	"github.com/solomonadzape95/workbench/internal/engines/ffmpeg"
	"github.com/solomonadzape95/workbench/internal/engines/ytdlp"
)

var downloadSpec = core.Spec{
	ID:      "video.download",
	Title:   "Download video",
	Summary: "Download a YouTube or direct video URL with yt-dlp",
	Domain:  core.Video,
	Accepts: nil,
	Params: []core.ParamSpec{
		{
			Key: "quality", Label: "Quality", Type: core.ParamEnum, Default: "best",
			Options: []core.Option{
				{Value: "best", Label: "Best available"},
				{Value: "1080p", Label: "Up to 1080p"},
				{Value: "720p", Label: "Up to 720p"},
				{Value: "480p", Label: "Up to 480p"},
			},
		},
		{
			Key: "container", Label: "Container", Type: core.ParamEnum, Default: "mp4",
			Options: []core.Option{{Value: "mp4", Label: "MP4"}, {Value: "mkv", Label: "Matroska"}},
			Help:    "yt-dlp merges streams with ffmpeg.",
		},
		{Key: "playlist", Label: "Playlist", Type: core.ParamBool, Default: false, Advanced: true,
			Help: "Download every item when the URL is a playlist."},
	},
	Output:         core.OutputSpec{FormatParam: "container", Kind: core.Video},
	Engines:        []core.EngineID{engines.YtDlp, engines.FFmpeg},
	RequireEngines: true,
	Source:         core.SourceURLs,
	Batchable:      true,
	Cost:           core.CostSlow,
}

type Download struct {
	YtDlp  *ytdlp.Client
	FFmpeg *ffmpeg.Client
}

func (d Download) Spec() core.Spec { return downloadSpec }

func (d Download) Validate(ctx context.Context, req core.Request) error {
	if err := core.Validate(downloadSpec.Params, req.Params); err != nil {
		return err
	}
	if len(req.Items) == 0 {
		return core.NewError(core.CodeUsage, downloadSpec.ID, "", "no URLs", "enter at least one http(s) URL")
	}
	if d.YtDlp == nil || d.YtDlp.Check() != nil {
		return core.NewError(core.CodeEngineMissing, downloadSpec.ID, "",
			"yt-dlp is not installed", ytdlp.InstallHint())
	}
	if d.FFmpeg == nil || d.FFmpeg.Check() != nil {
		return core.NewError(core.CodeEngineMissing, downloadSpec.ID, "",
			"ffmpeg is required to merge downloaded streams", ffmpeg.InstallHint())
	}
	for _, item := range req.Items {
		if err := validateURL(item.Path); err != nil {
			return core.NewError(core.CodeInput, downloadSpec.ID, item.Path, err.Error(), "enter a full http:// or https:// URL")
		}
	}
	return nil
}

func (d Download) Run(ctx context.Context, req core.Request, rep core.Reporter) (core.Result, error) {
	start := time.Now()
	opts := ytdlp.Options{
		Quality:   req.Params.String("quality"),
		Container: req.Params.String("container"),
		Playlist:  req.Params.Bool("playlist"),
	}

	var out []core.Artifact
	var bytesOut int64
	for _, item := range req.Items {
		if err := ctx.Err(); err != nil {
			return core.Result{}, err
		}
		files, err := d.YtDlp.Download(ctx, item.Path, req.Target.Dir, opts, rep)
		if err != nil {
			if ctx.Err() != nil {
				return core.Result{}, ctx.Err()
			}
			return core.Result{}, core.Wrap(core.CodeEngineFailed, downloadSpec.ID, item.Path, err, core.HintOf(err))
		}
		for _, file := range files {
			info, err := os.Stat(file)
			if err != nil {
				return core.Result{}, core.Wrap(core.CodeOutput, downloadSpec.ID, file, err, "")
			}
			ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(file), "."))
			kind := core.Video
			switch ext {
			case "m4a", "mp3", "opus", "flac", "wav":
				kind = core.Audio
			}
			out = append(out, core.Artifact{
				Path:          file,
				Kind:          kind,
				Format:        ext,
				Size:          info.Size(),
				SuggestedName: filepath.Base(file),
				Meta: map[string]any{
					"source": item.Path,
					"url":    item.Path,
				},
			})
			bytesOut += info.Size()
		}
		rep.Progress(1, 1)
	}

	return core.Result{
		Artifacts: out,
		Metrics: core.Metrics{
			BytesOut: bytesOut,
			Duration: time.Since(start),
		},
	}, nil
}

func validateURL(raw string) error {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return fmt.Errorf("invalid URL: %w", err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return fmt.Errorf("URL must start with http:// or https://")
	}
	if parsed.Host == "" {
		return fmt.Errorf("URL has no host")
	}
	return nil
}
