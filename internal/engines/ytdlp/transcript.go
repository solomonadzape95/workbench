package ytdlp

import (
	"bytes"
	"context"
	"fmt"
	"html"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/solomonadzape95/workbench/internal/core"
	"github.com/solomonadzape95/workbench/internal/engines/executil"
)

type TranscriptOptions struct {
	Language   string
	Auto       bool
	Timestamps bool
}

type TranscriptResult struct {
	Path     string
	Title    string
	Language string
}

// Transcript fetches captions with yt-dlp and writes a plain-text transcript.
// It prefers the requested language, falling back to the first caption track
// yt-dlp produces.
func (c *Client) Transcript(ctx context.Context, url, workDir string, opts TranscriptOptions, rep core.Reporter) (TranscriptResult, error) {
	binary, err := c.Resolve()
	if err != nil {
		return TranscriptResult{}, err
	}
	language := strings.TrimSpace(opts.Language)
	if language == "" {
		language = "en"
	}

	args := []string{
		"--newline",
		"--no-color",
		"--no-update",
		"--no-warnings",
		"--skip-download",
		"--write-subs",
		"--sub-langs", language,
		"--sub-format", "vtt",
		"-P", workDir,
		"-o", "%(title).180B [%(id)s].%(ext)s",
	}
	if opts.Auto {
		args = append(args, "--write-auto-subs")
	}
	args = append(args, "--no-playlist", url)

	cmd := exec.CommandContext(ctx, binary, args...)
	executil.Configure(cmd)
	cmd.WaitDelay = 10 * time.Second

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	rep.Stage("fetching captions")
	err = cmd.Run()
	if ctx.Err() != nil {
		return TranscriptResult{}, ctx.Err()
	}
	if err != nil {
		logStderr(rep, stderr.String())
		detail := lastLine(stderr.String())
		if detail == "" {
			detail = lastLine(stdout.String())
		}
		return TranscriptResult{}, core.Wrap(core.CodeEngineFailed, "", url,
			fmt.Errorf("yt-dlp subtitles failed: %s", detail), hintFor(stderr.String()+"\n"+stdout.String()))
	}

	files, err := subtitleFiles(workDir, language)
	if err != nil {
		return TranscriptResult{}, err
	}
	if len(files) == 0 {
		return TranscriptResult{}, core.NewError(core.CodeInput, "", url,
			fmt.Sprintf("no captions found in %q", language),
			"try another language such as en, or enable auto-captions")
	}

	vtt := files[0]
	data, err := os.ReadFile(vtt)
	if err != nil {
		return TranscriptResult{}, core.Wrap(core.CodeOutput, "", vtt, err, "check permissions on the Workbench cache")
	}
	text := parseVTT(string(data), opts.Timestamps)
	if strings.TrimSpace(text) == "" {
		return TranscriptResult{}, core.NewError(core.CodeInput, "", url,
			"the caption file was empty", "try another language or disable manual-only captions")
	}

	title := subtitleTitle(vtt)
	out := filepath.Join(workDir, core.SanitizeFilename(title+".txt"))
	if err := os.WriteFile(out, []byte(text), 0o644); err != nil {
		return TranscriptResult{}, core.Wrap(core.CodeOutput, "", out, err, "check permissions on the Workbench cache")
	}
	rep.Progress(1, 1)
	return TranscriptResult{Path: out, Title: title, Language: language}, nil
}

func subtitleFiles(dir, language string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	languages := make([]string, 0, 2)
	for _, part := range strings.Split(language, ",") {
		if part = strings.TrimSpace(part); part != "" && part != "all" {
			languages = append(languages, part)
		}
	}

	var all, preferred []string
	for _, entry := range entries {
		if entry.IsDir() || !strings.EqualFold(filepath.Ext(entry.Name()), ".vtt") {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		all = append(all, path)
		for _, lang := range languages {
			if strings.Contains(entry.Name(), "."+lang+".") {
				preferred = append(preferred, path)
				break
			}
		}
	}
	sort.Strings(preferred)
	sort.Strings(all)
	if len(preferred) > 0 {
		return preferred, nil
	}
	return all, nil
}

func subtitleTitle(path string) string {
	base := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	if i := strings.LastIndex(base, "."); i > 0 && looksLikeLanguage(base[i+1:]) {
		base = base[:i]
	}
	return core.SanitizeStem(base)
}

func looksLikeLanguage(s string) bool {
	if len(s) < 2 || len(s) > 8 {
		return false
	}
	letter := false
	for _, r := range s {
		switch {
		case (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z'):
			letter = true
		case (r >= '0' && r <= '9') || r == '-':
		default:
			return false
		}
	}
	return letter
}

var cueTag = regexp.MustCompile(`<[^>]*>`)

// parseVTT turns WebVTT into readable text. YouTube's rolling auto-captions
// repeat the previous line at the start of each cue, so consecutive duplicate
// lines are dropped while real repeated speech is preserved.
func parseVTT(data string, timestamps bool) string {
	data = strings.ReplaceAll(data, "\r\n", "\n")
	data = strings.ReplaceAll(data, "\r", "\n")

	var lines []string
	var times []string
	last := ""
	for _, block := range strings.Split(data, "\n\n") {
		block = strings.TrimSpace(block)
		if block == "" || strings.HasPrefix(block, "WEBVTT") || strings.HasPrefix(block, "NOTE") ||
			strings.HasPrefix(block, "STYLE") || strings.HasPrefix(block, "REGION") {
			continue
		}
		rows := strings.Split(block, "\n")
		cue := -1
		for i, row := range rows {
			if strings.Contains(row, "-->") {
				cue = i
				break
			}
		}
		if cue < 0 {
			continue
		}
		start := strings.TrimSpace(strings.SplitN(rows[cue], "-->", 2)[0])
		for _, row := range rows[cue+1:] {
			text := cleanCue(row)
			if text == "" || text == last {
				continue
			}
			lines = append(lines, text)
			times = append(times, cueClock(start))
			last = text
		}
	}

	if len(lines) == 0 {
		return ""
	}
	if timestamps {
		for i := range lines {
			lines[i] = "[" + times[i] + "] " + lines[i]
		}
	}
	return strings.Join(lines, "\n") + "\n"
}

func cleanCue(line string) string {
	line = cueTag.ReplaceAllString(line, "")
	line = html.UnescapeString(line)
	line = strings.ReplaceAll(line, "\u00a0", " ")
	return strings.Join(strings.Fields(line), " ")
}

func cueClock(start string) string {
	start = strings.TrimSpace(strings.SplitN(start, ".", 2)[0])
	parts := strings.Split(start, ":")
	nums := make([]int, len(parts))
	for i, part := range parts {
		nums[i], _ = strconv.Atoi(part)
	}
	if len(nums) == 3 {
		return fmt.Sprintf("%02d:%02d:%02d", nums[0], nums[1], nums[2])
	}
	if len(nums) == 2 {
		return fmt.Sprintf("00:%02d:%02d", nums[0], nums[1])
	}
	return "00:00:00"
}
