package image

import (
	"image/color"
	"path/filepath"
	"testing"

	"github.com/solomonadzape95/workbench/internal/core"
)

func TestWatermarkDrawsText(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src.png")
	writePNGImage(t, src, solidRGBA(200, 100, color.RGBA{A: 255}))

	result, err := runOp(t, Watermark{}, src, core.Values{
		"text": "WM", "position": "bottom-right", "opacity": 1.0, "margin": 12,
	})
	if err != nil {
		t.Fatal(err)
	}
	out := result.Artifacts[0]
	if out.Format != "png" {
		t.Errorf("format = %q, want png", out.Format)
	}
	img := decodeOutput(t, result)
	if img.Bounds().Dx() != 200 || img.Bounds().Dy() != 100 {
		t.Fatalf("bounds = %v, want 200x100", img.Bounds())
	}

	minX, minY := 200, 100
	maxX, maxY := -1, -1
	for y := 0; y < 100; y++ {
		for x := 0; x < 200; x++ {
			if nrgbaAt(img, x, y).R > 20 {
				minX = min(minX, x)
				minY = min(minY, y)
				maxX = max(maxX, x)
				maxY = max(maxY, y)
			}
		}
	}
	if maxX < 0 {
		t.Fatal("no watermark pixels were drawn")
	}
	if minX < 100 || minY < 50 {
		t.Errorf("ink bbox starts at (%d,%d), want it inside the bottom-right quadrant", minX, minY)
	}
	if maxX > 199 || maxY > 99 {
		t.Errorf("ink bbox ends at (%d,%d), outside the image", maxX, maxY)
	}
	if out.Meta["watermark"] != "WM" {
		t.Errorf("watermark meta = %v, want WM", out.Meta["watermark"])
	}
}

func TestWatermarkPositions(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src.png")
	writePNGImage(t, src, solidRGBA(120, 120, color.RGBA{A: 255}))

	cases := []struct {
		position string
		// Assert the ink's center falls in this region.
		minX, maxX, minY, maxY int
	}{
		{"top-left", 0, 60, 0, 60},
		{"top-right", 60, 120, 0, 60},
		{"bottom-left", 0, 60, 60, 120},
		{"bottom-right", 60, 120, 60, 120},
		{"center", 30, 90, 30, 90},
	}
	for _, tt := range cases {
		t.Run(tt.position, func(t *testing.T) {
			result, err := runOp(t, Watermark{}, src, core.Values{
				"text": "x", "position": tt.position, "opacity": 1.0, "margin": 8,
			})
			if err != nil {
				t.Fatal(err)
			}
			img := decodeOutput(t, result)
			sumX, sumY, n := 0, 0, 0
			for y := 0; y < 120; y++ {
				for x := 0; x < 120; x++ {
					if nrgbaAt(img, x, y).R > 20 {
						sumX += x
						sumY += y
						n++
					}
				}
			}
			if n == 0 {
				t.Fatal("no watermark pixels were drawn")
			}
			cx, cy := sumX/n, sumY/n
			if cx < tt.minX || cx >= tt.maxX || cy < tt.minY || cy >= tt.maxY {
				t.Errorf("ink center = (%d,%d), want inside [%d,%d)x[%d,%d)",
					cx, cy, tt.minX, tt.maxX, tt.minY, tt.maxY)
			}
		})
	}
}

func TestWatermarkRequiresText(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src.png")
	writePNG(t, src, 20, 20)
	if _, err := runOp(t, Watermark{}, src, core.Values{"text": "", "position": "center"}); err == nil {
		t.Fatal("empty watermark text passed validation")
	}
	if _, err := runOp(t, Watermark{}, src, core.Values{"text": "ok", "opacity": 2.0}); err == nil {
		t.Fatal("opacity 2.0 passed validation")
	}
}

func TestWatermarkPreservesSourceFormat(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src.jpg")
	writeJPEG(t, src, 60, 40, 95)

	result, err := runOp(t, Watermark{}, src, core.Values{
		"text": "demo", "position": "center", "opacity": 0.6, "margin": 10,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Artifacts[0].Format != "jpeg" {
		t.Errorf("format = %q, want jpeg", result.Artifacts[0].Format)
	}
	img := decodeOutput(t, result)
	if img.Bounds().Dx() != 60 || img.Bounds().Dy() != 40 {
		t.Errorf("bounds = %v, want 60x40", img.Bounds())
	}
}
