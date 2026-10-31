package audio

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/solomonadzape95/workbench/internal/core"
	"github.com/solomonadzape95/workbench/internal/engines/demucs"
	"github.com/solomonadzape95/workbench/internal/engines/ffmpeg"
)

func TestExtractVocalsCenter(t *testing.T) {
	dir := t.TempDir()
	src := tone(t, dir)
	out := filepath.Join(dir, "out")
	if err := os.Mkdir(out, 0o755); err != nil {
		t.Fatal(err)
	}
	client := ffmpeg.NewClient("")
	op := ExtractVocals{FFmpeg: client}
	req := core.Request{
		Items:  []core.Artifact{src},
		Params: core.Values{"method": "center", "format": "wav", "bitrate": "192k"},
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
	if result.Artifacts[0].Meta["engine"] != "ffmpeg-center" {
		t.Errorf("engine meta = %v", result.Artifacts[0].Meta["engine"])
	}
	probe, err := client.Probe(context.Background(), result.Artifacts[0].Path)
	if err != nil {
		t.Fatal(err)
	}
	if !probe.HasAudio {
		t.Error("extracted vocal file has no audio")
	}
}

func TestExtractVocalsDemucs(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake demucs script needs a POSIX shell")
	}
	dir := t.TempDir()
	src := tone(t, dir)
	out := filepath.Join(dir, "out")
	if err := os.Mkdir(out, 0o755); err != nil {
		t.Fatal(err)
	}

	fakeBin := filepath.Join(dir, "bin")
	if err := os.Mkdir(fakeBin, 0o755); err != nil {
		t.Fatal(err)
	}
	script := `#!/bin/sh
out=""
input=""
prev=""
for arg in "$@"; do
  if [ "$prev" = "-o" ]; then out="$arg"; fi
  prev="$arg"
  input="$arg"
done
mkdir -p "$out/htdemucs/song"
cp "$input" "$out/htdemucs/song/vocals.wav"
`
	scriptPath := filepath.Join(fakeBin, "demucs")
	if err := os.WriteFile(scriptPath, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", fakeBin+string(os.PathListSeparator)+os.Getenv("PATH"))

	client := ffmpeg.NewClient("")
	op := ExtractVocals{FFmpeg: client, Demucs: demucs.NewClient("demucs")}
	req := core.Request{
		Items:  []core.Artifact{src},
		Params: core.Values{"method": "demucs", "model": "htdemucs", "format": "wav", "bitrate": "192k"},
		Target: core.Target{Dir: out},
	}
	if err := op.Validate(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	result, err := op.Run(context.Background(), req, nopReporter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Artifacts) != 1 {
		t.Fatalf("artifacts = %+v", result.Artifacts)
	}
	if result.Artifacts[0].Meta["engine"] != "demucs" {
		t.Errorf("engine meta = %v", result.Artifacts[0].Meta["engine"])
	}
}

func TestExtractVocalsDemucsRequiresEngine(t *testing.T) {
	dir := t.TempDir()
	src := tone(t, dir)
	op := ExtractVocals{FFmpeg: ffmpeg.NewClient("")}
	err := op.Validate(context.Background(), core.Request{
		Items:  []core.Artifact{src},
		Params: core.Values{"method": "demucs", "format": "wav"},
		Target: core.Target{Dir: dir},
	})
	if err == nil {
		t.Fatal("demucs method validated without a demucs client")
	}
}

func TestExtractInstrumentalCenter(t *testing.T) {
	dir := t.TempDir()
	src := tone(t, dir)
	out := filepath.Join(dir, "out")
	if err := os.Mkdir(out, 0o755); err != nil {
		t.Fatal(err)
	}
	op := ExtractInstrumental{FFmpeg: ffmpeg.NewClient("")}
	req := core.Request{
		Items:  []core.Artifact{src},
		Params: core.Values{"method": "center", "format": "wav", "bitrate": "192k"},
		Target: core.Target{Dir: out},
	}
	if err := op.Validate(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	result, err := op.Run(context.Background(), req, nopReporter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Artifacts) != 1 || result.Artifacts[0].Meta["stem"] != "no_vocals" {
		t.Fatalf("artifacts = %+v", result.Artifacts)
	}
}

func TestExtractInstrumentalDemucs(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake demucs script needs a POSIX shell")
	}
	dir := t.TempDir()
	src := tone(t, dir)
	out := filepath.Join(dir, "out")
	if err := os.Mkdir(out, 0o755); err != nil {
		t.Fatal(err)
	}
	fakeBin := filepath.Join(dir, "bin")
	if err := os.Mkdir(fakeBin, 0o755); err != nil {
		t.Fatal(err)
	}
	script := `#!/bin/sh
out=""; input=""; prev=""
for arg in "$@"; do
  if [ "$prev" = "-o" ]; then out="$arg"; fi
  prev="$arg"; input="$arg"
done
mkdir -p "$out/htdemucs/song"
cp "$input" "$out/htdemucs/song/vocals.wav"
cp "$input" "$out/htdemucs/song/no_vocals.wav"
`
	if err := os.WriteFile(filepath.Join(fakeBin, "demucs"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", fakeBin+string(os.PathListSeparator)+os.Getenv("PATH"))
	op := ExtractInstrumental{FFmpeg: ffmpeg.NewClient(""), Demucs: demucs.NewClient("demucs")}
	req := core.Request{
		Items:  []core.Artifact{src},
		Params: core.Values{"method": "demucs", "model": "htdemucs", "format": "wav", "bitrate": "192k"},
		Target: core.Target{Dir: out},
	}
	if err := op.Validate(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	result, err := op.Run(context.Background(), req, nopReporter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Artifacts) != 1 || result.Artifacts[0].Meta["engine"] != "demucs" {
		t.Fatalf("artifacts = %+v", result.Artifacts)
	}
}
