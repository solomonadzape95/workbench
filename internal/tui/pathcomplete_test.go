package tui

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/solomonadzape95/workbench/internal/config"
)

func TestCompleteInputAliasesAndPaths(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "Pictures"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "photo.png"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	aliases := []config.Alias{{Name: "shots", Path: dir}}

	got := completeInput("@sh", aliases)
	if len(got) != 1 || got[0] != "@shots" {
		t.Fatalf("alias completion = %v", got)
	}
	if got := completeInput("@", aliases); len(got) == 0 || got[0] != "@last" {
		t.Fatalf("empty alias completion = %v", got)
	}
	got = completeInput(dir+"/ph", aliases)
	if len(got) != 1 || filepath.Base(got[0]) != "photo.png" {
		t.Fatalf("path completion = %v", got)
	}
	got = completeInput(dir+"/Pi", aliases)
	if len(got) != 1 || !filepath.IsAbs(got[0]) || filepath.Base(got[0]) != "Pictures" {
		t.Fatalf("directory completion = %v", got)
	}
	if got := replaceLastToken("old, "+dir+"/ph", dir+"/photo.png"); got != "old, "+dir+"/photo.png" {
		t.Errorf("replace = %q", got)
	}
}

func TestCompleteAliasDirectory(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "note.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	aliases := []config.Alias{{Name: "notes", Path: dir}}
	got := completeInput("@notes/", aliases)
	if len(got) == 0 || got[0] != "@notes/note.txt" {
		t.Fatalf("alias directory completion = %v", got)
	}
}
