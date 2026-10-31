package image

import (
	"context"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/solomonadzape95/workbench/internal/core"
)

type nopReporter struct{}

func (nopReporter) Stage(string)                   {}
func (nopReporter) Progress(int, int)              {}
func (nopReporter) Detail(core.Artifact, string)   {}
func (nopReporter) Log(core.Level, string, ...any) {}

func writePNG(t *testing.T, path string, width, height int) {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			img.Set(x, y, color.RGBA{R: uint8(x * 20), G: uint8(y * 20), B: 120, A: 255})
		}
	}
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestConvertPNGToJPEG(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "input.png")
	writePNG(t, src, 8, 6)
	art, err := core.Probe(src)
	if err != nil {
		t.Fatal(err)
	}

	work := filepath.Join(dir, "work")
	if err := os.Mkdir(work, 0o755); err != nil {
		t.Fatal(err)
	}
	op := Convert{}
	req := core.Request{
		Items:  []core.Artifact{art},
		Params: core.Values{"format": "jpeg", "quality": 90},
		Target: core.Target{Dir: work},
	}
	if err := op.Validate(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	result, err := op.Run(context.Background(), req, nopReporter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Artifacts) != 1 {
		t.Fatalf("got %d artifacts, want 1", len(result.Artifacts))
	}
	out := result.Artifacts[0]
	if filepath.Ext(out.Path) != ".jpg" {
		t.Errorf("output = %q, want a .jpg", out.Path)
	}
	if out.Meta["width"] != 8 || out.Meta["height"] != 6 {
		t.Errorf("meta = %+v, want 8x6", out.Meta)
	}

	f, err := os.Open(out.Path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	img, format, err := image.Decode(f)
	if err != nil {
		t.Fatal(err)
	}
	if format != "jpeg" {
		t.Errorf("encoded format = %q, want jpeg", format)
	}
	if img.Bounds().Dx() != 8 || img.Bounds().Dy() != 6 {
		t.Errorf("decoded bounds = %v, want 8x6", img.Bounds())
	}
}

func TestConvertRejectsNonImage(t *testing.T) {
	op := Convert{}
	req := core.Request{
		Items:  []core.Artifact{{Path: "/tmp/not-image.txt", Kind: core.File, Format: "txt"}},
		Params: core.Values{"format": "png", "quality": 85},
	}
	if err := op.Validate(context.Background(), req); err == nil {
		t.Fatal("non-image input passed validation")
	}
}

func TestConvertJPEGQualityIsUsed(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "input.png")
	writePNG(t, src, 32, 32)
	art, _ := core.Probe(src)
	work := filepath.Join(dir, "work")
	if err := os.Mkdir(work, 0o755); err != nil {
		t.Fatal(err)
	}

	run := func(quality int) int64 {
		req := core.Request{
			Items:  []core.Artifact{art},
			Params: core.Values{"format": "jpeg", "quality": quality},
			Target: core.Target{Dir: work},
		}
		result, err := Convert{}.Run(context.Background(), req, nopReporter{})
		if err != nil {
			t.Fatal(err)
		}
		info, err := os.Stat(result.Artifacts[0].Path)
		if err != nil {
			t.Fatal(err)
		}
		_ = os.Remove(result.Artifacts[0].Path)
		return info.Size()
	}

	low, high := run(10), run(95)
	if low >= high {
		t.Errorf("quality 10 size = %d, quality 95 size = %d; want low < high", low, high)
	}
}
