package file

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/solomonadzape95/workbench/internal/core"
)

func TestTrashMovesFileToRecoverableFolder(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "keep.txt")
	if err := os.WriteFile(src, []byte("precious"), 0o644); err != nil {
		t.Fatal(err)
	}
	art, err := core.Probe(src)
	if err != nil {
		t.Fatal(err)
	}
	trashDir := filepath.Join(dir, "trash")
	result, err := (Trash{}).Run(context.Background(), core.Request{
		Items:  []core.Artifact{art},
		Params: core.Values{"trash_dir": trashDir},
		Target: core.Target{Dir: filepath.Join(dir, "out")},
	}, nopReporter{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(src); !os.IsNotExist(err) {
		t.Fatalf("source still exists: %v", err)
	}
	if len(result.Artifacts) != 1 {
		t.Fatalf("artifacts = %+v", result.Artifacts)
	}
	trashed := result.Artifacts[0].Path
	if rel, err := filepath.Rel(trashDir, trashed); err != nil || rel == ".." || filepath.IsAbs(rel) {
		t.Fatalf("trashed path %q is outside %q", trashed, trashDir)
	}
	data, err := os.ReadFile(trashed)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "precious" {
		t.Fatalf("trashed contents = %q", data)
	}
	if result.Artifacts[0].Meta["trashed"] != true {
		t.Fatalf("meta = %+v", result.Artifacts[0].Meta)
	}
}
