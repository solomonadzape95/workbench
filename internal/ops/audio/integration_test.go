package audio

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/solomonadzape95/workbench/internal/core"
	"github.com/solomonadzape95/workbench/internal/engines/ffmpeg"
	"github.com/solomonadzape95/workbench/internal/engines/whisper"
)

type nopReporter struct{}

func (nopReporter) Stage(string)                   {}
func (nopReporter) Progress(int, int)              {}
func (nopReporter) Detail(core.Artifact, string)   {}
func (nopReporter) Log(core.Level, string, ...any) {}

func tone(t *testing.T, dir string) core.Artifact {
	t.Helper()
	ffmpegPath, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg is not installed")
	}
	path := filepath.Join(dir, "tone.wav")
	cmd := exec.Command(ffmpegPath,
		"-hide_banner", "-loglevel", "error", "-y",
		"-f", "lavfi", "-i", "sine=frequency=440:duration=1",
		"-ac", "2", "-ar", "44100",
		path,
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Skipf("cannot generate test audio: %v (%s)", err, out)
	}
	art, err := core.Probe(path)
	if err != nil {
		t.Fatal(err)
	}
	return art
}

func TestConvertRealAudio(t *testing.T) {
	dir := t.TempDir()
	src := tone(t, dir)
	outDir := filepath.Join(dir, "out")
	if err := os.Mkdir(outDir, 0o755); err != nil {
		t.Fatal(err)
	}
	client := ffmpeg.NewClient("")
	op := Convert{Client: client}
	req := core.Request{
		Items: []core.Artifact{src},
		Params: core.Values{
			"format": "mp3", "bitrate": "128k",
			"sample_rate": "auto", "channels": "auto",
		},
		Target: core.Target{Dir: outDir},
	}
	if err := op.Validate(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	result, err := op.Run(context.Background(), req, nopReporter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Artifacts) != 1 || result.Artifacts[0].Format != "mp3" {
		t.Fatalf("artifacts = %+v, want one mp3", result.Artifacts)
	}
	media, err := client.Probe(context.Background(), result.Artifacts[0].Path)
	if err != nil {
		t.Fatal(err)
	}
	if !media.HasAudio {
		t.Error("converted file has no audio stream")
	}
	if media.Duration < 900*time.Millisecond || media.Duration > 1100*time.Millisecond {
		t.Errorf("duration = %s, want about 1s", media.Duration)
	}
}

func TestSpeedRealAudio(t *testing.T) {
	dir := t.TempDir()
	src := tone(t, dir)
	outDir := filepath.Join(dir, "out")
	if err := os.Mkdir(outDir, 0o755); err != nil {
		t.Fatal(err)
	}
	client := ffmpeg.NewClient("")
	op := Speed{Client: client}
	req := core.Request{
		Items:  []core.Artifact{src},
		Params: core.Values{"factor": 2.0, "preserve_pitch": true},
		Target: core.Target{Dir: outDir},
	}
	if err := op.Validate(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	result, err := op.Run(context.Background(), req, nopReporter{})
	if err != nil {
		t.Fatal(err)
	}
	media, err := client.Probe(context.Background(), result.Artifacts[0].Path)
	if err != nil {
		t.Fatal(err)
	}
	if media.Duration < 400*time.Millisecond || media.Duration > 600*time.Millisecond {
		t.Errorf("sped-up duration = %s, want about 0.5s", media.Duration)
	}
}

func TestTranscribeRealSpeech(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("say is macOS-only")
	}
	whisperPath, err := exec.LookPath("whisper-cli")
	if err != nil {
		t.Skip("whisper-cli is not installed")
	}
	model, err := whisper.NewClient("").ModelPath("small.en")
	if err != nil {
		t.Skipf("whisper model not installed: %v", err)
	}
	ffmpegPath, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg is not installed")
	}
	dir := t.TempDir()
	aiff := filepath.Join(dir, "speech.aiff")
	wav := filepath.Join(dir, "speech.wav")
	if out, err := exec.Command("say", "-o", aiff, "Hello from Workbench. This is a transcription test.").CombinedOutput(); err != nil {
		t.Skipf("say failed: %v (%s)", err, out)
	}
	convert := exec.Command(ffmpegPath, "-hide_banner", "-loglevel", "error", "-y", "-i", aiff, "-ar", "16000", "-ac", "1", wav)
	if out, err := convert.CombinedOutput(); err != nil {
		t.Fatalf("converting speech: %v (%s)", err, out)
	}
	art, err := core.Probe(wav)
	if err != nil {
		t.Fatal(err)
	}
	outDir := t.TempDir()
	op := Transcribe{Whisper: whisper.NewClient(whisperPath), FFmpeg: ffmpeg.NewClient("")}
	req := core.Request{
		Items:  []core.Artifact{art},
		Params: core.Values{"model": "small.en", "language": "en", "format": "txt"},
		Target: core.Target{Dir: outDir},
	}
	if err := op.Validate(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	result, err := op.Run(context.Background(), req, nopReporter{})
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(result.Artifacts[0].Path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.ToLower(string(data)), "workbench") {
		t.Fatalf("transcript = %q; model=%s", data, model)
	}
}
