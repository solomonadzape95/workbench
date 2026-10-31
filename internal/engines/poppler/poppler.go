// Package poppler probes the pdftotext/pdftoppm tools used for PDF previews.
package poppler

import (
	"context"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/solomonadzape95/workbench/internal/engines"
)

type Engine struct {
	Path string
}

func New(path string) Engine { return Engine{Path: strings.TrimSpace(path)} }

func (e Engine) ID() engines.ID { return engines.Poppler }

func (e Engine) Probe(parent context.Context) engines.Info {
	info := engines.Info{ID: engines.Poppler, Name: "poppler", Detail: "PDF text and previews"}
	name := e.Path
	if name == "" {
		name = "pdftotext"
	}
	path, err := exec.LookPath(name)
	if err != nil {
		info.Hint = InstallHint()
		info.Err = err
		return info
	}
	info.Path = path

	ctx, cancel := context.WithTimeout(parent, 3*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, path, "-v").CombinedOutput()
	if err != nil && len(out) == 0 {
		return info
	}
	info.Available = true
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(lines) > 0 {
		info.Version = strings.TrimSpace(lines[0])
	}
	return info
}

func InstallHint() string {
	switch runtime.GOOS {
	case "darwin":
		return "brew install poppler"
	case "linux":
		return "sudo apt install poppler-utils"
	case "windows":
		return "install poppler and ensure pdftotext is on PATH"
	default:
		return "install poppler and ensure pdftotext is on PATH"
	}
}
