package image

import (
	"bytes"
	"encoding/binary"
	"hash/crc32"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/solomonadzape95/workbench/internal/core"
)

func TestStripMetaDropsPNGTextChunk(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src.png")
	writePNGWithText(t, src, solidRGBA(12, 9, color.RGBA{R: 40, G: 80, B: 120, A: 255}), "Comment", "secret-marker")

	before, err := os.ReadFile(src)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(before, []byte("secret-marker")) {
		t.Fatal("test setup did not embed the metadata marker")
	}

	result, err := runOp(t, StripMeta{}, src, core.Values{"format": "auto", "quality": 90})
	if err != nil {
		t.Fatal(err)
	}
	out := result.Artifacts[0]
	if out.Format != "png" {
		t.Errorf("format = %q, want png", out.Format)
	}
	if filepath.Ext(out.Path) != ".png" {
		t.Errorf("extension = %q, want .png", filepath.Ext(out.Path))
	}
	if out.Meta["metadata_stripped"] != true {
		t.Errorf("metadata_stripped meta = %v, want true", out.Meta["metadata_stripped"])
	}

	after, err := os.ReadFile(out.Path)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(after, []byte("secret-marker")) {
		t.Error("output still contains the tEXt metadata marker")
	}

	img := decodeOutput(t, result)
	if img.Bounds().Dx() != 12 || img.Bounds().Dy() != 9 {
		t.Errorf("bounds = %v, want 12x9", img.Bounds())
	}
}

func TestStripMetaConvertsFormat(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src.png")
	writePNG(t, src, 20, 10)

	result, err := runOp(t, StripMeta{}, src, core.Values{"format": "jpeg", "quality": 75})
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
	if out.Meta["width"] != 20 || out.Meta["height"] != 10 {
		t.Errorf("meta = %+v, want 20x10", out.Meta)
	}
}

func TestStripMetaRejectsUnknownFormat(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src.png")
	writePNG(t, src, 4, 4)
	if _, err := runOp(t, StripMeta{}, src, core.Values{"format": "webp", "quality": 90}); err == nil {
		t.Fatal("unknown format passed validation")
	}
}

// writePNGWithText encodes img and splices a tEXt chunk directly after IHDR so
// the test has real ancillary metadata to strip.
func writePNGWithText(t *testing.T, path string, img image.Image, keyword, text string) {
	t.Helper()
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	raw := buf.Bytes()

	data := append([]byte(keyword+"\x00"), []byte(text)...)
	var chunk bytes.Buffer
	if err := binary.Write(&chunk, binary.BigEndian, uint32(len(data))); err != nil {
		t.Fatal(err)
	}
	chunk.WriteString("tEXt")
	chunk.Write(data)
	crc := crc32.ChecksumIEEE(append([]byte("tEXt"), data...))
	if err := binary.Write(&chunk, binary.BigEndian, crc); err != nil {
		t.Fatal(err)
	}

	const insertAt = 8 + 25 // PNG signature + IHDR chunk
	if len(raw) < insertAt {
		t.Fatal("encoded PNG is shorter than the PNG header")
	}
	out := make([]byte, 0, len(raw)+chunk.Len())
	out = append(out, raw[:insertAt]...)
	out = append(out, chunk.Bytes()...)
	out = append(out, raw[insertAt:]...)

	if err := os.WriteFile(path, out, 0o644); err != nil {
		t.Fatal(err)
	}
}
