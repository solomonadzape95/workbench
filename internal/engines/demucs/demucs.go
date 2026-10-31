// Package demucs detects and runs the optional Demucs source-separation tool.
// Demucs is not required: audio.extract_vocals falls back to an ffmpeg
// center-channel isolation when it is unavailable.
package demucs

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

func New(path string) Engine { return Engine{Path: path} }

func (e Engine) ID() engines.ID { return engines.Demucs }

func (e Engine) Probe(parent context.Context) engines.Info {
	info := engines.Info{ID: engines.Demucs, Name: "demucs", Detail: "source separation for audio.extract_vocals"}
	configured := strings.TrimSpace(e.Path)
	name := configured
	if name == "" {
		name = "demucs"
	}
	resolved, err := exec.LookPath(name)
	if err != nil {
		info.Path = configured
		info.Detail = "not found; vocal extraction falls back to ffmpeg center isolation"
		info.Hint = InstallHint()
		info.Err = err
		return info
	}
	ctx, cancel := context.WithTimeout(parent, 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, resolved, "--help")
	executil.Configure(cmd)
	if out, err := cmd.CombinedOutput(); err != nil {
		info.Path = resolved
		info.Detail = "found but failed to run"
		info.Hint = InstallHint()
		info.Err = fmt.Errorf("%s --help: %w (%s)", resolved, err, firstLine(out))
		return info
	}
	info.Path = resolved
	info.Available = true
	info.Version = "installed"
	return info
}

func InstallHint() string {
	switch runtime.GOOS {
	case "darwin", "linux":
		return "python3 -m pip install demucs   # or: pipx install demucs"
	case "windows":
		return "py -m pip install demucs"
	default:
		return "install demucs and make sure the demucs command is on PATH"
	}
}

type Client struct {
	Path string
}

func NewClient(path string) *Client { return &Client{Path: path} }

func (c *Client) Binary() (string, error) {
	name := strings.TrimSpace(c.Path)
	if name == "" {
		name = "demucs"
	}
	resolved, err := exec.LookPath(name)
	if err != nil {
		return "", core.NewError(core.CodeEngineMissing, "audio.extract_vocals", "",
			"demucs is not installed", InstallHint())
	}
	return resolved, nil
}

func (c *Client) Check() error {
	_, err := c.Binary()
	return err
}

// Separate writes a vocals stem under outDir. Demucs creates
// <outDir>/<model>/<stem>/vocals.wav; callers locate it with FindVocals.
func (c *Client) Separate(ctx context.Context, input, model, outDir string) error {
	binary, err := c.Binary()
	if err != nil {
		return err
	}
	if strings.TrimSpace(model) == "" {
		model = "htdemucs"
	}
	args := []string{"--two-stems=vocals", "-n", model, "-o", outDir, input}
	cmd := exec.CommandContext(ctx, binary, args...)
	executil.Configure(cmd)
	cmd.WaitDelay = 5 * time.Second
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		msg := strings.TrimSpace(stderr.String())
		if len(msg) > 600 {
			msg = msg[len(msg)-600:]
		}
		return fmt.Errorf("demucs: %w: %s", err, msg)
	}
	return nil
}

func firstLine(out []byte) string {
	line := strings.TrimSpace(string(out))
	if i := strings.IndexByte(line, '\n'); i >= 0 {
		line = line[:i]
	}
	if len(line) > 160 {
		line = line[:160]
	}
	return line
}
