package image

import (
	"context"
	"image"
	"image/color"
	stddraw "image/draw"
	"os"
	"path/filepath"
	"time"

	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/math/fixed"

	"github.com/solomonadzape95/workbench/internal/core"
	"github.com/solomonadzape95/workbench/internal/engines"
)

var contactSheetSpec = core.Spec{
	ID:      "image.contact_sheet",
	Title:   "Contact sheet",
	Summary: "Arrange images into one labeled grid",
	Domain:  core.Image,
	Accepts: []core.Accept{{Kind: core.Image}},
	Params: []core.ParamSpec{
		{Key: "columns", Label: "Columns", Type: core.ParamInt, Default: 4, Min: floatPtr(1)},
		{Key: "thumb_width", Label: "Cell width", Type: core.ParamInt, Default: 320, Min: floatPtr(32)},
		{Key: "thumb_height", Label: "Cell height", Type: core.ParamInt, Default: 240, Min: floatPtr(32)},
		{Key: "gap", Label: "Gap", Type: core.ParamInt, Default: 12, Min: floatPtr(0)},
		{Key: "margin", Label: "Margin", Type: core.ParamInt, Default: 12, Min: floatPtr(0)},
		{Key: "labels", Label: "Labels", Type: core.ParamBool, Default: true},
		{Key: "format", Label: "Format", Type: core.ParamEnum, Default: "png", Options: []core.Option{{Value: "png", Label: "PNG"}, {Value: "jpeg", Label: "JPEG"}}},
	},
	Output:  core.OutputSpec{FormatParam: "format", Kind: core.Image},
	Engines: []core.EngineID{engines.Internal},
	Mode:    core.ModeAggregate,
	Cost:    core.CostMedium,
}

type ContactSheet struct{}

func (ContactSheet) Spec() core.Spec { return contactSheetSpec }

func (ContactSheet) Validate(ctx context.Context, req core.Request) error {
	if err := core.Validate(contactSheetSpec.Params, req.Params); err != nil {
		return err
	}
	if len(req.Items) == 0 {
		return core.NewError(core.CodeUsage, contactSheetSpec.ID, "", "no images", "")
	}
	for _, item := range req.Items {
		if !contactSheetSpec.AcceptsArtifact(item) {
			return core.NewError(core.CodeInput, contactSheetSpec.ID, item.Path, "not an image", "")
		}
	}
	return nil
}

func (ContactSheet) Run(ctx context.Context, req core.Request, rep core.Reporter) (core.Result, error) {
	start := time.Now()
	columns := req.Params.Int("columns")
	if columns < 1 {
		columns = 4
	}
	cellW := req.Params.Int("thumb_width")
	cellH := req.Params.Int("thumb_height")
	if cellW < 32 {
		cellW = 320
	}
	if cellH < 32 {
		cellH = 240
	}
	gap := max(req.Params.Int("gap"), 0)
	margin := max(req.Params.Int("margin"), 0)
	labels := req.Params.Bool("labels")
	format := req.Params.String("format")
	if format == "" {
		format = "png"
	}
	labelH := 0
	if labels {
		labelH = 16
	}
	rows := (len(req.Items) + columns - 1) / columns
	width := margin*2 + columns*cellW + max(columns-1, 0)*gap
	height := margin*2 + rows*(cellH+labelH) + max(rows-1, 0)*gap
	canvas := image.NewRGBA(image.Rect(0, 0, width, height))
	stddraw.Draw(canvas, canvas.Bounds(), &image.Uniform{C: color.White}, image.Point{}, stddraw.Src)
	rep.Stage("building contact sheet")
	rep.Progress(0, len(req.Items))

	for i, item := range req.Items {
		if err := ctx.Err(); err != nil {
			return core.Result{}, err
		}
		img, _, err := decode(item.Path)
		if err != nil {
			return core.Result{}, core.Wrap(core.CodeInput, contactSheetSpec.ID, item.Path, err,
				"the image may be corrupt")
		}
		thumb, err := resizeImage(img, cellW, cellH, "fill", "smooth")
		if err != nil {
			return core.Result{}, core.Wrap(core.CodeInput, contactSheetSpec.ID, item.Path, err, "")
		}
		col := i % columns
		row := i / columns
		x := margin + col*(cellW+gap)
		y := margin + row*(cellH+labelH+gap)
		stddraw.Draw(canvas, image.Rect(x, y, x+cellW, y+cellH), thumb, image.Point{}, stddraw.Src)
		if labels {
			stddraw.Draw(canvas, image.Rect(x, y+cellH, x+cellW, y+cellH+labelH),
				&image.Uniform{C: color.White}, image.Point{}, stddraw.Src)
			drawLabel(canvas, x+4, y+cellH+12, item.Name())
		}
		rep.Progress(i+1, len(req.Items))
	}

	name := core.SanitizeStem(req.Items[0].Stem()) + "-contact." + core.ExtFor(format, req.Items[0])
	output := filepath.Join(req.Target.Dir, name)
	if err := encodeFile(output, canvas, format, 90); err != nil {
		return core.Result{}, core.Wrap(core.CodeOutput, contactSheetSpec.ID, output, err, "")
	}
	info, err := os.Stat(output)
	if err != nil {
		return core.Result{}, core.Wrap(core.CodeOutput, contactSheetSpec.ID, output, err, "")
	}
	var bytesIn int64
	for _, item := range req.Items {
		bytesIn += item.Size
	}
	artifact := core.Artifact{
		Path: output, Kind: core.Image, Format: format, Size: info.Size(),
		SuggestedName: name,
		Meta: map[string]any{
			"images":  len(req.Items),
			"columns": columns,
			"width":   width,
			"height":  height,
		},
	}
	return core.Result{
		Artifacts: []core.Artifact{artifact},
		Metrics: core.Metrics{
			BytesIn: bytesIn, BytesOut: info.Size(), Duration: time.Since(start), Engine: engines.Internal,
		},
	}, nil
}

func drawLabel(dst *image.RGBA, x, y int, text string) {
	face := basicfont.Face7x13
	text = truncateLabel(text, 48)
	d := font.Drawer{
		Dst:  dst,
		Src:  image.NewUniform(color.RGBA{R: 40, G: 40, B: 40, A: 255}),
		Face: face,
		Dot:  fixed.P(x, y),
	}
	d.DrawString(text)
}

func truncateLabel(text string, width int) string {
	runes := []rune(text)
	if len(runes) <= width {
		return text
	}
	return string(runes[:width-1]) + "…"
}
