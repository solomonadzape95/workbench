package video

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// outputBase returns the filename the runner should suggest for an ffmpeg
// output.
func outputBase(path string) string { return filepath.Base(path) }

// requireExistingFile rejects an empty path or a path that is missing or not a
// regular file.
func requireExistingFile(path, label string) error {
	path = strings.TrimSpace(path)
	if path == "" {
		return fmt.Errorf("%s is required", label)
	}
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("%s: %w", label, err)
	}
	if info.IsDir() {
		return fmt.Errorf("%s is a directory, not a file", label)
	}
	return nil
}

// escapeFilterPath makes a path safe to embed inside a single-quoted ffmpeg
// filter option. It handles backslashes, quotes, and the filtergraph's own
// separators.
func escapeFilterPath(path string) string {
	path = filepath.ToSlash(path)
	var b strings.Builder
	b.Grow(len(path) + 8)
	for _, r := range path {
		switch r {
		case '\\':
			b.WriteString(`\\`)
		case '\'':
			b.WriteString(`'\''`)
		case ':':
			b.WriteString(`\:`)
		case '[':
			b.WriteString(`\[`)
		case ']':
			b.WriteString(`\]`)
		case ',':
			b.WriteString(`\,`)
		case ';':
			b.WriteString(`\;`)
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}
