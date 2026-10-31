package audio

import (
	"fmt"
	"strconv"
	"time"

	"github.com/solomonadzape95/workbench/internal/core"
	"github.com/solomonadzape95/workbench/internal/ops/media"
)

// resolveAudioSource maps an input artifact to its container format and the
// ffmpeg encoder that keeps that container.
func resolveAudioSource(spec core.Spec, item core.Artifact) (format, encoder string, err error) {
	format, err = audioFormat(item)
	if err != nil {
		return "", "", core.NewError(core.CodeInput, spec.ID, item.Path, err.Error(), "")
	}
	encoder, _, ok := media.AudioEncoder(format)
	if !ok {
		return "", "", core.NewError(core.CodeInput, spec.ID, item.Path,
			fmt.Sprintf("unsupported audio format %q", format), "")
	}
	return format, encoder, nil
}

// validateAudioSources rejects unsupported containers before ffmpeg runs.
func validateAudioSources(spec core.Spec, req core.Request) error {
	for _, item := range req.Items {
		if _, _, err := resolveAudioSource(spec, item); err != nil {
			return err
		}
	}
	return nil
}

// audioSeconds renders a duration as the decimal seconds ffmpeg accepts.
func audioSeconds(d time.Duration) string {
	return strconv.FormatFloat(d.Seconds(), 'f', 3, 64)
}

// audioDecimal renders a numeric filter argument without trailing zeros.
func audioDecimal(v float64) string {
	return strconv.FormatFloat(v, 'f', -1, 64)
}
