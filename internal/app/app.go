// Package app is the composition root: it loads configuration and wires the
// engines the rest of the program uses.
package app

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/solomonadzape95/workbench/internal/config"
	"github.com/solomonadzape95/workbench/internal/core"
	"github.com/solomonadzape95/workbench/internal/engines"
	"github.com/solomonadzape95/workbench/internal/engines/demucs"
	"github.com/solomonadzape95/workbench/internal/engines/ffmpeg"
	"github.com/solomonadzape95/workbench/internal/engines/ghostscript"
	"github.com/solomonadzape95/workbench/internal/engines/libreoffice"
	"github.com/solomonadzape95/workbench/internal/engines/opencode"
	"github.com/solomonadzape95/workbench/internal/engines/pandoc"
	"github.com/solomonadzape95/workbench/internal/engines/pdf2docx"
	"github.com/solomonadzape95/workbench/internal/engines/poppler"
	"github.com/solomonadzape95/workbench/internal/engines/realesrgan"
	"github.com/solomonadzape95/workbench/internal/engines/rembg"
	"github.com/solomonadzape95/workbench/internal/engines/tesseract"
	"github.com/solomonadzape95/workbench/internal/engines/whisper"
	"github.com/solomonadzape95/workbench/internal/engines/ytdlp"
	"github.com/solomonadzape95/workbench/internal/history"
	audioops "github.com/solomonadzape95/workbench/internal/ops/audio"
	detectops "github.com/solomonadzape95/workbench/internal/ops/detect"
	docops "github.com/solomonadzape95/workbench/internal/ops/doc"
	"github.com/solomonadzape95/workbench/internal/ops/file"
	humanizeops "github.com/solomonadzape95/workbench/internal/ops/humanize"
	imageops "github.com/solomonadzape95/workbench/internal/ops/image"
	videoops "github.com/solomonadzape95/workbench/internal/ops/video"
	"github.com/solomonadzape95/workbench/internal/pipeline"
	"github.com/solomonadzape95/workbench/internal/runner"
	"github.com/solomonadzape95/workbench/internal/state"
)

type App struct {
	Config     config.Config
	Paths      config.Paths
	Registry   *engines.Registry
	Operations *core.Registry
	Pipelines  *pipeline.Store
	History    *history.Store
	State      *state.Store
	Saved      state.State
	FirstRun   bool
	Warnings   []string
}

// Boot loads config and builds the engine registry. It never fails: a missing
// or broken config degrades to defaults with warnings attached.
func Boot() *App {
	paths := config.DefaultPaths()
	_, statErr := os.Stat(paths.ConfigFile)
	firstRun := errors.Is(statErr, fs.ErrNotExist)
	cfg, warnings := config.Load(paths.ConfigFile)

	stateStore := state.New(paths.DataDir)
	saved, stateErr := stateStore.Load()
	if stateErr != nil {
		warnings = append(warnings, "state: "+stateErr.Error())
	}

	return &App{
		Config:     cfg,
		Paths:      paths,
		Registry:   buildRegistry(cfg),
		Operations: buildOperations(cfg),
		Pipelines:  pipeline.NewStore(filepath.Join(paths.DataDir, "pipelines")),
		History:    history.New(paths.DataDir),
		State:      stateStore,
		Saved:      saved,
		FirstRun:   firstRun,
		Warnings:   warnings,
	}
}

// NewRunner creates the job runner for a TUI session. Callers own Close.
func (a *App) NewRunner() *runner.Runner {
	runner.CleanStale(a.Paths.CacheDir, 24*time.Hour)
	logDir := filepath.Join(a.Paths.DataDir, "logs")
	if a.Config.Logs.MaxAgeDays > 0 || a.Config.Logs.MaxFiles > 0 {
		_ = runner.PruneLogs(logDir, time.Duration(a.Config.Logs.MaxAgeDays)*24*time.Hour, a.Config.Logs.MaxFiles, time.Now())
	}
	return runner.New(runner.Options{
		Operations:   a.Operations,
		CacheDir:     a.Paths.CacheDir,
		LogDir:       logDir,
		UndoDir:      filepath.Join(a.Paths.DataDir, "undo"),
		MaxJobs:      a.Config.General.MaxJobs,
		EngineLimits: a.Config.Engines.Limits,
		History:      a.History,
	})
}

func buildOperations(cfg config.Config) *core.Registry {
	enabled := make(map[string]bool, len(cfg.Engines.Enabled))
	for _, id := range cfg.Engines.Enabled {
		enabled[id] = true
	}

	ops := []core.Operation{
		imageops.Convert{},
		imageops.Compress{},
		imageops.Resize{},
		imageops.Crop{},
		imageops.Rotate{},
		imageops.StripMeta{},
		imageops.ToICO{},
		imageops.Enhance{},
		imageops.Watermark{},
		imageops.ContactSheet{},
		detectops.Detect{LogProbCommand: cfg.Detect.LogProbCommand, LogProbWeight: cfg.Detect.LogProbWeight},
		docops.Edit{},
	}

	if enabled[string(engines.Rembg)] {
		ops = append(ops, imageops.RemoveBG{
			Rembg: rembg.NewClient(config.ExpandHome(cfg.Engines.Paths[string(engines.Rembg)])),
		})
	}
	if enabled[string(engines.RealESRGAN)] {
		ops = append(ops, imageops.Upscale{
			Client: realesrgan.NewClient(config.ExpandHome(cfg.Engines.Paths[string(engines.RealESRGAN)])),
		})
	}

	var ff *ffmpeg.Client
	if enabled[string(engines.FFmpeg)] {
		ff = ffmpeg.NewClient(config.ExpandHome(cfg.Engines.Paths[string(engines.FFmpeg)]))
		var demucsClient *demucs.Client
		if enabled[string(engines.Demucs)] {
			demucsClient = demucs.NewClient(config.ExpandHome(cfg.Engines.Paths[string(engines.Demucs)]))
		}
		ops = append(ops,
			audioops.Convert{Client: ff},
			audioops.Compress{Client: ff},
			audioops.Speed{Client: ff},
			audioops.Normalize{Client: ff},
			audioops.Trim{Client: ff},
			audioops.Split{Client: ff},
			audioops.Fade{Client: ff},
			audioops.SilenceTrim{Client: ff},
			audioops.Enhance{Client: ff},
			audioops.ExtractVocals{FFmpeg: ff, Demucs: demucsClient},
			audioops.ExtractInstrumental{FFmpeg: ff, Demucs: demucsClient},
			audioops.Join{Client: ff},
			videoops.Convert{Client: ff},
			videoops.Compress{Client: ff},
			videoops.Extract{Client: ff},
			videoops.Speed{Client: ff},
			videoops.Trim{Client: ff},
			videoops.Thumbnail{Client: ff},
			videoops.ToGif{Client: ff},
			videoops.Remux{Client: ff},
			videoops.BurnSubs{Client: ff},
			videoops.Watermark{Client: ff},
			videoops.Concat{Client: ff},
		)
	}
	if enabled[string(engines.Whisper)] {
		wh := whisper.NewClient(config.ExpandHome(cfg.Engines.Paths[string(engines.Whisper)]))
		ops = append(ops,
			audioops.Transcribe{Whisper: wh, FFmpeg: ff},
			videoops.Transcribe{Whisper: wh, FFmpeg: ff},
		)
	}
	if enabled[string(engines.YtDlp)] {
		yt := ytdlp.NewClient(config.ExpandHome(cfg.Engines.Paths[string(engines.YtDlp)]))
		ops = append(ops,
			videoops.Download{YtDlp: yt, FFmpeg: ff},
			videoops.Transcript{YtDlp: yt},
			videoops.Info{YtDlp: yt},
		)
	}

	var lo *libreoffice.Client
	if enabled[string(engines.LibreOffice)] {
		lo = libreoffice.NewClient(config.ExpandHome(cfg.Engines.Paths[string(engines.LibreOffice)]))
	}
	var pd *pandoc.Client
	if enabled[string(engines.Pandoc)] {
		pd = pandoc.NewClient(config.ExpandHome(cfg.Engines.Paths[string(engines.Pandoc)]))
	}
	var p2d *pdf2docx.Client
	if enabled[string(engines.PDF2DOCX)] {
		p2d = pdf2docx.NewClient(config.ExpandHome(cfg.Engines.Paths[string(engines.PDF2DOCX)]))
	}
	if lo != nil || pd != nil || p2d != nil {
		convert := docops.Convert{LibreOffice: lo, Pandoc: pd, PDF2DOCX: p2d}
		ops = append(ops, convert, docops.ExtractText{Convert: convert}, docops.StripMeta{})
	}
	if enabled[string(engines.Ghostscript)] {
		gs := ghostscript.NewClient(config.ExpandHome(cfg.Engines.Paths[string(engines.Ghostscript)]))
		ops = append(ops, docops.Compress{Ghostscript: gs}, docops.Merge{Ghostscript: gs})
	}
	var popplerClient *poppler.Client
	if enabled[string(engines.Poppler)] {
		popplerClient = poppler.NewClient(config.ExpandHome(cfg.Engines.Paths[string(engines.Poppler)]))
		ops = append(ops, docops.Split{Poppler: popplerClient})
	}
	if enabled[string(engines.Tesseract)] {
		ops = append(ops, docops.OCR{
			Tesseract: tesseract.NewClient(config.ExpandHome(cfg.Engines.Paths[string(engines.Tesseract)])),
			Poppler:   popplerClient,
		})
	}
	if enabled[string(engines.OpenCode)] {
		client := opencode.NewClient(config.ExpandHome(cfg.Engines.Paths[string(engines.OpenCode)]))
		client.ServerURL = cfg.OpenCode.URL
		client.Password = cfg.OpenCode.Password
		ops = append(ops,
			&humanizeops.Humanize{Client: client},
			docops.Translate{Client: client},
			docops.Summarize{Client: client},
			docops.Rewrite{Client: client},
		)
	}

	ops = append(ops, file.Archive{}, file.Extract{}, file.Copy{}, file.Move{}, file.Rename{}, file.Trash{}, file.Hash{}, file.Verify{}, file.Split{}, file.Manifest{}, file.Dedupe{})
	return core.NewRegistry(ops...)
}

func buildRegistry(cfg config.Config) *engines.Registry {
	enabled := make(map[string]bool, len(cfg.Engines.Enabled))
	for _, id := range cfg.Engines.Enabled {
		enabled[id] = true
	}

	// The built-in engine is the floor: it has no install requirements and is
	// always registered.
	list := []engines.Engine{engines.Builtin{}}
	if enabled[string(engines.FFmpeg)] {
		list = append(list, ffmpeg.New(config.ExpandHome(cfg.Engines.Paths[string(engines.FFmpeg)])))
	}
	if enabled[string(engines.YtDlp)] {
		list = append(list, ytdlp.NewClient(config.ExpandHome(cfg.Engines.Paths[string(engines.YtDlp)])))
	}
	if enabled[string(engines.LibreOffice)] {
		list = append(list, libreoffice.NewClient(config.ExpandHome(cfg.Engines.Paths[string(engines.LibreOffice)])))
	}
	if enabled[string(engines.Pandoc)] {
		list = append(list, pandoc.NewClient(config.ExpandHome(cfg.Engines.Paths[string(engines.Pandoc)])))
	}
	if enabled[string(engines.Ghostscript)] {
		list = append(list, ghostscript.New(config.ExpandHome(cfg.Engines.Paths[string(engines.Ghostscript)])))
	}
	if enabled[string(engines.Tesseract)] {
		list = append(list, tesseract.New(config.ExpandHome(cfg.Engines.Paths[string(engines.Tesseract)])))
	}
	if enabled[string(engines.PDF2DOCX)] {
		list = append(list, pdf2docx.NewClient(config.ExpandHome(cfg.Engines.Paths[string(engines.PDF2DOCX)])))
	}
	if enabled[string(engines.Poppler)] {
		list = append(list, poppler.New(config.ExpandHome(cfg.Engines.Paths[string(engines.Poppler)])))
	}
	if enabled[string(engines.Whisper)] {
		list = append(list, whisper.NewClient(config.ExpandHome(cfg.Engines.Paths[string(engines.Whisper)])))
	}
	if enabled[string(engines.Rembg)] {
		list = append(list, rembg.NewClient(config.ExpandHome(cfg.Engines.Paths[string(engines.Rembg)])))
	}
	if enabled[string(engines.Demucs)] {
		list = append(list, demucs.New(config.ExpandHome(cfg.Engines.Paths[string(engines.Demucs)])))
	}
	if enabled[string(engines.RealESRGAN)] {
		list = append(list, realesrgan.New(config.ExpandHome(cfg.Engines.Paths[string(engines.RealESRGAN)])))
	}
	if enabled[string(engines.OpenCode)] {
		client := opencode.NewClient(config.ExpandHome(cfg.Engines.Paths[string(engines.OpenCode)]))
		client.ServerURL = cfg.OpenCode.URL
		client.Password = cfg.OpenCode.Password
		list = append(list, client)
	}
	return engines.NewRegistry(list...)
}
