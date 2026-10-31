package ffmpeg

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/solomonadzape95/workbench/internal/core"
)

type recordReporter struct {
	stages   []string
	progress [][2]int
}

func (r *recordReporter) Stage(name string) { r.stages = append(r.stages, name) }
func (r *recordReporter) Progress(done, total int) {
	r.progress = append(r.progress, [2]int{done, total})
}
func (r *recordReporter) Detail(core.Artifact, string)   {}
func (r *recordReporter) Log(core.Level, string, ...any) {}

func writeExecutable(t *testing.T, dir, name, body string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestRunReportsProgress(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell-script fixture")
	}
	dir := t.TempDir()
	writeExecutable(t, dir, "ffmpeg", `#!/bin/sh
echo "out_time_us=250000"
echo "progress=continue"
echo "out_time_us=500000"
echo "progress=continue"
echo "out_time_us=1000000"
echo "progress=end"
`)
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))

	rep := &recordReporter{}
	err := NewClient("").Run(context.Background(), []string{"-i", "in", "out"}, time.Second, rep)
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.progress) < 2 {
		t.Fatalf("progress reports = %v, want at least 2", rep.progress)
	}
	last := rep.progress[len(rep.progress)-1]
	if last[0] != 1000 || last[1] != 1000 {
		t.Errorf("last progress = %v, want 1000/1000", last)
	}
}

func TestRunMissingBinary(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	rep := &recordReporter{}
	err := NewClient("").Run(context.Background(), nil, 0, rep)
	if err == nil || core.CodeOf(err) != core.CodeEngineMissing {
		t.Fatalf("error = %v, want engine_missing", err)
	}
	if core.HintOf(err) == "" {
		t.Error("missing engine has no hint")
	}
}

func TestRunMapsFFmpegFailure(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell-script fixture")
	}
	dir := t.TempDir()
	writeExecutable(t, dir, "ffmpeg", `#!/bin/sh
echo "Invalid data found when processing input" >&2
exit 1
`)
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))

	err := NewClient("").Run(context.Background(), nil, 0, &recordReporter{})
	if err == nil || core.CodeOf(err) != core.CodeEngineFailed {
		t.Fatalf("error = %v, want engine_failed", err)
	}
	if hint := core.HintOf(err); !strings.Contains(hint, "corrupt") {
		t.Errorf("hint = %q, want a corrupt-file hint", hint)
	}
}

func TestRunCancelKillsProcessGroup(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("process-group fixture")
	}
	dir := t.TempDir()
	writeExecutable(t, dir, "ffmpeg", "#!/bin/sh\nsleep 30\n")
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- NewClient("").Run(ctx, nil, 0, &recordReporter{})
	}()

	time.Sleep(150 * time.Millisecond)
	start := time.Now()
	cancel()

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("canceled run returned no error")
		}
		if time.Since(start) > 3*time.Second {
			t.Errorf("cancellation took %s, want < 3s", time.Since(start))
		}
	case <-time.After(5 * time.Second):
		t.Fatal("canceled run did not return")
	}
}

func TestProbeParsesMedia(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell-script fixture")
	}
	dir := t.TempDir()
	writeExecutable(t, dir, "ffmpeg", "#!/bin/sh\nexit 0\n")
	writeExecutable(t, dir, "ffprobe", `#!/bin/sh
cat <<'JSON'
{"format":{"duration":"1.500000","bit_rate":"128000"},"streams":[
 {"codec_type":"video","codec_name":"h264","width":320,"height":240},
 {"codec_type":"audio","codec_name":"aac","sample_rate":"48000"}]}
JSON
`)
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))

	media, err := NewClient("").Probe(context.Background(), "/tmp/fake.mp4")
	if err != nil {
		t.Fatal(err)
	}
	if media.Duration != 1500*time.Millisecond {
		t.Errorf("duration = %s, want 1.5s", media.Duration)
	}
	if !media.HasVideo || !media.HasAudio {
		t.Errorf("streams = %+v, want video and audio", media)
	}
	if media.Width != 320 || media.Height != 240 {
		t.Errorf("size = %dx%d, want 320x240", media.Width, media.Height)
	}
	if media.SampleRate != 48000 {
		t.Errorf("sample rate = %d, want 48000", media.SampleRate)
	}
}
