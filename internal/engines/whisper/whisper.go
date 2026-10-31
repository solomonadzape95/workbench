// Package whisper transcribes audio with the whisper.cpp CLI.
package whisper

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

func (c *Client) ID() engines.ID { return engines.Whisper }

func (c *Client) Resolve() (string, error) {
	name := c.Path
	if name == "" {
		for _, candidate := range []string{"whisper-cli", "whisper-cpp", "whisper"} {
			if resolved, err := exec.LookPath(candidate); err == nil {
				return resolved, nil
			}
		}
		return "", core.NewError(core.CodeEngineMissing, "", "",
			"whisper.cpp is not installed", InstallHint())
	}
	resolved, err := exec.LookPath(name)
	if err != nil {
		return "", core.NewError(core.CodeEngineMissing, "", "",
			"whisper.cpp is not installed", InstallHint())
	}
	return resolved, nil
}

func (c *Client) Check() error {
	_, err := c.Resolve()
	return err
}

func (c *Client) Probe(parent context.Context) engines.Info {
	info := engines.Info{ID: engines.Whisper, Name: "whisper.cpp", Detail: "audio transcription (small.en model)"}
	path, err := c.Resolve()
	if err != nil {
		info.Hint = InstallHint()
		info.Err = err
		return info
	}
	info.Path = path
	model, err := c.ModelPath("small.en")
	if err != nil {
		info.Hint = modelHint("small.en")
		info.Detail = "installed; no ggml-small.en.bin found"
		info.Err = err
		return info
	}
	info.Available = true
	info.Version = "installed"
	info.Detail = "small.en: " + filepath.Base(model)
	return info
}

// ModelPath resolves a model name/path to a local ggml file.
func (c *Client) ModelPath(model string) (string, error) {
	model = strings.TrimSpace(model)
	if model == "" {
		model = "small.en"
	}
	if strings.ContainsAny(model, "/\\") {
		if fileExists(model) {
			return model, nil
		}
		return "", fmt.Errorf("model file not found: %s", model)
	}
	name := model
	if !strings.HasPrefix(name, "ggml-") {
		name = "ggml-" + name
	}
	if !strings.HasSuffix(name, ".bin") {
		name += ".bin"
	}
	home, _ := os.UserHomeDir()
	dirs := []string{
		os.Getenv("WORKBENCH_WHISPER_MODEL_DIR"),
		filepath.Join(os.Getenv("XDG_DATA_HOME"), "whisper-models"),
		filepath.Join(home, ".local", "share", "whisper-models"),
		filepath.Join(home, ".cache", "whisper"),
		filepath.Join(home, ".local", "share", "workbench", "models"),
		"models",
	}
	for _, dir := range dirs {
		if strings.TrimSpace(dir) == "" {
			continue
		}
		path := filepath.Join(dir, name)
		if fileExists(path) {
			return path, nil
		}
	}
	return "", fmt.Errorf("whisper model %s is not installed", model)
}

type Options struct {
	Model    string // resolved model path
	Language string
	Format   string // txt, srt, vtt, json
}

// Transcribe writes <prefix>.<format> and returns that path.
func (c *Client) Transcribe(ctx context.Context, input, prefix string, opts Options, rep core.Reporter) (string, error) {
	binary, err := c.Resolve()
	if err != nil {
		return "", err
	}
	format := strings.ToLower(strings.TrimSpace(opts.Format))
	flag := ""
	switch format {
	case "", "txt":
		format, flag = "txt", "-otxt"
	case "srt":
		flag = "-osrt"
	case "vtt":
		flag = "-ovtt"
	case "json":
		flag = "-oj"
	default:
		return "", core.NewError(core.CodeUsage, "", "", "unsupported transcript format "+format, "choose txt, srt, vtt, or json")
	}
	language := strings.TrimSpace(opts.Language)
	if language == "" {
		language = "en"
	}
	args := []string{"-m", opts.Model, "-f", input, "-l", language, "-of", prefix, flag}
	cmd := exec.CommandContext(ctx, binary, args...)
	executil.Configure(cmd)
	cmd.WaitDelay = 10 * time.Second

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	rep.Stage("transcribing")
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		detail := lastLine(stderr.String())
		if detail == "" {
			detail = lastLine(stdout.String())
		}
		if detail != "" {
			rep.Log(core.LevelError, "whisper: %s", detail)
		}
		return "", core.Wrap(core.CodeEngineFailed, "", input,
			fmt.Errorf("whisper failed: %s", detail), hintFor(stdout.String()+"\n"+stderr.String()))
	}
	output := prefix + "." + format
	if info, err := os.Stat(output); err != nil || info.IsDir() {
		return "", core.NewError(core.CodeOutput, "", input,
			"whisper reported success but produced no transcript",
			"try a shorter audio file or a different model")
	}
	return output, nil
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
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
	case strings.Contains(lower, "failed to open"), strings.Contains(lower, "model"):
		return "check the model path; download a ggml model and point WORKBENCH_WHISPER_MODEL_DIR at it"
	case strings.Contains(lower, "no such file"), strings.Contains(lower, "not found"):
		return "check that the audio file still exists"
	case strings.Contains(lower, "unsupported"):
		return "whisper accepts WAV, MP3, FLAC, and OGG; convert other audio first"
	default:
		return "try a smaller model (base.en) or shorter audio"
	}
}

func modelHint(model string) string {
	return "download " + model + ": curl -L -o ~/.local/share/whisper-models/ggml-" + model + ".bin https://huggingface.co/ggerganov/whisper.cpp/resolve/main/ggml-" + model + ".bin"
}

func InstallHint() string {
	switch runtime.GOOS {
	case "darwin":
		return "brew install whisper-cpp   # then download a ggml model"
	case "linux":
		return "build whisper.cpp or install whisper-cpp, then download a ggml model"
	case "windows":
		return "install whisper.cpp and add whisper-cli to PATH, then download a ggml model"
	default:
		return "install whisper.cpp and download a ggml model"
	}
}
