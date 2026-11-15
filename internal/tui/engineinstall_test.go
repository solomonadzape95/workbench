package tui

import (
	"runtime"
	"testing"

	"github.com/solomonadzape95/workbench/internal/engines"
)

func TestEngineInstallerSelectsMissingAutomatic(t *testing.T) {
	in := (App{}).newEngineInstaller(screenEngines, true)
	expected := 0
	for _, entry := range in.entries {
		if entry.available || !entry.spec.Auto() {
			continue
		}
		expected++
		if !entry.selected {
			t.Errorf("%s: missing automatic engine was not preselected", entry.spec.ID)
		}
	}
	if got := in.selectedCount(); got != expected {
		t.Fatalf("selected = %d, want %d", got, expected)
	}
	if len(in.entries) != len(engines.InstallCatalog()) {
		t.Fatalf("entries = %d, want %d", len(in.entries), len(engines.InstallCatalog()))
	}
}

func TestEngineInstallerSelectsNothingByDefault(t *testing.T) {
	in := (App{}).newEngineInstaller(screenEngines, false)
	if got := in.selectedCount(); got != 0 {
		t.Fatalf("selected = %d, want 0", got)
	}
}

func TestEngineInstallerSortsMissingFirst(t *testing.T) {
	in := &engineInstaller{entries: []*engineInstallEntry{
		{spec: engines.InstallSpec{ID: engines.FFmpeg}, available: true},
		{spec: engines.InstallSpec{ID: engines.RealESRGAN}},
		{spec: engines.InstallSpec{
			ID:       engines.Pandoc,
			Commands: map[string]engines.InstallCommand{runtime.GOOS: {Args: []string{"pandoc"}}},
		}},
	}}
	if engineInstallRank(in.entries[0]) != 2 {
		t.Fatal("installed engine should rank last")
	}
	if engineInstallRank(in.entries[1]) != 1 {
		t.Fatal("manual engine should rank in the middle")
	}
	if engineInstallRank(in.entries[2]) != 0 {
		t.Fatal("automatic engine should rank first")
	}
}

func TestEngineInstallerSummaryAndCounts(t *testing.T) {
	in := &engineInstaller{entries: []*engineInstallEntry{
		{available: true},
		{selected: true},
		{status: "done"},
		{status: "failed"},
	}}
	missing, selected, installed := in.summary()
	if missing != 3 || selected != 1 || installed != 1 {
		t.Fatalf("summary = %d/%d/%d, want 3/1/1", missing, selected, installed)
	}
	done, failed := in.counts()
	if done != 1 || failed != 1 {
		t.Fatalf("counts = %d/%d, want 1/1", done, failed)
	}
}
