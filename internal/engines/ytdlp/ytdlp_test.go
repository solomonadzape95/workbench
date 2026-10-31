package ytdlp

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/solomonadzape95/workbench/internal/core"
)

type recordReporter struct {
	progress [][2]int
	logs     []core.Level
}

func (r *recordReporter) Stage(string) {}
func (r *recordReporter) Progress(done, total int) {
	r.progress = append(r.progress, [2]int{done, total})
}
func (r *recordReporter) Detail(core.Artifact, string) {}
func (r *recordReporter) Log(level core.Level, _ string, _ ...any) {
	r.logs = append(r.logs, level)
}

func fakeBin(t *testing.T, dir, name, body string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("shell-script fixture")
	}
	if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
}

func TestDownloadParsesProgressAndFindsOutput(t *testing.T) {
	dir := t.TempDir()
	fakeBin(t, dir, "yt-dlp", `#!/bin/sh
if [ "$1" = "--version" ]; then echo "2026.01.01"; exit 0; fi
out=""
while [ $# -gt 0 ]; do
  case "$1" in -P) out="$2"; shift 2;; *) shift;; esac
done
echo "workbench:500|1000|1000"
echo "workbench:1000|1000|1000"
printf 'video' > "$out/fake video [id].mp4"
`)
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	work := t.TempDir()
	rep := &recordReporter{}
	files, err := NewClient("").Download(context.Background(), "https://example.com/v", work, Options{Quality: "720p", Container: "mp4"}, rep)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 || filepath.Base(files[0]) != "fake video [id].mp4" {
		t.Fatalf("files = %v", files)
	}
	if len(rep.progress) == 0 || rep.progress[len(rep.progress)-1][1] != 1000 {
		t.Errorf("progress = %v", rep.progress)
	}
}

func TestMissingYtDlp(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	_, err := NewClient("").Download(context.Background(), "https://example.com/v", t.TempDir(), Options{}, &recordReporter{})
	if err == nil || core.CodeOf(err) != core.CodeEngineMissing {
		t.Fatalf("error = %v, want engine_missing", err)
	}
}

func TestFormatSelector(t *testing.T) {
	if got := FormatSelector("1080p"); !strings.Contains(got, "height<=1080") {
		t.Errorf("1080p selector = %q", got)
	}
	if got := FormatSelector("best"); got != "bv*+ba/b" {
		t.Errorf("best selector = %q", got)
	}
}

func TestStderrLevelsAndHints(t *testing.T) {
	rep := &recordReporter{}
	logStderr(rep, "WARNING: old version\n\nERROR: HTTP Error 403: Forbidden")
	if len(rep.logs) != 2 || rep.logs[0] != core.LevelWarn || rep.logs[1] != core.LevelError {
		t.Fatalf("levels = %v", rep.logs)
	}
	if hint := hintFor("ERROR: HTTP Error 403: Forbidden"); !strings.Contains(hint, "Update yt-dlp") {
		t.Errorf("403 hint = %q", hint)
	}
}

func TestProbe(t *testing.T) {
	dir := t.TempDir()
	fakeBin(t, dir, "yt-dlp", "#!/bin/sh\necho 2026.01.01\n")
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	info := NewClient("").Probe(context.Background())
	if !info.Available || info.Version != "2026.01.01" {
		t.Errorf("info = %+v", info)
	}
}

func TestTranscriptParsesRollingCaptions(t *testing.T) {
	dir := t.TempDir()
	fakeBin(t, dir, "yt-dlp", `#!/bin/sh
out=""
while [ $# -gt 0 ]; do
  case "$1" in -P) out="$2"; shift 2;; *) shift;; esac
done
cat > "$out/Demo clip [abc].en.vtt" <<'VTT'
WEBVTT
Kind: captions
Language: en

00:00:01.000 --> 00:00:03.000
I<00:00:01.200><c> was</c><00:00:01.400><c> here</c>

00:00:03.000 --> 00:00:03.100
I was here

00:00:03.100 --> 00:00:05.000
I was here
friends<00:00:03.500><c> arrived</c>
VTT
`)
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	work := t.TempDir()
	result, err := NewClient("").Transcript(context.Background(), "https://example.com/v", work,
		TranscriptOptions{Language: "en", Auto: true}, &recordReporter{})
	if err != nil {
		t.Fatal(err)
	}
	if result.Title != "Demo clip [abc]" {
		t.Errorf("title = %q", result.Title)
	}
	data, err := os.ReadFile(result.Path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	if strings.Contains(text, "I was here\nI was here") {
		t.Errorf("duplicate cue was not removed: %q", text)
	}
	if !strings.Contains(text, "friends arrived") || !strings.Contains(text, "I was here") {
		t.Errorf("transcript = %q", text)
	}
}

func TestParseVTTTimestamps(t *testing.T) {
	got := parseVTT("WEBVTT\n\n01:02:03.000 --> 01:02:05.000\nHello <b>world</b>\n", true)
	if got != "[01:02:03] Hello world\n" {
		t.Errorf("timed transcript = %q", got)
	}
}

func TestInfoWritesNormalizedJSON(t *testing.T) {
	dir := t.TempDir()
	fakeBin(t, dir, "yt-dlp", `#!/bin/sh
cat <<'JSON'
{"id":"abc","title":"Demo clip","duration":12.5,"view_count":42,"uploader":"Someone","webpage_url":"https://example.com/v","formats":[{"format_id":"18","ext":"mp4","height":360,"vcodec":"avc1","acodec":"mp4a"}],"subtitles":{"en":[{"ext":"vtt"}]},"automatic_captions":{"es":[{"ext":"vtt"}]}}
JSON
`)
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	work := t.TempDir()
	result, err := NewClient("").Info(context.Background(), "https://example.com/v", work, InfoOptions{}, &recordReporter{})
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(result.Path)
	if err != nil {
		t.Fatal(err)
	}
	var info VideoInfo
	if err := json.Unmarshal(data, &info); err != nil {
		t.Fatal(err)
	}
	if info.ID != "abc" || info.Title != "Demo clip" || info.DurationSeconds != 12.5 || info.ViewCount != 42 {
		t.Errorf("info = %+v", info)
	}
	if len(info.Formats) != 1 || info.Formats[0].Height != 360 {
		t.Errorf("formats = %+v", info.Formats)
	}
	if len(info.Subtitles) != 1 || info.Subtitles[0] != "en" {
		t.Errorf("subtitles = %v", info.Subtitles)
	}
}
