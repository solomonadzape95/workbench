package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	xansi "github.com/charmbracelet/x/ansi"

	"github.com/solomonadzape95/workbench/internal/config"
	"github.com/solomonadzape95/workbench/internal/core"
	"github.com/solomonadzape95/workbench/internal/editor"
	imageops "github.com/solomonadzape95/workbench/internal/ops/image"
	"github.com/solomonadzape95/workbench/internal/runner"
)

func goldenApp(t *testing.T, width, height int, opts Options) App {
	t.Helper()
	m := New(opts)
	next, _ := m.Update(tea.WindowSizeMsg{Width: width, Height: height})
	return next.(App)
}

func TestGoldenFrames(t *testing.T) {
	sizes := []struct{ width, height int }{{80, 24}, {100, 32}}
	cases := []struct {
		name  string
		setup func(t *testing.T, width, height int) App
	}{
		{
			name: "home",
			setup: func(t *testing.T, width, height int) App {
				m := goldenApp(t, width, height, Options{Config: config.Defaults()})
				m.screen = screenHome
				return m
			},
		},
		{
			name: "compose",
			setup: func(t *testing.T, width, height int) App {
				m := goldenApp(t, width, height, Options{Config: config.Defaults()})
				m.compose = newCompose(imageops.Convert{}, m.opts.Config)
				m.screen = screenCompose
				return m
			},
		},
		{
			name: "jobs",
			setup: func(t *testing.T, width, height int) App {
				m := goldenApp(t, width, height, Options{Config: config.Defaults()})
				m.jobs = []runner.JobView{
					{
						ID: "20260101-120000-aaaa", Name: "Web image", Status: core.StatusDone,
						Items: []runner.ItemView{{ID: "a", Status: core.ItemStatusDone}},
						Steps: []runner.StepView{{ID: "step-1", Op: "image.convert", Title: "Convert image", Index: 0, Status: core.StatusDone}},
					},
					{
						ID: "20260101-115900-bbbb", Name: "Podcast package", Status: core.StatusFailed,
						Err:   core.NewError(core.CodeEngineFailed, "audio.normalize", "", "ffmpeg failed", "check ffmpeg"),
						Items: []runner.ItemView{{ID: "b", Status: core.ItemStatusFailed}},
					},
				}
				m.screen = screenJobs
				m.selectedJob = "20260101-120000-aaaa"
				return m
			},
		},
		{
			name: "editor",
			setup: func(t *testing.T, width, height int) App {
				m := goldenApp(t, width, height, Options{Config: config.Defaults()})
				m.screen = screenEditor
				m.editorReturn = screenHome
				m.editor = &editorState{
					buf:   editor.New("# Notes\n\nHello, world.\n\n- one\n- two\n", "notes.md"),
					start: "/work",
				}
				return m
			},
		},
	}

	update := os.Getenv("UPDATE_GOLDEN") == "1"
	for _, size := range sizes {
		for _, tc := range cases {
			name := tc.name + "-" + itoa(size.width) + "x" + itoa(size.height)
			t.Run(name, func(t *testing.T) {
				m := tc.setup(t, size.width, size.height)
				got := normalizeGolden(xansi.Strip(m.View().Content))
				path := filepath.Join("testdata", "golden", name+".txt")
				if update {
					if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
						t.Fatal(err)
					}
					if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
						t.Fatal(err)
					}
					return
				}
				wantBytes, err := os.ReadFile(path)
				if err != nil {
					t.Fatalf("golden file missing; run UPDATE_GOLDEN=1 go test ./internal/tui -run TestGoldenFrames: %v", err)
				}
				want := string(wantBytes)
				if got != want {
					t.Errorf("frame changed\ngot:\n%s\nwant:\n%s", got, want)
				}
			})
		}
	}
}

func normalizeGolden(text string) string {
	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	for i := range lines {
		lines[i] = strings.TrimRight(lines[i], " ")
	}
	for len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "" {
		lines = lines[:len(lines)-1]
	}
	return strings.Join(lines, "\n") + "\n"
}

func itoa(value int) string {
	if value == 0 {
		return "0"
	}
	var digits [8]byte
	i := len(digits)
	for value > 0 {
		i--
		digits[i] = byte('0' + value%10)
		value /= 10
	}
	return string(digits[i:])
}
