package video

import (
	"context"
	"os"
	"path/filepath"
	"time"

	"github.com/solomonadzape95/workbench/internal/core"
	"github.com/solomonadzape95/workbench/internal/engines"
	"github.com/solomonadzape95/workbench/internal/engines/ffmpeg"
	"github.com/solomonadzape95/workbench/internal/engines/whisper"
	"github.com/solomonadzape95/workbench/internal/ops/media"
)

var transcribeSpec = core.Spec{
	ID:      "video.transcribe",
	Title:   "Transcribe video",
	Summary: "Extract speech from a video locally with whisper.cpp",
	Domain:  core.Video,
	Accepts: []core.Accept{{Kind: core.Video}},
	Params: []core.ParamSpec{
		{
			Key: "model", Label: "Model", Type: core.ParamEnum, Default: "small.en",
			Options: []core.Option{
				{Value: "tiny.en", Label: "Tiny English (fastest)"},
				{Value: "base.en", Label: "Base English"},
				{Value: "small.en", Label: "Small English (default)"},
				{Value: "medium.en", Label: "Medium English"},
				{Value: "tiny", Label: "Tiny multilingual"},
				{Value: "base", Label: "Base multilingual"},
				{Value: "small", Label: "Small multilingual"},
			},
		},
		{Key: "language", Label: "Language", Type: core.ParamString, Default: "en"},
		{
			Key: "format", Label: "Output", Type: core.ParamEnum, Default: "txt",
			Options: []core.Option{
				{Value: "txt", Label: "Plain text"},
				{Value: "srt", Label: "SRT subtitles"},
				{Value: "vtt", Label: "WebVTT subtitles"},
				{Value: "json", Label: "JSON"},
			},
		},
	},
	Output:         core.OutputSpec{FormatParam: "format", Kind: core.Document},
	Engines:        []core.EngineID{engines.Whisper},
	RequireEngines: true,
	Batchable:      true,
	Cost:           core.CostSlow,
}

type Transcribe struct {
	Whisper *whisper.Client
	FFmpeg  *ffmpeg.Client
}

func (t Transcribe) Spec() core.Spec { return transcribeSpec }

func (t Transcribe) Validate(ctx context.Context, req core.Request) error {
	if err := core.Validate(transcribeSpec.Params, req.Params); err != nil {
		return err
	}
	if len(req.Items) == 0 {
		return core.NewError(core.CodeUsage, transcribeSpec.ID, "", "no input videos", "")
	}
	if t.Whisper == nil {
		return core.NewError(core.CodeEngineMissing, transcribeSpec.ID, "", "whisper.cpp is not installed", whisper.InstallHint())
	}
	if err := t.Whisper.Check(); err != nil {
		return err
	}
	model := req.Params.String("model")
	if model == "" {
		model = "small.en"
	}
	if _, err := t.Whisper.ModelPath(model); err != nil {
		return core.NewError(core.CodeEngineMissing, transcribeSpec.ID, "", err.Error(),
			"download ggml-"+model+".bin into ~/.local/share/whisper-models")
	}
	if t.FFmpeg == nil {
		return core.NewError(core.CodeEngineMissing, transcribeSpec.ID, "", "ffmpeg is needed to extract audio", ffmpeg.InstallHint())
	}
	if err := t.FFmpeg.Check(); err != nil {
		return err
	}
	for _, item := range req.Items {
		if !transcribeSpec.AcceptsArtifact(item) {
			return core.NewError(core.CodeInput, transcribeSpec.ID, item.Path, "not a video file", "choose video files")
		}
	}
	return nil
}

func (t Transcribe) Run(ctx context.Context, req core.Request, rep core.Reporter) (core.Result, error) {
	start := time.Now()
	modelName := req.Params.String("model")
	if modelName == "" {
		modelName = "small.en"
	}
	modelPath, err := t.Whisper.ModelPath(modelName)
	if err != nil {
		return core.Result{}, core.NewError(core.CodeEngineMissing, transcribeSpec.ID, "", err.Error(), "")
	}
	language := req.Params.String("language")
	format := req.Params.String("format")
	if format == "" {
		format = "txt"
	}

	var out []core.Artifact
	var bytesIn, bytesOut int64
	for _, item := range req.Items {
		if err := ctx.Err(); err != nil {
			return core.Result{}, err
		}
		m, err := media.Probe(ctx, t.FFmpeg, transcribeSpec.ID, item, rep)
		if err != nil {
			return core.Result{}, err
		}
		if !m.HasVideo {
			return core.Result{}, core.NewError(core.CodeInput, transcribeSpec.ID, item.Path, "no video stream found", "")
		}
		work, err := os.MkdirTemp("", "workbench-video-whisper-*")
		if err != nil {
			return core.Result{}, core.Wrap(core.CodeInternal, transcribeSpec.ID, item.Path, err, "")
		}
		audioPath := filepath.Join(work, "audio.wav")
		rep.Stage("extracting audio")
		args := []string{"-i", item.Path, "-vn", "-ac", "1", "-ar", "16000", audioPath}
		if err := t.FFmpeg.Run(ctx, args, m.Duration, rep); err != nil {
			os.RemoveAll(work)
			if ctx.Err() != nil {
				return core.Result{}, ctx.Err()
			}
			return core.Result{}, core.Wrap(core.CodeEngineFailed, transcribeSpec.ID, item.Path, err, core.HintOf(err))
		}
		prefix := filepath.Join(req.Target.Dir, core.SanitizeStem(item.Stem()))
		output, err := t.Whisper.Transcribe(ctx, audioPath, prefix, whisper.Options{
			Model: modelPath, Language: language, Format: format,
		}, rep)
		os.RemoveAll(work)
		if err != nil {
			if ctx.Err() != nil {
				return core.Result{}, ctx.Err()
			}
			return core.Result{}, core.Wrap(core.CodeEngineFailed, transcribeSpec.ID, item.Path, err, core.HintOf(err))
		}
		info, err := os.Stat(output)
		if err != nil {
			return core.Result{}, core.Wrap(core.CodeOutput, transcribeSpec.ID, output, err, "")
		}
		out = append(out, core.Artifact{
			Path: output, Kind: core.Document, Format: format, Size: info.Size(),
			SuggestedName: filepath.Base(output),
			Meta: map[string]any{
				"source": item.Path, "model": modelName, "language": language,
			},
		})
		bytesIn += item.Size
		bytesOut += info.Size()
		rep.Progress(1, 1)
	}
	return core.Result{
		Artifacts: out,
		Metrics: core.Metrics{
			BytesIn: bytesIn, BytesOut: bytesOut, Duration: time.Since(start),
			Engine: engines.Whisper, Model: modelName,
		},
	}, nil
}
