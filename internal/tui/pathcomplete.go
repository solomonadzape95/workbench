package tui

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/solomonadzape95/workbench/internal/alias"
	"github.com/solomonadzape95/workbench/internal/config"
)

var builtinAliasNames = []string{"@last", "@clipboard", "@cwd", "@home", "@downloads", "@documents", "@development", "@projects", "@desktop", "@screenshots", "@pictures", "@videos", "@music"}

// completeInput suggests aliases and filesystem paths for the last comma- or
// newline-separated token. It is intentionally read-only and bounded.
func completeInput(raw string, aliases []config.Alias) []string {
	tokenStart := strings.LastIndexAny(raw, ",\n") + 1
	token := strings.TrimSpace(raw[tokenStart:])
	if token == "" {
		out := append([]string(nil), builtinAliasNames...)
		for _, alias := range aliases {
			out = append(out, "@"+alias.Name)
		}
		return out
	}
	if strings.HasPrefix(token, "http://") || strings.HasPrefix(token, "https://") || strings.ContainsAny(token, "*?[") {
		return nil
	}
	if strings.HasPrefix(token, "@") {
		if strings.Contains(token, "/") {
			base, ok := alias.BasePath(token, aliases)
			if !ok {
				return nil
			}
			if strings.HasSuffix(token, "/") && !strings.HasSuffix(base, "/") {
				base += "/"
			}
			prefix := token[:strings.LastIndex(token, "/")+1]
			return completeDirectory(base, prefix)
		}
		var out []string
		for _, name := range builtinAliasNames {
			if strings.HasPrefix(name, token) {
				out = append(out, name)
			}
		}
		for _, a := range aliases {
			name := "@" + a.Name
			if strings.HasPrefix(name, token) {
				out = append(out, name)
			}
		}
		return out
	}

	expanded := config.ExpandHome(token)
	dir, base := filepath.Split(expanded)
	if dir == "" {
		dir = "."
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var out []string
	for _, entry := range entries {
		name := entry.Name()
		if strings.HasPrefix(name, ".") && !strings.HasPrefix(base, ".") {
			continue
		}
		if !strings.HasPrefix(strings.ToLower(name), strings.ToLower(base)) {
			continue
		}
		path := filepath.Join(dir, name)
		if strings.HasPrefix(token, "~/") {
			if home, err := os.UserHomeDir(); err == nil && strings.HasPrefix(path, home+string(filepath.Separator)) {
				path = "~/" + strings.TrimPrefix(path, home+string(filepath.Separator))
			}
		}
		if entry.IsDir() {
			path += string(filepath.Separator)
		}
		out = append(out, path)
	}
	sort.SliceStable(out, func(i, j int) bool {
		di := strings.HasSuffix(out[i], string(filepath.Separator))
		dj := strings.HasSuffix(out[j], string(filepath.Separator))
		if di != dj {
			return di
		}
		return strings.ToLower(out[i]) < strings.ToLower(out[j])
	})
	if strings.HasSuffix(token, string(filepath.Separator)) {
		out = append([]string{token + ".." + string(filepath.Separator)}, out...)
	}
	if len(out) > 6 {
		out = out[:6]
	}
	return out
}

// completeDirectory lists a directory while keeping an @alias/... display
// prefix, so alias browsing does not leak an absolute home path into the
// picker.
func completeDirectory(expanded, displayPrefix string) []string {
	dir, base := filepath.Split(expanded)
	if dir == "" {
		dir = "."
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var out []string
	for _, entry := range entries {
		name := entry.Name()
		if strings.HasPrefix(name, ".") && !strings.HasPrefix(base, ".") {
			continue
		}
		if !strings.HasPrefix(strings.ToLower(name), strings.ToLower(base)) {
			continue
		}
		display := displayPrefix + name
		if entry.IsDir() {
			display += string(filepath.Separator)
		}
		out = append(out, display)
	}
	if base == "" && displayPrefix == "" {
		out = append([]string{".." + string(filepath.Separator)}, out...)
	}
	sort.SliceStable(out, func(i, j int) bool {
		di := strings.HasSuffix(out[i], string(filepath.Separator))
		dj := strings.HasSuffix(out[j], string(filepath.Separator))
		if di != dj {
			return di
		}
		return strings.ToLower(out[i]) < strings.ToLower(out[j])
	})
	if len(out) > 6 {
		out = out[:6]
	}
	return out
}

func replaceLastToken(raw, value string) string {
	start := strings.LastIndexAny(raw, ",\n") + 1
	prefix := raw[:start]
	if start > 0 {
		prefix = strings.TrimRight(prefix, " ")
		if prefix != "" {
			prefix += " "
		}
	}
	return prefix + value
}
