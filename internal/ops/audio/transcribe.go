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
	"github.com/solomonadzape95/workbench/internal/engines/whisper"
)

var transcribeSpec = core.Spec{
	ID:      "audio.transcribe",
	Title:   "Transcribe audio",
	Summary: "Turn speech in an audio file into text, subtitles, or JSON",
	Domain:  core.Audio,
	Accepts: []core.Accept{{Kind: core.Audio}},
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
			Help: "Models live in ~/.local/share/whisper-models; .en models are English-only.",
		},
		{
			Key: "language", Label: "Language", Type: core.ParamString, Default: "en",
			Help: "Language code such as en, es, or auto; use a multilingual model for non-English audio.",
		},
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

func (Transcribe) Spec() core.Spec { return transcribeSpec }

func (c Transcribe) Validate(ctx context.Context, req core.Request) error {
	if err := core.Validate(transcribeSpec.Params, req.Params); err != nil {
		return err
	}
	if len(req.Items) == 0 {
		return core.NewError(core.CodeUsage, transcribeSpec.ID, "", "no input audio", "")
	}
	if c.Whisper == nil {
		return core.NewError(core.CodeEngineMissing, transcribeSpec.ID, "",
			"whisper.cpp is not installed", whisper.InstallHint())
	}
	if err := c.Whisper.Check(); err != nil {
		return err
	}
	model := req.Params.String("model")
	if model == "" {
		model = "small.en"
	}
	if _, err := c.Whisper.ModelPath(model); err != nil {
		return core.NewError(core.CodeEngineMissing, transcribeSpec.ID, "",
			err.Error(), whisperModelHint(model))
	}
	for _, item := range req.Items {
		if !transcribeSpec.AcceptsArtifact(item) {
			return core.NewError(core.CodeInput, transcribeSpec.ID, item.Path,
				fmt.Sprintf("%s is not an audio file", item.Format), "supported inputs: audio files")
		}
		if !directWhisperInput(item.Ext()) && c.FFmpeg == nil {
			return core.NewError(core.CodeEngineMissing, transcribeSpec.ID, item.Path,
				"ffmpeg is needed to prepare this audio format", ffmpeg.InstallHint())
		}
	}
	return nil
}

func (c Transcribe) Run(ctx context.Context, req core.Request, rep core.Reporter) (core.Result, error) {
	start := time.Now()
	modelName := req.Params.String("model")
	if modelName == "" {
		modelName = "small.en"
	}
	modelPath, err := c.Whisper.ModelPath(modelName)
	if err != nil {
		return core.Result{}, core.NewError(core.CodeEngineMissing, transcribeSpec.ID, "",
			err.Error(), whisperModelHint(modelName))
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
		work, err := os.MkdirTemp("", "workbench-whisper-*")
		if err != nil {
			return core.Result{}, core.Wrap(core.CodeInternal, transcribeSpec.ID, item.Path, err, "")
		}
		audioPath := item.Path
		if !directWhisperInput(item.Ext()) {
			audioPath = filepath.Join(work, "input.wav")
			rep.Stage("preparing audio")
			args := []string{"-i", item.Path, "-vn", "-ac", "1", "-ar", "16000", audioPath}
			if err := c.FFmpeg.Run(ctx, args, 0, rep); err != nil {
				os.RemoveAll(work)
				if ctx.Err() != nil {
					return core.Result{}, ctx.Err()
				}
				return core.Result{}, core.Wrap(core.CodeEngineFailed, transcribeSpec.ID, item.Path, err, core.HintOf(err))
			}
		}

		prefix := filepath.Join(req.Target.Dir, core.SanitizeStem(item.Stem()))
		rep.Progress(0, 1)
		output, err := c.Whisper.Transcribe(ctx, audioPath, prefix, whisper.Options{
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
			Path:   output,
			Kind:   core.Document,
			Format: format,
			Size:   info.Size(),
			Meta: map[string]any{
				"source":   item.Path,
				"model":    modelName,
				"language": language,
			},
		})
		bytesIn += item.Size
		bytesOut += info.Size()
		rep.Progress(1, 1)
	}

	return core.Result{
		Artifacts: out,
		Metrics: core.Metrics{
			BytesIn:  bytesIn,
			BytesOut: bytesOut,
			Duration: time.Since(start),
			Engine:   engines.Whisper,
			Model:    modelName,
		},
	}, nil
}

func directWhisperInput(ext string) bool {
	switch strings.ToLower(strings.TrimPrefix(ext, ".")) {
	case "wav", "mp3", "flac", "ogg":
		return true
	default:
		return false
	}
}

func whisperModelHint(model string) string {
	return "download ggml-" + model + ".bin into ~/.local/share/whisper-models (or set WORKBENCH_WHISPER_MODEL_DIR)"
}
