package image

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/solomonadzape95/workbench/internal/core"
)

func writePDFTestPNG(t *testing.T, path string, w, h int, c color.RGBA) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.SetRGBA(x, y, c)
		}
	}
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestToPDFCombinesImages(t *testing.T) {
	dir := t.TempDir()
	var items []core.Artifact
	for i, spec := range []struct {
		name string
		w, h int
		c    color.RGBA
	}{{"one.png", 80, 60, color.RGBA{R: 255, A: 255}}, {"two.png", 60, 80, color.RGBA{B: 255, A: 255}}} {
		path := filepath.Join(dir, spec.name)
		writePDFTestPNG(t, path, spec.w, spec.h, spec.c)
		art, err := core.Probe(path)
		if err != nil {
			t.Fatal(err)
		}
		items = append(items, art)
		_ = i
	}
	out := filepath.Join(dir, "out")
	op := ToPDF{}
	req := core.Request{
		Items:  items,
		Params: core.Values{"page_size": "a4", "orientation": "auto", "margin": 18},
		Target: core.Target{Dir: out},
	}
	if err := op.Validate(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	result, err := op.Run(context.Background(), req, nopReporter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Artifacts) != 1 || result.Artifacts[0].Format != "pdf" {
		t.Fatalf("artifacts = %+v", result.Artifacts)
	}
	data, err := os.ReadFile(result.Artifacts[0].Path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(data, []byte("%PDF-")) {
		t.Fatalf("not a PDF: %q", data[:8])
	}
	if pages := strings.Count(string(data), "/Type /Page"); pages < 2 {
		t.Fatalf("pages = %d, want at least 2", pages)
	}
	if result.Metrics.BytesOut != int64(len(data)) || result.Artifacts[0].Meta["images"] != 2 {
		t.Fatalf("result = %+v", result)
	}
}

func TestToPDFSingleImagePageSize(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "photo.png")
	writePDFTestPNG(t, path, 120, 80, color.RGBA{G: 255, A: 255})
	art, err := core.Probe(path)
	if err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, "out")
	result, err := (ToPDF{}).Run(context.Background(), core.Request{
		Items:  []core.Artifact{art},
		Params: core.Values{"page_size": "image", "orientation": "auto", "margin": 0},
		Target: core.Target{Dir: out},
	}, nopReporter{})
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(result.Artifacts[0].Path) != "photo.pdf" {
		t.Fatalf("output = %q", result.Artifacts[0].Path)
	}
}

func TestToPDFRejectsNonImage(t *testing.T) {
	err := (ToPDF{}).Validate(context.Background(), core.Request{
		Items:  []core.Artifact{{Path: "notes.txt", Kind: core.Document, Format: "txt"}},
		Target: core.Target{Dir: t.TempDir()},
	})
	if err == nil {
		t.Fatal("text file passed validation")
	}
}
