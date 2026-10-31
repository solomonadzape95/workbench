package ytdlp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/solomonadzape95/workbench/internal/core"
	"github.com/solomonadzape95/workbench/internal/engines/executil"
)

type InfoOptions struct {
	Playlist bool
}

type InfoResult struct {
	Path  string
	Title string
	ID    string
}

// VideoInfo is the normalized subset of yt-dlp's metadata that is useful to a
// person or an agent. Keeping this stable is more valuable than exposing every
// extractor-specific field.
type VideoInfo struct {
	ID                string       `json:"id"`
	Title             string       `json:"title"`
	Description       string       `json:"description,omitempty"`
	Uploader          string       `json:"uploader,omitempty"`
	Channel           string       `json:"channel,omitempty"`
	UploadDate        string       `json:"upload_date,omitempty"`
	DurationSeconds   float64      `json:"duration_seconds,omitempty"`
	ViewCount         int64        `json:"view_count,omitempty"`
	LikeCount         int64        `json:"like_count,omitempty"`
	CommentCount      int64        `json:"comment_count,omitempty"`
	WebpageURL        string       `json:"webpage_url,omitempty"`
	OriginalURL       string       `json:"original_url,omitempty"`
	Thumbnail         string       `json:"thumbnail,omitempty"`
	Extractor         string       `json:"extractor,omitempty"`
	Format            string       `json:"format,omitempty"`
	Width             int          `json:"width,omitempty"`
	Height            int          `json:"height,omitempty"`
	FPS               float64      `json:"fps,omitempty"`
	VCodec            string       `json:"vcodec,omitempty"`
	ACodec            string       `json:"acodec,omitempty"`
	IsLive            bool         `json:"is_live,omitempty"`
	Formats           []FormatInfo `json:"formats,omitempty"`
	Subtitles         []string     `json:"subtitles,omitempty"`
	AutomaticCaptions []string     `json:"automatic_captions,omitempty"`
	Entries           []EntryInfo  `json:"entries,omitempty"`
	PlaylistCount     int          `json:"playlist_count,omitempty"`
}

type FormatInfo struct {
	ID       string  `json:"format_id,omitempty"`
	Ext      string  `json:"ext,omitempty"`
	Width    int     `json:"width,omitempty"`
	Height   int     `json:"height,omitempty"`
	FPS      float64 `json:"fps,omitempty"`
	VCodec   string  `json:"vcodec,omitempty"`
	ACodec   string  `json:"acodec,omitempty"`
	Filesize int64   `json:"filesize,omitempty"`
	TBR      float64 `json:"tbr,omitempty"`
	Note     string  `json:"note,omitempty"`
}

type EntryInfo struct {
	ID              string  `json:"id"`
	Title           string  `json:"title"`
	URL             string  `json:"url,omitempty"`
	DurationSeconds float64 `json:"duration_seconds,omitempty"`
}

// Info reads metadata without downloading media and writes a normalized JSON
// document. Playlists produce one document containing entry summaries.
func (c *Client) Info(ctx context.Context, url, workDir string, opts InfoOptions, rep core.Reporter) (InfoResult, error) {
	binary, err := c.Resolve()
	if err != nil {
		return InfoResult{}, err
	}

	args := []string{
		"--no-color",
		"--no-update",
		"--no-warnings",
		"--dump-single-json",
	}
	if opts.Playlist {
		args = append(args, "--flat-playlist")
	} else {
		args = append(args, "--no-playlist")
	}
	args = append(args, url)

	cmd := exec.CommandContext(ctx, binary, args...)
	executil.Configure(cmd)
	cmd.WaitDelay = 10 * time.Second

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	rep.Stage("reading metadata")
	err = cmd.Run()
	if ctx.Err() != nil {
		return InfoResult{}, ctx.Err()
	}
	if err != nil {
		logStderr(rep, stderr.String())
		detail := lastLine(stderr.String())
		if detail == "" {
			detail = lastLine(stdout.String())
		}
		return InfoResult{}, core.Wrap(core.CodeEngineFailed, "", url,
			fmt.Errorf("yt-dlp metadata failed: %s", detail), hintFor(stderr.String()+"\n"+stdout.String()))
	}

	var raw map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &raw); err != nil {
		return InfoResult{}, core.Wrap(core.CodeEngineFailed, "", url,
			fmt.Errorf("yt-dlp returned invalid JSON: %w", err),
			"update yt-dlp if this keeps happening")
	}

	info := normalizeInfo(raw)
	if info.Title == "" {
		info.Title = info.ID
	}
	if info.Title == "" {
		info.Title = "media info"
	}
	base := info.Title
	if info.ID != "" && !strings.Contains(base, info.ID) {
		base = fmt.Sprintf("%s [%s]", base, info.ID)
	}
	out := filepath.Join(workDir, core.SanitizeFilename(core.SanitizeStem(base)+".json"))

	data, err := json.MarshalIndent(info, "", "  ")
	if err != nil {
		return InfoResult{}, core.Wrap(core.CodeInternal, "", url, err, "")
	}
	data = append(data, '\n')
	if err := os.WriteFile(out, data, 0o644); err != nil {
		return InfoResult{}, core.Wrap(core.CodeOutput, "", out, err, "check permissions on the Workbench cache")
	}
	rep.Progress(1, 1)
	return InfoResult{Path: out, Title: base, ID: info.ID}, nil
}

func normalizeInfo(raw map[string]any) VideoInfo {
	info := VideoInfo{
		ID:              asString(raw["id"]),
		Title:           asString(raw["title"]),
		Description:     asString(raw["description"]),
		Uploader:        asString(raw["uploader"]),
		Channel:         asString(raw["channel"]),
		UploadDate:      asString(raw["upload_date"]),
		DurationSeconds: asFloat(raw["duration"]),
		ViewCount:       asInt(raw["view_count"]),
		LikeCount:       asInt(raw["like_count"]),
		CommentCount:    asInt(raw["comment_count"]),
		WebpageURL:      asString(raw["webpage_url"]),
		OriginalURL:     asString(raw["original_url"]),
		Thumbnail:       asString(raw["thumbnail"]),
		Extractor:       asString(raw["extractor"]),
		Format:          asString(raw["format"]),
		Width:           int(asInt(raw["width"])),
		Height:          int(asInt(raw["height"])),
		FPS:             asFloat(raw["fps"]),
		VCodec:          asString(raw["vcodec"]),
		ACodec:          asString(raw["acodec"]),
		IsLive:          asBool(raw["is_live"]),
	}
	if info.WebpageURL == "" {
		info.WebpageURL = asString(raw["webpage_url_basename"])
	}

	for _, value := range asSlice(raw["formats"]) {
		format, ok := value.(map[string]any)
		if !ok {
			continue
		}
		info.Formats = append(info.Formats, FormatInfo{
			ID:       asString(format["format_id"]),
			Ext:      asString(format["ext"]),
			Width:    int(asInt(format["width"])),
			Height:   int(asInt(format["height"])),
			FPS:      asFloat(format["fps"]),
			VCodec:   asString(format["vcodec"]),
			ACodec:   asString(format["acodec"]),
			Filesize: asInt(format["filesize"]),
			TBR:      asFloat(format["tbr"]),
			Note:     asString(format["format_note"]),
		})
	}

	info.Subtitles = languageKeys(raw["subtitles"])
	info.AutomaticCaptions = languageKeys(raw["automatic_captions"])
	for _, value := range asSlice(raw["entries"]) {
		entry, ok := value.(map[string]any)
		if !ok {
			continue
		}
		info.Entries = append(info.Entries, EntryInfo{
			ID:              asString(entry["id"]),
			Title:           asString(entry["title"]),
			URL:             firstString(entry["url"], entry["webpage_url"]),
			DurationSeconds: asFloat(entry["duration"]),
		})
	}
	info.PlaylistCount = int(asInt(raw["playlist_count"]))
	if info.PlaylistCount == 0 && len(info.Entries) > 0 {
		info.PlaylistCount = len(info.Entries)
	}
	return info
}

func languageKeys(value any) []string {
	m, ok := value.(map[string]any)
	if !ok || len(m) == 0 {
		return nil
	}
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func asString(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case float64:
		return fmt.Sprint(x)
	default:
		return ""
	}
}

func asFloat(v any) float64 {
	switch x := v.(type) {
	case float64:
		return x
	case int:
		return float64(x)
	case int64:
		return float64(x)
	default:
		return 0
	}
}

func asInt(v any) int64 {
	switch x := v.(type) {
	case float64:
		return int64(x)
	case int:
		return int64(x)
	case int64:
		return x
	default:
		return 0
	}
}

func asBool(v any) bool {
	b, _ := v.(bool)
	return b
}

func asSlice(v any) []any {
	s, _ := v.([]any)
	return s
}

func firstString(values ...any) string {
	for _, value := range values {
		if s := asString(value); s != "" {
			return s
		}
	}
	return ""
}
