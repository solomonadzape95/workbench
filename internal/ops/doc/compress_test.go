package doc

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/solomonadzape95/workbench/internal/core"
	"github.com/solomonadzape95/workbench/internal/engines/ghostscript"
)

func TestCompressPDFWithGhostscript(t *testing.T) {
	fakeBin(t, "gs", `#!/bin/sh
out=""
input=""
for arg in "$@"; do
  case "$arg" in
    -sOutputFile=*) out="${arg#-sOutputFile=}";;
    *) input="$arg";;
  esac
done
cp "$input" "$out"
`)
	dir := t.TempDir()
	src := filepath.Join(dir, "paper.pdf")
	if err := os.WriteFile(src, []byte("%PDF-1.4 source"), 0o644); err != nil {
		t.Fatal(err)
	}
	artifact, err := core.Probe(src)
	if err != nil {
		t.Fatal(err)
	}
	outDir := filepath.Join(dir, "out")
	if err := os.Mkdir(outDir, 0o755); err != nil {
		t.Fatal(err)
	}
	op := Compress{Ghostscript: ghostscript.NewClient("gs")}
	req := core.Request{
		Items:  []core.Artifact{artifact},
		Params: core.Values{"preset": "ebook"},
		Target: core.Target{Dir: outDir},
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
	if result.Artifacts[0].Meta["preset"] != "ebook" {
		t.Errorf("meta = %+v", result.Artifacts[0].Meta)
	}
	if _, err := os.Stat(result.Artifacts[0].Path); err != nil {
		t.Fatal(err)
	}
}

func TestCompressPDFRequiresGhostscript(t *testing.T) {
	err := (Compress{}).Validate(context.Background(), core.Request{
		Items:  []core.Artifact{{Path: "/tmp/a.pdf", Format: "pdf", Kind: core.Document}},
		Params: core.Values{"preset": "ebook"},
	})
	if err == nil {
		t.Fatal("missing ghostscript client passed validation")
	}
}

func TestMergePDFsWithGhostscript(t *testing.T) {
	fakeBin(t, "gs", `#!/bin/sh
out=""
input=""
for arg in "$@"; do
  case "$arg" in
    -sOutputFile=*) out="${arg#-sOutputFile=}";;
    -*) ;;
    *) input="$arg";;
  esac
done
cp "$input" "$out"
`)
	dir := t.TempDir()
	var items []core.Artifact
	for _, name := range []string{"one.pdf", "two.pdf"} {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte("%PDF-1.4 "+name), 0o644); err != nil {
			t.Fatal(err)
		}
		artifact, err := core.Probe(path)
		if err != nil {
			t.Fatal(err)
		}
		items = append(items, artifact)
	}
	outDir := filepath.Join(dir, "out")
	if err := os.Mkdir(outDir, 0o755); err != nil {
		t.Fatal(err)
	}
	op := Merge{Ghostscript: ghostscript.NewClient("gs")}
	req := core.Request{
		Items:  items,
		Params: core.Values{"name": "bundle.pdf", "preset": "ebook"},
		Target: core.Target{Dir: outDir},
	}
	if err := op.Validate(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	result, err := op.Run(context.Background(), req, nopReporter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Artifacts) != 1 || result.Artifacts[0].SuggestedName != "bundle.pdf" {
		t.Fatalf("artifacts = %+v", result.Artifacts)
	}
	if _, err := os.Stat(result.Artifacts[0].Path); err != nil {
		t.Fatal(err)
	}
}
