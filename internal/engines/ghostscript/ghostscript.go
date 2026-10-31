// Package ghostscript probes and runs Ghostscript for PDF compression.
package ghostscript

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

func (e Engine) ID() engines.ID { return engines.Ghostscript }

func (e Engine) Probe(parent context.Context) engines.Info {
	info := engines.Info{ID: engines.Ghostscript, Name: "ghostscript", Detail: "PDF compression"}
	name := e.Path
	if name == "" {
		name = "gs"
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
		return "brew install ghostscript"
	case "linux":
		return "sudo apt install ghostscript"
	case "windows":
		return "winget install ArtifexSoftware.GhostScript"
	default:
		return "install Ghostscript and make sure gs is on PATH"
	}
}

type Client struct {
	Path string
}

func NewClient(path string) *Client { return &Client{Path: strings.TrimSpace(path)} }

func (c *Client) Resolve() (string, error) {
	name := c.Path
	if name == "" {
		name = "gs"
	}
	resolved, err := exec.LookPath(name)
	if err != nil {
		return "", core.NewError(core.CodeEngineMissing, "doc.compress", "",
			"ghostscript is not installed", InstallHint())
	}
	return resolved, nil
}

func (c *Client) Check() error {
	_, err := c.Resolve()
	return err
}

func (c *Client) Compress(ctx context.Context, input, output, preset string, rep core.Reporter) error {
	binary, err := c.Resolve()
	if err != nil {
		return err
	}
	if preset == "" {
		preset = "ebook"
	}
	args := []string{
		"-sDEVICE=pdfwrite",
		"-dCompatibilityLevel=1.7",
		"-dPDFSETTINGS=/" + preset,
		"-dNOPAUSE",
		"-dQUIET",
		"-dBATCH",
		"-dSAFER",
		"-dDetectDuplicateImages=true",
		"-sOutputFile=" + output,
		input,
	}
	cmd := exec.CommandContext(ctx, binary, args...)
	executil.Configure(cmd)
	cmd.WaitDelay = 5 * time.Second
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if rep != nil {
		rep.Stage("compressing PDF")
	}
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		msg := strings.TrimSpace(stderr.String())
		if len(msg) > 500 {
			msg = msg[len(msg)-500:]
		}
		return fmt.Errorf("ghostscript: %w: %s", err, msg)
	}
	return nil
}

// Merge combines several PDFs into one file with Ghostscript's pdfwrite device.
func (c *Client) Merge(ctx context.Context, inputs []string, output, preset string, rep core.Reporter) error {
	binary, err := c.Resolve()
	if err != nil {
		return err
	}
	if len(inputs) == 0 {
		return fmt.Errorf("no input PDFs")
	}
	if preset == "" {
		preset = "prepress"
	}
	args := []string{
		"-sDEVICE=pdfwrite",
		"-dCompatibilityLevel=1.7",
		"-dPDFSETTINGS=/" + preset,
		"-dNOPAUSE",
		"-dQUIET",
		"-dBATCH",
		"-dSAFER",
		"-sOutputFile=" + output,
	}
	args = append(args, inputs...)
	cmd := exec.CommandContext(ctx, binary, args...)
	executil.Configure(cmd)
	cmd.WaitDelay = 5 * time.Second
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if rep != nil {
		rep.Stage("merging PDFs")
	}
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		msg := strings.TrimSpace(stderr.String())
		if len(msg) > 500 {
			msg = msg[len(msg)-500:]
		}
		return fmt.Errorf("ghostscript merge: %w: %s", err, msg)
	}
	return nil
}
