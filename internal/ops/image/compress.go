package image

import (
	"context"
	"fmt"
	"image"
	"image/gif"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"time"

	"github.com/solomonadzape95/workbench/internal/core"
	"github.com/solomonadzape95/workbench/internal/engines"
)

var compressSpec = core.Spec{
	ID:      "image.compress",
	Title:   "Compress image",
	Summary: "Re-encode at a lower quality to reduce file size",
	Domain:  core.Image,
	Accepts: []core.Accept{{Kind: core.Image, Formats: []string{"png", "jpg", "jpeg", "gif"}}},
	Params: []core.ParamSpec{
		{
			Key: "quality", Label: "JPEG quality", Type: core.ParamInt, Default: 75,
			Min: floatPtr(1), Max: floatPtr(100),
			Help: "Lower is smaller. PNG uses maximum compression instead.",
		},
	},
	Engines:   []core.EngineID{engines.Internal},
	Batchable: true,
	Cost:      core.CostMedium,
}

type Compress struct{}

func (Compress) Spec() core.Spec { return compressSpec }

func (Compress) Validate(ctx context.Context, req core.Request) error {
	if err := core.Validate(compressSpec.Params, req.Params); err != nil {
		return err
	}
	if len(req.Items) == 0 {
		return core.NewError(core.CodeUsage, compressSpec.ID, "", "no input files", "")
	}
	for _, item := range req.Items {
		if !compressSpec.AcceptsArtifact(item) {
			return core.NewError(core.CodeInput, compressSpec.ID, item.Path,
				fmt.Sprintf("%s is not a supported image format", item.Format),
				"supported inputs: PNG, JPEG, GIF")
		}
	}
	return nil
}

func (Compress) Run(ctx context.Context, req core.Request, rep core.Reporter) (core.Result, error) {
	start := time.Now()
	quality := req.Params.Int("quality")
	if quality == 0 {
		quality = 75
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
			return core.Result{}, core.Wrap(core.CodeInput, compressSpec.ID, item.Path, err,
				"the file may be corrupt or use an unsupported image encoding")
		}

		format := normalizeFormat(item.Format)
		if !supportedFormat(format) {
			return core.Result{}, core.NewError(core.CodeInput, compressSpec.ID, item.Path,
				fmt.Sprintf("unsupported format %q", item.Format), "supported inputs: PNG, JPEG, GIF")
		}

		rep.Stage("encoding")
		path := filepath.Join(req.Target.Dir, core.SanitizeStem(item.Stem())+"."+core.ExtFor(format, item))
		if err := encodeCompressed(path, img, format, quality); err != nil {
			return core.Result{}, core.Wrap(core.CodeOutput, compressSpec.ID, item.Path, err,
				"check that the output directory is writable")
		}
		info, err := os.Stat(path)
		if err != nil {
			return core.Result{}, core.Wrap(core.CodeOutput, compressSpec.ID, path, err, "")
		}

		bounds := img.Bounds()
		out = append(out, core.Artifact{
			Path:   path,
			Kind:   core.Image,
			Format: format,
			Size:   info.Size(),
			Meta: map[string]any{
				"width":  bounds.Dx(),
				"height": bounds.Dy(),
				"source": item.Path,
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

func encodeCompressed(path string, img image.Image, format string, quality int) (err error) {
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

	switch format {
	case "png":
		enc := png.Encoder{CompressionLevel: png.BestCompression}
		return enc.Encode(f, img)
	case "jpeg":
		return jpeg.Encode(f, img, &jpeg.Options{Quality: quality})
	case "gif":
		return gif.Encode(f, img, nil)
	default:
		return fmt.Errorf("unsupported output format %q", format)
	}
}
