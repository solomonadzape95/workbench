// Package watch provides a small polling watcher for directory-driven jobs. It
// deliberately has no platform-specific dependencies: a scan is cheap, and a
// two-second settle window matters more than event latency for file writes.
package watch

import (
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type Options struct {
	Recursive  bool
	SkipHidden bool
	Settle     time.Duration
	Ignore     []string
}

type fileState struct {
	Size    int64
	ModTime int64
}

type Watcher struct {
	dir        string
	recursive  bool
	skipHidden bool
	settle     time.Duration
	ignore     []string

	known    map[string]fileState
	pending  map[string]time.Time
	observed map[string]fileState
	primed   bool
}

func New(dir string, opts Options) (*Watcher, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	settle := opts.Settle
	if settle <= 0 {
		settle = 2 * time.Second
	}
	w := &Watcher{
		dir:        abs,
		recursive:  opts.Recursive,
		skipHidden: opts.SkipHidden,
		settle:     settle,
		known:      map[string]fileState{},
		pending:    map[string]time.Time{},
		observed:   map[string]fileState{},
	}
	for _, ignore := range opts.Ignore {
		if strings.TrimSpace(ignore) == "" {
			continue
		}
		absIgnore, err := filepath.Abs(ignore)
		if err != nil {
			continue
		}
		w.ignore = append(w.ignore, absIgnore)
	}
	return w, nil
}

// Baseline records the current tree without emitting it. Call it once before
// the watch loop so existing files are not treated as new.
func (w *Watcher) Baseline() error {
	current, err := w.snapshot()
	if err != nil {
		return err
	}
	w.known = current
	w.observed = map[string]fileState{}
	w.pending = map[string]time.Time{}
	w.primed = true
	return nil
}

// Scan returns paths whose state has been stable for the settle window. The
// first call primes the watcher if Baseline was not called.
func (w *Watcher) Scan(now time.Time) ([]string, error) {
	current, err := w.snapshot()
	if err != nil {
		return nil, err
	}
	if !w.primed {
		w.known = current
		w.primed = true
		return nil, nil
	}

	for path := range w.known {
		if _, ok := current[path]; !ok {
			delete(w.known, path)
			delete(w.pending, path)
			delete(w.observed, path)
		}
	}

	for path, state := range current {
		known, exists := w.known[path]
		switch {
		case exists && known == state:
			// A write that reverted to the observed content is not a change.
			delete(w.pending, path)
			delete(w.observed, path)
		case !exists:
			if _, queued := w.pending[path]; !queued {
				w.pending[path] = now
				w.observed[path] = state
			} else if w.observed[path] != state {
				w.pending[path] = now
				w.observed[path] = state
			}
		default: // known != state
			if _, queued := w.pending[path]; !queued {
				w.pending[path] = now
				w.observed[path] = state
			} else if w.observed[path] != state {
				w.pending[path] = now
				w.observed[path] = state
			}
		}
	}

	var ready []string
	for path, since := range w.pending {
		if now.Sub(since) < w.settle {
			continue
		}
		state, ok := current[path]
		if !ok {
			delete(w.pending, path)
			delete(w.observed, path)
			continue
		}
		if state != w.observed[path] {
			w.pending[path] = now
			w.observed[path] = state
			continue
		}
		ready = append(ready, path)
		w.known[path] = state
		delete(w.pending, path)
		delete(w.observed, path)
	}
	sort.Strings(ready)
	return ready, nil
}

func (w *Watcher) snapshot() (map[string]fileState, error) {
	out := make(map[string]fileState)
	if !w.recursive {
		entries, err := os.ReadDir(w.dir)
		if err != nil {
			return nil, err
		}
		for _, entry := range entries {
			if entry.IsDir() || (w.skipHidden && strings.HasPrefix(entry.Name(), ".")) {
				continue
			}
			path := filepath.Join(w.dir, entry.Name())
			if w.ignored(path) {
				continue
			}
			info, err := entry.Info()
			if err != nil {
				continue
			}
			out[path] = fileState{Size: info.Size(), ModTime: info.ModTime().UnixNano()}
		}
		return out, nil
	}

	err := filepath.WalkDir(w.dir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
		if path == w.dir {
			return nil
		}
		if w.skipHidden && strings.HasPrefix(entry.Name(), ".") {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if w.ignored(path) {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.IsDir() {
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return nil
		}
		out[path] = fileState{Size: info.Size(), ModTime: info.ModTime().UnixNano()}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (w *Watcher) ignored(path string) bool {
	for _, dir := range w.ignore {
		rel, err := filepath.Rel(dir, path)
		if err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return true
		}
	}
	return false
}
