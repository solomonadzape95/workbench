// Package tesseract probes and runs the Tesseract OCR CLI.
package tesseract

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/solomonadzape95/workbench/internal/core"
	"github.com/solomonadzape95/workbench/internal/engines"
	"github.com/solomonadzape95/workbench/internal/engines/executil"
)

type Engine struct {
	Path string
}

func New(path string) Engine { return Engine{Path: strings.TrimSpace(path)} }

func (e Engine) ID() engines.ID { return engines.Tesseract }

func (e Engine) Probe(parent context.Context) engines.Info {
	info := engines.Info{ID: engines.Tesseract, Name: "tesseract", Detail: "OCR for scanned images and PDF pages"}
	name := e.Path
	if name == "" {
		name = "tesseract"
	}
	resolved, err := exec.LookPath(name)
	if err != nil {
		info.Hint = InstallHint()
		info.Err = err
		return info
	}
	info.Path = resolved
	ctx, cancel := context.WithTimeout(parent, 3*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, resolved, "--version").CombinedOutput()
	if err != nil {
		info.Hint = InstallHint()
		info.Detail = "found but failed to run"
		info.Err = err
		return info
	}
	info.Available = true
	info.Version = strings.TrimSpace(strings.Split(string(out), "\n")[0])
	return info
}

func InstallHint() string {
	switch runtime.GOOS {
	case "darwin":
		return "brew install tesseract"
	case "linux":
		return "sudo apt install tesseract-ocr"
	case "windows":
		return "winget install UB-Mannheim.TesseractOCR"
	default:
		return "install tesseract and make sure it is on PATH"
	}
}

type Client struct {
	Path string
}

func NewClient(path string) *Client { return &Client{Path: strings.TrimSpace(path)} }

func (c *Client) Resolve() (string, error) {
	name := c.Path
	if name == "" {
		name = "tesseract"
	}
	resolved, err := exec.LookPath(name)
	if err != nil {
		return "", core.NewError(core.CodeEngineMissing, "doc.ocr", "",
			"tesseract is not installed", InstallHint())
	}
	return resolved, nil
}

func (c *Client) Check() error {
	_, err := c.Resolve()
	return err
}

// OCR writes <base>.<format> and returns that path. format is txt or pdf.
func (c *Client) OCR(ctx context.Context, input, base, format, language string, rep core.Reporter) (string, error) {
	binary, err := c.Resolve()
	if err != nil {
		return "", err
	}
	format = strings.ToLower(strings.TrimSpace(format))
	switch format {
	case "", "txt":
		format = "txt"
	case "pdf":
	default:
		return "", core.NewError(core.CodeUsage, "doc.ocr", "", "unsupported OCR output "+format, "choose txt or pdf")
	}
	language = strings.TrimSpace(language)
	if language == "" {
		language = "eng"
	}
	cmd := exec.CommandContext(ctx, binary, input, base, "-l", language, format)
	executil.Configure(cmd)
	cmd.WaitDelay = 5 * time.Second
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if rep != nil {
		rep.Stage("running OCR")
	}
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		msg := strings.TrimSpace(stderr.String())
		if len(msg) > 500 {
			msg = msg[len(msg)-500:]
		}
		return "", fmt.Errorf("tesseract: %w: %s", err, msg)
	}
	return base + "." + format, nil
}
