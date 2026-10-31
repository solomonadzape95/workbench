package doc

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/solomonadzape95/workbench/internal/core"
	"github.com/solomonadzape95/workbench/internal/engines/poppler"
	"github.com/solomonadzape95/workbench/internal/engines/tesseract"
)

func fakeOCRBin(t *testing.T, name, body string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("shell-script fixture")
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func TestOCRImage(t *testing.T) {
	fakeOCRBin(t, "tesseract", `#!/bin/sh
input="$1"; base="$2"; shift 2
format="txt"
for arg in "$@"; do format="$arg"; done
echo "OCR: $input" > "$base.$format"
`)
	dir := t.TempDir()
	src := filepath.Join(dir, "scan.png")
	if err := os.WriteFile(src, []byte("not really a png"), 0o644); err != nil {
		t.Fatal(err)
	}
	art, err := core.Probe(src)
	if err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, "out")
	if err := os.Mkdir(out, 0o755); err != nil {
		t.Fatal(err)
	}
	op := OCR{Tesseract: tesseract.NewClient("tesseract"), Poppler: poppler.NewClient("")}
	req := core.Request{Items: []core.Artifact{art}, Params: core.Values{"format": "txt", "language": "eng", "dpi": 300}, Target: core.Target{Dir: out}}
	if err := op.Validate(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	result, err := op.Run(context.Background(), req, nopReporter{})
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(result.Artifacts[0].Path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "OCR:") {
		t.Fatalf("text = %q", data)
	}
}

func TestOCRPDF(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell-script fixture")
	}
	dir := t.TempDir()
	bin := filepath.Join(dir, "bin")
	if err := os.Mkdir(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	scripts := map[string]string{
		"pdftoppm": `#!/bin/sh
prefix=""
for arg in "$@"; do prefix="$arg"; done
echo page > "$prefix-1.png"
`,
		"tesseract": `#!/bin/sh
input="$1"; base="$2"; shift 2
format="txt"
for arg in "$@"; do format="$arg"; done
echo "page text" > "$base.$format"
`,
	}
	for name, body := range scripts {
		if err := os.WriteFile(filepath.Join(bin, name), []byte(body), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	src := filepath.Join(dir, "scan.pdf")
	if err := os.WriteFile(src, []byte("%PDF-1.4"), 0o644); err != nil {
		t.Fatal(err)
	}
	art, err := core.Probe(src)
	if err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, "out")
	if err := os.Mkdir(out, 0o755); err != nil {
		t.Fatal(err)
	}
	op := OCR{Tesseract: tesseract.NewClient("tesseract"), Poppler: poppler.NewClient("")}
	req := core.Request{Items: []core.Artifact{art}, Params: core.Values{"format": "txt", "language": "eng", "dpi": 300}, Target: core.Target{Dir: out}}
	if err := op.Validate(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	result, err := op.Run(context.Background(), req, nopReporter{})
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(result.Artifacts[0].Path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "page text") {
		t.Fatalf("text = %q", data)
	}
}
