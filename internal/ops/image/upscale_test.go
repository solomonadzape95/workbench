package image

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/solomonadzape95/workbench/internal/core"
	"github.com/solomonadzape95/workbench/internal/engines/realesrgan"
)

func TestUpscaleRunsRealESRGAN(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell-script fixture")
	}
	dir := t.TempDir()
	binDir := filepath.Join(dir, "bin")
	if err := os.Mkdir(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	script := `#!/bin/sh
input=""; output=""
while [ $# -gt 0 ]; do
  case "$1" in
    -i) input="$2"; shift 2 ;;
    -o) output="$2"; shift 2 ;;
    *) shift ;;
  esac
done
cp "$input" "$output"
`
	if err := os.WriteFile(filepath.Join(binDir, "realesrgan-ncnn-vulkan"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	src := filepath.Join(dir, "photo.png")
	if err := os.WriteFile(src, []byte("not-a-real-png"), 0o644); err != nil {
		t.Fatal(err)
	}
	art, err := core.Probe(src)
	if err != nil {
		t.Fatal(err)
	}
	outDir := filepath.Join(dir, "out")
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		t.Fatal(err)
	}
	op := Upscale{Client: realesrgan.NewClient("")}
	req := core.Request{
		Items: []core.Artifact{art},
		Params: core.Values{
			"scale": "4", "model": "realesrgan-x4plus", "format": "png", "tile": 0,
		},
		Target: core.Target{Dir: outDir},
	}
	if err := op.Validate(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	result, err := op.Run(context.Background(), req, nopReporter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Artifacts) != 1 {
		t.Fatalf("artifacts = %+v", result.Artifacts)
	}
	if got := filepath.Base(result.Artifacts[0].Path); got != "photo.png" {
		t.Fatalf("output = %q", got)
	}
	if result.Artifacts[0].Meta["scale"] != "4" {
		t.Fatalf("meta = %+v", result.Artifacts[0].Meta)
	}
	if data, err := os.ReadFile(result.Artifacts[0].Path); err != nil || string(data) != "not-a-real-png" {
		t.Fatalf("output data = %q, %v", data, err)
	}
}

func TestUpscaleMissingEngineFailsValidation(t *testing.T) {
	err := (Upscale{}).Validate(context.Background(), core.Request{
		Items:  []core.Artifact{{Path: "photo.png", Kind: core.Image, Format: "png"}},
		Target: core.Target{Dir: t.TempDir()},
	})
	if err == nil {
		t.Fatal("missing client passed validation")
	}
}
