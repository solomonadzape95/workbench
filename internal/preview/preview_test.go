package preview

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"image"
	"image/color"
	"image/png"
)

func writePNG(t *testing.T, path string) {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 16, 8))
	for y := 0; y < 8; y++ {
		for x := 0; x < 16; x++ {
			img.Set(x, y, color.RGBA{R: uint8(x * 16), G: uint8(y * 32), B: 200, A: 255})
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

func TestImagePreviewRendersHalfBlocks(t *testing.T) {
	path := filepath.Join(t.TempDir(), "a.png")
	writePNG(t, path)

	p, err := Generate(path, 40, 10)
	if err != nil {
		t.Fatal(err)
	}
	if p.Kind != KindImage {
		t.Errorf("kind = %q, want image", p.Kind)
	}
	if !strings.Contains(p.ANSI, "▀") {
		t.Error("ANSI preview has no half blocks")
	}
	if !strings.Contains(p.Text, "PNG") {
		t.Errorf("text fallback = %q", p.Text)
	}
}

func TestTextPreview(t *testing.T) {
	path := filepath.Join(t.TempDir(), "a.txt")
	if err := os.WriteFile(path, []byte("hello\nworld\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	p, err := Generate(path, 40, 10)
	if err != nil {
		t.Fatal(err)
	}
	if p.Kind != KindText || !strings.Contains(p.Text, "hello") {
		t.Errorf("preview = %+v", p)
	}
}

func TestBinaryPreview(t *testing.T) {
	path := filepath.Join(t.TempDir(), "blob.bin")
	if err := os.WriteFile(path, []byte{0, 1, 2, 3, 0xff}, 0o644); err != nil {
		t.Fatal(err)
	}
	p, err := Generate(path, 60, 10)
	if err != nil {
		t.Fatal(err)
	}
	if p.Kind != KindBinary {
		t.Errorf("kind = %q, want binary", p.Kind)
	}
	if !strings.Contains(strings.Join(p.Meta, " "), "binary") {
		t.Errorf("meta = %v, want a binary note", p.Meta)
	}
}

func TestMissingFile(t *testing.T) {
	if _, err := Generate(filepath.Join(t.TempDir(), "nope.png"), 40, 10); err == nil {
		t.Fatal("missing file produced a preview")
	}
}
