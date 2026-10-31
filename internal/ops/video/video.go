// Package video implements ffmpeg-backed video operations.
package video

import (
	"fmt"
	"strings"

	"github.com/solomonadzape95/workbench/internal/core"
	"github.com/solomonadzape95/workbench/internal/engines/ffmpeg"
	"github.com/solomonadzape95/workbench/internal/ops/media"
)

func containerOptions() []core.Option {
	return []core.Option{
		{Value: "mp4", Label: "MP4"},
		{Value: "mkv", Label: "Matroska"},
		{Value: "webm", Label: "WebM"},
		{Value: "mov", Label: "QuickTime"},
	}
}

func codecOptions() []core.Option {
	return []core.Option{
		{Value: "auto", Label: "Auto"},
		{Value: "h264", Label: "H.264"},
		{Value: "hevc", Label: "H.265 / HEVC"},
		{Value: "vp9", Label: "VP9"},
	}
}

func presetOptions() []core.Option {
	return []core.Option{
		{Value: "ultrafast", Label: "Ultrafast"},
		{Value: "fast", Label: "Fast"},
		{Value: "medium", Label: "Medium"},
		{Value: "slow", Label: "Slow"},
	}
}

func resolutionOptions() []core.Option {
	return []core.Option{
		{Value: "source", Label: "Keep source"},
		{Value: "1080p", Label: "1080p"},
		{Value: "720p", Label: "720p"},
		{Value: "480p", Label: "480p"},
	}
}

func maxBitrateOptions() []core.Option {
	return []core.Option{
		{Value: "none", Label: "No cap"},
		{Value: "2M", Label: "2 Mbps"},
		{Value: "5M", Label: "5 Mbps"},
		{Value: "10M", Label: "10 Mbps"},
	}
}

func audioBitrateOptions() []core.Option {
	return []core.Option{
		{Value: "128k", Label: "128 kbps"},
		{Value: "192k", Label: "192 kbps"},
		{Value: "256k", Label: "256 kbps"},
		{Value: "320k", Label: "320 kbps"},
	}
}

func audioFormatOptions() []core.Option {
	return []core.Option{
		{Value: "mp3", Label: "MP3"},
		{Value: "m4a", Label: "M4A (AAC)"},
		{Value: "wav", Label: "WAV"},
		{Value: "flac", Label: "FLAC"},
		{Value: "ogg", Label: "Ogg Vorbis"},
	}
}

func videoContainer(item core.Artifact) (string, error) {
	container := strings.ToLower(strings.TrimPrefix(item.Ext(), "."))
	if container == "" {
		return "", fmt.Errorf("cannot determine the source video container")
	}
	return container, nil
}

func checkVideoInputs(spec core.Spec, req core.Request, client *ffmpeg.Client) error {
	if err := core.Validate(spec.Params, req.Params); err != nil {
		return err
	}
	if len(req.Items) == 0 {
		return core.NewError(core.CodeUsage, spec.ID, "", "no input files", "")
	}
	if err := media.Check(client, spec.ID); err != nil {
		return err
	}
	for _, item := range req.Items {
		if !spec.AcceptsArtifact(item) {
			return core.NewError(core.CodeInput, spec.ID, item.Path,
				fmt.Sprintf("%s is not a video file", item.Format),
				"supported inputs: MP4, MKV, WebM, MOV, AVI, M4V")
		}
	}
	return nil
}

// resolutionFilter returns a scale filter for the requested preset, or "".
func resolutionFilter(resolution string, sourceHeight int) string {
	var height int
	switch resolution {
	case "1080p":
		height = 1080
	case "720p":
		height = 720
	case "480p":
		height = 480
	default:
		return ""
	}
	if sourceHeight > 0 && sourceHeight <= height {
		return ""
	}
	return fmt.Sprintf("scale=-2:%d", height)
}

// maxRateArgs converts the UI cap into ffmpeg's -maxrate/-bufsize pair.
func maxRateArgs(cap string) []string {
	switch cap {
	case "2M":
		return []string{"-maxrate", "2M", "-bufsize", "4M"}
	case "5M":
		return []string{"-maxrate", "5M", "-bufsize", "10M"}
	case "10M":
		return []string{"-maxrate", "10M", "-bufsize", "20M"}
	default:
		return nil
	}
}
