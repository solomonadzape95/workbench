package video

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/solomonadzape95/workbench/internal/core"
	"github.com/solomonadzape95/workbench/internal/engines/ffmpeg"
	"github.com/solomonadzape95/workbench/internal/engines/ytdlp"
)

func fakeBin(t *testing.T, dir, name, body string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("shell-script fixture")
	}
	if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
}

func TestDownloadRealAdapterWithFakeYtDlp(t *testing.T) {
	dir := t.TempDir()
	fakeBin(t, dir, "ffmpeg", "#!/bin/sh\nexit 0\n")
	fakeBin(t, dir, "yt-dlp", `#!/bin/sh
out=""
while [ $# -gt 0 ]; do
  case "$1" in -P) out="$2"; shift 2;; *) shift;; esac
done
echo "workbench:1000|1000|1000"
printf 'video' > "$out/clip [abc].mp4"
`)
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))

	op := Download{
		YtDlp:  ytdlp.NewClient(""),
		FFmpeg: ffmpeg.NewClient(""),
	}
	req := core.Request{
		Items: []core.Artifact{{
			Path: "https://example.com/watch?v=abc", Kind: core.Video, Format: "url",
			Label: "https://example.com/watch?v=abc",
		}},
		Params: core.Values{"quality": "720p", "container": "mp4", "playlist": false},
		Target: core.Target{Dir: t.TempDir()},
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
	art := result.Artifacts[0]
	if art.SuggestedName != "clip [abc].mp4" || art.Format != "mp4" {
		t.Errorf("artifact = %+v", art)
	}
}

func TestDownloadRejectsBadURL(t *testing.T) {
	dir := t.TempDir()
	fakeBin(t, dir, "ffmpeg", "#!/bin/sh\nexit 0\n")
	fakeBin(t, dir, "yt-dlp", "#!/bin/sh\nexit 0\n")
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	op := Download{YtDlp: ytdlp.NewClient(""), FFmpeg: ffmpeg.NewClient("")}
	req := core.Request{
		Items:  []core.Artifact{{Path: "ftp://example.com/v", Kind: core.Video, Format: "url"}},
		Params: core.Values{"quality": "best", "container": "mp4", "playlist": false},
		Target: core.Target{Dir: t.TempDir()},
	}
	err := op.Validate(context.Background(), req)
	if err == nil || !strings.Contains(err.Error(), "http") {
		t.Fatalf("error = %v, want a URL scheme error", err)
	}
}

func TestTranscriptWithFakeYtDlp(t *testing.T) {
	dir := t.TempDir()
	fakeBin(t, dir, "yt-dlp", `#!/bin/sh
out=""
while [ $# -gt 0 ]; do
  case "$1" in -P) out="$2"; shift 2;; *) shift;; esac
done
cat > "$out/Clip [abc].en.vtt" <<'VTT'
WEBVTT

00:00:01.000 --> 00:00:02.000
Hello world
VTT
`)
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	op := Transcript{YtDlp: ytdlp.NewClient("")}
	work := t.TempDir()
	req := core.Request{
		Items:  []core.Artifact{{Path: "https://example.com/v", Kind: core.Unknown, Label: "https://example.com/v"}},
		Params: core.Values{"language": "en", "auto": true, "timestamps": false},
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
		t.Fatalf("artifacts = %+v", result.Artifacts)
	}
	art := result.Artifacts[0]
	if art.Format != "txt" || art.Kind != core.Document || art.SuggestedName != "Clip [abc].txt" {
		t.Errorf("artifact = %+v", art)
	}
	data, err := os.ReadFile(art.Path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(data)) != "Hello world" {
		t.Errorf("transcript = %q", data)
	}
}

func TestInfoWithFakeYtDlp(t *testing.T) {
	dir := t.TempDir()
	fakeBin(t, dir, "yt-dlp", `#!/bin/sh
echo '{"id":"abc","title":"Clip","duration":3,"view_count":7}'
`)
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	op := Info{YtDlp: ytdlp.NewClient("")}
	work := t.TempDir()
	req := core.Request{
		Items:  []core.Artifact{{Path: "https://example.com/v", Kind: core.Unknown, Label: "https://example.com/v"}},
		Params: core.Values{"playlist": false},
		Target: core.Target{Dir: work},
	}
	if err := op.Validate(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	result, err := op.Run(context.Background(), req, nopReporter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Artifacts) != 1 || result.Artifacts[0].Format != "json" {
		t.Fatalf("artifacts = %+v", result.Artifacts)
	}
	data, err := os.ReadFile(result.Artifacts[0].Path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"title": "Clip"`) {
		t.Errorf("info = %s", data)
	}
}
