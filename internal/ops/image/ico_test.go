package image

import (
	"bytes"
	"encoding/binary"
	"image"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/solomonadzape95/workbench/internal/core"
)

func TestToICOStructure(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src.png")
	writePNG(t, src, 64, 32)

	result, err := runOp(t, ToICO{}, src, core.Values{"sizes": "16,32,64"})
	if err != nil {
		t.Fatal(err)
	}
	out := result.Artifacts[0]
	if out.Format != "ico" {
		t.Errorf("format = %q, want ico", out.Format)
	}
	if filepath.Ext(out.Path) != ".ico" {
		t.Errorf("extension = %q, want .ico", filepath.Ext(out.Path))
	}
	if out.Meta["width"] != 64 || out.Meta["height"] != 64 {
		t.Errorf("meta width/height = %v/%v, want 64/64", out.Meta["width"], out.Meta["height"])
	}
	if sizes, ok := out.Meta["sizes"].([]int); !ok || !reflect.DeepEqual(sizes, []int{16, 32, 64}) {
		t.Errorf("meta sizes = %v, want [16 32 64]", out.Meta["sizes"])
	}

	data, err := os.ReadFile(out.Path)
	if err != nil {
		t.Fatal(err)
	}
	reader := bytes.NewReader(data)

	var header struct {
		Reserved uint16
		Type     uint16
		Count    uint16
	}
	if err := binary.Read(reader, binary.LittleEndian, &header); err != nil {
		t.Fatal(err)
	}
	if header.Reserved != 0 {
		t.Errorf("reserved = %d, want 0", header.Reserved)
	}
	if header.Type != 1 {
		t.Errorf("type = %d, want 1 (icon)", header.Type)
	}
	if header.Count != 3 {
		t.Fatalf("entry count = %d, want 3", header.Count)
	}

	wantSizes := []int{16, 32, 64}
	for i := 0; i < int(header.Count); i++ {
		var entry struct {
			Width      uint8
			Height     uint8
			Colors     uint8
			Reserved   uint8
			Planes     uint16
			BitCount   uint16
			BytesInRes uint32
			Offset     uint32
		}
		if err := binary.Read(reader, binary.LittleEndian, &entry); err != nil {
			t.Fatal(err)
		}
		want := icoDimension(wantSizes[i])
		if entry.Width != want || entry.Height != want {
			t.Errorf("entry %d dimensions = %dx%d, want %dx%d", i, entry.Width, entry.Height, want, want)
		}
		if entry.Planes != 1 {
			t.Errorf("entry %d planes = %d, want 1", i, entry.Planes)
		}
		if entry.BitCount != 32 {
			t.Errorf("entry %d bit count = %d, want 32", i, entry.BitCount)
		}
		if entry.Colors != 0 || entry.Reserved != 0 {
			t.Errorf("entry %d palette/reserved = %d/%d, want 0/0", i, entry.Colors, entry.Reserved)
		}

		start := int(entry.Offset)
		end := start + int(entry.BytesInRes)
		if start < 0 || end > len(data) || start >= end {
			t.Fatalf("entry %d range %d:%d is outside the %d-byte file", i, start, end, len(data))
		}
		payload := data[start:end]
		if !bytes.HasPrefix(payload, []byte("\x89PNG\r\n\x1a\n")) {
			t.Errorf("entry %d payload is not PNG data", i)
			continue
		}
		icon, format, err := image.Decode(bytes.NewReader(payload))
		if err != nil {
			t.Fatalf("entry %d PNG decode: %v", i, err)
		}
		if format != "png" {
			t.Errorf("entry %d format = %q, want png", i, format)
		}
		if icon.Bounds().Dx() != wantSizes[i] || icon.Bounds().Dy() != wantSizes[i] {
			t.Errorf("entry %d decoded bounds = %v, want %dx%d", i, icon.Bounds(), wantSizes[i], wantSizes[i])
		}
	}
}

func TestToICODefaultSizes(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src.png")
	writePNG(t, src, 300, 300)

	result, err := runOp(t, ToICO{}, src, core.Values{"sizes": "16,32,64,128,256"})
	if err != nil {
		t.Fatal(err)
	}
	sizes, ok := result.Artifacts[0].Meta["sizes"].([]int)
	if !ok || len(sizes) != 5 || sizes[4] != 256 {
		t.Errorf("sizes = %v, want five sizes ending in 256", result.Artifacts[0].Meta["sizes"])
	}
	data, err := os.ReadFile(result.Artifacts[0].Path)
	if err != nil {
		t.Fatal(err)
	}
	count := binary.LittleEndian.Uint16(data[4:6])
	if count != 5 {
		t.Errorf("entry count = %d, want 5", count)
	}
	// The first entry is 16px and the 256px entry stores 0 in the one-byte
	// width/height fields.
	if data[6] != 16 || data[7] != 16 {
		t.Errorf("first entry width/height = %d/%d, want 16/16", data[6], data[7])
	}
	last := 6 + 16*4
	if data[last] != 0 || data[last+1] != 0 {
		t.Errorf("256px entry width/height = %d/%d, want 0/0", data[last], data[last+1])
	}
}

func TestToICORejectsBadSizes(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src.png")
	writePNG(t, src, 10, 10)

	for _, sizes := range []string{"0", "300", "abc", "16,,32x"} {
		t.Run(sizes, func(t *testing.T) {
			if _, err := runOp(t, ToICO{}, src, core.Values{"sizes": sizes}); err == nil {
				t.Fatalf("sizes %q passed validation", sizes)
			}
		})
	}
}
