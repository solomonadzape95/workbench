package audio

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/solomonadzape95/workbench/internal/core"
	"github.com/solomonadzape95/workbench/internal/engines"
	"github.com/solomonadzape95/workbench/internal/engines/demucs"
	"github.com/solomonadzape95/workbench/internal/engines/ffmpeg"
	"github.com/solomonadzape95/workbench/internal/ops/media"
)

const (
	centerVocalsFilter       = "aformat=channel_layouts=stereo,pan=stereo|c0=0.5*c0+0.5*c1|c1=0.5*c0+0.5*c1"
	centerInstrumentalFilter = "aformat=channel_layouts=stereo,pan=stereo|c0=0.5*c0-0.5*c1|c1=0.5*c1-0.5*c0"
)

var extractVocalsSpec = core.Spec{
	ID:        "audio.extract_vocals",
	Title:     "Extract vocals",
	Summary:   "Isolate the vocal stem from a song",
	Domain:    core.Audio,
	Accepts:   []core.Accept{{Kind: core.Audio}},
	Params:    stemParams(),
	Output:    core.OutputSpec{FormatParam: "format"},
	Engines:   []core.EngineID{engines.Demucs, engines.FFmpeg},
	Batchable: true,
	Cost:      core.CostSlow,
}

var extractInstrumentalSpec = func() core.Spec {
	spec := extractVocalsSpec
	spec.ID = "audio.extract_instrumental"
	spec.Title = "Extract instrumental"
	spec.Summary = "Remove the vocal and keep the instrumental mix"
	return spec
}()

func stemParams() []core.ParamSpec {
	return []core.ParamSpec{
		{
			Key: "method", Label: "Method", Type: core.ParamEnum, Default: "auto",
			Options: []core.Option{
				{Value: "auto", Label: "Auto (Demucs if installed, else center)"},
				{Value: "demucs", Label: "Demucs (clean separation)"},
				{Value: "center", Label: "Center channel (ffmpeg only)"},
			},
			Help: "Demucs runs a source-separation model and is much slower. Center-channel isolation is a fast fallback.",
		},
		{
			Key: "model", Label: "Demucs model", Type: core.ParamEnum, Default: "htdemucs",
			Options: []core.Option{
				{Value: "htdemucs", Label: "htdemucs (balanced)"},
				{Value: "mdx_extra", Label: "mdx_extra (quality)"},
				{Value: "mdx_extra_q", Label: "mdx_extra_q (fast)"},
			},
			Advanced: true,
		},
		{Key: "format", Label: "Format", Type: core.ParamEnum, Default: "wav", Options: formatOptions()},
		{Key: "bitrate", Label: "Bitrate", Type: core.ParamEnum, Default: "192k", Options: bitrateOptions(), Advanced: true},
	}
}

type ExtractVocals struct {
	FFmpeg *ffmpeg.Client
	Demucs *demucs.Client
}

type ExtractInstrumental struct {
	FFmpeg *ffmpeg.Client
	Demucs *demucs.Client
}

func (e ExtractVocals) Spec() core.Spec { return extractVocalsSpec }
func (e ExtractVocals) Validate(ctx context.Context, req core.Request) error {
	return validateStem(extractVocalsSpec, req, e.FFmpeg, e.Demucs)
}
func (e ExtractVocals) Run(ctx context.Context, req core.Request, rep core.Reporter) (core.Result, error) {
	return runStem(ctx, req, rep, extractVocalsSpec, e.FFmpeg, e.Demucs, "vocals", centerVocalsFilter)
}

func (e ExtractInstrumental) Spec() core.Spec { return extractInstrumentalSpec }
func (e ExtractInstrumental) Validate(ctx context.Context, req core.Request) error {
	return validateStem(extractInstrumentalSpec, req, e.FFmpeg, e.Demucs)
}
func (e ExtractInstrumental) Run(ctx context.Context, req core.Request, rep core.Reporter) (core.Result, error) {
	return runStem(ctx, req, rep, extractInstrumentalSpec, e.FFmpeg, e.Demucs, "no_vocals", centerInstrumentalFilter)
}

func validateStem(spec core.Spec, req core.Request, ff *ffmpeg.Client, dm *demucs.Client) error {
	if err := checkAudioInputs(spec, req, ff); err != nil {
		return err
	}
	if _, _, ok := media.AudioEncoder(req.Params.String("format")); !ok {
		return core.NewError(core.CodeUsage, spec.ID, "",
			fmt.Sprintf("unsupported output format %q", req.Params.String("format")), "choose a supported audio format")
	}
	if req.Params.String("method") == "demucs" {
		if dm == nil {
			return core.NewError(core.CodeEngineMissing, spec.ID, "",
				"the Demucs method needs the demucs engine", demucs.InstallHint())
		}
		if err := dm.Check(); err != nil {
			return core.Wrap(core.CodeEngineMissing, spec.ID, "", err, demucs.InstallHint())
		}
	}
	return nil
}

func runStem(
	ctx context.Context,
	req core.Request,
	rep core.Reporter,
	spec core.Spec,
	ff *ffmpeg.Client,
	dm *demucs.Client,
	stemName, centerFilter string,
) (core.Result, error) {
	start := time.Now()
	method := req.Params.String("method")
	if method == "" {
		method = "auto"
	}
	model := req.Params.String("model")
	if model == "" {
		model = "htdemucs"
	}
	format := req.Params.String("format")
	if format == "" {
		format = "wav"
	}
	bitrate := req.Params.String("bitrate")
	encoder, lossy, ok := media.AudioEncoder(format)
	if !ok {
		return core.Result{}, core.NewError(core.CodeUsage, spec.ID, "",
			fmt.Sprintf("unsupported output format %q", format), "choose a supported audio format")
	}

	var out []core.Artifact
	var bytesIn, bytesOut int64
	usedDemucs := false
	for _, item := range req.Items {
		if err := ctx.Err(); err != nil {
			return core.Result{}, err
		}
		art, demucsUsed, err := runStemOne(ctx, req, item, rep, spec, ff, dm, method, model, format, encoder, lossy, bitrate, stemName, centerFilter)
		if err != nil {
			return core.Result{}, err
		}
		out = append(out, art)
		usedDemucs = usedDemucs || demucsUsed
		bytesIn += item.Size
		bytesOut += art.Size
	}
	engine := engines.FFmpeg
	if usedDemucs {
		engine = engines.Demucs
	}
	return core.Result{
		Artifacts: out,
		Metrics: core.Metrics{
			BytesIn: bytesIn, BytesOut: bytesOut, Duration: time.Since(start), Engine: engine,
		},
	}, nil
}

func runStemOne(
	ctx context.Context,
	req core.Request,
	item core.Artifact,
	rep core.Reporter,
	spec core.Spec,
	ff *ffmpeg.Client,
	dm *demucs.Client,
	method, model, format, encoder string,
	lossy bool,
	bitrate, stemName, centerFilter string,
) (core.Artifact, bool, error) {
	m, err := media.Probe(ctx, ff, spec.ID, item, rep)
	if err != nil {
		return core.Artifact{}, false, err
	}
	if !m.HasAudio {
		return core.Artifact{}, false, core.NewError(core.CodeInput, spec.ID, item.Path,
			"no audio stream found", "choose an audio file")
	}

	useDemucs := false
	switch method {
	case "demucs":
		if dm == nil {
			return core.Artifact{}, false, core.NewError(core.CodeEngineMissing, spec.ID, "",
				"the Demucs method needs the demucs engine", demucs.InstallHint())
		}
		if err := dm.Check(); err != nil {
			return core.Artifact{}, false, core.Wrap(core.CodeEngineMissing, spec.ID, "", err, demucs.InstallHint())
		}
		useDemucs = true
	case "center":
		useDemucs = false
	default:
		useDemucs = dm != nil && dm.Check() == nil
	}

	source := item.Path
	tempDir := ""
	if useDemucs {
		tempDir, err = os.MkdirTemp(req.Target.Dir, ".stems-")
		if err != nil {
			return core.Artifact{}, false, core.Wrap(core.CodeOutput, spec.ID, req.Target.Dir, err,
				"check permissions on the output directory")
		}
		defer os.RemoveAll(tempDir)
		rep.Stage("separating stems")
		if err := dm.Separate(ctx, item.Path, model, tempDir); err != nil {
			if ctx.Err() != nil {
				return core.Artifact{}, false, ctx.Err()
			}
			return core.Artifact{}, false, core.Wrap(core.CodeEngineFailed, spec.ID, item.Path, err,
				"check that Demucs can run on this file")
		}
		source, err = findStem(tempDir, stemName)
		if err != nil {
			return core.Artifact{}, false, core.Wrap(core.CodeEngineFailed, spec.ID, item.Path, err, "")
		}
	}

	output := media.OutputPath(req, item, format)
	args := []string{"-i", source, "-vn", "-map_metadata", "0"}
	if !useDemucs {
		args = append(args, "-filter:a", centerFilter)
	}
	args = append(args, "-c:a", encoder)
	if lossy {
		args = qualityArgs(args, encoder, bitrate)
	}
	args = append(args, output)

	rep.Stage("encoding stem")
	if err := ff.Run(ctx, args, m.Duration, rep); err != nil {
		if ctx.Err() != nil {
			return core.Artifact{}, useDemucs, ctx.Err()
		}
		return core.Artifact{}, useDemucs, core.Wrap(core.CodeEngineFailed, spec.ID, item.Path, err, core.HintOf(err))
	}
	engineName := "ffmpeg-center"
	if useDemucs {
		engineName = "demucs"
	}
	art, err := media.Finish(req, item, output, core.Audio, format, m, m.Duration, map[string]any{
		"method": method,
		"engine": engineName,
		"model":  model,
		"stem":   stemName,
	})
	if err != nil {
		return core.Artifact{}, useDemucs, err
	}
	return art, useDemucs, nil
}

func findStem(root, stem string) (string, error) {
	var found string
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return nil
		}
		name := strings.ToLower(entry.Name())
		if name == strings.ToLower(stem)+".wav" || name == strings.ToLower(stem)+".flac" {
			found = path
			return filepath.SkipAll
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	if found == "" {
		return "", fmt.Errorf("demucs finished but produced no %s stem", stem)
	}
	return found, nil
}
