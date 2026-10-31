package image

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/solomonadzape95/workbench/internal/core"
	"github.com/solomonadzape95/workbench/internal/engines/rembg"
)

func TestRemoveBackgroundInvokesRembg(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell-script fixture")
	}
	dir := t.TempDir()
	argsPath := filepath.Join(dir, "args")
	script := `#!/bin/sh
printf '%s\n' "$@" > ` + argsPath + `
prev=""; cur=""
for arg in "$@"; do prev="$cur"; cur="$arg"; done
printf 'png' > "$cur"
`
	if err := os.WriteFile(filepath.Join(dir, "rembg"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))

	src := filepath.Join(dir, "subject.png")
	writePNG(t, src, 16, 16)
	art, err := core.Probe(src)
	if err != nil {
		t.Fatal(err)
	}
	op := RemoveBG{Rembg: rembg.NewClient("")}
	req := core.Request{
		Items:  []core.Artifact{art},
		Params: core.Values{"model": "bria-rmbg", "alpha_matting": false},
		Target: core.Target{Dir: t.TempDir()},
	}
	if err := op.Validate(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	result, err := op.Run(context.Background(), req, nopReporter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Artifacts) != 1 || result.Artifacts[0].Format != "png" {
		t.Fatalf("artifacts = %+v", result.Artifacts)
	}
	if result.Metrics.Engine != "rembg" || result.Metrics.Model != "bria-rmbg" {
		t.Errorf("metrics = %+v", result.Metrics)
	}
	args, err := os.ReadFile(argsPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(args), "-m\nbria-rmbg") {
		t.Fatalf("model argument missing:\n%s", args)
	}
	if strings.Contains(string(args), "\n-a\n") {
		t.Fatalf("alpha matting was requested but not enabled:\n%s", args)
	}
}
