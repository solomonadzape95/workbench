// Package pdf2docx converts PDFs to editable DOCX with layout reconstruction.
package pdf2docx

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/solomonadzape95/workbench/internal/core"
	"github.com/solomonadzape95/workbench/internal/engines"
	"github.com/solomonadzape95/workbench/internal/engines/executil"
)

type Client struct {
	Path string
}

func NewClient(path string) *Client {
	return &Client{Path: strings.TrimSpace(path)}
}

func (c *Client) ID() engines.ID { return engines.PDF2DOCX }

func (c *Client) Resolve() (string, error) {
	name := c.Path
	if name == "" {
		name = "pdf2docx"
	}
	resolved, err := exec.LookPath(name)
	if err != nil {
		return "", core.NewError(core.CodeEngineMissing, "", "",
			"pdf2docx is not installed", InstallHint())
	}
	return resolved, nil
}

func (c *Client) Check() error {
	_, err := c.Resolve()
	return err
}

func (c *Client) Probe(parent context.Context) engines.Info {
	info := engines.Info{ID: engines.PDF2DOCX, Name: "pdf2docx", Detail: "PDF→DOCX with tables and images"}
	path, err := c.Resolve()
	if err != nil {
		info.Hint = InstallHint()
		info.Err = err
		return info
	}
	info.Path = path

	ctx, cancel := context.WithTimeout(parent, 15*time.Second)
	defer cancel()
	// The pdf2docx CLI has no --version flag; a successful help exit is the
	// cheapest way to prove the Python package and its dependencies load.
	if out, err := exec.CommandContext(ctx, path, "--help").CombinedOutput(); err != nil {
		info.Hint = InstallHint()
		info.Detail = "found but failed to run"
		info.Err = fmt.Errorf("%s --help: %w\n%s", path, err, out)
		return info
	}
	info.Available = true
	info.Version = "installed"
	return info
}

func (c *Client) Convert(ctx context.Context, input, output string, rep core.Reporter) error {
	binary, err := c.Resolve()
	if err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, binary, "convert", input, output)
	executil.Configure(cmd)
	cmd.WaitDelay = 10 * time.Second

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	rep.Stage("converting with pdf2docx")
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		detail := lastLine(stderr.String())
		if detail == "" {
			detail = lastLine(stdout.String())
		}
		if detail != "" {
			rep.Log(core.LevelError, "pdf2docx: %s", detail)
		}
		return core.Wrap(core.CodeEngineFailed, "", input,
			fmt.Errorf("pdf2docx failed: %s", detail), hintFor(stdout.String()+"\n"+stderr.String()))
	}
	if info, err := os.Stat(output); err != nil || info.IsDir() {
		return core.NewError(core.CodeOutput, "", input,
			"pdf2docx reported success but produced no DOCX",
			"check that the PDF is not encrypted or damaged")
	}
	return nil
}

func lastLine(text string) string {
	lines := strings.Split(strings.TrimSpace(text), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		if line := strings.TrimSpace(lines[i]); line != "" {
			if len(line) > 300 {
				line = line[:300] + "…"
			}
			return line
		}
	}
	return ""
}

func hintFor(text string) string {
	lower := strings.ToLower(text)
	switch {
	case strings.Contains(lower, "password"), strings.Contains(lower, "encrypted"):
		return "the PDF is encrypted; remove the password first"
	case strings.Contains(lower, "no such file"):
		return "check that the input PDF still exists"
	case strings.Contains(lower, "permission"):
		return "check permissions on the input and output paths"
	case strings.Contains(lower, "module"), strings.Contains(lower, "import"):
		return `reinstall: python3 -m pip install --upgrade pdf2docx`
	default:
		return "try a simpler PDF, or fall back to LibreOffice in the step editor"
	}
}

func InstallHint() string {
	switch runtime.GOOS {
	case "darwin", "linux", "windows":
		return `python3 -m pip install pdf2docx`
	default:
		return "install pdf2docx with pip and make sure it is on PATH"
	}
}
