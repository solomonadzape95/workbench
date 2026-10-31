package file

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/solomonadzape95/workbench/internal/core"
)

func TestSplitWritesNumberedParts(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "blob.bin")
	if err := os.WriteFile(src, []byte("abcdefghij"), 0o644); err != nil {
		t.Fatal(err)
	}
	artifact, err := core.Probe(src)
	if err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, "out")
	if err := os.Mkdir(out, 0o755); err != nil {
		t.Fatal(err)
	}
	req := core.Request{
		Items:  []core.Artifact{artifact},
		Params: core.Values{"size": "3"},
		Target: core.Target{Dir: out},
	}
	if err := (Split{}).Validate(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	result, err := (Split{}).Run(context.Background(), req, nopReporter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Artifacts) != 4 {
		t.Fatalf("parts = %d, want 4: %+v", len(result.Artifacts), result.Artifacts)
	}
	var joined strings.Builder
	for _, part := range result.Artifacts {
		data, err := os.ReadFile(part.Path)
		if err != nil {
			t.Fatal(err)
		}
		joined.Write(data)
	}
	if joined.String() != "abcdefghij" {
		t.Fatalf("joined = %q", joined.String())
	}
	if result.Metrics.BytesOut != 10 || result.Artifacts[0].SuggestedName == "" {
		t.Fatalf("metrics = %+v meta = %+v", result.Metrics, result.Artifacts[0])
	}
}

func TestSplitRejectsBadSize(t *testing.T) {
	err := (Split{}).Validate(context.Background(), core.Request{
		Items:  []core.Artifact{{Path: "/tmp/a"}},
		Params: core.Values{"size": "nope"},
	})
	if err == nil {
		t.Fatal("bad size passed validation")
	}
}
