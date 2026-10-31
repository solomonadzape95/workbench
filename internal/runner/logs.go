package runner

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// PruneLogs removes old job logs. It keeps at most maxFiles entries and drops
// anything older than maxAge. Zero values disable that limit. Missing
// directories are not an error: a fresh profile has no logs yet.
func PruneLogs(dir string, maxAge time.Duration, maxFiles int, now time.Time) error {
	if dir == "" {
		return nil
	}
	entries, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	type logFile struct {
		path    string
		modTime time.Time
	}
	var logs []logFile
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(strings.ToLower(entry.Name()), ".log") {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		logs = append(logs, logFile{path: filepath.Join(dir, entry.Name()), modTime: info.ModTime()})
	}
	sort.Slice(logs, func(i, j int) bool { return logs[i].modTime.After(logs[j].modTime) })
	var errs []error
	for i, log := range logs {
		expired := maxAge > 0 && now.Sub(log.modTime) > maxAge
		overCount := maxFiles > 0 && i >= maxFiles
		if !expired && !overCount {
			continue
		}
		if err := os.Remove(log.path); err != nil && !errors.Is(err, fs.ErrNotExist) {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}
