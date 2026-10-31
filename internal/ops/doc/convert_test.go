package doc

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/solomonadzape95/workbench/internal/core"
	"github.com/solomonadzape95/workbench/internal/engines/libreoffice"
	"github.com/solomonadzape95/workbench/internal/engines/pandoc"
)

type nopReporter struct{}

func (nopReporter) Stage(string)                   {}
func (nopReporter) Progress(int, int)              {}
func (nopReporter) Detail(core.Artifact, string)   {}
func (nopReporter) Log(core.Level, string, ...any) {}

func fakeBin(t *testing.T, name, body string) {
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

func source(t *testing.T, name string) core.Artifact {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte("source document"), 0o644); err != nil {
		t.Fatal(err)
	}
	art, err := core.Probe(path)
	if err != nil {
		t.Fatal(err)
	}
	return art
}

func TestPDFToDOCXWithLibreOffice(t *testing.T) {
	fakeBin(t, "soffice", `#!/bin/sh
outdir=""; target=""; input=""
while [ $# -gt 0 ]; do
  case "$1" in
    --outdir) outdir="$2"; shift 2;;
    --convert-to) target="$2"; shift 2;;
    -env:*) shift;;
    --headless|--norestore|--invisible) shift;;
    *) input="$1"; shift;;
  esac
done
base=$(basename "$input"); stem=${base%.*}
echo converted > "$outdir/$stem.$target"
`)
	src := source(t, "paper.pdf")
	outDir := t.TempDir()
	op := Convert{LibreOffice: libreoffice.NewClient("")}
	req := core.Request{
		Items:  []core.Artifact{src},
		Params: core.Values{"format": "docx", "engine": "auto"},
		Target: core.Target{Dir: outDir},
	}
	if err := op.Validate(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	result, err := op.Run(context.Background(), req, nopReporter{})
	if err != nil {
		t.Fatal(err)
	}
	if result.Artifacts[0].Format != "docx" || result.Artifacts[0].Meta["engine"] != "libreoffice" {
		t.Errorf("artifact = %+v", result.Artifacts[0])
	}
	data, err := os.ReadFile(result.Artifacts[0].Path)
	if err != nil || !strings.Contains(string(data), "converted") {
		t.Errorf("output = %q, %v", data, err)
	}
}

func TestMarkdownToHTMLWithPandoc(t *testing.T) {
	fakeBin(t, "pandoc", `#!/bin/sh
input="$1"; shift
out=""
while [ $# -gt 0 ]; do
  case "$1" in
    -o) out="$2"; shift 2;;
    -t) shift 2;;
    *) shift;;
  esac
done
cp "$input" "$out"
`)
	src := source(t, "notes.md")
	outDir := t.TempDir()
	op := Convert{Pandoc: pandoc.NewClient("")}
	req := core.Request{
		Items:  []core.Artifact{src},
		Params: core.Values{"format": "html", "engine": "auto"},
		Target: core.Target{Dir: outDir},
	}
	if err := op.Validate(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	result, err := op.Run(context.Background(), req, nopReporter{})
	if err != nil {
		t.Fatal(err)
	}
	if result.Artifacts[0].Format != "html" || result.Artifacts[0].Meta["engine"] != "pandoc" {
		t.Errorf("artifact = %+v", result.Artifacts[0])
	}
}

func TestNoDocumentEngine(t *testing.T) {
	src := source(t, "paper.pdf")
	op := Convert{}
	req := core.Request{
		Items:  []core.Artifact{src},
		Params: core.Values{"format": "docx", "engine": "auto"},
		Target: core.Target{Dir: t.TempDir()},
	}
	err := op.Validate(context.Background(), req)
	if err == nil || core.CodeOf(err) != core.CodeEngineMissing {
		t.Fatalf("error = %v, want engine_missing", err)
	}
	if !strings.Contains(core.HintOf(err), "install") {
		t.Errorf("hint = %q", core.HintOf(err))
	}
}

func TestPDFToMarkdownExplainsChain(t *testing.T) {
	fakeBin(t, "soffice", "#!/bin/sh\nexit 0\n")
	src := source(t, "paper.pdf")
	op := Convert{LibreOffice: libreoffice.NewClient("")}
	req := core.Request{
		Items:  []core.Artifact{src},
		Params: core.Values{"format": "md", "engine": "auto"},
		Target: core.Target{Dir: t.TempDir()},
	}
	err := op.Validate(context.Background(), req)
	if err == nil {
		t.Fatal("PDF→Markdown passed validation")
	}
	if !strings.Contains(core.HintOf(err), "DOCX") {
		t.Errorf("hint = %q, want a two-step hint", core.HintOf(err))
	}
}

func TestExtractTextWithPandoc(t *testing.T) {
	fakeBin(t, "pandoc", `#!/bin/sh
input="$1"; shift
out=""
while [ $# -gt 0 ]; do
  case "$1" in
    -o) out="$2"; shift 2;;
    -t) shift 2;;
    *) shift;;
  esac
done
cp "$input" "$out"
`)
	src := source(t, "notes.md")
	op := ExtractText{Convert: Convert{Pandoc: pandoc.NewClient("")}}
	req := core.Request{
		Items:  []core.Artifact{src},
		Params: core.Values{"format": "txt", "engine": "auto"},
		Target: core.Target{Dir: t.TempDir()},
	}
	if err := op.Validate(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	result, err := op.Run(context.Background(), req, nopReporter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Artifacts) != 1 || result.Artifacts[0].Format != "txt" {
		t.Fatalf("artifacts = %+v", result.Artifacts)
	}
}
