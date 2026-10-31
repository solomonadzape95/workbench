// Package audio implements ffmpeg-backed audio operations.
package audio

import (
	"fmt"
	"strings"

	"github.com/solomonadzape95/workbench/internal/core"
	"github.com/solomonadzape95/workbench/internal/engines/ffmpeg"
	"github.com/solomonadzape95/workbench/internal/ops/media"
)

func bitrateOptions() []core.Option {
	return []core.Option{
		{Value: "96k", Label: "96 kbps"},
		{Value: "128k", Label: "128 kbps"},
		{Value: "160k", Label: "160 kbps"},
		{Value: "192k", Label: "192 kbps"},
		{Value: "256k", Label: "256 kbps"},
		{Value: "320k", Label: "320 kbps"},
	}
}

func sampleRateOptions() []core.Option {
	return []core.Option{
		{Value: "auto", Label: "Keep source"},
		{Value: "44100", Label: "44.1 kHz"},
		{Value: "48000", Label: "48 kHz"},
	}
}

func channelOptions() []core.Option {
	return []core.Option{
		{Value: "auto", Label: "Keep source"},
		{Value: "mono", Label: "Mono"},
		{Value: "stereo", Label: "Stereo"},
	}
}

func formatOptions() []core.Option {
	return []core.Option{
		{Value: "mp3", Label: "MP3"},
		{Value: "m4a", Label: "M4A (AAC)"},
		{Value: "wav", Label: "WAV"},
		{Value: "flac", Label: "FLAC"},
		{Value: "ogg", Label: "Ogg Vorbis"},
		{Value: "opus", Label: "Opus"},
	}
}

// applyChannels adds -ar/-ac when the user overrides the source settings.
func applyChannels(args []string, sampleRate, channels string) []string {
	if sampleRate != "" && sampleRate != "auto" {
		args = append(args, "-ar", sampleRate)
	}
	switch channels {
	case "mono":
		args = append(args, "-ac", "1")
	case "stereo":
		args = append(args, "-ac", "2")
	}
	return args
}

func qualityArgs(args []string, encoder, bitrate string) []string {
	if !media.IsLossless(encoder) && bitrate != "" {
		args = append(args, "-b:a", bitrate)
	}
	return args
}

func audioFormat(item core.Artifact) (string, error) {
	format := strings.ToLower(strings.TrimPrefix(item.Ext(), "."))
	if format == "" {
		return "", fmt.Errorf("cannot determine the source audio format")
	}
	return format, nil
}

func checkAudioInputs(spec core.Spec, req core.Request, client *ffmpeg.Client) error {
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
				fmt.Sprintf("%s is not an audio file", item.Format),
				"supported inputs: MP3, M4A, WAV, FLAC, OGG, OPUS")
		}
	}
	return nil
}
