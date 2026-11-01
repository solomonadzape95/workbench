package image

import (
	"bytes"
	"context"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-pdf/fpdf"
	_ "golang.org/x/image/bmp"
	_ "golang.org/x/image/tiff"
	_ "golang.org/x/image/webp"

	"github.com/solomonadzape95/workbench/internal/core"
	"github.com/solomonadzape95/workbench/internal/engines"
)

const maxPDFImagePage = 1440.0 // points; keeps huge camera images sane

var toPDFSpec = core.Spec{
	ID:      "image.to_pdf",
	Title:   "Images to PDF",
	Summary: "Combine one or more images into one PDF, one image per page",
	Domain:  core.Image,
	Accepts: []core.Accept{{Kind: core.Image, Formats: []string{
		"png", "jpg", "jpeg", "gif", "webp", "tif", "tiff", "bmp",
	}}},
	Params: []core.ParamSpec{
		{
			Key: "page_size", Label: "Page size", Type: core.ParamEnum, Default: "a4",
			Options: []core.Option{
				{Value: "a4", Label: "A4"},
				{Value: "letter", Label: "Letter"},
				{Value: "image", Label: "Match each image"},
			},
			Help: "A4 and Letter fit each image inside a page; Match each image makes one page per image size.",
		},
		{
			Key: "orientation", Label: "Orientation", Type: core.ParamEnum, Default: "auto",
			Options: []core.Option{
				{Value: "auto", Label: "Auto"},
				{Value: "portrait", Label: "Portrait"},
				{Value: "landscape", Label: "Landscape"},
			},
			Help: "Auto picks portrait or landscape from the image. Ignored when page size matches the image.",
		},
		{
			Key: "margin", Label: "Margin", Type: core.ParamInt, Default: 18, Min: floatPtr(0),
			Help: "Points of white space around each image on A4 or Letter pages.",
		},
	},
	Output:  core.OutputSpec{StaticFormat: "pdf", Kind: core.Document},
	Engines: []core.EngineID{engines.Internal},
	Mode:    core.ModeAggregate,
	Cost:    core.CostMedium,
}

type ToPDF struct{}

func (ToPDF) Spec() core.Spec { return toPDFSpec }

func (ToPDF) Validate(ctx context.Context, req core.Request) error {
	if err := core.Validate(toPDFSpec.Params, req.Params); err != nil {
		return err
	}
	if len(req.Items) == 0 {
		return core.NewError(core.CodeUsage, toPDFSpec.ID, "", "no images", "")
	}
	for _, item := range req.Items {
		if !toPDFSpec.AcceptsArtifact(item) {
			return core.NewError(core.CodeInput, toPDFSpec.ID, item.Path,
				fmt.Sprintf("%s is not a supported image format", item.Format),
				"supported inputs: PNG, JPEG, GIF, WebP, TIFF, and BMP")
		}
	}
	return nil
}

func (ToPDF) Run(ctx context.Context, req core.Request, rep core.Reporter) (core.Result, error) {
	start := time.Now()
	pageSize := req.Params.String("page_size")
	if pageSize == "" {
		pageSize = "a4"
	}
	orientation := req.Params.String("orientation")
	if orientation == "" {
		orientation = "auto"
	}
	margin := req.Params.Int("margin")
	if margin < 0 {
		margin = 18
	}

	pdf := fpdf.New("P", "pt", "A4", "")
	pdf.SetMargins(0, 0, 0)
	pdf.SetAutoPageBreak(false, 0)
	rep.Stage("building PDF")
	rep.Progress(0, len(req.Items))

	var bytesIn int64
	for i, item := range req.Items {
		if err := ctx.Err(); err != nil {
			return core.Result{}, err
		}
		info, err := registerPDFImage(pdf, item, i)
		if err != nil {
			return core.Result{}, core.Wrap(core.CodeInput, toPDFSpec.ID, item.Path, err,
				"the image may be corrupt or use an unsupported encoding")
		}
		imageW, imageH := info.Width(), info.Height()
		if imageW <= 0 || imageH <= 0 {
			return core.Result{}, core.NewError(core.CodeInput, toPDFSpec.ID, item.Path,
				"image has no usable dimensions", "")
		}

		pageW, pageH := pdfPageSize(pageSize, orientation, imageW, imageH)
		pdf.AddPageFormat(pageOrientation(pageW, pageH), fpdf.SizeType{Wd: pageW, Ht: pageH})
		drawW, drawH := imageW, imageH
		if pageSize != "image" {
			availW := pageW - float64(margin)*2
			availH := pageH - float64(margin)*2
			scale := minFloat(availW/imageW, availH/imageH)
			drawW, drawH = imageW*scale, imageH*scale
		}
		x := (pageW - drawW) / 2
		y := (pageH - drawH) / 2
		imageName := fmt.Sprintf("workbench-image-%d", i)
		pdf.ImageOptions(imageName, x, y, drawW, drawH, false, fpdf.ImageOptions{ImageType: pdfImageType(item)}, 0, "")
		if err := pdf.Error(); err != nil {
			return core.Result{}, core.Wrap(core.CodeEngineFailed, toPDFSpec.ID, item.Path, err,
				"the image could not be embedded in the PDF")
		}
		bytesIn += item.Size
		rep.Progress(i+1, len(req.Items))
	}

	if err := os.MkdirAll(req.Target.Dir, 0o755); err != nil {
		return core.Result{}, core.Wrap(core.CodeOutput, toPDFSpec.ID, req.Target.Dir, err,
			"check permissions on the output directory")
	}
	name := "images.pdf"
	if len(req.Items) == 1 {
		name = core.SanitizeStem(req.Items[0].Stem()) + ".pdf"
	}
	output := filepath.Join(req.Target.Dir, name)
	if err := pdf.OutputFileAndClose(output); err != nil {
		_ = os.Remove(output)
		return core.Result{}, core.Wrap(core.CodeOutput, toPDFSpec.ID, output, err,
			"check permissions on the output directory")
	}
	info, err := os.Stat(output)
	if err != nil {
		return core.Result{}, core.Wrap(core.CodeOutput, toPDFSpec.ID, output, err, "")
	}
	return core.Result{
		Artifacts: []core.Artifact{{
			Path: output, Kind: core.Document, Format: "pdf", Size: info.Size(),
			SuggestedName: name,
			Meta: map[string]any{
				"images":    len(req.Items),
				"pages":     len(req.Items),
				"page_size": pageSize,
				"source":    req.Items[0].Path,
			},
		}},
		Metrics: core.Metrics{
			BytesIn: bytesIn, BytesOut: info.Size(), Duration: time.Since(start), Engine: engines.Internal,
		},
	}, nil
}

// registerPDFImage embeds one image in the PDF and returns its native size in
// points. JPEG, PNG, and GIF are passed through; formats fpdf cannot parse are
// decoded and re-encoded as lossless PNG.
func registerPDFImage(pdf *fpdf.Fpdf, item core.Artifact, index int) (*fpdf.ImageInfoType, error) {
	name := fmt.Sprintf("workbench-image-%d", index)
	ext := strings.ToLower(item.Ext())
	switch ext {
	case "jpg", "jpeg", "png", "gif":
		f, err := os.Open(item.Path)
		if err != nil {
			return nil, err
		}
		info := pdf.RegisterImageOptionsReader(name, fpdf.ImageOptions{ImageType: strings.ToUpper(ext)}, f)
		closeErr := f.Close()
		if err := pdf.Error(); err != nil {
			return nil, err
		}
		if closeErr != nil {
			return nil, closeErr
		}
		return info, nil
	default:
		f, err := os.Open(item.Path)
		if err != nil {
			return nil, err
		}
		img, _, decodeErr := image.Decode(f)
		closeErr := f.Close()
		if decodeErr != nil {
			return nil, decodeErr
		}
		if closeErr != nil {
			return nil, closeErr
		}
		var encoded bytes.Buffer
		if err := png.Encode(&encoded, img); err != nil {
			return nil, err
		}
		info := pdf.RegisterImageOptionsReader(name, fpdf.ImageOptions{ImageType: "PNG"}, bytes.NewReader(encoded.Bytes()))
		if err := pdf.Error(); err != nil {
			return nil, err
		}
		return info, nil
	}
}

func pdfPageSize(pageSize, orientation string, imageW, imageH float64) (float64, float64) {
	if pageSize == "image" {
		factor := minFloat(1, maxPDFImagePage/maxFloat(imageW, imageH))
		return imageW * factor, imageH * factor
	}
	pageW, pageH := 595.28, 841.89 // A4
	if pageSize == "letter" {
		pageW, pageH = 612, 792
	}
	landscape := orientation == "landscape" || (orientation == "auto" && imageW > imageH)
	if landscape {
		return pageH, pageW
	}
	return pageW, pageH
}

func pageOrientation(pageW, pageH float64) string {
	if pageW > pageH {
		return "L"
	}
	return "P"
}

func pdfImageType(item core.Artifact) string {
	switch strings.ToLower(item.Ext()) {
	case "jpg", "jpeg":
		return "JPG"
	case "png":
		return "PNG"
	case "gif":
		return "GIF"
	default:
		return "PNG"
	}
}

func minFloat(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}

func maxFloat(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}
