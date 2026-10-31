package doc

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/solomonadzape95/workbench/internal/core"
	"github.com/solomonadzape95/workbench/internal/engines/poppler"
)

func TestSplitPDFGroups(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell-script fixture")
	}
	dir := t.TempDir()
	bin := filepath.Join(dir, "bin")
	if err := os.Mkdir(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	scripts := map[string]string{
		"pdfseparate": `#!/bin/sh
input="$1"; pattern="$2"
for i in 1 2 3; do cp "$input" "$(printf "$pattern" "$i")"; done
`,
		"pdfunite": `#!/bin/sh
out=""
for arg in "$@"; do out="$arg"; done
cp "$1" "$out"
`,
	}
	for name, body := range scripts {
		if err := os.WriteFile(filepath.Join(bin, name), []byte(body), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	src := filepath.Join(dir, "book.pdf")
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
	op := Split{Poppler: poppler.NewClient("")}
	req := core.Request{Items: []core.Artifact{art}, Params: core.Values{"every": 2}, Target: core.Target{Dir: out}}
	if err := op.Validate(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	result, err := op.Run(context.Background(), req, nopReporter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Artifacts) != 2 {
		t.Fatalf("artifacts = %+v", result.Artifacts)
	}
}
