package tui

import (
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/solomonadzape95/workbench/internal/config"
	"github.com/solomonadzape95/workbench/internal/core"
	"github.com/solomonadzape95/workbench/internal/history"
	fileops "github.com/solomonadzape95/workbench/internal/ops/file"
	"github.com/solomonadzape95/workbench/internal/runner"
	"github.com/solomonadzape95/workbench/internal/state"
)

func TestStateRestoresDraftAndSelectedJob(t *testing.T) {
	saved := state.State{
		LastInput:   "~/Pictures/in.png",
		LastOutput:  "~/Out",
		SelectedJob: "job-2",
		Draft: &state.Draft{
			Pipeline: core.Pipeline{
				Name: "draft", Version: core.PipelineVersion,
				Steps: []core.Step{{Op: "file.copy"}},
			},
			Input:    "~/Pictures/in.png",
			Output:   "~/Out",
			Selected: 0,
		},
	}
	store := state.New(t.TempDir())
	if err := store.Save(saved); err != nil {
		t.Fatal(err)
	}
	hist := history.New(t.TempDir())
	if err := hist.Append(runner.Record{Version: 1, View: runner.JobView{
		ID: "job-2", Name: "copy", Status: core.StatusDone,
	}}); err != nil {
		t.Fatal(err)
	}

	m := New(Options{
		Config:     config.Defaults(),
		Operations: core.NewRegistry(fileops.Copy{}),
		History:    hist,
		State:      store,
		Saved:      saved,
	})
	if m.screen != screenCompose || m.compose == nil {
		t.Fatalf("screen = %v, compose = %v", m.screen, m.compose)
	}
	if m.compose.input != saved.Draft.Input || m.compose.output != saved.Draft.Output {
		t.Fatalf("restored compose input/output = %q / %q", m.compose.input, m.compose.output)
	}
	if len(m.compose.steps) != 1 || m.compose.steps[0].spec.ID != "file.copy" {
		t.Fatalf("restored steps = %+v", m.compose.steps)
	}
	if m.selectedJob != "job-2" {
		t.Fatalf("selected job = %q", m.selectedJob)
	}
	if m.toast == "" {
		t.Fatal("restore did not announce the draft")
	}
}

func TestSaveStateWritesDraft(t *testing.T) {
	store := state.New(t.TempDir())
	m := New(Options{
		Config:     config.Defaults(),
		Operations: core.NewRegistry(fileops.Copy{}),
		State:      store,
	})
	op, ok := m.opts.Operations.Get("file.copy")
	if !ok {
		t.Fatal("file.copy missing")
	}
	m.compose = m.newCompose(op)
	m.compose.input = "~/Pictures/in.png"
	m.compose.output = "~/Out"
	m.compose.dirty = true
	m.selectedJob = "job-9"
	m.saveState()

	got, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if got.LastInput != m.compose.input || got.LastOutput != m.compose.output {
		t.Fatalf("last input/output = %q / %q", got.LastInput, got.LastOutput)
	}
	if got.SelectedJob != "job-9" {
		t.Fatalf("selected job = %q", got.SelectedJob)
	}
	if got.Draft == nil || got.Draft.Input != m.compose.input {
		t.Fatalf("draft = %+v", got.Draft)
	}
	if len(got.Draft.Pipeline.Steps) != 1 || got.Draft.Pipeline.Steps[0].Op != "file.copy" {
		t.Fatalf("draft steps = %+v", got.Draft.Pipeline.Steps)
	}
}

func TestSaveStateOnQuit(t *testing.T) {
	store := state.New(t.TempDir())
	m := New(Options{
		Config:     config.Defaults(),
		Operations: core.NewRegistry(fileops.Copy{}),
		State:      store,
	})
	m.selectedJob = "job-3"
	if _, cmd := m.requestQuit(); cmd == nil {
		t.Fatal("quit returned no command")
	}
	got, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if got.SelectedJob != "job-3" {
		t.Fatalf("selected job after quit = %q", got.SelectedJob)
	}
}

func TestCompactShellStillHandlesWindowResize(t *testing.T) {
	m := New(Options{Config: config.Defaults()})
	next, _ := m.Update(tea.WindowSizeMsg{Width: 30, Height: 10})
	if !next.(App).ready {
		t.Fatal("window resize did not mark the app ready")
	}
}
