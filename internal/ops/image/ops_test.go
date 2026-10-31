package image

import (
	"context"
	"image"
	"image/color"
	"image/jpeg"
	"os"
	"path/filepath"
	"testing"

	"github.com/solomonadzape95/workbench/internal/core"
)

func runOp(t *testing.T, op core.Operation, src string, params core.Values) (core.Result, error) {
	t.Helper()
	art, err := core.Probe(src)
	if err != nil {
		t.Fatal(err)
	}
	work := t.TempDir()
	req := core.Request{
		Items:  []core.Artifact{art},
		Params: params,
		Target: core.Target{Dir: work},
	}
	if err := op.Validate(context.Background(), req); err != nil {
		return core.Result{}, err
	}
	return op.Run(context.Background(), req, nopReporter{})
}

func decodeOutput(t *testing.T, result core.Result) image.Image {
	t.Helper()
	if len(result.Artifacts) != 1 {
		t.Fatalf("got %d artifacts, want 1", len(result.Artifacts))
	}
	f, err := os.Open(result.Artifacts[0].Path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	img, _, err := image.Decode(f)
	if err != nil {
		t.Fatal(err)
	}
	return img
}

func TestResizeFitExactAndFill(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src.png")
	writePNG(t, src, 200, 100)

	cases := []struct {
		name   string
		params core.Values
		w, h   int
	}{
		{"fit both", core.Values{"width": 100, "height": 100, "mode": "fit", "filter": "smooth"}, 100, 50},
		{"fit width only", core.Values{"width": 50, "height": 0, "mode": "fit", "filter": "smooth"}, 50, 25},
		{"fit height only", core.Values{"width": 0, "height": 50, "mode": "fit", "filter": "nearest"}, 100, 50},
		{"exact", core.Values{"width": 120, "height": 80, "mode": "exact", "filter": "smooth"}, 120, 80},
		{"fill", core.Values{"width": 100, "height": 100, "mode": "fill", "filter": "smooth"}, 100, 100},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			result, err := runOp(t, Resize{}, src, tt.params)
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

func TestResizeRejectsMissingDimensions(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src.png")
	writePNG(t, src, 10, 10)
	if _, err := runOp(t, Resize{}, src, core.Values{"width": 0, "height": 0, "mode": "fit"}); err == nil {
		t.Fatal("resize with no dimensions passed validation")
	}
}

func TestCropCenterAndOffset(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src.png")
	writePNG(t, src, 100, 80)

	result, err := runOp(t, Crop{}, src, core.Values{"width": 50, "height": 40, "gravity": "center", "x": 0, "y": 0})
	if err != nil {
		t.Fatal(err)
	}
	img := decodeOutput(t, result)
	if img.Bounds().Dx() != 50 || img.Bounds().Dy() != 40 {
		t.Errorf("bounds = %v, want 50x40", img.Bounds())
	}

	if _, err := runOp(t, Crop{}, src, core.Values{"width": 50, "height": 40, "gravity": "top-left", "x": 90, "y": 0}); err == nil {
		t.Fatal("out-of-bounds crop passed")
	}
	if _, err := runOp(t, Crop{}, src, core.Values{"width": 200, "height": 40, "gravity": "center"}); err == nil {
		t.Fatal("oversized crop passed")
	}
}

func TestCompressJPEGGetsSmaller(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src.jpg")
	writeJPEG(t, src, 160, 120, 100)
	before, err := os.Stat(src)
	if err != nil {
		t.Fatal(err)
	}

	result, err := runOp(t, Compress{}, src, core.Values{"quality": 10})
	if err != nil {
		t.Fatal(err)
	}
	img := decodeOutput(t, result)
	if img.Bounds().Dx() != 160 || img.Bounds().Dy() != 120 {
		t.Errorf("bounds = %v, want 160x120", img.Bounds())
	}
	after, err := os.Stat(result.Artifacts[0].Path)
	if err != nil {
		t.Fatal(err)
	}
	if after.Size() >= before.Size() {
		t.Errorf("compressed size = %d, original = %d; want smaller", after.Size(), before.Size())
	}
}

func writeJPEG(t *testing.T, path string, width, height, quality int) {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			img.Set(x, y, color.RGBA{R: uint8(x % 256), G: uint8(y % 256), B: 128, A: 255})
		}
	}
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := jpeg.Encode(f, img, &jpeg.Options{Quality: quality}); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
}
