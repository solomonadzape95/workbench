package video

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/solomonadzape95/workbench/internal/core"
	"github.com/solomonadzape95/workbench/internal/engines/ffmpeg"
	"github.com/solomonadzape95/workbench/internal/engines/whisper"
)

func TestVideoTranscribeWithFakeWhisper(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake whisper needs a POSIX shell")
	}
	dir := t.TempDir()
	src := clip(t, dir)
	out := outputDir(t, dir)

	fakeBin := filepath.Join(dir, "bin")
	if err := os.Mkdir(fakeBin, 0o755); err != nil {
		t.Fatal(err)
	}
	script := `#!/bin/sh
prefix=""
while [ $# -gt 0 ]; do
  case "$1" in
    -of) prefix="$2"; shift 2;;
    -otxt|-osrt|-ovtt|-oj) shift;;
    *) shift;;
  esac
done
echo "hello from video" > "$prefix.txt"
`
	if err := os.WriteFile(filepath.Join(fakeBin, "whisper-cli"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", fakeBin+string(os.PathListSeparator)+os.Getenv("PATH"))
	modelDir := filepath.Join(dir, "models")
	if err := os.Mkdir(modelDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(modelDir, "ggml-small.en.bin"), []byte("model"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("WORKBENCH_WHISPER_MODEL_DIR", modelDir)

	op := Transcribe{Whisper: whisper.NewClient(""), FFmpeg: ffmpeg.NewClient("")}
	req := core.Request{
		Items:  []core.Artifact{src},
		Params: core.Values{"model": "small.en", "language": "en", "format": "txt"},
		Target: core.Target{Dir: out},
	}
	if err := op.Validate(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	result, err := op.Run(context.Background(), req, nopReporter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Artifacts) != 1 || result.Artifacts[0].Kind != core.Document {
		t.Fatalf("artifacts = %+v", result.Artifacts)
	}
	data, err := os.ReadFile(result.Artifacts[0].Path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "hello from video\n" {
		t.Fatalf("transcript = %q", data)
	}
}
