package audio

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/solomonadzape95/workbench/internal/core"
	"github.com/solomonadzape95/workbench/internal/engines/ffmpeg"
)

func TestJoinAudio(t *testing.T) {
	dir := t.TempDir()
	firstDir := filepath.Join(dir, "one")
	secondDir := filepath.Join(dir, "two")
	if err := os.Mkdir(firstDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(secondDir, 0o755); err != nil {
		t.Fatal(err)
	}
	first := tone(t, firstDir)
	second := tone(t, secondDir)
	out := filepath.Join(dir, "out")
	if err := os.Mkdir(out, 0o755); err != nil {
		t.Fatal(err)
	}
	client := ffmpeg.NewClient("")
	op := Join{Client: client}
	req := core.Request{
		Items:  []core.Artifact{first, second},
		Params: core.Values{"format": "wav", "bitrate": "192k", "crossfade": "0s"},
		Target: core.Target{Dir: out},
	}
	if err := op.Validate(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	result, err := op.Run(context.Background(), req, nopReporter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Artifacts) != 1 || result.Artifacts[0].Format != "wav" {
		t.Fatalf("artifacts = %+v", result.Artifacts)
	}
	probe, err := client.Probe(context.Background(), result.Artifacts[0].Path)
	if err != nil {
		t.Fatal(err)
	}
	if !probe.HasAudio {
		t.Error("joined file has no audio")
	}
}
