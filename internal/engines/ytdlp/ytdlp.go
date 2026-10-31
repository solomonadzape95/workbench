// Package ytdlp downloads media from URLs with yt-dlp.
package ytdlp

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
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

func (c *Client) ID() engines.ID { return engines.YtDlp }

func (c *Client) Resolve() (string, error) {
	name := c.Path
	if name == "" {
		name = "yt-dlp"
	}
	resolved, err := exec.LookPath(name)
	if err != nil {
		return "", core.NewError(core.CodeEngineMissing, "", "",
			"yt-dlp is not installed", InstallHint())
	}
	return resolved, nil
}

func (c *Client) Check() error {
	_, err := c.Resolve()
	return err
}

func (c *Client) Probe(parent context.Context) engines.Info {
	info := engines.Info{ID: engines.YtDlp, Name: "yt-dlp", Detail: "video and audio downloads"}
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
	info.Version = strings.TrimSpace(string(out))
	return info
}

type Options struct {
	Quality   string // best, 1080p, 720p, 480p
	Container string // mp4, mkv
	Playlist  bool
}

// Download fetches url into workDir and returns the produced media files.
func (c *Client) Download(ctx context.Context, url, workDir string, opts Options, rep core.Reporter) ([]string, error) {
	binary, err := c.Resolve()
	if err != nil {
		return nil, err
	}
	container := opts.Container
	if container == "" {
		container = "mp4"
	}
	args := []string{
		"--newline",
		"--no-color",
		"--no-update",
		"--no-write-subs",
		"--no-write-thumbnail",
		"--no-write-info-json",
		"--progress",
		"--progress-template", "workbench:%(progress.downloaded_bytes)s|%(progress.total_bytes)s|%(progress.total_bytes_estimate)s",
		"-f", FormatSelector(opts.Quality),
		"--format-sort", "vcodec:h264,acodec:aac",
		"--merge-output-format", container,
		"-P", workDir,
		"-o", "%(title).180B [%(id)s].%(ext)s",
	}
	if !opts.Playlist {
		args = append(args, "--no-playlist")
	}
	args = append(args, url)

	cmd := exec.CommandContext(ctx, binary, args...)
	executil.Configure(cmd)
	cmd.WaitDelay = 10 * time.Second

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, core.Wrap(core.CodeInternal, "", "", err, "")
	}
	var stderr bytes.Buffer
	var transcript strings.Builder
	cmd.Stderr = &stderr

	rep.Stage("downloading")
	if err := cmd.Start(); err != nil {
		return nil, core.Wrap(core.CodeEngineFailed, "", url, err, "check the yt-dlp installation")
	}

	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 64*1024), 2<<20)
	for scanner.Scan() {
		line := scanner.Text()
		transcript.WriteString(line)
		transcript.WriteByte('\n')
		parseProgress(line, rep)
	}
	scanErr := scanner.Err()
	waitErr := cmd.Wait()

	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if waitErr != nil {
		logStderr(rep, stderr.String())
		detail := lastLine(stderr.String())
		if detail == "" {
			detail = lastLine(transcript.String())
		}
		return nil, core.Wrap(core.CodeEngineFailed, "", url,
			fmt.Errorf("yt-dlp failed: %s", detail), hintFor(stderr.String()+"\n"+transcript.String()))
	}
	if scanErr != nil {
		return nil, core.Wrap(core.CodeEngineFailed, "", url, scanErr, "")
	}

	files, err := mediaFiles(workDir)
	if err != nil {
		return nil, err
	}
	if len(files) == 0 {
		return nil, core.NewError(core.CodeOutput, "", url,
			"yt-dlp reported success but produced no media file",
			"check the URL and whether the site requires authentication")
	}
	return files, nil
}

func FormatSelector(quality string) string {
	switch quality {
	case "1080p":
		return "bv*[height<=1080]+ba/b[height<=1080]"
	case "720p":
		return "bv*[height<=720]+ba/b[height<=720]"
	case "480p":
		return "bv*[height<=480]+ba/b[height<=480]"
	default:
		return "bv*+ba/b"
	}
}

func parseProgress(line string, rep core.Reporter) {
	if strings.HasPrefix(line, "workbench:") {
		parts := strings.Split(strings.TrimPrefix(line, "workbench:"), "|")
		if len(parts) >= 2 {
			done, _ := strconv.ParseInt(strings.TrimSpace(parts[0]), 10, 64)
			total, _ := strconv.ParseInt(strings.TrimSpace(parts[1]), 10, 64)
			if total <= 0 && len(parts) >= 3 {
				total, _ = strconv.ParseInt(strings.TrimSpace(parts[2]), 10, 64)
			}
			if total > 0 {
				rep.Progress(int(min64(done, total)), int(total))
				return
			}
			if done > 0 {
				rep.Progress(int(done), 0)
			}
		}
		return
	}
	if strings.Contains(line, "[download]") && strings.Contains(line, "%") {
		fields := strings.Fields(line)
		for _, field := range fields {
			if !strings.HasSuffix(field, "%") {
				continue
			}
			pct, err := strconv.ParseFloat(strings.TrimSuffix(field, "%"), 64)
			if err == nil {
				rep.Progress(int(pct), 100)
				return
			}
		}
	}
}

func mediaFiles(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if strings.HasSuffix(name, ".part") || strings.HasSuffix(name, ".ytdl") || strings.HasSuffix(name, ".temp") {
			continue
		}
		switch strings.ToLower(filepath.Ext(name)) {
		case ".mp4", ".mkv", ".webm", ".mov", ".m4v", ".m4a", ".mp3", ".opus", ".flac", ".wav":
			out = append(out, filepath.Join(dir, name))
		}
	}
	sort.Strings(out)
	return out, nil
}

func logStderr(rep core.Reporter, text string) {
	for _, line := range strings.Split(strings.TrimSpace(text), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		level := core.LevelInfo
		upper := strings.ToUpper(line)
		switch {
		case strings.Contains(upper, "ERROR"):
			level = core.LevelError
		case strings.Contains(upper, "WARNING"):
			level = core.LevelWarn
		}
		rep.Log(level, "yt-dlp: %s", line)
	}
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
	case strings.Contains(lower, "unsupported url"):
		return "check that the URL is a supported video page or direct media link"
	case strings.Contains(lower, "sign in"), strings.Contains(lower, "cookies"):
		return "the site requires authentication; cookies are not supported yet"
	case strings.Contains(lower, "ffmpeg"):
		return "install ffmpeg so yt-dlp can merge video and audio"
	case strings.Contains(lower, "video unavailable"), strings.Contains(lower, "private video"):
		return "the video is unavailable, private, or region-locked"
	case strings.Contains(lower, "http error 403"), strings.Contains(lower, "403 forbidden"):
		return "the site rejected the download (HTTP 403). Update yt-dlp (`yt-dlp -U`), then retry. If it persists, the site may require browser cookies."
	case strings.Contains(lower, "http error 404"):
		return "the URL returned 404; check the link"
	case strings.Contains(lower, "requested format is not available"):
		return "the selected quality is unavailable for this video; retry with Best or another quality"
	case strings.Contains(lower, "not a bot"), strings.Contains(lower, "sign in to confirm"):
		return "the site is asking for a signed-in browser session; update yt-dlp or add cookies (not supported yet)"
	default:
		return "check the URL with `yt-dlp --simulate <url>` outside Workbench"
	}
}

func min64(a, b int64) int64 {
	if a < b {
		return a
	}
	return b
}

func InstallHint() string {
	switch runtime.GOOS {
	case "darwin":
		return "brew install yt-dlp"
	case "linux":
		return "pipx install yt-dlp   # or your distribution's package manager"
	case "windows":
		return "winget install yt-dlp.yt-dlp"
	default:
		return "install yt-dlp and ensure it is on PATH"
	}
}
