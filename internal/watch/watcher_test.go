package watch

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func writeFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func newTestWatcher(t *testing.T, dir string, opts Options) *Watcher {
	t.Helper()
	if opts.Settle == 0 {
		opts.Settle = 100 * time.Millisecond
	}
	w, err := New(dir, opts)
	if err != nil {
		t.Fatal(err)
	}
	if err := w.Baseline(); err != nil {
		t.Fatal(err)
	}
	return w
}

func TestBaselineDoesNotEmitExistingFiles(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "old.txt"), "old")
	w := newTestWatcher(t, dir, Options{})
	ready, err := w.Scan(time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if len(ready) != 0 {
		t.Fatalf("baseline emitted %v", ready)
	}
}

func TestNewFileWaitsForSettle(t *testing.T) {
	dir := t.TempDir()
	w := newTestWatcher(t, dir, Options{Settle: 100 * time.Millisecond})
	start := time.Now()
	file := filepath.Join(dir, "new.txt")
	writeFile(t, file, "one")

	ready, err := w.Scan(start)
	if err != nil || len(ready) != 0 {
		t.Fatalf("first scan = %v, %v", ready, err)
	}
	ready, err = w.Scan(start.Add(50 * time.Millisecond))
	if err != nil || len(ready) != 0 {
		t.Fatalf("settle scan = %v, %v", ready, err)
	}
	ready, err = w.Scan(start.Add(101 * time.Millisecond))
	if err != nil {
		t.Fatal(err)
	}
	if len(ready) != 1 || ready[0] != file {
		t.Fatalf("ready = %v, want %s", ready, file)
	}
}

func TestModificationResetsSettle(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "edit.txt")
	writeFile(t, file, "one")
	w := newTestWatcher(t, dir, Options{Settle: 100 * time.Millisecond})
	start := time.Now()

	writeFile(t, file, "one longer")
	if ready, _ := w.Scan(start); len(ready) != 0 {
		t.Fatalf("early emit = %v", ready)
	}
	writeFile(t, file, "one longer again")
	if ready, _ := w.Scan(start.Add(60 * time.Millisecond)); len(ready) != 0 {
		t.Fatalf("reset emit = %v", ready)
	}
	if ready, _ := w.Scan(start.Add(130 * time.Millisecond)); len(ready) != 0 {
		t.Fatalf("partial settle emit = %v", ready)
	}
	if ready, _ := w.Scan(start.Add(161 * time.Millisecond)); len(ready) != 1 {
		t.Fatalf("final emit = %v", ready)
	}
}

func TestRecursiveAndHidden(t *testing.T) {
	dir := t.TempDir()
	shallow := newTestWatcher(t, dir, Options{SkipHidden: true, Settle: 10 * time.Millisecond})
	deep := newTestWatcher(t, dir, Options{Recursive: true, SkipHidden: true, Settle: 10 * time.Millisecond})

	nested := filepath.Join(dir, "sub", "nested.txt")
	hidden := filepath.Join(dir, ".hidden.txt")
	writeFile(t, nested, "nested")
	writeFile(t, hidden, "hidden")

	start := time.Now()
	if ready, _ := deep.Scan(start); len(ready) != 0 {
		t.Fatalf("early deep emit = %v", ready)
	}
	if ready, _ := shallow.Scan(start.Add(20 * time.Millisecond)); len(ready) != 0 {
		t.Fatalf("shallow watcher emitted %v", ready)
	}
	ready, err := deep.Scan(start.Add(20 * time.Millisecond))
	if err != nil {
		t.Fatal(err)
	}
	if len(ready) != 1 || ready[0] != nested {
		t.Fatalf("recursive ready = %v, want %s", ready, nested)
	}
}

func TestIgnoreDirectory(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "out")
	w := newTestWatcher(t, dir, Options{Recursive: true, Settle: 10 * time.Millisecond, Ignore: []string{out}})
	start := time.Now()
	writeFile(t, filepath.Join(out, "result.txt"), "result")
	writeFile(t, filepath.Join(dir, "input.txt"), "input")
	if ready, _ := w.Scan(start); len(ready) != 0 {
		t.Fatalf("first scan emitted %v", ready)
	}
	ready, err := w.Scan(start.Add(20 * time.Millisecond))
	if err != nil {
		t.Fatal(err)
	}
	if len(ready) != 1 || ready[0] != filepath.Join(dir, "input.txt") {
		t.Fatalf("ready = %v", ready)
	}
}
