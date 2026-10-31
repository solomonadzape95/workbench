package doc

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/solomonadzape95/workbench/internal/core"
)

func TestEditRunsConfiguredEditor(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell-script fixture")
	}
	dir := t.TempDir()
	editor := filepath.Join(dir, "editor.sh")
	if err := os.WriteFile(editor, []byte("#!/bin/sh\nprintf '\\nedited\\n' >> \"$1\"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	src := filepath.Join(dir, "notes.md")
	if err := os.WriteFile(src, []byte("# Notes\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	art, err := core.Probe(src)
	if err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, "out")
	req := core.Request{
		Items:  []core.Artifact{art},
		Params: core.Values{"editor": editor, "format": "md"},
		Target: core.Target{Dir: out},
	}
	op := Edit{}
	if err := op.Validate(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	result, err := op.Run(context.Background(), req, nopReporter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Artifacts) != 1 || filepath.Base(result.Artifacts[0].Path) != "notes.edited.md" {
		t.Fatalf("artifacts = %+v", result.Artifacts)
	}
	data, err := os.ReadFile(result.Artifacts[0].Path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "# Notes") || !strings.Contains(string(data), "edited") {
		t.Fatalf("edited output = %q", data)
	}
}
