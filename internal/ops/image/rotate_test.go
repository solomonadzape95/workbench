package image

import (
	"image"
	"image/color"
	"path/filepath"
	"testing"

	"github.com/solomonadzape95/workbench/internal/core"
)

func TestRotateDimensions(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src.png")
	writePNG(t, src, 30, 20)

	cases := []struct {
		angle string
		w, h  int
	}{
		{"90", 20, 30},
		{"180", 30, 20},
		{"270", 20, 30},
	}
	for _, tt := range cases {
		t.Run(tt.angle, func(t *testing.T) {
			result, err := runOp(t, Rotate{}, src, core.Values{"angle": tt.angle, "flip": "none"})
			if err != nil {
				t.Fatal(err)
			}
			img := decodeOutput(t, result)
			if got := img.Bounds().Dx(); got != tt.w {
				t.Errorf("width = %d, want %d", got, tt.w)
			}
			if got := img.Bounds().Dy(); got != tt.h {
				t.Errorf("height = %d, want %d", got, tt.h)
			}
		})
	}
}

func TestRotateAndFlipPixels(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "quad.png")

	red := color.RGBA{R: 255, A: 255}
	green := color.RGBA{G: 255, A: 255}
	blue := color.RGBA{B: 255, A: 255}
	yellow := color.RGBA{R: 255, G: 255, A: 255}

	quad := image.NewRGBA(image.Rect(0, 0, 2, 2))
	quad.Set(0, 0, red)
	quad.Set(1, 0, green)
	quad.Set(0, 1, blue)
	quad.Set(1, 1, yellow)
	writePNGImage(t, src, quad)

	// Expected pixels in row-major order after the transform.
	cases := []struct {
		name  string
		angle string
		flip  string
		want  [4]color.RGBA
	}{
		{"90", "90", "none", [4]color.RGBA{blue, red, yellow, green}},
		{"180", "180", "none", [4]color.RGBA{yellow, blue, green, red}},
		{"270", "270", "none", [4]color.RGBA{green, yellow, red, blue}},
		{"90 horizontal", "90", "horizontal", [4]color.RGBA{red, blue, green, yellow}},
		{"90 vertical", "90", "vertical", [4]color.RGBA{yellow, green, blue, red}},
		{"180 horizontal", "180", "horizontal", [4]color.RGBA{blue, yellow, red, green}},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			result, err := runOp(t, Rotate{}, src, core.Values{"angle": tt.angle, "flip": tt.flip})
			if err != nil {
				t.Fatal(err)
			}
			img := decodeOutput(t, result)
			if img.Bounds().Dx() != 2 || img.Bounds().Dy() != 2 {
				t.Fatalf("bounds = %v, want 2x2", img.Bounds())
			}
			for i, want := range tt.want {
				x, y := i%2, i/2
				got := nrgbaAt(img, x, y)
				wantNRGBA := color.NRGBAModel.Convert(want).(color.NRGBA)
				if got != wantNRGBA {
					t.Errorf("pixel (%d,%d) = %v, want %v", x, y, got, wantNRGBA)
				}
			}
		})
	}
}

func TestRotatePreservesFormat(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src.jpg")
	writeJPEG(t, src, 16, 12, 95)

	result, err := runOp(t, Rotate{}, src, core.Values{"angle": "90", "flip": "none"})
	if err != nil {
		t.Fatal(err)
	}
	out := result.Artifacts[0]
	if out.Format != "jpeg" {
		t.Errorf("format = %q, want jpeg", out.Format)
	}
	if filepath.Ext(out.Path) != ".jpg" {
		t.Errorf("extension = %q, want .jpg", filepath.Ext(out.Path))
	}
}
