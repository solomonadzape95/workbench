// Package realesrgan detects and runs the optional Real-ESRGAN ncnn Vulkan
// upscaler. It is optional: image.upscale is only registered when the engine is
// enabled, and its probe reports an install hint when the binary is missing.
package realesrgan

import (
	"context"
	"os/exec"
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

func (e Engine) ID() engines.ID { return engines.RealESRGAN }

func (e Engine) Probe(parent context.Context) engines.Info {
	info := engines.Info{
		ID:     engines.RealESRGAN,
		Name:   "realesrgan",
		Detail: "Real-ESRGAN ncnn Vulkan upscaler for image.upscale",
	}
	name := strings.TrimSpace(e.Path)
	if name == "" {
		name = "realesrgan-ncnn-vulkan"
	}
	resolved, err := exec.LookPath(name)
	if err != nil {
		info.Path = strings.TrimSpace(e.Path)
		info.Hint = InstallHint()
		info.Err = err
		return info
	}
	ctx, cancel := context.WithTimeout(parent, 3*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, resolved, "-h")
	executil.Configure(cmd)
	// Some builds return a non-zero status for -h while still printing usage.
	// A resolved executable is enough to offer the operation, so the probe does
	// not turn a working binary into a false negative.
	_, _ = cmd.CombinedOutput()
	info.Path = resolved
	info.Available = true
	info.Version = "installed"
	return info
}

type Client struct {
	Path string
}

func NewClient(path string) *Client { return &Client{Path: strings.TrimSpace(path)} }

func (c *Client) Binary() (string, error) {
	name := c.Path
	if name == "" {
		name = "realesrgan-ncnn-vulkan"
	}
	resolved, err := exec.LookPath(name)
	if err != nil {
		return "", core.NewError(core.CodeEngineMissing, "image.upscale", "",
			"Real-ESRGAN is not installed", InstallHint())
	}
	return resolved, nil
}

func (c *Client) Check() error {
	_, err := c.Binary()
	return err
}

func InstallHint() string {
	return "download realesrgan-ncnn-vulkan from https://github.com/xinntao/Real-ESRGAN/releases and put it on PATH"
}
