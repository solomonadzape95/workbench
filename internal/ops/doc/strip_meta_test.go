package doc

import (
	"archive/zip"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/solomonadzape95/workbench/internal/core"
)

func TestStripMetaDOCX(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "doc.docx")
	f, err := os.Create(src)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(f)
	for name, body := range map[string]string{
		"[Content_Types].xml": "types",
		"docProps/core.xml":   "author",
		"word/document.xml":   "body",
	} {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = w.Write([]byte(body))
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	f.Close()
	art, err := core.Probe(src)
	if err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, "out")
	if err := os.Mkdir(out, 0o755); err != nil {
		t.Fatal(err)
	}
	req := core.Request{Items: []core.Artifact{art}, Target: core.Target{Dir: out}}
	if err := (StripMeta{}).Validate(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	result, err := (StripMeta{}).Run(context.Background(), req, nopReporter{})
	if err != nil {
		t.Fatal(err)
	}
	r, err := zip.OpenReader(result.Artifacts[0].Path)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	names := map[string]bool{}
	for _, entry := range r.File {
		names[entry.Name] = true
	}
	if names["docProps/core.xml"] {
		t.Error("metadata entry survived")
	}
	if !names["word/document.xml"] || !names["[Content_Types].xml"] {
		t.Errorf("content lost: %v", names)
	}
}
