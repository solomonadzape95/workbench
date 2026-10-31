package video

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/solomonadzape95/workbench/internal/core"
	"github.com/solomonadzape95/workbench/internal/engines/ffmpeg"
)

func requireFFmpeg(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg is not installed")
	}
}

func fakeVideoArtifact() core.Artifact {
	return core.Artifact{Path: "clip.mp4", Kind: core.Video, Format: "mp4"}
}

func TestTrimRealVideo(t *testing.T) {
	dir := t.TempDir()
	src := clip(t, dir)
	client := ffmpeg.NewClient("")
	op := Trim{Client: client}
	req := core.Request{
		Items: []core.Artifact{src},
		Params: core.Values{
			"start": 200 * time.Millisecond,
			"end":   700 * time.Millisecond,
			"fast":  false,
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
	art := result.Artifacts[0]
	if art.Kind != core.Video || art.Format != "mp4" {
		t.Fatalf("artifact = %+v, want mp4 video", art)
	}
	probed, err := client.Probe(context.Background(), art.Path)
	if err != nil {
		t.Fatal(err)
	}
	if !probed.HasVideo {
		t.Error("trimmed file has no video stream")
	}
	if probed.Duration < 350*time.Millisecond || probed.Duration > 750*time.Millisecond {
		t.Errorf("trimmed duration = %s, want about 0.5s", probed.Duration)
	}
}

func TestTrimFastCopyRealVideo(t *testing.T) {
	dir := t.TempDir()
	src := clip(t, dir)
	client := ffmpeg.NewClient("")
	op := Trim{Client: client}
	req := core.Request{
		Items:  []core.Artifact{src},
		Params: core.Values{"start": 0, "end": 500 * time.Millisecond, "fast": true},
		Target: core.Target{Dir: outputDir(t, dir)},
	}
	if err := op.Validate(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	result, err := op.Run(context.Background(), req, nopReporter{})
	if err != nil {
		t.Fatal(err)
	}
	probed, err := client.Probe(context.Background(), result.Artifacts[0].Path)
	if err != nil {
		t.Fatal(err)
	}
	if !probed.HasVideo {
		t.Error("stream-copied trim has no video stream")
	}
}

func TestThumbnailRealVideo(t *testing.T) {
	dir := t.TempDir()
	src := clip(t, dir)
	client := ffmpeg.NewClient("")
	op := Thumbnail{Client: client}
	req := core.Request{
		Items: []core.Artifact{src},
		Params: core.Values{
			"at":     500 * time.Millisecond,
			"format": "png",
			"width":  80,
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
	art := result.Artifacts[0]
	if art.Kind != core.Image || art.Format != "png" {
		t.Fatalf("artifact = %+v, want png image", art)
	}
	if filepath.Ext(art.Path) != ".png" {
		t.Errorf("path = %q, want a .png file", art.Path)
	}
	if art.SuggestedName != filepath.Base(art.Path) {
		t.Errorf("suggested name = %q, want %q", art.SuggestedName, filepath.Base(art.Path))
	}
	if got := art.Meta["duration_ms"]; got != int64(0) {
		t.Errorf("duration_ms = %v, want 0", got)
	}
	probed, err := client.Probe(context.Background(), art.Path)
	if err != nil {
		t.Fatal(err)
	}
	if !probed.HasVideo || probed.Width != 80 {
		t.Errorf("thumbnail media = %+v, want 80px wide image", probed)
	}
}

func TestThumbnailJPEGRealVideo(t *testing.T) {
	dir := t.TempDir()
	src := clip(t, dir)
	client := ffmpeg.NewClient("")
	op := Thumbnail{Client: client}
	req := core.Request{
		Items:  []core.Artifact{src},
		Params: core.Values{"at": 0, "format": "jpeg", "width": 0},
		Target: core.Target{Dir: outputDir(t, dir)},
	}
	if err := op.Validate(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	result, err := op.Run(context.Background(), req, nopReporter{})
	if err != nil {
		t.Fatal(err)
	}
	art := result.Artifacts[0]
	if art.Kind != core.Image || art.Format != "jpeg" {
		t.Fatalf("artifact = %+v, want jpeg image", art)
	}
	if filepath.Ext(art.Path) != ".jpg" {
		t.Errorf("path = %q, want a .jpg file", art.Path)
	}
}

func TestToGifRealVideo(t *testing.T) {
	dir := t.TempDir()
	src := clip(t, dir)
	client := ffmpeg.NewClient("")
	op := ToGif{Client: client}
	req := core.Request{
		Items: []core.Artifact{src},
		Params: core.Values{
			"start":    0,
			"duration": 0,
			"fps":      10,
			"width":    80,
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
	art := result.Artifacts[0]
	if art.Kind != core.File || art.Format != "gif" {
		t.Fatalf("artifact = %+v, want gif file", art)
	}
	if filepath.Ext(art.Path) != ".gif" {
		t.Errorf("path = %q, want a .gif file", art.Path)
	}
	probed, err := client.Probe(context.Background(), art.Path)
	if err != nil {
		t.Fatal(err)
	}
	if !probed.HasVideo || probed.Width != 80 {
		t.Errorf("gif media = %+v, want 80px wide animation", probed)
	}
}

func TestRemuxRealVideo(t *testing.T) {
	dir := t.TempDir()
	src := clip(t, dir)
	client := ffmpeg.NewClient("")
	op := Remux{Client: client}
	req := core.Request{
		Items:  []core.Artifact{src},
		Params: core.Values{"container": "mkv"},
		Target: core.Target{Dir: outputDir(t, dir)},
	}
	if err := op.Validate(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	result, err := op.Run(context.Background(), req, nopReporter{})
	if err != nil {
		t.Fatal(err)
	}
	art := result.Artifacts[0]
	if art.Kind != core.Video || art.Format != "mkv" {
		t.Fatalf("artifact = %+v, want mkv video", art)
	}
	probed, err := client.Probe(context.Background(), art.Path)
	if err != nil {
		t.Fatal(err)
	}
	if !probed.HasVideo {
		t.Error("remuxed file has no video stream")
	}
}

func TestBurnSubsRealVideo(t *testing.T) {
	dir := t.TempDir()
	src := clip(t, dir)
	client := ffmpeg.NewClient("")
	if !ffmpegHasFilter(t, "subtitles") {
		t.Skip("ffmpeg build has no subtitles filter")
	}
	subtitle := filepath.Join(dir, "subs.srt")
	body := "1\n00:00:00,000 --> 00:00:00,900\nHello\n"
	if err := os.WriteFile(subtitle, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	op := BurnSubs{Client: client}
	req := core.Request{
		Items:  []core.Artifact{src},
		Params: core.Values{"subtitle": subtitle, "font_size": 18},
		Target: core.Target{Dir: outputDir(t, dir)},
	}
	if err := op.Validate(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	result, err := op.Run(context.Background(), req, nopReporter{})
	if err != nil {
		t.Fatal(err)
	}
	probed, err := client.Probe(context.Background(), result.Artifacts[0].Path)
	if err != nil {
		t.Fatal(err)
	}
	if !probed.HasVideo {
		t.Error("subtitle-burned file has no video stream")
	}
}

func TestWatermarkRealVideo(t *testing.T) {
	dir := t.TempDir()
	src := clip(t, dir)
	client := ffmpeg.NewClient("")
	srcMedia, err := client.Probe(context.Background(), src.Path)
	if err != nil {
		t.Fatal(err)
	}
	image := pngImage(t, dir)
	op := Watermark{Client: client}
	req := core.Request{
		Items: []core.Artifact{src},
		Params: core.Values{
			"watermark": image,
			"position":  "top-left",
			"opacity":   0.5,
			"margin":    4,
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
	probed, err := client.Probe(context.Background(), result.Artifacts[0].Path)
	if err != nil {
		t.Fatal(err)
	}
	if !probed.HasVideo || probed.Width != srcMedia.Width || probed.Height != srcMedia.Height {
		t.Errorf("watermarked media = %+v, want %dx%d", probed, srcMedia.Width, srcMedia.Height)
	}
}

func TestTrimRejectsEndBeforeStart(t *testing.T) {
	requireFFmpeg(t)
	op := Trim{Client: ffmpeg.NewClient("")}
	req := core.Request{
		Items:  []core.Artifact{fakeVideoArtifact()},
		Params: core.Values{"start": 5 * time.Second, "end": 2 * time.Second},
	}
	err := op.Validate(context.Background(), req)
	if err == nil {
		t.Fatal("Validate accepted end before start")
	}
	if core.CodeOf(err) != core.CodeUsage {
		t.Errorf("code = %q, want usage", core.CodeOf(err))
	}
}

func TestRemuxRejectsInvalidContainer(t *testing.T) {
	requireFFmpeg(t)
	op := Remux{Client: ffmpeg.NewClient("")}
	req := core.Request{
		Items:  []core.Artifact{fakeVideoArtifact()},
		Params: core.Values{"container": "avi"},
	}
	err := op.Validate(context.Background(), req)
	if err == nil {
		t.Fatal("Validate accepted an unsupported container")
	}
	if core.CodeOf(err) != core.CodeUsage {
		t.Errorf("code = %q, want usage", core.CodeOf(err))
	}
}

func TestRemuxRejectsNonVideoFile(t *testing.T) {
	requireFFmpeg(t)
	path := filepath.Join(t.TempDir(), "not-video.mp4")
	if err := os.WriteFile(path, []byte("this is not a video"), 0o644); err != nil {
		t.Fatal(err)
	}
	art, err := core.Probe(path)
	if err != nil {
		t.Fatal(err)
	}
	op := Remux{Client: ffmpeg.NewClient("")}
	req := core.Request{
		Items:  []core.Artifact{art},
		Params: core.Values{"container": "mp4"},
	}
	if err := op.Validate(context.Background(), req); err == nil {
		t.Fatal("Validate accepted a file with no video stream")
	}
}

func TestBurnSubsRequiresExistingSubtitle(t *testing.T) {
	requireFFmpeg(t)
	op := BurnSubs{Client: ffmpeg.NewClient("")}
	req := core.Request{
		Items: []core.Artifact{fakeVideoArtifact()},
		Params: core.Values{
			"subtitle":  filepath.Join(t.TempDir(), "missing.srt"),
			"font_size": 24,
		},
	}
	err := op.Validate(context.Background(), req)
	if err == nil {
		t.Fatal("Validate accepted a missing subtitle file")
	}
	if core.CodeOf(err) != core.CodeInput {
		t.Errorf("code = %q, want input", core.CodeOf(err))
	}
}

func TestWatermarkRequiresExistingImage(t *testing.T) {
	requireFFmpeg(t)
	op := Watermark{Client: ffmpeg.NewClient("")}
	req := core.Request{
		Items: []core.Artifact{fakeVideoArtifact()},
		Params: core.Values{
			"watermark": filepath.Join(t.TempDir(), "missing.png"),
		},
	}
	err := op.Validate(context.Background(), req)
	if err == nil {
		t.Fatal("Validate accepted a missing watermark")
	}
	if core.CodeOf(err) != core.CodeInput {
		t.Errorf("code = %q, want input", core.CodeOf(err))
	}
}

func TestWatermarkRejectsNonImage(t *testing.T) {
	requireFFmpeg(t)
	path := filepath.Join(t.TempDir(), "notes.txt")
	if err := os.WriteFile(path, []byte("not an image"), 0o644); err != nil {
		t.Fatal(err)
	}
	op := Watermark{Client: ffmpeg.NewClient("")}
	req := core.Request{
		Items:  []core.Artifact{fakeVideoArtifact()},
		Params: core.Values{"watermark": path},
	}
	err := op.Validate(context.Background(), req)
	if err == nil {
		t.Fatal("Validate accepted a text file as a watermark")
	}
	if core.CodeOf(err) != core.CodeInput {
		t.Errorf("code = %q, want input", core.CodeOf(err))
	}
}

func TestTrimWindow(t *testing.T) {
	cases := []struct {
		name         string
		source       time.Duration
		start, end   time.Duration
		wantStart    time.Duration
		wantDuration time.Duration
	}{
		{name: "whole clip", source: 10 * time.Second, wantStart: 0, wantDuration: 10 * time.Second},
		{name: "range", source: 10 * time.Second, start: 2 * time.Second, end: 5 * time.Second, wantStart: 2 * time.Second, wantDuration: 3 * time.Second},
		{name: "open end", source: 10 * time.Second, start: 2 * time.Second, wantStart: 2 * time.Second, wantDuration: 8 * time.Second},
		{name: "end past source", source: 10 * time.Second, end: 20 * time.Second, wantStart: 0, wantDuration: 10 * time.Second},
		{name: "start past source", source: 10 * time.Second, start: 12 * time.Second, wantStart: 10 * time.Second, wantDuration: 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			start, duration := trimWindow(tc.source, tc.start, tc.end)
			if start != tc.wantStart || duration != tc.wantDuration {
				t.Errorf("trimWindow() = (%s, %s), want (%s, %s)",
					start, duration, tc.wantStart, tc.wantDuration)
			}
		})
	}
}

func TestGifFilter(t *testing.T) {
	filter := gifFilter(12, 480)
	for _, want := range []string{"fps=12", "scale=480:-1:flags=lanczos", "split", "palettegen", "paletteuse"} {
		if !strings.Contains(filter, want) {
			t.Errorf("gifFilter() = %q, missing %q", filter, want)
		}
	}
	if plain := gifFilter(0, 0); !strings.HasPrefix(plain, "split") {
		t.Errorf("gifFilter(0, 0) = %q, want an unscaled chain", plain)
	}
}

func TestSubtitlesFilter(t *testing.T) {
	got := subtitlesFilter("/tmp/a:b.srt", 30)
	want := `subtitles=filename='/tmp/a\:b.srt':force_style='FontSize=30'`
	if got != want {
		t.Errorf("subtitlesFilter() = %q, want %q", got, want)
	}
}

func TestOverlayFilter(t *testing.T) {
	plain := overlayFilter("center", 1, 0)
	if strings.Contains(plain, "colorchannelmixer") {
		t.Errorf("opaque overlay = %q, want no alpha mixer", plain)
	}
	if !strings.Contains(plain, "overlay=(W-w)/2:(H-h)/2[out]") {
		t.Errorf("opaque overlay = %q, want center placement", plain)
	}
	faded := overlayFilter("bottom-right", 0.5, 16)
	for _, want := range []string{
		"[1:v]format=rgba,colorchannelmixer=aa=0.5[wm]",
		"overlay=W-w-16:H-h-16[out]",
	} {
		if !strings.Contains(faded, want) {
			t.Errorf("faded overlay = %q, missing %q", faded, want)
		}
	}
}

func TestOverlayPosition(t *testing.T) {
	cases := []struct {
		position string
		wantX    string
		wantY    string
	}{
		{"top-left", "4", "4"},
		{"top-right", "W-w-4", "4"},
		{"bottom-left", "4", "H-h-4"},
		{"bottom-right", "W-w-4", "H-h-4"},
		{"center", "(W-w)/2", "(H-h)/2"},
	}
	for _, tc := range cases {
		x, y := overlayPosition(tc.position, 4)
		if x != tc.wantX || y != tc.wantY {
			t.Errorf("overlayPosition(%q) = (%q, %q), want (%q, %q)",
				tc.position, x, y, tc.wantX, tc.wantY)
		}
	}
}

func TestEscapeFilterPath(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"/tmp/a b.png", "/tmp/a b.png"},
		{"/tmp/x:y.png", `/tmp/x\:y.png`},
		{"/tmp/it's.png", `/tmp/it'\''s.png`},
		{`/tmp/a\b.png`, `/tmp/a\\b.png`},
		{"/tmp/a[1],b;c.png", `/tmp/a\[1\]\,b\;c.png`},
	}
	for _, tc := range cases {
		if got := escapeFilterPath(tc.in); got != tc.want {
			t.Errorf("escapeFilterPath(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestSubtitleOutputContainer(t *testing.T) {
	cases := map[string]string{
		"mp4":  "mp4",
		"mkv":  "mkv",
		"mov":  "mov",
		"webm": "mp4",
		"avi":  "mp4",
	}
	for in, want := range cases {
		if got := subtitleOutputContainer(in); got != want {
			t.Errorf("subtitleOutputContainer(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestFormatSeconds(t *testing.T) {
	if got := formatSeconds(0); got != "0" {
		t.Errorf("formatSeconds(0) = %q, want 0", got)
	}
	if got := formatSeconds(1500 * time.Millisecond); got != "1.500" {
		t.Errorf("formatSeconds(1.5s) = %q, want 1.500", got)
	}
}

func pngImage(t *testing.T, dir string) string {
	t.Helper()
	path := filepath.Join(dir, "watermark.png")
	cmd := exec.Command("ffmpeg",
		"-hide_banner", "-loglevel", "error", "-y",
		"-f", "lavfi", "-i", "color=c=red:s=16x16:d=1",
		"-frames:v", "1", "-update", "1",
		path,
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("cannot generate watermark image: %v (%s)", err, out)
	}
	return path
}

func ffmpegHasFilter(t *testing.T, name string) bool {
	t.Helper()
	out, err := exec.Command("ffmpeg", "-hide_banner", "-filters").Output()
	if err != nil {
		return false
	}
	for _, line := range strings.Split(string(out), "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 2 && fields[1] == name {
			return true
		}
	}
	return false
}

func TestSpecsBatchableAndFFmpeg(t *testing.T) {
	specs := []core.Spec{
		trimSpec, thumbnailSpec, toGifSpec, remuxSpec, burnSubsSpec, watermarkSpec,
	}
	for _, spec := range specs {
		if !spec.Batchable {
			t.Errorf("%s: Batchable = false, want true", spec.ID)
		}
		if spec.Domain != core.Video {
			t.Errorf("%s: Domain = %q, want video", spec.ID, spec.Domain)
		}
		if len(spec.Engines) != 1 || spec.Engines[0] != "ffmpeg" {
			t.Errorf("%s: Engines = %v, want [ffmpeg]", spec.ID, spec.Engines)
		}
		found := false
		for _, accept := range spec.Accepts {
			if accept.Kind == core.Video {
				found = true
			}
		}
		if !found {
			t.Errorf("%s: does not accept video", spec.ID)
		}
	}
}

func TestConcatRealVideos(t *testing.T) {
	dir := t.TempDir()
	first := clip(t, dir)
	secondDir := filepath.Join(dir, "two")
	if err := os.Mkdir(secondDir, 0o755); err != nil {
		t.Fatal(err)
	}
	second := clip(t, secondDir)
	out := outputDir(t, dir)
	client := ffmpeg.NewClient("")
	op := Concat{Client: client}
	req := core.Request{
		Items:  []core.Artifact{first, second},
		Params: core.Values{"container": "mp4", "codec": "auto", "crf": 20, "preset": "fast"},
		Target: core.Target{Dir: out},
	}
	if err := op.Validate(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	result, err := op.Run(context.Background(), req, nopReporter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Artifacts) != 1 || result.Artifacts[0].Kind != core.Video {
		t.Fatalf("artifacts = %+v", result.Artifacts)
	}
	probe, err := client.Probe(context.Background(), result.Artifacts[0].Path)
	if err != nil {
		t.Fatal(err)
	}
	if !probe.HasVideo {
		t.Error("joined video has no video stream")
	}
}
