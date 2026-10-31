package audio

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/solomonadzape95/workbench/internal/core"
	"github.com/solomonadzape95/workbench/internal/engines/ffmpeg"
)

// filterOutDir gives an operation a directory separate from the source so the
// output cannot overwrite tone.wav.
func filterOutDir(t *testing.T, dir string) string {
	t.Helper()
	out := filepath.Join(dir, "out")
	if err := os.MkdirAll(out, 0o755); err != nil {
		t.Fatal(err)
	}
	return out
}

func requireAudioDuration(t *testing.T, client *ffmpeg.Client, path string, min, max time.Duration) {
	t.Helper()
	media, err := client.Probe(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	if !media.HasAudio {
		t.Fatalf("%s has no audio stream", path)
	}
	if media.Duration < min || media.Duration > max {
		t.Errorf("%s duration = %s, want %s–%s", filepath.Base(path), media.Duration, min, max)
	}
}

func TestLoudnessTargets(t *testing.T) {
	cases := []struct {
		preset   string
		lufs     float64
		truePeak float64
		lra      float64
		wantLufs float64
		wantTP   float64
		wantLRA  float64
	}{
		{"podcast", -20, -3, 7, -16, -1.5, 11},
		{"music", -20, -3, 7, -14, -1.5, 11},
		{"custom", -20, -3, 7, -20, -3, 7},
	}
	for _, tt := range cases {
		lufs, tp, lra := loudnessTargets(tt.preset, tt.lufs, tt.truePeak, tt.lra)
		if lufs != tt.wantLufs || tp != tt.wantTP || lra != tt.wantLRA {
			t.Errorf("loudnessTargets(%q) = %v, %v, %v; want %v, %v, %v",
				tt.preset, lufs, tp, lra, tt.wantLufs, tt.wantTP, tt.wantLRA)
		}
	}
}

func TestLoudnormFilter(t *testing.T) {
	cases := map[string]string{
		loudnormFilter(-16, -1.5, 11):  "loudnorm=I=-16:TP=-1.5:LRA=11",
		loudnormFilter(-14.5, -2, 9.5): "loudnorm=I=-14.5:TP=-2:LRA=9.5",
	}
	for got, want := range cases {
		if got != want {
			t.Errorf("loudnorm filter = %q, want %q", got, want)
		}
	}
}

func TestTrimWindow(t *testing.T) {
	cases := []struct {
		start, end, total time.Duration
		want              time.Duration
		wantErr           bool
	}{
		{0, 0, 10 * time.Second, 10 * time.Second, false},
		{2 * time.Second, 0, 10 * time.Second, 8 * time.Second, false},
		{2 * time.Second, 5 * time.Second, 10 * time.Second, 3 * time.Second, false},
		{0, 5 * time.Second, 10 * time.Second, 5 * time.Second, false},
		{2 * time.Second, 20 * time.Second, 10 * time.Second, 8 * time.Second, false},
		{10 * time.Second, 0, 10 * time.Second, 0, true},
		{5 * time.Second, 5 * time.Second, 10 * time.Second, 0, true},
	}
	for _, tt := range cases {
		got, err := trimWindow(tt.start, tt.end, tt.total)
		if tt.wantErr {
			if err == nil {
				t.Errorf("trimWindow(%s, %s, %s) = %s, want error", tt.start, tt.end, tt.total, got)
			}
			continue
		}
		if err != nil || got != tt.want {
			t.Errorf("trimWindow(%s, %s, %s) = %s, %v; want %s", tt.start, tt.end, tt.total, got, err, tt.want)
		}
	}
}

func TestSegmentMuxer(t *testing.T) {
	cases := map[string]string{
		"mp3": "mp3", "m4a": "mp4", "aac": "mp4", "wav": "wav",
		"flac": "flac", "ogg": "ogg", "oga": "ogg", "opus": "opus",
	}
	for format, want := range cases {
		got, ok := segmentMuxer(format)
		if !ok || got != want {
			t.Errorf("segmentMuxer(%q) = %q, %v; want %q, true", format, got, ok, want)
		}
	}
	if got, ok := segmentMuxer("xyz"); ok || got != "" {
		t.Errorf("segmentMuxer(xyz) = %q, %v; want empty, false", got, ok)
	}
}

func TestFadeChain(t *testing.T) {
	cases := []struct {
		total   time.Duration
		in, out time.Duration
		want    string
	}{
		{10 * time.Second, time.Second, 2 * time.Second,
			"afade=t=in:st=0:d=1.000,afade=t=out:st=8.000:d=2.000"},
		{10 * time.Second, 0, 0, ""},
		{0, time.Second, 0, "afade=t=in:st=0:d=1.000"},
		{10 * time.Second, 20 * time.Second, 0, "afade=t=in:st=0:d=10.000"},
		{10 * time.Second, 0, 20 * time.Second, "afade=t=out:st=0.000:d=10.000"},
	}
	for _, tt := range cases {
		if got := fadeChain(tt.total, tt.in, tt.out); got != tt.want {
			t.Errorf("fadeChain(%s, %s, %s) = %q, want %q", tt.total, tt.in, tt.out, got, tt.want)
		}
	}
}

func TestSilenceHelpers(t *testing.T) {
	valid := []string{"-50dB", "-30dBFS", "-45.5", "0.01", "+3dB"}
	for _, value := range valid {
		if !validSilenceThreshold(value) {
			t.Errorf("validSilenceThreshold(%q) = false, want true", value)
		}
	}
	invalid := []string{"", "loud", "-50dbx", "--5dB"}
	for _, value := range invalid {
		if validSilenceThreshold(value) {
			t.Errorf("validSilenceThreshold(%q) = true, want false", value)
		}
	}
	filter := silenceFilter("-50dB", 500*time.Millisecond)
	for _, want := range []string{
		"start_periods=1",
		"stop_periods=-1",
		"start_duration=0.500",
		"start_threshold=-50dB",
		"stop_threshold=-50dB",
	} {
		if !strings.Contains(filter, want) {
			t.Errorf("silence filter = %q, want %q", filter, want)
		}
	}
}

func TestEnhanceFilter(t *testing.T) {
	cases := map[string]string{
		"voice":   "highpass=f=90,afftdn=nf=-25,loudnorm=I=-16:TP=-1.5:LRA=11",
		"podcast": "afftdn=nf=-25,loudnorm=I=-16",
		"music":   "loudnorm=I=-14:TP=-1.5:LRA=11",
	}
	for preset, want := range cases {
		got, err := enhanceFilter(preset)
		if err != nil || got != want {
			t.Errorf("enhanceFilter(%q) = %q, %v; want %q", preset, got, err, want)
		}
	}
	if _, err := enhanceFilter("radio"); err == nil {
		t.Error("enhanceFilter(radio) returned no error")
	}
}

func TestResolveAudioSource(t *testing.T) {
	format, encoder, err := resolveAudioSource(trimSpec, core.Artifact{Path: "/in/a.mp3", Format: "mp3"})
	if err != nil || format != "mp3" || encoder != "libmp3lame" {
		t.Errorf("resolveAudioSource(mp3) = %q, %q, %v", format, encoder, err)
	}
	if _, _, err := resolveAudioSource(trimSpec, core.Artifact{Path: "/in/a.xyz", Format: "xyz"}); core.CodeOf(err) != core.CodeInput {
		t.Errorf("resolveAudioSource(xyz) error = %v, want input", err)
	}
}

func TestParamValidationWithoutEngine(t *testing.T) {
	cases := []struct {
		name string
		op   core.Operation
		req  core.Request
	}{
		{"normalize preset", Normalize{}, core.Request{Params: core.Values{"preset": "radio"}}},
		{"normalize loudness", Normalize{}, core.Request{Params: core.Values{"preset": "custom", "lufs": -90.0}}},
		{"trim window", Trim{}, core.Request{Params: core.Values{"start": 2 * time.Second, "end": time.Second}}},
		{"split interval", Split{}, core.Request{Params: core.Values{"interval": 0 * time.Second}}},
		{"silence threshold", SilenceTrim{}, core.Request{Params: core.Values{"threshold": "quiet", "min_silence": 500 * time.Millisecond}}},
		{"silence period", SilenceTrim{}, core.Request{Params: core.Values{"threshold": "-50dB", "min_silence": 0 * time.Second}}},
		{"enhance preset", Enhance{}, core.Request{Params: core.Values{"preset": "radio"}}},
	}
	for _, tt := range cases {
		err := tt.op.Validate(context.Background(), tt.req)
		if core.CodeOf(err) != core.CodeUsage {
			t.Errorf("%s: error = %v, want usage", tt.name, err)
		}
	}
}

func TestNormalizeRealAudio(t *testing.T) {
	dir := t.TempDir()
	src := tone(t, dir)
	client := ffmpeg.NewClient("")
	op := Normalize{Client: client}
	req := core.Request{
		Items: []core.Artifact{src},
		Params: core.Values{
			"preset": "podcast", "lufs": -16.0, "true_peak": -1.5, "lra": 11.0,
		},
		Target: core.Target{Dir: filterOutDir(t, dir)},
	}
	if err := op.Validate(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	result, err := op.Run(context.Background(), req, nopReporter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Artifacts) != 1 || result.Artifacts[0].Format != "wav" {
		t.Fatalf("artifacts = %+v, want one wav", result.Artifacts)
	}
	if got := result.Artifacts[0].Meta["lufs"]; got != -16.0 {
		t.Errorf("meta lufs = %v, want -16", got)
	}
	requireAudioDuration(t, client, result.Artifacts[0].Path, 900*time.Millisecond, 1100*time.Millisecond)
}

func TestTrimRealAudio(t *testing.T) {
	dir := t.TempDir()
	src := tone(t, dir)
	client := ffmpeg.NewClient("")
	op := Trim{Client: client}
	req := core.Request{
		Items:  []core.Artifact{src},
		Params: core.Values{"start": 200 * time.Millisecond, "end": 700 * time.Millisecond},
		Target: core.Target{Dir: filterOutDir(t, dir)},
	}
	if err := op.Validate(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	result, err := op.Run(context.Background(), req, nopReporter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Artifacts) != 1 || result.Artifacts[0].Format != "wav" {
		t.Fatalf("artifacts = %+v, want one wav", result.Artifacts)
	}
	if got := result.Artifacts[0].Meta["start_ms"]; got != int64(200) {
		t.Errorf("meta start_ms = %v, want 200", got)
	}
	requireAudioDuration(t, client, result.Artifacts[0].Path, 400*time.Millisecond, 600*time.Millisecond)
}

func TestSplitRealAudio(t *testing.T) {
	dir := t.TempDir()
	src := tone(t, dir)
	client := ffmpeg.NewClient("")
	op := Split{Client: client}
	req := core.Request{
		Items:  []core.Artifact{src},
		Params: core.Values{"interval": 300 * time.Millisecond},
		Target: core.Target{Dir: filterOutDir(t, dir)},
	}
	if err := op.Validate(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	result, err := op.Run(context.Background(), req, nopReporter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Artifacts) < 2 {
		t.Fatalf("artifacts = %d, want at least 2 segments", len(result.Artifacts))
	}
	seen := map[string]bool{}
	var total time.Duration
	for _, art := range result.Artifacts {
		if art.Format != "wav" || art.Kind != core.Audio {
			t.Errorf("segment = %+v, want wav audio", art)
		}
		if art.SuggestedName == "" || seen[art.SuggestedName] {
			t.Errorf("segment name = %q is empty or duplicated", art.SuggestedName)
		}
		seen[art.SuggestedName] = true
		if !strings.HasPrefix(filepath.Base(art.SuggestedName), "tone-") {
			t.Errorf("segment name = %q, want a tone- prefix", art.SuggestedName)
		}
		if ms, ok := art.Meta["duration_ms"].(int64); ok {
			total += time.Duration(ms) * time.Millisecond
		}
		requireAudioDuration(t, client, art.Path, time.Millisecond, 500*time.Millisecond)
	}
	if total < 800*time.Millisecond || total > 1250*time.Millisecond {
		t.Errorf("segment durations total %s, want about 1s", total)
	}
}

func TestFadeRealAudio(t *testing.T) {
	dir := t.TempDir()
	src := tone(t, dir)
	client := ffmpeg.NewClient("")
	op := Fade{Client: client}
	req := core.Request{
		Items:  []core.Artifact{src},
		Params: core.Values{"fade_in": 200 * time.Millisecond, "fade_out": 200 * time.Millisecond},
		Target: core.Target{Dir: filterOutDir(t, dir)},
	}
	if err := op.Validate(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	result, err := op.Run(context.Background(), req, nopReporter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Artifacts) != 1 {
		t.Fatalf("artifacts = %d, want 1", len(result.Artifacts))
	}
	requireAudioDuration(t, client, result.Artifacts[0].Path, 900*time.Millisecond, 1100*time.Millisecond)
}

func TestSilenceTrimRealAudio(t *testing.T) {
	dir := t.TempDir()
	src := tone(t, dir)
	client := ffmpeg.NewClient("")
	op := SilenceTrim{Client: client}
	req := core.Request{
		Items:  []core.Artifact{src},
		Params: core.Values{"threshold": "-50dB", "min_silence": 100 * time.Millisecond},
		Target: core.Target{Dir: filterOutDir(t, dir)},
	}
	if err := op.Validate(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	result, err := op.Run(context.Background(), req, nopReporter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Artifacts) != 1 {
		t.Fatalf("artifacts = %d, want 1", len(result.Artifacts))
	}
	requireAudioDuration(t, client, result.Artifacts[0].Path, 500*time.Millisecond, 1200*time.Millisecond)
}

func TestEnhanceRealAudio(t *testing.T) {
	dir := t.TempDir()
	src := tone(t, dir)
	client := ffmpeg.NewClient("")
	op := Enhance{Client: client}
	req := core.Request{
		Items:  []core.Artifact{src},
		Params: core.Values{"preset": "voice"},
		Target: core.Target{Dir: filterOutDir(t, dir)},
	}
	if err := op.Validate(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	result, err := op.Run(context.Background(), req, nopReporter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Artifacts) != 1 || result.Artifacts[0].Format != "wav" {
		t.Fatalf("artifacts = %+v, want one wav", result.Artifacts)
	}
	if got := result.Artifacts[0].Meta["preset"]; got != "voice" {
		t.Errorf("meta preset = %v, want voice", got)
	}
	requireAudioDuration(t, client, result.Artifacts[0].Path, 900*time.Millisecond, 1100*time.Millisecond)
}
