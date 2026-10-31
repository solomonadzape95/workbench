// Package rembg removes image backgrounds with the rembg CLI.
package rembg

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

func (c *Client) ID() engines.ID { return engines.Rembg }

func (c *Client) Resolve() (string, error) {
	name := c.Path
	if name == "" {
		name = "rembg"
	}
	resolved, err := exec.LookPath(name)
	if err != nil {
		return "", core.NewError(core.CodeEngineMissing, "", "",
			"rembg is not installed", InstallHint())
	}
	return resolved, nil
}

func (c *Client) Check() error {
	_, err := c.Resolve()
	return err
}

func (c *Client) Probe(parent context.Context) engines.Info {
	info := engines.Info{ID: engines.Rembg, Name: "rembg", Detail: "background removal (first run downloads a model)"}
	path, err := c.Resolve()
	if err != nil {
		info.Hint = InstallHint()
		info.Err = err
		return info
	}
	info.Path = path

	ctx, cancel := context.WithTimeout(parent, 15*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, path, "--version").CombinedOutput()
	if err != nil {
		info.Hint = InstallHint()
		info.Detail = "found but failed to run"
		info.Err = err
		return info
	}
	info.Available = true
	info.Version = strings.TrimSpace(strings.TrimPrefix(string(out), "rembg, version "))
	return info
}

type Options struct {
	Model        string
	AlphaMatting bool
}

func (c *Client) Remove(ctx context.Context, input, output string, opts Options, rep core.Reporter) error {
	binary, err := c.Resolve()
	if err != nil {
		return err
	}
	model := strings.TrimSpace(opts.Model)
	if model == "" {
		model = "bria-rmbg"
	}
	args := []string{"i", "-m", model}
	if opts.AlphaMatting {
		args = append(args, "-a")
	}
	args = append(args, input, output)

	cmd := exec.CommandContext(ctx, binary, args...)
	executil.Configure(cmd)
	cmd.WaitDelay = 10 * time.Second

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	rep.Stage("removing background")
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		detail := lastLine(stderr.String())
		if detail == "" {
			detail = lastLine(stdout.String())
		}
		if detail != "" {
			rep.Log(core.LevelError, "rembg: %s", detail)
		}
		return core.Wrap(core.CodeEngineFailed, "", input,
			fmt.Errorf("rembg failed: %s", detail), hintFor(stderr.String()+"\n"+stdout.String()))
	}
	if info, err := os.Stat(output); err != nil || info.IsDir() {
		return core.NewError(core.CodeOutput, "", input,
			"rembg reported success but produced no image",
			"check that the input is a supported image")
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
	case strings.Contains(lower, "onnxruntime"):
		return `install the CPU backend: python3 -m pip install "rembg[cpu]"`
	case strings.Contains(lower, "no module named"):
		return `install the CLI extras: python3 -m pip install "rembg[cli,cpu]"`
	case strings.Contains(lower, "download"), strings.Contains(lower, "http"), strings.Contains(lower, "connection"):
		return "the model could not be downloaded; check your network and ~/.rembg permissions"
	case strings.Contains(lower, "cannot identify image"), strings.Contains(lower, "unsupported image"):
		return "use a PNG, JPEG, or GIF input"
	default:
		return "run rembg i <input> <output> once to warm the model, then retry"
	}
}

func InstallHint() string {
	switch runtime.GOOS {
	case "darwin", "linux":
		return `python3 -m pip install "rembg[cli,cpu]"   # model downloads on first use`
	case "windows":
		return `py -m pip install "rembg[cli,cpu]"   # model downloads on first use`
	default:
		return `install rembg with pip and make sure rembg is on PATH`
	}
}

// OutputName is the PNG name a background removal produces.
func OutputName(dir, stem string) string {
	return filepath.Join(dir, core.SanitizeStem(stem)+".png")
}
