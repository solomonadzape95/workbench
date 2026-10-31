// Package ffmpeg detects and describes an ffmpeg installation.
package ffmpeg

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"

	"github.com/solomonadzape95/workbench/internal/engines"
)

var versionRE = regexp.MustCompile(`(?m)^\s*ffmpeg version ([^\s]+)`)

// Engine probes the ffmpeg binary. Path may be empty (look on PATH) or an
// explicit path/name from the config file.
type Engine struct {
	Path string
}

func New(path string) Engine {
	return Engine{Path: path}
}

func (e Engine) ID() engines.ID { return engines.FFmpeg }

func (e Engine) Probe(parent context.Context) engines.Info {
	info := engines.Info{ID: engines.FFmpeg, Name: "ffmpeg"}

	ctx, cancel := context.WithTimeout(parent, 3*time.Second)
	defer cancel()

	configured := strings.TrimSpace(e.Path)
	path := configured
	if path == "" {
		path = "ffmpeg"
	}
	resolved, err := exec.LookPath(path)
	if err != nil {
		info.Path = configured
		info.Hint = InstallHint()
		info.Detail = "not found"
		info.Err = err
		return info
	}
	path = resolved
	info.Path = path

	out, err := exec.CommandContext(ctx, path, "-version").CombinedOutput()
	if err != nil {
		info.Hint = InstallHint()
		info.Detail = "found but failed to run"
		info.Err = fmt.Errorf("%s -version: %w", path, err)
		return info
	}

	info.Available = true
	info.Version = parseVersion(out)
	info.Detail = probeFFprobe(path)
	return info
}

func parseVersion(out []byte) string {
	if m := versionRE.FindSubmatch(out); len(m) == 2 {
		return string(m[1])
	}
	return "unknown"
}

// probeFFprobe reports whether ffprobe sits next to ffmpeg or on PATH. Duration
// and stream metadata depend on it, so the answer belongs in doctor output.
func probeFFprobe(ffmpegPath string) string {
	candidates := []string{filepath.Join(filepath.Dir(ffmpegPath), "ffprobe"), "ffprobe"}
	for _, candidate := range candidates {
		if p, err := exec.LookPath(candidate); err == nil {
			return "ffprobe: " + p
		}
	}
	return "ffprobe missing (duration and stream metadata unavailable)"
}

// InstallHint returns a copy-pasteable install command for the current OS.
func InstallHint() string {
	switch runtime.GOOS {
	case "darwin":
		return "brew install ffmpeg"
	case "linux":
		return "sudo apt install ffmpeg   # or your distribution's package manager"
	case "windows":
		return "winget install Gyan.FFmpeg"
	default:
		return "install ffmpeg and make sure it is on PATH"
	}
}
