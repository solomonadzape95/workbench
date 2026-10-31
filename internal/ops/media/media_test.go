package media

import (
	"strings"
	"testing"

	"github.com/solomonadzape95/workbench/internal/core"
)

func TestAudioEncoder(t *testing.T) {
	cases := map[string]struct {
		encoder string
		lossy   bool
		ok      bool
	}{
		"mp3":  {"libmp3lame", true, true},
		"m4a":  {"aac", true, true},
		"wav":  {"pcm_s16le", false, true},
		"flac": {"flac", false, true},
		"opus": {"libopus", true, true},
		"xyz":  {"", false, false},
	}
	for format, want := range cases {
		encoder, lossy, ok := AudioEncoder(format)
		if encoder != want.encoder || lossy != want.lossy || ok != want.ok {
			t.Errorf("AudioEncoder(%q) = %q, %v, %v; want %+v", format, encoder, lossy, ok, want)
		}
	}
}

func TestVideoEncoderCompatibility(t *testing.T) {
	cases := []struct {
		container string
		codec     string
		want      string
		wantErr   bool
	}{
		{"mp4", "auto", "libx264", false},
		{"webm", "auto", "libvpx-vp9", false},
		{"mkv", "vp9", "libvpx-vp9", false},
		{"mp4", "vp9", "", true},
		{"webm", "h264", "", true},
		{"avi", "auto", "", true},
		{"mp4", "hevc", "libx265", false},
	}
	for _, tt := range cases {
		got, err := VideoEncoder(tt.container, tt.codec)
		if tt.wantErr {
			if err == nil {
				t.Errorf("VideoEncoder(%q, %q) = %q, want error", tt.container, tt.codec, got)
			}
			continue
		}
		if err != nil || got != tt.want {
			t.Errorf("VideoEncoder(%q, %q) = %q, %v; want %q", tt.container, tt.codec, got, err, tt.want)
		}
	}
}

func TestAtempoChain(t *testing.T) {
	cases := map[float64]string{
		1.0:  "atempo=1.0000",
		1.5:  "atempo=1.5000",
		2.0:  "atempo=2.0000",
		4.0:  "atempo=2.0,atempo=2.0000",
		0.25: "atempo=0.5,atempo=0.5000",
	}
	for factor, want := range cases {
		if got := AtempoChain(factor); got != want {
			t.Errorf("AtempoChain(%v) = %q, want %q", factor, got, want)
		}
	}
}

func TestOutputPath(t *testing.T) {
	req := core.Request{Target: core.Target{Dir: "/out"}}
	item := core.Artifact{Path: "/in/My Song.MP3", Format: "mp3"}
	got := OutputPath(req, item, "flac")
	if got != "/out/My Song.flac" {
		t.Errorf("OutputPath = %q", got)
	}
}

func TestVideoArgsWebM(t *testing.T) {
	args, err := VideoArgs("webm", "auto", "30", "medium")
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(args, " ")
	for _, want := range []string{"libvpx-vp9", "libopus", "-crf 30", "-b:v 0"} {
		if !strings.Contains(joined, want) {
			t.Errorf("args = %q, want %q", joined, want)
		}
	}
}
