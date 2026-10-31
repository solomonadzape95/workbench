// Package image implements image operations backed by the Go standard library.
package image

import (
	"context"
	"fmt"
	"image"
	"image/gif"
	"image/jpeg"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/solomonadzape95/workbench/internal/core"
	"github.com/solomonadzape95/workbench/internal/engines"
)

var spec = core.Spec{
	ID:      "image.convert",
	Title:   "Convert image",
	Summary: "Re-encode an image as PNG, JPEG, or GIF",
	Domain:  core.Image,
	Accepts: []core.Accept{{Kind: core.Image, Formats: []string{"png", "jpg", "jpeg", "gif"}}},
	Params: []core.ParamSpec{
		{
			Key: "format", Label: "Format", Type: core.ParamEnum, Required: true, Default: "png",
			Options: []core.Option{{Value: "png", Label: "PNG"}, {Value: "jpeg", Label: "JPEG"}, {Value: "gif", Label: "GIF"}},
			Help:    "JPEG has no transparency; GIF writes the first frame only.",
		},
		{
			Key: "quality", Label: "JPEG quality", Type: core.ParamInt, Default: 85,
			Min: floatPtr(1), Max: floatPtr(100), Advanced: true,
			Help: "1–100, only used for JPEG output",
		},
	},
	Output:    core.OutputSpec{FormatParam: "format"},
	Engines:   []core.EngineID{engines.Internal},
	Batchable: true,
	Cost:      core.CostMedium,
}

type Convert struct{}

func (Convert) Spec() core.Spec { return spec }

func (Convert) Validate(ctx context.Context, req core.Request) error {
	if err := core.Validate(spec.Params, req.Params); err != nil {
		return err
	}
	if len(req.Items) == 0 {
		return core.NewError(core.CodeUsage, spec.ID, "", "no input files", "")
	}
	switch strings.ToLower(req.Params.String("format")) {
	case "png", "jpeg", "jpg", "gif":
	default:
		return core.NewError(core.CodeUsage, spec.ID, "",
			fmt.Sprintf("unsupported output format %q", req.Params.String("format")), "choose png, jpeg, or gif")
	}
	for _, item := range req.Items {
		if !spec.AcceptsArtifact(item) {
			return core.NewError(core.CodeInput, spec.ID, item.Path,
				fmt.Sprintf("%s is not a supported image format", item.Format),
				"supported inputs: PNG, JPEG, GIF")
		}
	}
	return nil
}

func (Convert) Run(ctx context.Context, req core.Request, rep core.Reporter) (core.Result, error) {
	start := time.Now()
	format := strings.ToLower(req.Params.String("format"))
	quality := req.Params.Int("quality")
	if quality == 0 {
		quality = 85
	}

	var out []core.Artifact
	var bytesIn, bytesOut int64

	for _, item := range req.Items {
		if err := ctx.Err(); err != nil {
			return core.Result{}, err
		}

		rep.Stage("decoding")
		rep.Progress(0, 1)
		img, _, err := decode(item.Path)
		if err != nil {
			return core.Result{}, core.Wrap(core.CodeInput, spec.ID, item.Path, err,
				"the file may be corrupt or use an unsupported image encoding")
		}

		if err := ctx.Err(); err != nil {
			return core.Result{}, err
		}

		ext := core.ExtFor(format, item)
		outPath := filepath.Join(req.Target.Dir, core.SanitizeStem(item.Stem())+"."+ext)

		rep.Stage("encoding")
		if err := encodeFile(outPath, img, format, quality); err != nil {
			return core.Result{}, core.Wrap(core.CodeOutput, spec.ID, item.Path, err,
				"check that the output directory is writable")
		}

		info, err := os.Stat(outPath)
		if err != nil {
			return core.Result{}, core.Wrap(core.CodeOutput, spec.ID, outPath, err, "")
		}
		bounds := img.Bounds()
		out = append(out, core.Artifact{
			Path:   outPath,
			Kind:   core.Image,
			Format: format,
			Size:   info.Size(),
			Meta: map[string]any{
				"width":  bounds.Dx(),
				"height": bounds.Dy(),
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
			Engine:   engines.Internal,
		},
	}, nil
}

func decode(path string) (image.Image, string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, "", err
	}
	defer f.Close()
	return image.Decode(f)
}

func encodeFile(path string, img image.Image, format string, quality int) (err error) {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer func() {
		closeErr := f.Close()
		if err == nil {
			err = closeErr
		}
		if err != nil {
			_ = os.Remove(path)
		}
	}()
	return encode(f, img, format, quality)
}

func encode(w io.Writer, img image.Image, format string, quality int) error {
	switch format {
	case "png":
		return png.Encode(w, img)
	case "jpeg", "jpg":
		return jpeg.Encode(w, img, &jpeg.Options{Quality: quality})
	case "gif":
		return gif.Encode(w, img, nil)
	default:
		return fmt.Errorf("unsupported output format %q", format)
	}
}

func floatPtr(v float64) *float64 { return &v }
