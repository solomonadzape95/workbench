package doc

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/solomonadzape95/workbench/internal/core"
	"github.com/solomonadzape95/workbench/internal/engines"
	"github.com/solomonadzape95/workbench/internal/engines/poppler"
	"github.com/solomonadzape95/workbench/internal/engines/tesseract"
)

var ocrSpec = core.Spec{
	ID:      "doc.ocr",
	Title:   "OCR document",
	Summary: "Extract text from a scanned image or PDF, or make a searchable PDF",
	Domain:  core.Document,
	Accepts: []core.Accept{
		{Kind: core.Document, Formats: []string{"pdf"}},
		{Kind: core.Image, Formats: []string{"png", "jpg", "jpeg", "gif", "tif", "tiff", "bmp", "webp"}},
	},
	Params: []core.ParamSpec{
		{Key: "format", Label: "Output", Type: core.ParamEnum, Default: "txt",
			Options: []core.Option{{Value: "txt", Label: "Text"}, {Value: "pdf", Label: "Searchable PDF"}}},
		{Key: "language", Label: "Language", Type: core.ParamString, Default: "eng",
			Help: "Tesseract language code, for example eng, deu, or fra."},
		{Key: "dpi", Label: "PDF DPI", Type: core.ParamInt, Default: 300, Min: floatPtr(72), Advanced: true},
	},
	Output:  core.OutputSpec{FormatParam: "format", Kind: core.Document},
	Engines: []core.EngineID{engines.Tesseract, engines.Poppler},
	Cost:    core.CostSlow,
}

type OCR struct {
	Tesseract *tesseract.Client
	Poppler   *poppler.Client
}

func (o OCR) Spec() core.Spec { return ocrSpec }

func (o OCR) Validate(ctx context.Context, req core.Request) error {
	if err := core.Validate(ocrSpec.Params, req.Params); err != nil {
		return err
	}
	if len(req.Items) == 0 {
		return core.NewError(core.CodeUsage, ocrSpec.ID, "", "no input images or PDFs", "")
	}
	if o.Tesseract == nil {
		return core.NewError(core.CodeEngineMissing, ocrSpec.ID, "", "tesseract is not installed", tesseract.InstallHint())
	}
	if err := o.Tesseract.Check(); err != nil {
		return err
	}
	format := req.Params.String("format")
	for _, item := range req.Items {
		if !ocrSpec.AcceptsArtifact(item) {
			return core.NewError(core.CodeInput, ocrSpec.ID, item.Path, "unsupported OCR input", "choose a PNG, JPEG, TIFF, or PDF")
		}
		if strings.EqualFold(item.Ext(), "pdf") {
			if format == "pdf" {
				return core.NewError(core.CodeUsage, ocrSpec.ID, item.Path,
					"PDF→searchable PDF is not supported yet", "choose text output, or OCR image pages")
			}
			if o.Poppler == nil {
				return core.NewError(core.CodeEngineMissing, ocrSpec.ID, item.Path,
					"poppler is needed to rasterize PDF pages", poppler.InstallHint())
			}
			if _, err := o.Poppler.ResolvePdfToPpm(); err != nil {
				return err
			}
		}
	}
	return nil
}

func (o OCR) Run(ctx context.Context, req core.Request, rep core.Reporter) (core.Result, error) {
	start := time.Now()
	format := req.Params.String("format")
	if format == "" {
		format = "txt"
	}
	language := req.Params.String("language")
	if language == "" {
		language = "eng"
	}
	dpi := req.Params.Int("dpi")
	if dpi <= 0 {
		dpi = 300
	}

	var out []core.Artifact
	var bytesIn, bytesOut int64
	for _, item := range req.Items {
		if err := ctx.Err(); err != nil {
			return core.Result{}, err
		}
		var artifact core.Artifact
		var err error
		if strings.EqualFold(item.Ext(), "pdf") {
			artifact, err = o.ocrPDF(ctx, req, item, language, dpi, rep)
		} else {
			artifact, err = o.ocrImage(ctx, req, item, format, language, rep)
		}
		if err != nil {
			return core.Result{}, err
		}
		out = append(out, artifact)
		bytesIn += item.Size
		bytesOut += artifact.Size
		rep.Progress(1, 1)
	}
	return core.Result{
		Artifacts: out,
		Metrics: core.Metrics{
			BytesIn: bytesIn, BytesOut: bytesOut, Duration: time.Since(start), Engine: engines.Tesseract,
		},
	}, nil
}

func (o OCR) ocrImage(ctx context.Context, req core.Request, item core.Artifact, format, language string, rep core.Reporter) (core.Artifact, error) {
	base := filepath.Join(req.Target.Dir, core.SanitizeStem(item.Stem()))
	output, err := o.Tesseract.OCR(ctx, item.Path, base, format, language, rep)
	if err != nil {
		if ctx.Err() != nil {
			return core.Artifact{}, ctx.Err()
		}
		return core.Artifact{}, core.Wrap(core.CodeEngineFailed, ocrSpec.ID, item.Path, err, core.HintOf(err))
	}
	return artifactFor(req, item, output, format, language, 1)
}

func (o OCR) ocrPDF(ctx context.Context, req core.Request, item core.Artifact, language string, dpi int, rep core.Reporter) (core.Artifact, error) {
	work, err := os.MkdirTemp(req.Target.Dir, ".ocr-")
	if err != nil {
		return core.Artifact{}, core.Wrap(core.CodeOutput, ocrSpec.ID, req.Target.Dir, err, "")
	}
	defer os.RemoveAll(work)
	pages, err := o.Poppler.RenderPages(ctx, item.Path, work, dpi, rep)
	if err != nil {
		return core.Artifact{}, core.Wrap(core.CodeEngineFailed, ocrSpec.ID, item.Path, err, core.HintOf(err))
	}
	var text strings.Builder
	for i, page := range pages {
		base := filepath.Join(work, fmt.Sprintf("page-%04d", i+1))
		output, err := o.Tesseract.OCR(ctx, page, base, "txt", language, rep)
		if err != nil {
			return core.Artifact{}, core.Wrap(core.CodeEngineFailed, ocrSpec.ID, page, err, core.HintOf(err))
		}
		data, err := os.ReadFile(output)
		if err != nil {
			return core.Artifact{}, core.Wrap(core.CodeOutput, ocrSpec.ID, output, err, "")
		}
		text.Write(data)
		if len(data) > 0 && data[len(data)-1] != '\n' {
			text.WriteByte('\n')
		}
	}
	outPath := filepath.Join(req.Target.Dir, core.SanitizeStem(item.Stem())+".txt")
	if err := os.WriteFile(outPath, []byte(text.String()), 0o644); err != nil {
		return core.Artifact{}, core.Wrap(core.CodeOutput, ocrSpec.ID, outPath, err, "")
	}
	artifact, err := artifactFor(req, item, outPath, "txt", language, len(pages))
	if err != nil {
		return core.Artifact{}, err
	}
	artifact.Meta["dpi"] = dpi
	return artifact, nil
}

func artifactFor(req core.Request, item core.Artifact, path, format, language string, pages int) (core.Artifact, error) {
	info, err := os.Stat(path)
	if err != nil {
		return core.Artifact{}, core.Wrap(core.CodeOutput, ocrSpec.ID, path, err,
			"the OCR engine reported success but produced no output")
	}
	return core.Artifact{
		Path: path, Kind: core.Document, Format: format, Size: info.Size(),
		SuggestedName: filepath.Base(path),
		Meta: map[string]any{
			"source": item.Path, "language": language, "pages": pages,
			"engine": string(engines.Tesseract),
		},
	}, nil
}

func floatPtr(v float64) *float64 { return &v }
