package image

import (
	"image"
	"image/color"
	"path/filepath"
	"testing"

	"github.com/solomonadzape95/workbench/internal/core"
)

func TestEnhanceBrightnessAndContrast(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src.png")
	writePNGImage(t, src, solidRGBA(32, 24, color.RGBA{R: 100, G: 100, B: 100, A: 255}))

	bright, err := runOp(t, Enhance{}, src, core.Values{
		"contrast": 1.0, "brightness": 40, "sharpen": false,
	})
	if err != nil {
		t.Fatal(err)
	}
	img := decodeOutput(t, bright)
	if img.Bounds().Dx() != 32 || img.Bounds().Dy() != 24 {
		t.Fatalf("bounds = %v, want 32x24", img.Bounds())
	}
	got := nrgbaAt(img, 5, 5)
	if got.R < 201 || got.R > 203 {
		t.Errorf("brightened red = %d, want about 202", got.R)
	}
	if got.G != got.R || got.B != got.R {
		t.Errorf("channels diverged: %v", got)
	}
	if got.A != 255 {
		t.Errorf("alpha = %d, want 255", got.A)
	}

	contrast, err := runOp(t, Enhance{}, src, core.Values{
		"contrast": 2.0, "brightness": 0, "sharpen": false,
	})
	if err != nil {
		t.Fatal(err)
	}
	got = nrgbaAt(decodeOutput(t, contrast), 5, 5)
	if got.R < 71 || got.R > 73 {
		t.Errorf("contrasted red = %d, want about 72", got.R)
	}
}

func TestEnhanceSharpenEdge(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "edge.png")

	edge := image.NewRGBA(image.Rect(0, 0, 16, 16))
	for y := 0; y < 16; y++ {
		for x := 0; x < 16; x++ {
			v := uint8(100)
			if x >= 8 {
				v = 200
			}
			edge.Set(x, y, color.RGBA{R: v, G: v, B: v, A: 255})
		}
	}
	writePNGImage(t, src, edge)

	result, err := runOp(t, Enhance{}, src, core.Values{
		"contrast": 1.0, "brightness": 0, "sharpen": true,
	})
	if err != nil {
		t.Fatal(err)
	}
	img := decodeOutput(t, result)
	if img.Bounds().Dx() != 16 || img.Bounds().Dy() != 16 {
		t.Fatalf("bounds = %v, want 16x16", img.Bounds())
	}
	dark := nrgbaAt(img, 7, 8).R
	light := nrgbaAt(img, 8, 8).R
	if dark >= 100 {
		t.Errorf("dark side of edge = %d, want overshoot below 100", dark)
	}
	if light <= 200 {
		t.Errorf("light side of edge = %d, want overshoot above 200", light)
	}
}

func TestEnhanceRejectsOutOfRangeParams(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src.png")
	writePNG(t, src, 8, 8)

	cases := []core.Values{
		{"contrast": 3.0, "brightness": 0, "sharpen": true},
		{"contrast": 1.0, "brightness": 200, "sharpen": true},
		{"contrast": 0.1, "brightness": 0, "sharpen": true},
	}
	for _, params := range cases {
		if _, err := runOp(t, Enhance{}, src, params); err == nil {
			t.Errorf("params %v passed validation", params)
		}
	}
}
