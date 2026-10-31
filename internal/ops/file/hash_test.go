package file

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/solomonadzape95/workbench/internal/core"
)

type nopReporter struct{}

func (nopReporter) Stage(string)                   {}
func (nopReporter) Progress(int, int)              {}
func (nopReporter) Detail(core.Artifact, string)   {}
func (nopReporter) Log(core.Level, string, ...any) {}

func TestHashWritesManifest(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "hello.txt")
	if err := os.WriteFile(src, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	art, err := core.Probe(src)
	if err != nil {
		t.Fatal(err)
	}
	work := filepath.Join(dir, "work")
	if err := os.Mkdir(work, 0o755); err != nil {
		t.Fatal(err)
	}

	op := Hash{}
	req := core.Request{
		Items:  []core.Artifact{art},
		Params: core.Values{"algo": "sha256"},
		Target: core.Target{Dir: work},
	}
	if err := op.Validate(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	result, err := op.Run(context.Background(), req, nopReporter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Artifacts) != 1 {
		t.Fatalf("got %d artifacts, want 1", len(result.Artifacts))
	}

	data, err := os.ReadFile(result.Artifacts[0].Path)
	if err != nil {
		t.Fatal(err)
	}
	// sha256("hello")
	const want = "2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824"
	if !strings.Contains(string(data), want) || !strings.Contains(string(data), "hello.txt") {
		t.Errorf("manifest = %q, want digest and filename", data)
	}
}

func TestHashRejectsUnknownAlgorithm(t *testing.T) {
	req := core.Request{
		Items:  []core.Artifact{{Path: "/tmp/a", Kind: core.File}},
		Params: core.Values{"algo": "crc32"},
	}
	if err := (Hash{}).Validate(context.Background(), req); err == nil {
		t.Fatal("unknown algorithm passed validation")
	}
}
