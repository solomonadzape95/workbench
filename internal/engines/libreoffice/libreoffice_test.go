package libreoffice

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/solomonadzape95/workbench/internal/core"
)

type nopReporter struct{}

func (nopReporter) Stage(string)                   {}
func (nopReporter) Progress(int, int)              {}
func (nopReporter) Detail(core.Artifact, string)   {}
func (nopReporter) Log(core.Level, string, ...any) {}

func TestPDFUsesWriterImportFilter(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell-script fixture")
	}
	dir := t.TempDir()
	argsPath := filepath.Join(dir, "args")
	script := `#!/bin/sh
printf '%s\n' "$@" > ` + argsPath + `
outdir=""; target=""; input=""
while [ $# -gt 0 ]; do
  case "$1" in
    --outdir) outdir="$2"; shift 2;;
    --convert-to) target="$2"; shift 2;;
    -env:*|--infilter=*|--headless|--norestore|--invisible) shift;;
    *) input="$1"; shift;;
  esac
done
base=$(basename "$input"); stem=${base%.*}
printf 'converted' > "$outdir/$stem.$target"
`
	if err := os.WriteFile(filepath.Join(dir, "soffice"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))

	input := filepath.Join(dir, "paper.pdf")
	if err := os.WriteFile(input, []byte("%PDF-1.4\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	outDir := t.TempDir()
	out, err := NewClient("").Convert(context.Background(), input, outDir, "docx", nopReporter{})
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(out) != "paper.docx" {
		t.Fatalf("output = %q", out)
	}
	args, err := os.ReadFile(argsPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(args), "--infilter=writer_pdf_import") {
		t.Fatalf("PDF conversion did not request the Writer PDF import filter:\n%s", args)
	}
}
