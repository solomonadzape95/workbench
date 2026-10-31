package image

import (
	"context"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"math"
	"time"

	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/math/fixed"

	"github.com/solomonadzape95/workbench/internal/core"
	"github.com/solomonadzape95/workbench/internal/engines"
)

var watermarkSpec = core.Spec{
	ID:      "image.watermark",
	Title:   "Watermark image",
	Summary: "Stamp text onto an image with adjustable position and opacity",
	Domain:  core.Image,
	Accepts: []core.Accept{{Kind: core.Image, Formats: []string{"png", "jpg", "jpeg", "gif"}}},
	Params: []core.ParamSpec{
		{Key: "text", Label: "Text", Type: core.ParamString, Required: true, Help: "Single line of text to draw."},
		{
			Key: "position", Label: "Position", Type: core.ParamEnum, Default: "bottom-right",
			Options: []core.Option{
				{Value: "top-left", Label: "Top left"},
				{Value: "top-right", Label: "Top right"},
				{Value: "bottom-left", Label: "Bottom left"},
				{Value: "bottom-right", Label: "Bottom right"},
				{Value: "center", Label: "Center"},
			},
		},
		{
			Key: "opacity", Label: "Opacity", Type: core.ParamFloat, Default: 0.6,
			Min: floatPtr(0.1), Max: floatPtr(1.0),
		},
		{
			Key: "margin", Label: "Margin", Type: core.ParamInt, Default: 12,
			Min:  floatPtr(0),
			Help: "Pixels between the text and the nearest edge.",
		},
	},
	Engines:   []core.EngineID{engines.Internal},
	Batchable: true,
	Cost:      core.CostFast,
}

type Watermark struct{}

func (Watermark) Spec() core.Spec { return watermarkSpec }

func (Watermark) Validate(ctx context.Context, req core.Request) error {
	if err := core.Validate(watermarkSpec.Params, req.Params); err != nil {
		return err
	}
	if len(req.Items) == 0 {
		return core.NewError(core.CodeUsage, watermarkSpec.ID, "", "no input files", "")
	}
	if len(req.Params.String("text")) > 200 {
		return core.NewError(core.CodeUsage, watermarkSpec.ID, "",
			"watermark text is too long", "use at most 200 characters")
	}
	for _, item := range req.Items {
		if !watermarkSpec.AcceptsArtifact(item) {
			return core.NewError(core.CodeInput, watermarkSpec.ID, item.Path,
				fmt.Sprintf("%s is not a supported image format", item.Format),
				"supported inputs: PNG, JPEG, GIF")
		}
	}
	return nil
}

func (Watermark) Run(ctx context.Context, req core.Request, rep core.Reporter) (core.Result, error) {
	start := time.Now()
	text := req.Params.String("text")
	position := req.Params.String("position")
	if position == "" {
		position = "bottom-right"
	}
	opacity := req.Params.Float("opacity")
	if opacity == 0 {
		opacity = 0.6
	}
	margin := 12
	if raw, ok := req.Params["margin"]; ok && raw != nil {
		margin = req.Params.Int("margin")
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
			return core.Result{}, core.Wrap(core.CodeInput, watermarkSpec.ID, item.Path, err,
				"the file may be corrupt or use an unsupported image encoding")
		}

		rep.Stage("drawing watermark")
		stamped := watermarkImage(img, text, position, opacity, margin)

		rep.Stage("encoding")
		format := normalizeFormat(item.Format)
		if !supportedFormat(format) {
			format = "png"
		}
		art, err := writeOutput(req, item, stamped, format, 92)
		if err != nil {
			return core.Result{}, err
		}
		art.Meta["watermark"] = text
		art.Meta["position"] = position
		art.Meta["opacity"] = opacity
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

// watermarkImage copies img and draws text with the basic bitmap face. A soft
// dark shadow keeps the white text readable over light backgrounds.
func watermarkImage(img image.Image, text, position string, opacity float64, margin int) *image.RGBA {
	bounds := img.Bounds()
	width, height := bounds.Dx(), bounds.Dy()
	dst := image.NewRGBA(image.Rect(0, 0, width, height))
	draw.Draw(dst, dst.Bounds(), img, bounds.Min, draw.Src)

	face := basicfont.Face7x13
	textWidth := font.MeasureString(face, text).Ceil()
	metrics := face.Metrics()
	ascent := metrics.Ascent.Ceil()
	descent := metrics.Descent.Ceil()
	textHeight := ascent + descent

	maxX := max(0, width-textWidth-margin)
	maxY := max(0, height-textHeight-margin)

	var x, y int
	switch position {
	case "top-left":
		x, y = margin, margin
	case "top-right":
		x, y = maxX, margin
	case "bottom-left":
		x, y = margin, maxY
	case "bottom-right":
		x, y = maxX, maxY
	default: // center
		x, y = (width-textWidth)/2, (height-textHeight)/2
	}
	x = min(max(x, 0), max(0, width-textWidth))
	y = min(max(y, 0), max(0, height-textHeight))

	alpha := uint8(math.Round(min(max(opacity, 0), 1) * 255))
	if alpha == 0 {
		return dst
	}

	shadowAlpha := uint8(float64(alpha) * 0.6)
	shadow := image.NewUniform(color.NRGBA{A: shadowAlpha})
	drawer := &font.Drawer{
		Dst:  dst,
		Src:  shadow,
		Face: face,
		Dot:  fixed.P(x+1, y+ascent+1),
	}
	drawer.DrawString(text)

	ink := image.NewUniform(color.NRGBA{R: 255, G: 255, B: 255, A: alpha})
	drawer.Src = ink
	drawer.Dot = fixed.P(x, y+ascent)
	drawer.DrawString(text)

	return dst
}
