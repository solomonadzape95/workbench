package image

import (
	"context"
	"fmt"
	"image"
	"image/draw"
	"time"

	"github.com/solomonadzape95/workbench/internal/core"
	"github.com/solomonadzape95/workbench/internal/engines"
)

var cropSpec = core.Spec{
	ID:      "image.crop",
	Title:   "Crop image",
	Summary: "Cut a rectangle out of an image",
	Domain:  core.Image,
	Accepts: []core.Accept{{Kind: core.Image, Formats: []string{"png", "jpg", "jpeg", "gif"}}},
	Params: []core.ParamSpec{
		{Key: "width", Label: "Width", Type: core.ParamInt, Default: 0, Min: floatPtr(0), Help: "pixels; 0 = full width"},
		{Key: "height", Label: "Height", Type: core.ParamInt, Default: 0, Min: floatPtr(0), Help: "pixels; 0 = full height"},
		{
			Key: "gravity", Label: "Anchor", Type: core.ParamEnum, Default: "center",
			Options: []core.Option{
				{Value: "center", Label: "Center"},
				{Value: "top-left", Label: "Top left"},
				{Value: "top", Label: "Top"},
				{Value: "top-right", Label: "Top right"},
				{Value: "left", Label: "Left"},
				{Value: "right", Label: "Right"},
				{Value: "bottom-left", Label: "Bottom left"},
				{Value: "bottom", Label: "Bottom"},
				{Value: "bottom-right", Label: "Bottom right"},
			},
		},
		{Key: "x", Label: "X offset", Type: core.ParamInt, Default: 0, Advanced: true},
		{Key: "y", Label: "Y offset", Type: core.ParamInt, Default: 0, Advanced: true},
	},
	Engines:   []core.EngineID{engines.Internal},
	Batchable: true,
	Cost:      core.CostFast,
}

type Crop struct{}

func (Crop) Spec() core.Spec { return cropSpec }

func (Crop) Validate(ctx context.Context, req core.Request) error {
	if err := core.Validate(cropSpec.Params, req.Params); err != nil {
		return err
	}
	if len(req.Items) == 0 {
		return core.NewError(core.CodeUsage, cropSpec.ID, "", "no input files", "")
	}
	for _, item := range req.Items {
		if !cropSpec.AcceptsArtifact(item) {
			return core.NewError(core.CodeInput, cropSpec.ID, item.Path,
				fmt.Sprintf("%s is not a supported image format", item.Format),
				"supported inputs: PNG, JPEG, GIF")
		}
	}
	return nil
}

func (Crop) Run(ctx context.Context, req core.Request, rep core.Reporter) (core.Result, error) {
	start := time.Now()
	width, height := req.Params.Int("width"), req.Params.Int("height")
	gravity := req.Params.String("gravity")
	ox, oy := req.Params.Int("x"), req.Params.Int("y")

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
			return core.Result{}, core.Wrap(core.CodeInput, cropSpec.ID, item.Path, err,
				"the file may be corrupt or use an unsupported image encoding")
		}

		rep.Stage("cropping")
		cropped, err := cropImage(img, width, height, gravity, ox, oy)
		if err != nil {
			return core.Result{}, err
		}

		rep.Stage("encoding")
		art, err := writeOutput(req, item, cropped, item.Format, 90)
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

func cropImage(img image.Image, width, height int, gravity string, offsetX, offsetY int) (image.Image, error) {
	bounds := img.Bounds()
	srcW, srcH := bounds.Dx(), bounds.Dy()

	cropW, cropH := width, height
	if cropW == 0 {
		cropW = srcW
	}
	if cropH == 0 {
		cropH = srcH
	}
	if cropW > srcW || cropH > srcH {
		return nil, core.NewError(core.CodeInput, cropSpec.ID, "",
			fmt.Sprintf("crop %dx%d is larger than the image (%dx%d)", cropW, cropH, srcW, srcH),
			"reduce the crop dimensions or set them to 0 for full size")
	}

	x, y := gravityOrigin(gravity, srcW, srcH, cropW, cropH)
	x += offsetX
	y += offsetY
	if x < 0 || y < 0 || x+cropW > srcW || y+cropH > srcH {
		return nil, core.NewError(core.CodeInput, cropSpec.ID, "",
			fmt.Sprintf("crop rectangle %dx%d at (%d,%d) falls outside the image", cropW, cropH, x, y),
			"adjust the anchor or the offsets")
	}

	dst := image.NewRGBA(image.Rect(0, 0, cropW, cropH))
	draw.Draw(dst, dst.Bounds(), img, image.Pt(x, y), draw.Src)
	return dst, nil
}

func gravityOrigin(gravity string, srcW, srcH, cropW, cropH int) (int, int) {
	centerX := (srcW - cropW) / 2
	centerY := (srcH - cropH) / 2
	maxX := srcW - cropW
	maxY := srcH - cropH

	switch gravity {
	case "top-left":
		return 0, 0
	case "top":
		return centerX, 0
	case "top-right":
		return maxX, 0
	case "left":
		return 0, centerY
	case "right":
		return maxX, centerY
	case "bottom-left":
		return 0, maxY
	case "bottom":
		return centerX, maxY
	case "bottom-right":
		return maxX, maxY
	default:
		return centerX, centerY
	}
}
