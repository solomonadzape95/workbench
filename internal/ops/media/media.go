// Package media holds helpers shared by ffmpeg-backed audio and video
// operations: output naming, codec mapping, and artifact construction.
package media

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/solomonadzape95/workbench/internal/core"
	"github.com/solomonadzape95/workbench/internal/engines/ffmpeg"
)

func OutputPath(req core.Request, item core.Artifact, format string) string {
	ext := core.ExtFor(format, item)
	return filepath.Join(req.Target.Dir, core.SanitizeStem(item.Stem())+"."+ext)
}

func Check(client *ffmpeg.Client, op string) error {
	if client == nil {
		return core.NewError(core.CodeEngineMissing, op, "", "ffmpeg is not configured", ffmpeg.InstallHint())
	}
	if err := client.Check(); err != nil {
		return core.NewError(core.CodeEngineMissing, op, "", "ffmpeg is not installed", ffmpeg.InstallHint())
	}
	return nil
}

func Probe(ctx context.Context, client *ffmpeg.Client, op string, item core.Artifact, rep core.Reporter) (ffmpeg.Media, error) {
	rep.Stage("probing")
	m, err := client.Probe(ctx, item.Path)
	if err != nil {
		if ctx.Err() != nil {
			return ffmpeg.Media{}, ctx.Err()
		}
		return ffmpeg.Media{}, core.Wrap(core.CodeInput, op, item.Path, err, core.HintOf(err))
	}
	return m, nil
}

// Finish stats an ffmpeg output and builds the artifact the runner will commit.
func Finish(req core.Request, item core.Artifact, output string, kind core.Kind, format string, src ffmpeg.Media, duration time.Duration, extra map[string]any) (core.Artifact, error) {
	info, err := os.Stat(output)
	if err != nil {
		return core.Artifact{}, core.Wrap(core.CodeOutput, "", output, err,
			"ffmpeg reported success but produced no output")
	}
	if duration <= 0 {
		duration = src.Duration
	}
	meta := map[string]any{
		"source":      item.Path,
		"duration_ms": duration.Milliseconds(),
	}
	if src.HasVideo {
		meta["width"] = src.Width
		meta["height"] = src.Height
		meta["video_codec"] = src.VideoCodec
	}
	if src.HasAudio {
		meta["audio_codec"] = src.AudioCodec
	}
	for key, value := range extra {
		meta[key] = value
	}
	return core.Artifact{
		Path:   output,
		Kind:   kind,
		Format: format,
		Size:   info.Size(),
		Meta:   meta,
	}, nil
}

// AudioEncoder maps a container/format to an ffmpeg audio encoder. lossy says
// whether -b:a applies.
func AudioEncoder(format string) (encoder string, lossy, ok bool) {
	switch strings.ToLower(strings.TrimPrefix(format, ".")) {
	case "mp3":
		return "libmp3lame", true, true
	case "m4a", "aac":
		return "aac", true, true
	case "ogg", "oga":
		return "libvorbis", true, true
	case "opus":
		return "libopus", true, true
	case "wav":
		return "pcm_s16le", false, true
	case "flac":
		return "flac", false, true
	default:
		return "", false, false
	}
}

// AudioEncoderForContainer picks an audio encoder that matches a video
// container.
func AudioEncoderForContainer(container string) string {
	if strings.EqualFold(container, "webm") {
		return "libopus"
	}
	return "aac"
}

// VideoEncoder resolves the auto choice and enforces container compatibility.
func VideoEncoder(container, requested string) (string, error) {
	container = strings.ToLower(strings.TrimPrefix(container, "."))
	requested = strings.ToLower(strings.TrimSpace(requested))
	if requested == "" || requested == "auto" {
		if container == "webm" {
			requested = "vp9"
		} else {
			requested = "h264"
		}
	}

	switch requested {
	case "h264", "hevc", "vp9", "vp8":
	default:
		return "", fmt.Errorf("unknown video codec %q", requested)
	}

	switch container {
	case "webm":
		if requested != "vp8" && requested != "vp9" {
			return "", fmt.Errorf("WebM needs VP8 or VP9, not %s", requested)
		}
	case "mp4", "mov", "m4v":
		if requested == "vp8" || requested == "vp9" {
			return "", fmt.Errorf("%s cannot carry %s video", container, requested)
		}
	case "mkv":
		// Matroska accepts everything we expose.
	default:
		return "", fmt.Errorf("unsupported video container %q", container)
	}

	switch requested {
	case "h264":
		return "libx264", nil
	case "hevc":
		return "libx265", nil
	case "vp9":
		return "libvpx-vp9", nil
	default:
		return "libvpx", nil
	}
}

// IsLossless reports whether a bitrate has no effect on this encoder.
func IsLossless(encoder string) bool {
	return encoder == "flac" || strings.HasPrefix(encoder, "pcm_")
}

// AtempoChain builds an atempo filter chain for factors outside the 0.5–2.0
// range supported by a single instance.
func AtempoChain(factor float64) string {
	if factor <= 0 {
		return "atempo=1.0"
	}
	var parts []string
	for factor < 0.5 {
		parts = append(parts, "atempo=0.5")
		factor /= 0.5
	}
	for factor > 2.0 {
		parts = append(parts, "atempo=2.0")
		factor /= 2.0
	}
	parts = append(parts, fmt.Sprintf("atempo=%.4f", factor))
	return strings.Join(parts, ",")
}

// VideoArgs appends codec, quality, and audio settings shared by video ops.
func VideoArgs(container, requestedCodec, crf, preset string) ([]string, error) {
	encoder, err := VideoEncoder(container, requestedCodec)
	if err != nil {
		return nil, err
	}
	args := []string{"-c:v", encoder}
	switch encoder {
	case "libvpx-vp9", "libvpx":
		args = append(args, "-crf", crf, "-b:v", "0")
	default:
		args = append(args, "-crf", crf, "-preset", preset)
	}
	args = append(args, "-c:a", AudioEncoderForContainer(container), "-b:a", "128k")
	return args, nil
}
