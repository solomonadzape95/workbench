package history

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/solomonadzape95/workbench/internal/core"
	"github.com/solomonadzape95/workbench/internal/runner"
)

func sampleRecord(id string, queued time.Time) runner.Record {
	return runner.Record{
		Version: 1,
		View: runner.JobView{
			ID:         id,
			Name:       "resize " + id,
			Status:     core.StatusFailed,
			QueuedAt:   queued,
			StartedAt:  queued.Add(time.Second),
			FinishedAt: queued.Add(2 * time.Second),
			Metrics:    core.Metrics{BytesIn: 10, BytesOut: 5},
			Target:     core.Target{Dir: "/tmp/out", Template: "{stem}.{ext}", Collision: core.CollisionSuffix},
			Pipeline: core.Pipeline{Name: "resize", Version: 1, Steps: []core.Step{{
				ID: "step-1", Op: "image.resize", Params: core.Values{"width": 100},
			}}},
			Items: []runner.ItemView{{
				ID: "item-1", Input: core.Artifact{Path: "/tmp/in.png", Kind: core.Image, Format: "png"},
				Status: core.ItemStatusFailed, Err: errors.New("boom"), Step: 1,
			}},
			Steps: []runner.StepView{{
				ID: "step-1", Op: "image.resize", Title: "Resize", Index: 0, Status: core.StatusFailed,
			}},
			Err:     core.NewError(core.CodeInput, "image.resize", "/tmp/in.png", "bad input", "try a png"),
			LogPath: "/tmp/logs/" + id + ".log",
		},
	}
}

func TestAppendAndRecent(t *testing.T) {
	store := New(t.TempDir())
	now := time.Now().UTC().Truncate(time.Second)
	if err := store.Append(sampleRecord("job-1", now)); err != nil {
		t.Fatal(err)
	}
	if err := store.Append(sampleRecord("job-2", now.Add(time.Minute))); err != nil {
		t.Fatal(err)
	}
	records, err := store.Recent(10)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 2 {
		t.Fatalf("records = %d, want 2", len(records))
	}
	if records[0].View.ID != "job-2" {
		t.Errorf("newest first = %q, want job-2", records[0].View.ID)
	}
	view := records[1].View
	if !view.Historical {
		t.Error("history view is not marked historical")
	}
	if view.Err == nil || core.HintOf(view.Err) != "try a png" {
		t.Errorf("error hint = %v", view.Err)
	}
	if got := view.Pipeline.Steps[0].Params["width"]; got != 100 {
		t.Errorf("normalized param = %#v (%T), want int 100", got, got)
	}
	if view.LogPath == "" || view.Target.Dir != "/tmp/out" {
		t.Errorf("metadata lost: %+v", view)
	}
}

func TestRotationKeepsBothFiles(t *testing.T) {
	store := New(t.TempDir())
	store.MaxBytes = 200
	now := time.Now().UTC().Truncate(time.Second)
	for i := 0; i < 6; i++ {
		if err := store.Append(sampleRecord("job-"+string(rune('a'+i)), now.Add(time.Duration(i)*time.Minute))); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := os.Stat(store.Path + ".1"); err != nil {
		t.Fatalf("rotated file missing: %v", err)
	}
	records, err := store.Recent(100)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) < 2 {
		t.Fatalf("records after rotation = %d, want at least 2", len(records))
	}
	// Newest first even across the rotated boundary.
	for i := 1; i < len(records); i++ {
		if records[i-1].View.QueuedAt.Before(records[i].View.QueuedAt) {
			t.Fatalf("records are not newest-first at %d", i)
		}
	}
}

func TestMalformedLineIsSkipped(t *testing.T) {
	store := New(t.TempDir())
	if err := os.MkdirAll(filepath.Dir(store.Path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(store.Path, []byte("{not json}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := store.Append(sampleRecord("job-1", time.Now())); err != nil {
		t.Fatal(err)
	}
	records, err := store.Recent(10)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 || records[0].View.ID != "job-1" {
		t.Fatalf("records = %+v", records)
	}
}

func TestClear(t *testing.T) {
	store := New(t.TempDir())
	store.MaxBytes = 100
	if err := store.Append(sampleRecord("job-1", time.Now())); err != nil {
		t.Fatal(err)
	}
	if err := store.Append(sampleRecord("job-2", time.Now().Add(time.Minute))); err != nil {
		t.Fatal(err)
	}
	if err := store.Clear(); err != nil {
		t.Fatal(err)
	}
	records, err := store.Recent(10)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 0 {
		t.Fatalf("records after clear = %d", len(records))
	}
}
