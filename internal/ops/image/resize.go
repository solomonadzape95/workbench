package image

import (
	"context"
	"fmt"
	"image"
	"math"
	"time"

	xdraw "golang.org/x/image/draw"

	"github.com/solomonadzape95/workbench/internal/core"
	"github.com/solomonadzape95/workbench/internal/engines"
)

var resizeSpec = core.Spec{
	ID:      "image.resize",
	Title:   "Resize image",
	Summary: "Scale or fit an image to new dimensions",
	Domain:  core.Image,
	Accepts: []core.Accept{{Kind: core.Image, Formats: []string{"png", "jpg", "jpeg", "gif"}}},
	Params: []core.ParamSpec{
		{Key: "width", Label: "Width", Type: core.ParamInt, Default: 0, Min: floatPtr(0), Help: "pixels; 0 = derive from height"},
		{Key: "height", Label: "Height", Type: core.ParamInt, Default: 0, Min: floatPtr(0), Help: "pixels; 0 = derive from width"},
		{
			Key: "mode", Label: "Mode", Type: core.ParamEnum, Default: "fit",
			Options: []core.Option{
				{Value: "fit", Label: "Fit inside"},
				{Value: "fill", Label: "Fill and crop"},
				{Value: "exact", Label: "Stretch to exact"},
			},
			Help: "Fit preserves the aspect ratio; fill crops the overflow; exact stretches.",
		},
		{
			Key: "filter", Label: "Filter", Type: core.ParamEnum, Default: "smooth",
			Options:  []core.Option{{Value: "smooth", Label: "Smooth"}, {Value: "nearest", Label: "Nearest"}},
			Advanced: true,
		},
	},
	Engines:   []core.EngineID{engines.Internal},
	Batchable: true,
	Cost:      core.CostMedium,
}

type Resize struct{}

func (Resize) Spec() core.Spec { return resizeSpec }

func (Resize) Validate(ctx context.Context, req core.Request) error {
	if err := core.Validate(resizeSpec.Params, req.Params); err != nil {
		return err
	}
	if len(req.Items) == 0 {
		return core.NewError(core.CodeUsage, resizeSpec.ID, "", "no input files", "")
	}
	width, height := req.Params.Int("width"), req.Params.Int("height")
	if width == 0 && height == 0 {
		return core.NewError(core.CodeUsage, resizeSpec.ID, "", "set a width or a height", "")
	}
	mode := req.Params.String("mode")
	if (mode == "exact" || mode == "fill") && (width == 0 || height == 0) {
		return core.NewError(core.CodeUsage, resizeSpec.ID, "",
			fmt.Sprintf("%s mode needs both width and height", mode), "")
	}
	for _, dim := range []int{width, height} {
		if dim > 0 {
			if err := checkTarget(dim); err != nil {
				return err
			}
		}
	}
	for _, item := range req.Items {
		if !resizeSpec.AcceptsArtifact(item) {
			return core.NewError(core.CodeInput, resizeSpec.ID, item.Path,
				fmt.Sprintf("%s is not a supported image format", item.Format),
				"supported inputs: PNG, JPEG, GIF")
		}
	}
	return nil
}

func (Resize) Run(ctx context.Context, req core.Request, rep core.Reporter) (core.Result, error) {
	start := time.Now()
	width, height := req.Params.Int("width"), req.Params.Int("height")
	mode := req.Params.String("mode")
	filter := req.Params.String("filter")

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
			return core.Result{}, core.Wrap(core.CodeInput, resizeSpec.ID, item.Path, err,
				"the file may be corrupt or use an unsupported image encoding")
		}

		rep.Stage("resizing")
		target, err := resizeImage(img, width, height, mode, filter)
		if err != nil {
			return core.Result{}, err
		}

		rep.Stage("encoding")
		art, err := writeOutput(req, item, target, item.Format, 90)
		if err != nil {
			return core.Result{}, err
		}
		out = append(out, art)
		bytesIn += item.Size
		bytesOut += art.Size
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

func resizeImage(img image.Image, width, height int, mode, filter string) (image.Image, error) {
	bounds := img.Bounds()
	srcW, srcH := bounds.Dx(), bounds.Dy()

	scaler := xdraw.Scaler(xdraw.CatmullRom)
	if filter == "nearest" {
		scaler = xdraw.NearestNeighbor
	}

	switch mode {
	case "exact":
		if err := checkTarget(width); err != nil {
			return nil, err
		}
		if err := checkTarget(height); err != nil {
			return nil, err
		}
		if err := checkPixels(width, height); err != nil {
			return nil, err
		}
		return scaleTo(img, width, height, scaler), nil

	case "fill":
		if err := checkPixels(width, height); err != nil {
			return nil, err
		}
		scale := math.Max(float64(width)/float64(srcW), float64(height)/float64(srcH))
		scaledW := max(int(math.Ceil(float64(srcW)*scale)), width)
		scaledH := max(int(math.Ceil(float64(srcH)*scale)), height)
		if err := checkTarget(scaledW); err != nil {
			return nil, err
		}
		if err := checkTarget(scaledH); err != nil {
			return nil, err
		}
		if err := checkPixels(scaledW, scaledH); err != nil {
			return nil, err
		}
		scaled := scaleTo(img, scaledW, scaledH, scaler)
		dst := image.NewRGBA(image.Rect(0, 0, width, height))
		offset := image.Pt((scaledW-width)/2, (scaledH-height)/2)
		xdraw.Draw(dst, dst.Bounds(), scaled, offset, xdraw.Src)
		return dst, nil

	default: // fit
		scale := 1.0
		switch {
		case width > 0 && height > 0:
			scale = math.Min(float64(width)/float64(srcW), float64(height)/float64(srcH))
		case width > 0:
			scale = float64(width) / float64(srcW)
		default:
			scale = float64(height) / float64(srcH)
		}
		targetW := max(1, int(math.Round(float64(srcW)*scale)))
		targetH := max(1, int(math.Round(float64(srcH)*scale)))
		if err := checkTarget(targetW); err != nil {
			return nil, err
		}
		if err := checkTarget(targetH); err != nil {
			return nil, err
		}
		if err := checkPixels(targetW, targetH); err != nil {
			return nil, err
		}
		return scaleTo(img, targetW, targetH, scaler), nil
	}
}

func scaleTo(src image.Image, width, height int, scaler xdraw.Scaler) *image.RGBA {
	dst := image.NewRGBA(image.Rect(0, 0, width, height))
	scaler.Scale(dst, dst.Bounds(), src, src.Bounds(), xdraw.Src, nil)
	return dst
}
