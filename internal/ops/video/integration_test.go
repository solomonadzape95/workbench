package video

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/solomonadzape95/workbench/internal/core"
	"github.com/solomonadzape95/workbench/internal/engines/ffmpeg"
)

type nopReporter struct{}

func (nopReporter) Stage(string)                   {}
func (nopReporter) Progress(int, int)              {}
func (nopReporter) Detail(core.Artifact, string)   {}
func (nopReporter) Log(core.Level, string, ...any) {}

func clip(t *testing.T, dir string) core.Artifact {
	t.Helper()
	ffmpegPath, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg is not installed")
	}
	path := filepath.Join(dir, "clip.mp4")
	cmd := exec.Command(ffmpegPath,
		"-hide_banner", "-loglevel", "error", "-y",
		"-f", "lavfi", "-i", "testsrc=duration=1:size=160x120:rate=10",
		"-f", "lavfi", "-i", "sine=frequency=440:duration=1",
		"-shortest",
		"-c:v", "libx264", "-pix_fmt", "yuv420p",
		"-c:a", "aac",
		path,
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Skipf("cannot generate test video: %v (%s)", err, out)
	}
	art, err := core.Probe(path)
	if err != nil {
		t.Fatal(err)
	}
	return art
}

func outputDir(t *testing.T, dir string) string {
	t.Helper()
	out := filepath.Join(dir, "out")
	if err := os.Mkdir(out, 0o755); err != nil {
		t.Fatal(err)
	}
	return out
}

func TestExtractAudioRealVideo(t *testing.T) {
	dir := t.TempDir()
	src := clip(t, dir)
	client := ffmpeg.NewClient("")
	op := Extract{Client: client}
	req := core.Request{
		Items:  []core.Artifact{src},
		Params: core.Values{"format": "mp3", "bitrate": "128k"},
		Target: core.Target{Dir: outputDir(t, dir)},
	}
	if err := op.Validate(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	result, err := op.Run(context.Background(), req, nopReporter{})
	if err != nil {
		t.Fatal(err)
	}
	if result.Artifacts[0].Kind != core.Audio {
		t.Errorf("kind = %q, want audio", result.Artifacts[0].Kind)
	}
	media, err := client.Probe(context.Background(), result.Artifacts[0].Path)
	if err != nil {
		t.Fatal(err)
	}
	if !media.HasAudio || media.HasVideo {
		t.Errorf("extracted media = %+v, want audio only", media)
	}
}

func TestConvertRealVideo(t *testing.T) {
	dir := t.TempDir()
	src := clip(t, dir)
	client := ffmpeg.NewClient("")
	op := Convert{Client: client}
	req := core.Request{
		Items: []core.Artifact{src},
		Params: core.Values{
			"format": "mkv", "codec": "auto",
			"crf": 30, "preset": "ultrafast",
		},
		Target: core.Target{Dir: outputDir(t, dir)},
	}
	if err := op.Validate(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	result, err := op.Run(context.Background(), req, nopReporter{})
	if err != nil {
		t.Fatal(err)
	}
	if result.Artifacts[0].Format != "mkv" {
		t.Errorf("format = %q, want mkv", result.Artifacts[0].Format)
	}
	media, err := client.Probe(context.Background(), result.Artifacts[0].Path)
	if err != nil {
		t.Fatal(err)
	}
	if !media.HasVideo {
		t.Error("converted file has no video stream")
	}
}

func TestSpeedRealVideo(t *testing.T) {
	dir := t.TempDir()
	src := clip(t, dir)
	client := ffmpeg.NewClient("")
	op := Speed{Client: client}
	req := core.Request{
		Items:  []core.Artifact{src},
		Params: core.Values{"factor": 2.0, "with_audio": true},
		Target: core.Target{Dir: outputDir(t, dir)},
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
	if media.Duration < 400*time.Millisecond || media.Duration > 700*time.Millisecond {
		t.Errorf("sped-up duration = %s, want about 0.5s", media.Duration)
	}
}
