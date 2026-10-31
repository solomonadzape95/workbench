package file

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/solomonadzape95/workbench/internal/core"
)

func TestMoveMovesSourceAndSuffixesCollision(t *testing.T) {
	dir := t.TempDir()
	srcDir := filepath.Join(dir, "src")
	outDir := filepath.Join(dir, "out")
	if err := os.MkdirAll(srcDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		t.Fatal(err)
	}
	src := filepath.Join(srcDir, "a.txt")
	if err := os.WriteFile(src, []byte("new"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(outDir, "a.txt"), []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	art, err := core.Probe(src)
	if err != nil {
		t.Fatal(err)
	}
	result, err := (Move{}).Run(context.Background(), core.Request{
		Items:  []core.Artifact{art},
		Target: core.Target{Dir: outDir, Collision: core.CollisionSuffix},
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
	if got, want := filepath.Base(result.Artifacts[0].Path), "a (1).txt"; got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
	data, err := os.ReadFile(result.Artifacts[0].Path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "new" {
		t.Fatalf("output = %q", data)
	}
	if data, _ := os.ReadFile(filepath.Join(outDir, "a.txt")); string(data) != "old" {
		t.Fatalf("collision overwrote existing file: %q", data)
	}
}

func TestRenameUsesCounterTemplate(t *testing.T) {
	dir := t.TempDir()
	outDir := filepath.Join(dir, "out")
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		t.Fatal(err)
	}
	var items []core.Artifact
	for _, name := range []string{"one.txt", "two.txt"} {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte(name), 0o644); err != nil {
			t.Fatal(err)
		}
		art, err := core.Probe(path)
		if err != nil {
			t.Fatal(err)
		}
		items = append(items, art)
	}
	req := core.Request{
		Items:  items,
		Params: core.Values{"template": "renamed-{counter}.{ext}"},
		Target: core.Target{Dir: outDir, Collision: core.CollisionSuffix},
	}
	if err := (Rename{}).Validate(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	result, err := (Rename{}).Run(context.Background(), req, nopReporter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Artifacts) != 2 {
		t.Fatalf("artifacts = %+v", result.Artifacts)
	}
	for i, want := range []string{"renamed-1.txt", "renamed-2.txt"} {
		if got := filepath.Base(result.Artifacts[i].Path); got != want {
			t.Errorf("artifact %d = %q, want %q", i, got, want)
		}
	}
	for _, item := range items {
		if _, err := os.Stat(item.Path); !os.IsNotExist(err) {
			t.Errorf("source %s still exists", item.Path)
		}
	}
}

func TestMoveRejectsSamePathWithOverwrite(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "a.txt")
	if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	art, err := core.Probe(path)
	if err != nil {
		t.Fatal(err)
	}
	_, err = (Move{}).Run(context.Background(), core.Request{
		Items:  []core.Artifact{art},
		Target: core.Target{Dir: dir, Collision: core.CollisionOverwrite},
	}, nopReporter{})
	if err == nil {
		t.Fatal("same-path move succeeded")
	}
	if _, statErr := os.Stat(path); statErr != nil {
		t.Fatalf("source was removed: %v", statErr)
	}
}
