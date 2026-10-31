package state

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/solomonadzape95/workbench/internal/core"
)

func TestSaveLoadRoundTrip(t *testing.T) {
	store := New(t.TempDir())
	want := State{
		LastInput:   "~/Pictures/*.png",
		LastOutput:  "~/Out",
		SelectedJob: "job-7",
		Draft: &Draft{
			Pipeline: core.Pipeline{
				Name: "draft", Version: core.PipelineVersion,
				Steps: []core.Step{{Op: "image.convert", Params: core.Values{"format": "webp"}}},
			},
			Input:     "~/Pictures/a.png",
			Output:    "~/Out",
			SavedName: "draft",
			Selected:  0,
		},
	}
	if err := store.Save(want); err != nil {
		t.Fatal(err)
	}
	got, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if got.Version != Version {
		t.Errorf("version = %d, want %d", got.Version, Version)
	}
	if got.LastInput != want.LastInput || got.LastOutput != want.LastOutput || got.SelectedJob != want.SelectedJob {
		t.Errorf("state = %+v", got)
	}
	if got.Draft == nil || got.Draft.Input != want.Draft.Input || got.Draft.Output != want.Draft.Output {
		t.Fatalf("draft = %+v", got.Draft)
	}
	if len(got.Draft.Pipeline.Steps) != 1 || got.Draft.Pipeline.Steps[0].Op != "image.convert" {
		t.Fatalf("draft steps = %+v", got.Draft.Pipeline.Steps)
	}
}

func TestLoadMissingAndClear(t *testing.T) {
	store := New(t.TempDir())
	got, err := store.Load()
	if err != nil || got != (State{}) {
		t.Fatalf("missing load = %+v, %v", got, err)
	}
	if err := store.Save(State{LastOutput: "~/Out"}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(store.Path); err != nil {
		t.Fatal(err)
	}
	if err := store.Clear(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(store.Path); !os.IsNotExist(err) {
		t.Fatalf("state file still exists: %v", err)
	}
	if err := store.Clear(); err != nil {
		t.Fatalf("clear missing: %v", err)
	}
}

func TestLoadCorruptReturnsError(t *testing.T) {
	store := New(t.TempDir())
	if err := os.MkdirAll(filepath.Dir(store.Path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(store.Path, []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Load(); err == nil {
		t.Fatal("corrupt state loaded without an error")
	}
}
