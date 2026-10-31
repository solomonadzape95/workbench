package image

import (
	"context"
	"fmt"
	"image"
	"time"

	"github.com/solomonadzape95/workbench/internal/core"
	"github.com/solomonadzape95/workbench/internal/engines"
)

var rotateSpec = core.Spec{
	ID:      "image.rotate",
	Title:   "Rotate image",
	Summary: "Rotate an image in 90° steps and optionally flip it",
	Domain:  core.Image,
	Accepts: []core.Accept{{Kind: core.Image, Formats: []string{"png", "jpg", "jpeg", "gif"}}},
	Params: []core.ParamSpec{
		{
			Key: "angle", Label: "Angle", Type: core.ParamEnum, Default: "90",
			Options: []core.Option{
				{Value: "90", Label: "90° clockwise"},
				{Value: "180", Label: "180°"},
				{Value: "270", Label: "270° clockwise"},
			},
			Help: "Rotation is applied before the flip.",
		},
		{
			Key: "flip", Label: "Flip", Type: core.ParamEnum, Default: "none",
			Options: []core.Option{
				{Value: "none", Label: "None"},
				{Value: "horizontal", Label: "Horizontal"},
				{Value: "vertical", Label: "Vertical"},
			},
			Help: "Mirrors the rotated result.",
		},
	},
	Engines:   []core.EngineID{engines.Internal},
	Batchable: true,
	Cost:      core.CostFast,
}

type Rotate struct{}

func (Rotate) Spec() core.Spec { return rotateSpec }

func (Rotate) Validate(ctx context.Context, req core.Request) error {
	if err := core.Validate(rotateSpec.Params, req.Params); err != nil {
		return err
	}
	if len(req.Items) == 0 {
		return core.NewError(core.CodeUsage, rotateSpec.ID, "", "no input files", "")
	}
	for _, item := range req.Items {
		if !rotateSpec.AcceptsArtifact(item) {
			return core.NewError(core.CodeInput, rotateSpec.ID, item.Path,
				fmt.Sprintf("%s is not a supported image format", item.Format),
				"supported inputs: PNG, JPEG, GIF")
		}
	}
	return nil
}

func (Rotate) Run(ctx context.Context, req core.Request, rep core.Reporter) (core.Result, error) {
	start := time.Now()
	angle := req.Params.Int("angle")
	switch angle {
	case 90, 180, 270:
	default:
		angle = 90
	}
	flip := req.Params.String("flip")
	if flip == "" {
		flip = "none"
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
			return core.Result{}, core.Wrap(core.CodeInput, rotateSpec.ID, item.Path, err,
				"the file may be corrupt or use an unsupported image encoding")
		}

		rep.Stage("rotating")
		rotated := transformImage(img, angle, flip)

		rep.Stage("encoding")
		format := normalizeFormat(item.Format)
		if !supportedFormat(format) {
			format = "png"
		}
		art, err := writeOutput(req, item, rotated, format, 90)
		if err != nil {
			return core.Result{}, err
		}
		art.Meta["angle"] = angle
		art.Meta["flip"] = flip
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

// transformImage rotates img clockwise by angle degrees (90, 180, or 270) and
// then mirrors the result according to flip. The returned image is always RGBA.
func transformImage(img image.Image, angle int, flip string) *image.RGBA {
	bounds := img.Bounds()
	w, h := bounds.Dx(), bounds.Dy()
	outW, outH := w, h
	if angle == 90 || angle == 270 {
		outW, outH = h, w
	}
	dst := image.NewRGBA(image.Rect(0, 0, outW, outH))

	for dy := 0; dy < outH; dy++ {
		for dx := 0; dx < outW; dx++ {
			// Undo the flip so the rotation sees unmirrored destination
			// coordinates.
			rdx, rdy := dx, dy
			switch flip {
			case "horizontal":
				rdx = outW - 1 - rdx
			case "vertical":
				rdy = outH - 1 - rdy
			}

			var sx, sy int
			switch angle {
			case 90:
				sx, sy = rdy, h-1-rdx
			case 180:
				sx, sy = w-1-rdx, h-1-rdy
			case 270:
				sx, sy = w-1-rdy, rdx
			default:
				sx, sy = rdx, rdy
			}
			dst.Set(dx, dy, img.At(bounds.Min.X+sx, bounds.Min.Y+sy))
		}
	}
	return dst
}
