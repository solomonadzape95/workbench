package image

import (
	"context"
	"fmt"
	"image"
	"image/color"
	"math"
	"time"

	"github.com/solomonadzape95/workbench/internal/core"
	"github.com/solomonadzape95/workbench/internal/engines"
)

var enhanceSpec = core.Spec{
	ID:      "image.enhance",
	Title:   "Enhance image",
	Summary: "Adjust contrast and brightness and optionally sharpen",
	Domain:  core.Image,
	Accepts: []core.Accept{{Kind: core.Image, Formats: []string{"png", "jpg", "jpeg", "gif"}}},
	Params: []core.ParamSpec{
		{
			Key: "contrast", Label: "Contrast", Type: core.ParamFloat, Default: 1.05,
			Min: floatPtr(0.5), Max: floatPtr(2.0),
			Help: "1.0 leaves contrast unchanged; values above 1 increase it.",
		},
		{
			Key: "brightness", Label: "Brightness", Type: core.ParamInt, Default: 0,
			Min: floatPtr(-100), Max: floatPtr(100),
			Help: "-100 to 100; 0 leaves brightness unchanged.",
		},
		{
			Key: "sharpen", Label: "Sharpen", Type: core.ParamBool, Default: true,
			Help: "Apply a light 3×3 unsharp convolution.",
		},
	},
	Engines:   []core.EngineID{engines.Internal},
	Batchable: true,
	Cost:      core.CostMedium,
}

type Enhance struct{}

func (Enhance) Spec() core.Spec { return enhanceSpec }

func (Enhance) Validate(ctx context.Context, req core.Request) error {
	if err := core.Validate(enhanceSpec.Params, req.Params); err != nil {
		return err
	}
	if len(req.Items) == 0 {
		return core.NewError(core.CodeUsage, enhanceSpec.ID, "", "no input files", "")
	}
	for _, item := range req.Items {
		if !enhanceSpec.AcceptsArtifact(item) {
			return core.NewError(core.CodeInput, enhanceSpec.ID, item.Path,
				fmt.Sprintf("%s is not a supported image format", item.Format),
				"supported inputs: PNG, JPEG, GIF")
		}
	}
	return nil
}

func (Enhance) Run(ctx context.Context, req core.Request, rep core.Reporter) (core.Result, error) {
	start := time.Now()
	contrast := req.Params.Float("contrast")
	if contrast == 0 {
		contrast = 1.05
	}
	brightness := req.Params.Int("brightness")
	sharpen := true
	if raw, ok := req.Params["sharpen"]; ok && raw != nil {
		sharpen = req.Params.Bool("sharpen")
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
			return core.Result{}, core.Wrap(core.CodeInput, enhanceSpec.ID, item.Path, err,
				"the file may be corrupt or use an unsupported image encoding")
		}

		rep.Stage("enhancing")
		enhanced := enhanceImage(img, contrast, brightness, sharpen)

		rep.Stage("encoding")
		format := normalizeFormat(item.Format)
		if !supportedFormat(format) {
			format = "png"
		}
		art, err := writeOutput(req, item, enhanced, format, 92)
		if err != nil {
			return core.Result{}, err
		}
		art.Meta["contrast"] = contrast
		art.Meta["brightness"] = brightness
		art.Meta["sharpen"] = sharpen
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

// enhanceImage applies a contrast/brightness lookup table and then an optional
// 3x3 sharpen kernel. Alpha is preserved untouched.
func enhanceImage(img image.Image, contrast float64, brightness int, sharpen bool) *image.NRGBA {
	bounds := img.Bounds()
	dst := image.NewNRGBA(image.Rect(0, 0, bounds.Dx(), bounds.Dy()))

	offset := float64(brightness) * 2.55
	var lut [256]uint8
	for i := range lut {
		v := (float64(i)-128)*contrast + 128 + offset
		lut[i] = clampByte(v)
	}

	for y := 0; y < bounds.Dy(); y++ {
		for x := 0; x < bounds.Dx(); x++ {
			c := color.NRGBAModel.Convert(img.At(bounds.Min.X+x, bounds.Min.Y+y)).(color.NRGBA)
			dst.SetNRGBA(x, y, color.NRGBA{R: lut[c.R], G: lut[c.G], B: lut[c.B], A: c.A})
		}
	}

	if !sharpen {
		return dst
	}
	return sharpenRGB(dst)
}

// sharpenRGB convolves RGB with
//
//	 0 -1  0
//	-1  5 -1
//	 0 -1  0
//
// using edge clamping so the output stays deterministic and opaque edges do not
// darken.
func sharpenRGB(src *image.NRGBA) *image.NRGBA {
	bounds := src.Bounds()
	dst := image.NewNRGBA(bounds)

	at := func(x, y int) color.NRGBA {
		x = min(max(x, bounds.Min.X), bounds.Max.X-1)
		y = min(max(y, bounds.Min.Y), bounds.Max.Y-1)
		return src.NRGBAAt(x, y)
	}

	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			c := at(x, y)
			left := at(x-1, y)
			right := at(x+1, y)
			up := at(x, y-1)
			down := at(x, y+1)
			dst.SetNRGBA(x, y, color.NRGBA{
				R: clampByte(5*float64(c.R) - float64(left.R) - float64(right.R) - float64(up.R) - float64(down.R)),
				G: clampByte(5*float64(c.G) - float64(left.G) - float64(right.G) - float64(up.G) - float64(down.G)),
				B: clampByte(5*float64(c.B) - float64(left.B) - float64(right.B) - float64(up.B) - float64(down.B)),
				A: c.A,
			})
		}
	}
	return dst
}

func clampByte(v float64) uint8 {
	v = math.Round(v)
	if v < 0 {
		return 0
	}
	if v > 255 {
		return 255
	}
	return uint8(v)
}
