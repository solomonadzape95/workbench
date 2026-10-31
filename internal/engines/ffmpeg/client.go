package ffmpeg

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/solomonadzape95/workbench/internal/core"
	"github.com/solomonadzape95/workbench/internal/engines/executil"
)

// Client executes ffmpeg and ffprobe for a resolved binary path. It is safe to
// share between operations and jobs.
type Client struct {
	Path string // configured ffmpeg path; empty means look on PATH
}

func NewClient(path string) *Client {
	return &Client{Path: strings.TrimSpace(path)}
}

// Check verifies the binary exists without running a conversion.
func (c *Client) Check() error {
	_, err := c.Binary()
	return err
}

func (c *Client) Binary() (string, error) {
	name := c.Path
	if name == "" {
		name = "ffmpeg"
	}
	resolved, err := exec.LookPath(name)
	if err != nil {
		return "", core.NewError(core.CodeEngineMissing, "", "",
			"ffmpeg is not installed", InstallHint())
	}
	return resolved, nil
}

func (c *Client) FFprobeBinary() (string, error) {
	ffmpegPath, err := c.Binary()
	if err != nil {
		return "", err
	}
	candidates := []string{filepath.Join(filepath.Dir(ffmpegPath), "ffprobe"), "ffprobe"}
	for _, candidate := range candidates {
		if resolved, err := exec.LookPath(candidate); err == nil {
			return resolved, nil
		}
	}
	return "", core.NewError(core.CodeEngineMissing, "", "",
		"ffprobe is not installed", InstallHint())
}

// Run executes ffmpeg with progress reporting. args must include the input and
// output (for example: -i in.mp3 -c:a aac out.m4a).
func (c *Client) Run(ctx context.Context, args []string, duration time.Duration, rep core.Reporter) error {
	binary, err := c.Binary()
	if err != nil {
		return err
	}

	full := append([]string{
		"-hide_banner",
		"-nostdin",
		"-y",
		"-loglevel", "error",
		"-nostats",
		"-progress", "pipe:1",
	}, args...)

	cmd := exec.CommandContext(ctx, binary, full...)
	executil.Configure(cmd)
	cmd.WaitDelay = 5 * time.Second

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return core.Wrap(core.CodeInternal, "", "", err, "")
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Start(); err != nil {
		return core.Wrap(core.CodeEngineFailed, "", "", err, "check the configured ffmpeg path")
	}

	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 64*1024), 1<<20)

	haveMicros := false
	for scanner.Scan() {
		line := scanner.Text()
		switch {
		case strings.HasPrefix(line, "out_time_us="):
			if us, err := strconv.ParseInt(strings.TrimPrefix(line, "out_time_us="), 10, 64); err == nil && us >= 0 {
				haveMicros = true
				reportProgress(rep, time.Duration(us)*time.Microsecond, duration)
			}
		case strings.HasPrefix(line, "out_time_ms=") && !haveMicros:
			// Despite the name, this field has historically carried microseconds.
			if us, err := strconv.ParseInt(strings.TrimPrefix(line, "out_time_ms="), 10, 64); err == nil && us >= 0 {
				reportProgress(rep, time.Duration(us)*time.Microsecond, duration)
			}
		}
	}
	scanErr := scanner.Err()
	waitErr := cmd.Wait()

	if ctx.Err() != nil {
		return ctx.Err()
	}
	if waitErr != nil {
		if text := strings.TrimSpace(stderr.String()); text != "" {
			rep.Log(core.LevelError, "ffmpeg stderr:\n%s", text)
		}
		return exitError(waitErr, stderr.String())
	}
	if scanErr != nil && !errors.Is(scanErr, os.ErrClosed) {
		return core.Wrap(core.CodeEngineFailed, "", "", scanErr, "ffmpeg closed its progress stream unexpectedly")
	}

	if duration > 0 {
		rep.Progress(int(duration.Milliseconds()), int(duration.Milliseconds()))
	}
	return nil
}

func reportProgress(rep core.Reporter, done, total time.Duration) {
	if total <= 0 {
		rep.Progress(int(done.Milliseconds()), 0)
		return
	}
	if done > total {
		done = total
	}
	rep.Progress(int(done.Milliseconds()), int(total.Milliseconds()))
}

func exitError(waitErr error, stderr string) error {
	line := lastLine(stderr)
	message := "ffmpeg failed"
	if line != "" {
		message = "ffmpeg: " + line
	}
	cause := fmt.Errorf("%s (%w)", message, waitErr)
	return core.Wrap(core.CodeEngineFailed, "", "", cause, hintFor(stderr))
}

func hintFor(stderr string) string {
	lower := strings.ToLower(stderr)
	switch {
	case strings.Contains(lower, "no such file or directory"):
		return "check that the input file still exists and is readable"
	case strings.Contains(lower, "invalid data found"):
		return "the file may be corrupt or not a media file"
	case strings.Contains(lower, "unknown encoder"), strings.Contains(lower, "encoder not found"):
		return "this ffmpeg build does not include the requested codec; choose another codec"
	case strings.Contains(lower, "permission denied"):
		return "check permissions on the input and output paths"
	case strings.Contains(lower, "does not contain any stream"):
		return "the input has no usable audio or video stream"
	case strings.Contains(lower, "invalid argument"):
		return "one of the encoding options is not supported by this ffmpeg build"
	default:
		return "run wb doctor, then retry with a different codec or format"
	}
}

func lastLine(s string) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if line != "" {
			if len(line) > 300 {
				line = line[:300] + "…"
			}
			return line
		}
	}
	return ""
}
