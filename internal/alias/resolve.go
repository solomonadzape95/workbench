// Package alias resolves user input into concrete files. It understands
// plain paths, globs, directories, and @alias shortcuts.
package alias

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/solomonadzape95/workbench/internal/config"
	"github.com/solomonadzape95/workbench/internal/core"
)

const maxInputs = 10_000

// Options filters and expands inputs beyond the base Resolve behaviour.
type Options struct {
	Recursive  bool
	Include    []string
	Exclude    []string
	MinSize    int64
	MaxSize    int64
	NewerThan  time.Time
	SkipHidden bool
}

// Resolve expands raw input into artifacts. Multiple patterns may be separated
// by commas or newlines. An empty raw value resolves to no artifacts.
func Resolve(raw string, aliases []config.Alias) ([]core.Artifact, error) {
	return ResolveOptions(raw, aliases, Options{})
}

// ResolveOptions is Resolve with recursive expansion and filters. Globs
// containing ** walk the tree automatically; directories are walked one level
// unless Recursive is set.
func ResolveOptions(raw string, aliases []config.Alias, opts Options) ([]core.Artifact, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}

	var paths []string
	var failures []string
	for _, pattern := range splitPatterns(raw) {
		matches, err := expandOne(pattern, aliases, opts)
		if err != nil {
			failures = append(failures, err.Error())
			continue
		}
		paths = append(paths, matches...)
	}

	paths = dedupe(paths)
	paths, err := filterPaths(paths, opts)
	if err != nil {
		return nil, err
	}
	if len(paths) == 0 {
		if len(failures) > 0 {
			return nil, fmt.Errorf("%s", strings.Join(failures, "; "))
		}
		return nil, fmt.Errorf("no files match %q", raw)
	}
	if len(paths) > maxInputs {
		paths = paths[:maxInputs]
	}
	sort.Strings(paths)

	out := make([]core.Artifact, 0, len(paths))
	for _, path := range paths {
		art, err := core.Probe(path)
		if err != nil {
			return nil, err
		}
		out = append(out, art)
	}
	return out, nil
}

func splitPatterns(raw string) []string {
	parts := strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == '\n'
	})
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if part = strings.TrimSpace(part); part != "" {
			out = append(out, part)
		}
	}
	return out
}

func expandOne(pattern string, aliases []config.Alias, opts Options) ([]string, error) {
	if strings.HasPrefix(pattern, "@") {
		return expandAlias(pattern, aliases, opts)
	}
	return expandPathWithOptions(config.ExpandHome(pattern), opts)
}

func expandAlias(pattern string, aliases []config.Alias, opts Options) ([]string, error) {
	name, rest := splitAlias(pattern)
	if name == "" {
		return nil, fmt.Errorf("empty alias in %q", pattern)
	}
	switch name {
	case "cwd":
		if rest == "" {
			return expandPathWithOptions(".", opts)
		}
		return expandPathWithOptions(config.ExpandHome(rest), opts)
	case "home", "downloads", "desktop", "screenshots", "documents", "development", "projects",
		"pictures", "videos", "music":
		home, err := os.UserHomeDir()
		if err != nil || home == "" {
			return nil, fmt.Errorf("@%s: cannot resolve the home directory", name)
		}
		base := home
		switch name {
		case "downloads":
			base = filepath.Join(home, "Downloads")
		case "desktop":
			base = filepath.Join(home, "Desktop")
		case "screenshots":
			base = filepath.Join(home, "Pictures", "Screenshots")
		case "documents":
			base = filepath.Join(home, "Documents")
		case "development", "projects":
			base = filepath.Join(home, "Documents", "Development")
		case "pictures":
			base = filepath.Join(home, "Pictures")
		case "videos":
			base = filepath.Join(home, "Videos")
			if _, err := os.Stat(base); err != nil {
				base = filepath.Join(home, "Movies")
			}
		case "music":
			base = filepath.Join(home, "Music")
		}
		if rest != "" {
			base = filepath.Join(base, rest)
		}
		return expandPathWithOptions(base, opts)
	}

	var found *config.Alias
	for i := range aliases {
		if aliases[i].Name == name {
			found = &aliases[i]
			break
		}
	}
	if found == nil {
		return nil, fmt.Errorf("unknown alias @%s", name)
	}

	if len(found.Files) > 0 && rest == "" {
		var out []string
		var failures []string
		for _, file := range found.Files {
			paths, err := expandPathWithOptions(config.ExpandHome(file), opts)
			if err != nil {
				failures = append(failures, err.Error())
				continue
			}
			out = append(out, paths...)
		}
		if len(out) == 0 && len(failures) > 0 {
			return nil, fmt.Errorf("@%s: %s", name, strings.Join(failures, "; "))
		}
		return out, nil
	}

	if strings.TrimSpace(found.Path) == "" {
		return nil, fmt.Errorf("@%s has no path to resolve %q against", name, rest)
	}

	base := config.ExpandHome(found.Path)
	if rest != "" {
		return expandPathWithOptions(filepath.Join(base, rest), opts)
	}
	if found.Glob == "" {
		return expandPathWithOptions(base, opts)
	}
	return expandPathWithOptions(filepath.Join(base, found.Glob), opts)
}

// BasePath resolves an @alias prefix to a directory for interactive path
// completion. It deliberately does not expand globs or enumerate files.
func BasePath(pattern string, aliases []config.Alias) (string, bool) {
	if !strings.HasPrefix(pattern, "@") {
		return "", false
	}
	name, rest := splitAlias(pattern)
	home, homeErr := os.UserHomeDir()
	base := ""
	switch name {
	case "cwd":
		base = "."
	case "home", "downloads", "desktop", "screenshots", "documents", "development", "projects", "pictures", "videos", "music":
		if homeErr != nil || home == "" {
			return "", false
		}
		base = home
		switch name {
		case "downloads":
			base = filepath.Join(home, "Downloads")
		case "desktop":
			base = filepath.Join(home, "Desktop")
		case "screenshots":
			base = filepath.Join(home, "Pictures", "Screenshots")
		case "documents":
			base = filepath.Join(home, "Documents")
		case "development", "projects":
			base = filepath.Join(home, "Documents", "Development")
		case "pictures":
			base = filepath.Join(home, "Pictures")
		case "videos":
			base = filepath.Join(home, "Videos")
			if _, err := os.Stat(base); err != nil {
				base = filepath.Join(home, "Movies")
			}
		case "music":
			base = filepath.Join(home, "Music")
		}
	default:
		for i := range aliases {
			if aliases[i].Name == name {
				if strings.TrimSpace(aliases[i].Path) == "" {
					return "", false
				}
				base = config.ExpandHome(aliases[i].Path)
				break
			}
		}
		if base == "" {
			return "", false
		}
	}
	if rest != "" {
		base = filepath.Join(base, rest)
	}
	return base, true
}

func splitAlias(pattern string) (name, rest string) {
	body := strings.TrimPrefix(pattern, "@")
	if i := strings.IndexByte(body, '/'); i >= 0 {
		return body[:i], body[i+1:]
	}
	return body, ""
}

func expandPathWithOptions(pattern string, opts Options) ([]string, error) {
	if pattern == "" {
		return nil, fmt.Errorf("empty path")
	}

	if strings.ContainsAny(pattern, "*?[") {
		if opts.Recursive || strings.Contains(pattern, "**") {
			return expandGlobRecursive(pattern, opts)
		}
		matches, err := filepath.Glob(pattern)
		if err != nil {
			return nil, fmt.Errorf("invalid glob %q: %w", pattern, err)
		}
		return filesOnly(matches), nil
	}

	info, err := os.Stat(pattern)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", pattern, err)
	}
	if !info.IsDir() {
		return []string{pattern}, nil
	}

	if opts.Recursive {
		return walkFiles(pattern, opts)
	}
	entries, err := os.ReadDir(pattern)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", pattern, err)
	}
	out := make([]string, 0, len(entries))
	for _, entry := range entries {
		if opts.SkipHidden && strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		path := filepath.Join(pattern, entry.Name())
		info, err := os.Stat(path)
		if err != nil || info.IsDir() {
			continue
		}
		out = append(out, path)
	}
	return out, nil
}

func walkFiles(root string, opts Options) ([]string, error) {
	var out []string
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
		if path == root {
			return nil
		}
		if opts.SkipHidden && strings.HasPrefix(entry.Name(), ".") {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.IsDir() {
			return nil
		}
		out = append(out, path)
		return nil
	})
	if os.IsNotExist(err) {
		return nil, nil
	}
	return out, err
}

func expandGlobRecursive(pattern string, opts Options) ([]string, error) {
	matchPattern := pattern
	if opts.Recursive && !strings.Contains(pattern, "**") {
		if slash := strings.LastIndexAny(pattern, `/\`); slash >= 0 {
			matchPattern = pattern[:slash+1] + "**/" + pattern[slash+1:]
		} else {
			matchPattern = "**/" + pattern
		}
	}
	re, err := globRegexp(matchPattern)
	if err != nil {
		return nil, fmt.Errorf("invalid glob %q: %w", pattern, err)
	}
	clean := filepath.ToSlash(pattern)
	root := clean
	if idx := strings.IndexAny(clean, "*?["); idx >= 0 {
		root = clean[:idx]
	}
	if i := strings.LastIndexByte(root, '/'); i >= 0 {
		root = root[:i]
	} else {
		root = "."
	}
	if root == "" {
		root = "/"
	}
	root = filepath.FromSlash(root)

	var out []string
	err = filepath.WalkDir(root, func(current string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			if os.IsNotExist(walkErr) {
				return nil
			}
			return walkErr
		}
		if current == root {
			return nil
		}
		if opts.SkipHidden && strings.HasPrefix(entry.Name(), ".") {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.IsDir() {
			return nil
		}
		if re.MatchString(filepath.ToSlash(current)) {
			out = append(out, current)
		}
		return nil
	})
	if os.IsNotExist(err) {
		return nil, nil
	}
	return out, err
}

func globRegexp(pattern string) (*regexp.Regexp, error) {
	p := filepath.ToSlash(pattern)
	var b strings.Builder
	b.WriteString("^")
	for i := 0; i < len(p); i++ {
		c := p[i]
		switch c {
		case '*':
			if i+1 < len(p) && p[i+1] == '*' {
				i++
				if i+1 < len(p) && p[i+1] == '/' {
					b.WriteString("(?:.*/)?")
					i++
				} else {
					b.WriteString(".*")
				}
			} else {
				b.WriteString("[^/]*")
			}
		case '?':
			b.WriteString("[^/]")
		case '[':
			close := strings.IndexByte(p[i:], ']')
			if close < 0 {
				b.WriteString(`\[`)
				continue
			}
			class := p[i : i+close+1]
			class = strings.Replace(class, "[!", "[^", 1)
			b.WriteString(class)
			i += close
		case '.', '+', '(', ')', '|', '^', '$', '\\', '{', '}':
			b.WriteByte('\\')
			b.WriteByte(c)
		default:
			b.WriteByte(c)
		}
	}
	b.WriteString("$")
	return regexp.Compile(b.String())
}

func filterPaths(paths []string, opts Options) ([]string, error) {
	if len(opts.Include) == 0 && len(opts.Exclude) == 0 && opts.MinSize == 0 &&
		opts.MaxSize == 0 && opts.NewerThan.IsZero() && !opts.SkipHidden {
		return paths, nil
	}
	out := make([]string, 0, len(paths))
	for _, p := range paths {
		info, err := os.Stat(p)
		if err != nil || !info.Mode().IsRegular() {
			continue
		}
		if opts.SkipHidden && hiddenPath(p) {
			continue
		}
		if opts.MinSize > 0 && info.Size() < opts.MinSize {
			continue
		}
		if opts.MaxSize > 0 && info.Size() > opts.MaxSize {
			continue
		}
		if !opts.NewerThan.IsZero() && !info.ModTime().After(opts.NewerThan) {
			continue
		}
		if len(opts.Include) > 0 && !matchesAny(opts.Include, p) {
			continue
		}
		if matchesAny(opts.Exclude, p) {
			continue
		}
		out = append(out, p)
	}
	return out, nil
}

func hiddenPath(path string) bool {
	for _, part := range strings.Split(filepath.ToSlash(path), "/") {
		if part == "" || part == "." || part == ".." {
			continue
		}
		if strings.HasPrefix(part, ".") {
			return true
		}
	}
	return false
}

func matchesAny(patterns []string, path string) bool {
	base := filepath.Base(path)
	slashed := filepath.ToSlash(path)
	for _, pattern := range patterns {
		pattern = strings.TrimSpace(pattern)
		if pattern == "" {
			continue
		}
		if ok, _ := filepath.Match(pattern, base); ok {
			return true
		}
		if ok, _ := filepath.Match(pattern, path); ok {
			return true
		}
		if !strings.ContainsAny(pattern, "*?[") {
			continue
		}
		if re, err := globRegexp(pattern); err == nil && re.MatchString(slashed) {
			return true
		}
		if !strings.HasPrefix(pattern, "/") {
			if re, err := globRegexp("**/" + pattern); err == nil && re.MatchString(slashed) {
				return true
			}
		}
	}
	return false
}

func filesOnly(paths []string) []string {
	out := make([]string, 0, len(paths))
	for _, path := range paths {
		info, err := os.Stat(path)
		if err == nil && !info.IsDir() {
			out = append(out, path)
		}
	}
	return out
}

func dedupe(paths []string) []string {
	seen := make(map[string]bool, len(paths))
	out := make([]string, 0, len(paths))
	for _, path := range paths {
		key := path
		if abs, err := filepath.Abs(path); err == nil {
			key = abs
		}
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, path)
	}
	return out
}
