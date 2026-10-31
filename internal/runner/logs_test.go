package runner

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestPruneLogsByAgeAndCount(t *testing.T) {
	dir := t.TempDir()
	now := time.Now()
	files := map[string]time.Time{
		"newest.log": now,
		"middle.log": now.Add(-48 * time.Hour),
		"old.log":    now.Add(-40 * 24 * time.Hour),
		"keep.txt":   now.Add(-40 * 24 * time.Hour),
	}
	for name, mod := range files {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte("log"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(path, mod, mod); err != nil {
			t.Fatal(err)
		}
	}
	if err := PruneLogs(dir, 30*24*time.Hour, 1, now); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"old.log", "middle.log"} {
		if _, err := os.Stat(filepath.Join(dir, name)); !os.IsNotExist(err) {
			t.Errorf("%s was not pruned: %v", name, err)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "newest.log")); err != nil {
		t.Errorf("newest log removed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "keep.txt")); err != nil {
		t.Errorf("non-log file removed: %v", err)
	}
}

func TestPruneLogsMissingDirectory(t *testing.T) {
	if err := PruneLogs(filepath.Join(t.TempDir(), "missing"), 24*time.Hour, 10, time.Now()); err != nil {
		t.Fatal(err)
	}
}
