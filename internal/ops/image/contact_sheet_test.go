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

func TestContactSheet(t *testing.T) {
	dir := t.TempDir()
	var items []core.Artifact
	for i, c := range []color.RGBA{{255, 0, 0, 255}, {0, 255, 0, 255}, {0, 0, 255, 255}} {
		img := image.NewRGBA(image.Rect(0, 0, 40, 30))
		for y := 0; y < 30; y++ {
			for x := 0; x < 40; x++ {
				img.SetRGBA(x, y, c)
			}
		}
		path := filepath.Join(dir, "img"+string(rune('a'+i))+".png")
		f, err := os.Create(path)
		if err != nil {
			t.Fatal(err)
		}
		if err := png.Encode(f, img); err != nil {
			t.Fatal(err)
		}
		f.Close()
		art, err := core.Probe(path)
		if err != nil {
			t.Fatal(err)
		}
		items = append(items, art)
	}
	out := filepath.Join(dir, "out")
	if err := os.Mkdir(out, 0o755); err != nil {
		t.Fatal(err)
	}
	req := core.Request{
		Items: items,
		Params: core.Values{
			"columns": 2, "thumb_width": 80, "thumb_height": 60,
			"gap": 4, "margin": 4, "labels": true, "format": "png",
		},
		Target: core.Target{Dir: out},
	}
	if err := (ContactSheet{}).Validate(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	result, err := (ContactSheet{}).Run(context.Background(), req, nopReporter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Artifacts) != 1 {
		t.Fatalf("artifacts = %+v", result.Artifacts)
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
	// 2 columns, 2 rows; labels add 16px per row.
	wantW := 4*2 + 2*80 + 4
	wantH := 4*2 + 2*(60+16) + 4
	if img.Bounds().Dx() != wantW || img.Bounds().Dy() != wantH {
		t.Fatalf("size = %dx%d, want %dx%d", img.Bounds().Dx(), img.Bounds().Dy(), wantW, wantH)
	}
}
