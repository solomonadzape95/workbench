// Package pandoc runs document conversions with the pandoc CLI.
package pandoc

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
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

func (c *Client) ID() engines.ID { return engines.Pandoc }

func (c *Client) Resolve() (string, error) {
	name := c.Path
	if name == "" {
		name = "pandoc"
	}
	resolved, err := exec.LookPath(name)
	if err != nil {
		return "", core.NewError(core.CodeEngineMissing, "", "",
			"pandoc is not installed", InstallHint())
	}
	return resolved, nil
}

func (c *Client) Check() error {
	_, err := c.Resolve()
	return err
}

func (c *Client) Probe(parent context.Context) engines.Info {
	info := engines.Info{ID: engines.Pandoc, Name: "pandoc", Detail: "Markdown, HTML, DOCX, ODT, EPUB"}
	path, err := c.Resolve()
	if err != nil {
		info.Hint = InstallHint()
		info.Err = err
		return info
	}
	info.Path = path

	ctx, cancel := context.WithTimeout(parent, 5*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, path, "--version").CombinedOutput()
	if err != nil {
		info.Hint = InstallHint()
		info.Detail = "found but failed to run"
		info.Err = err
		return info
	}
	info.Available = true
	info.Version = firstLine(out)
	return info
}

// Supports reports whether pandoc can read inputExt and write targetExt.
func (c *Client) Supports(inputExt, targetExt string) bool {
	in := normalizeExt(inputExt)
	out := normalizeExt(targetExt)
	switch in {
	case "md", "markdown", "html", "htm", "docx", "odt", "epub", "txt", "rst", "tex", "latex", "org":
	default:
		return false
	}
	switch out {
	case "md", "html", "docx", "odt", "epub", "txt", "rst", "tex":
		return true
	default:
		return false
	}
}

func (c *Client) Convert(ctx context.Context, input, output string, rep core.Reporter) error {
	binary, err := c.Resolve()
	if err != nil {
		return err
	}
	writer := writerFor(filepath.Ext(output))
	if writer == "" {
		return core.NewError(core.CodeUsage, "", "", "pandoc cannot write this format", "")
	}
	args := []string{input, "-o", output, "-t", writer}
	cmd := exec.CommandContext(ctx, binary, args...)
	executil.Configure(cmd)
	cmd.WaitDelay = 5 * time.Second

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	rep.Stage("converting with pandoc")
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		text := strings.TrimSpace(stderr.String())
		if text != "" {
			rep.Log(core.LevelError, "pandoc: %s", text)
		}
		return core.Wrap(core.CodeEngineFailed, "", input,
			fmt.Errorf("pandoc failed: %s", firstLine([]byte(text))), hintFor(text))
	}
	if info, err := os.Stat(output); err != nil || info.IsDir() {
		return core.NewError(core.CodeOutput, "", input,
			"pandoc reported success but produced no output", "check the input document")
	}
	return nil
}

func writerFor(ext string) string {
	switch normalizeExt(ext) {
	case "md", "markdown":
		return "markdown"
	case "html", "htm":
		return "html5"
	case "docx":
		return "docx"
	case "odt":
		return "odt"
	case "epub":
		return "epub"
	case "txt":
		return "plain"
	case "rst":
		return "rst"
	case "tex":
		return "latex"
	default:
		return ""
	}
}

func normalizeExt(ext string) string {
	ext = strings.ToLower(strings.TrimPrefix(ext, "."))
	switch ext {
	case "markdown":
		return "md"
	case "htm":
		return "html"
	default:
		return ext
	}
}

func firstLine(out []byte) string {
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(lines) == 0 {
		return ""
	}
	return strings.TrimSpace(lines[0])
}

func hintFor(stderr string) string {
	lower := strings.ToLower(stderr)
	switch {
	case strings.Contains(lower, "no such file"):
		return "check that the input file still exists"
	case strings.Contains(lower, "unknown reader"), strings.Contains(lower, "could not deduce format"):
		return "pandoc could not detect the input format; convert it to Markdown or DOCX first"
	case strings.Contains(lower, "permission denied"):
		return "check permissions on the input and output paths"
	default:
		return "try a different target format or open the document in LibreOffice"
	}
}

func InstallHint() string {
	switch runtime.GOOS {
	case "darwin":
		return "brew install pandoc"
	case "linux":
		return "sudo apt install pandoc   # or your distribution's package manager"
	case "windows":
		return "winget install JohnMacFarlane.Pandoc"
	default:
		return "install pandoc and ensure it is on PATH"
	}
}
