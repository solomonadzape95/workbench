package alias

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/solomonadzape95/workbench/internal/config"
	"github.com/solomonadzape95/workbench/internal/core"
)

func fixture(t *testing.T) (string, []config.Alias) {
	t.Helper()
	root := t.TempDir()
	for _, name := range []string{"a.png", "b.png", "notes.txt"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Mkdir(filepath.Join(root, "nested"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "nested", "c.png"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	aliases := []config.Alias{
		{Name: "shots", Path: root, Glob: "*.png"},
		{Name: "everything", Path: root},
		{Name: "notes", Files: []string{filepath.Join(root, "notes.txt")}},
	}
	return root, aliases
}

func names(arts []core.Artifact) []string {
	out := make([]string, 0, len(arts))
	for _, art := range arts {
		out = append(out, filepath.Base(art.Path))
	}
	return out
}

func TestResolveGlobAndDirectory(t *testing.T) {
	root, aliases := fixture(t)

	arts, err := Resolve(filepath.Join(root, "*.png"), aliases)
	if err != nil {
		t.Fatal(err)
	}
	if got := names(arts); len(got) != 2 || got[0] != "a.png" || got[1] != "b.png" {
		t.Errorf("glob = %v, want a.png b.png", got)
	}

	arts, err = Resolve(root, aliases)
	if err != nil {
		t.Fatal(err)
	}
	if len(arts) != 3 {
		t.Errorf("directory resolved %d files, want 3", len(arts))
	}
}

func TestResolveAliases(t *testing.T) {
	_, aliases := fixture(t)

	arts, err := Resolve("@shots", aliases)
	if err != nil {
		t.Fatal(err)
	}
	if got := names(arts); len(got) != 2 || got[0] != "a.png" {
		t.Errorf("@shots = %v, want two pngs", got)
	}

	arts, err = Resolve("@notes", aliases)
	if err != nil {
		t.Fatal(err)
	}
	if got := names(arts); len(got) != 1 || got[0] != "notes.txt" {
		t.Errorf("@notes = %v, want notes.txt", got)
	}

	arts, err = Resolve("@everything/nested/c.png", aliases)
	if err != nil {
		t.Fatal(err)
	}
	if got := names(arts); len(got) != 1 || got[0] != "c.png" {
		t.Errorf("@everything/nested/c.png = %v", got)
	}

	if _, err := Resolve("@nope", aliases); err == nil {
		t.Error("unknown alias resolved without an error")
	}
}

func TestResolveRecursiveGlobAndDirectory(t *testing.T) {
	root, aliases := fixture(t)
	arts, err := Resolve(filepath.Join(root, "**", "*.png"), aliases)
	if err != nil {
		t.Fatal(err)
	}
	got := names(arts)
	if len(got) != 3 {
		t.Fatalf("recursive glob = %v, want 3 pngs", got)
	}

	arts, err = ResolveOptions(root, aliases, Options{Recursive: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(arts) != 4 {
		t.Fatalf("recursive directory = %v, want 4 files", names(arts))
	}

	// A plain glob plus --recursive also walks nested directories.
	arts, err = ResolveOptions(filepath.Join(root, "*.png"), aliases, Options{Recursive: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(arts) != 3 {
		t.Fatalf("recursive simple glob = %v, want 3", names(arts))
	}
}

func TestResolveFilters(t *testing.T) {
	root := t.TempDir()
	old := filepath.Join(root, "old.txt")
	small := filepath.Join(root, "small.txt")
	large := filepath.Join(root, "large.txt")
	for path, body := range map[string]string{
		old:   "old",
		small: "s",
		large: strings.Repeat("x", 2048),
	} {
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	now := time.Now()
	if err := os.Chtimes(old, now.Add(-48*time.Hour), now.Add(-48*time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(small, now, now); err != nil {
		t.Fatal(err)
	}

	arts, err := ResolveOptions(root, nil, Options{Include: []string{"*.txt"}, MinSize: 1000})
	if err != nil {
		t.Fatal(err)
	}
	if got := names(arts); len(got) != 1 || got[0] != "large.txt" {
		t.Fatalf("min-size include = %v", got)
	}

	arts, err = ResolveOptions(root, nil, Options{Include: []string{"*.txt"}, Exclude: []string{"large.txt"}})
	if err != nil {
		t.Fatal(err)
	}
	if got := names(arts); len(got) != 2 {
		t.Fatalf("exclude = %v", got)
	}

	arts, err = ResolveOptions(root, nil, Options{Include: []string{"*.txt"}, NewerThan: now.Add(-24 * time.Hour)})
	if err != nil {
		t.Fatal(err)
	}
	if got := names(arts); len(got) != 2 || got[0] == "old.txt" {
		t.Fatalf("newer-than = %v", got)
	}

	hidden := filepath.Join(root, ".hidden.txt")
	if err := os.WriteFile(hidden, []byte("hidden"), 0o644); err != nil {
		t.Fatal(err)
	}
	arts, err = ResolveOptions(root, nil, Options{Recursive: true, SkipHidden: true})
	if err != nil {
		t.Fatal(err)
	}
	for _, art := range arts {
		if filepath.Base(art.Path) == ".hidden.txt" {
			t.Fatalf("hidden file leaked: %v", names(arts))
		}
	}
}

func TestResolveDedupesAndSplits(t *testing.T) {
	root, aliases := fixture(t)
	raw := filepath.Join(root, "a.png") + ", " + filepath.Join(root, "a.png") + "\n" + filepath.Join(root, "b.png")

	arts, err := Resolve(raw, aliases)
	if err != nil {
		t.Fatal(err)
	}
	if len(arts) != 2 {
		t.Errorf("resolved %d files, want 2: %v", len(arts), names(arts))
	}
}

func TestResolveEmptyAndMissing(t *testing.T) {
	if arts, err := Resolve("  ", nil); err != nil || len(arts) != 0 {
		t.Errorf("empty input = %v, %v; want no artifacts and no error", arts, err)
	}
	if _, err := Resolve(filepath.Join(t.TempDir(), "missing"), nil); err == nil {
		t.Error("missing path resolved without an error")
	}
}

func TestBuiltinAliases(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	for _, dir := range []string{"Downloads", "Desktop", "Documents", filepath.Join("Documents", "Development"), "Pictures", filepath.Join("Pictures", "Screenshots"), "Videos", "Music"} {
		full := filepath.Join(home, dir)
		if err := os.MkdirAll(full, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(full, "file.txt"), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	for _, name := range []string{"@downloads", "@documents", "@development", "@projects", "@pictures", "@videos", "@music", "@desktop", "@screenshots"} {
		arts, err := Resolve(name, nil)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if len(arts) != 1 || arts[0].Name() != "file.txt" {
			t.Errorf("%s = %+v", name, arts)
		}
	}
}
