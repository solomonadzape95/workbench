package app

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"github.com/solomonadzape95/workbench/internal/config"
	"github.com/solomonadzape95/workbench/internal/engines"
	"github.com/solomonadzape95/workbench/internal/engines/ffmpeg"
)

func doctorApp(t *testing.T, reg *engines.Registry) *App {
	t.Helper()
	return &App{
		Paths:    config.Paths{ConfigFile: filepath.Join(t.TempDir(), "config.toml")},
		Registry: reg,
	}
}

func TestPrintDoctorReportsReadyEngines(t *testing.T) {
	a := doctorApp(t, engines.NewRegistry(engines.Builtin{}))
	var buf bytes.Buffer
	if ok := a.PrintDoctor(&buf); !ok {
		t.Error("PrintDoctor reported a failure for a ready registry")
	}

	out := buf.String()
	for _, want := range []string{"Config", "(not found", "internal", "ready", "built-in"} {
		if !strings.Contains(out, want) {
			t.Errorf("doctor output missing %q:\n%s", want, out)
		}
	}
}

func TestPrintDoctorReportsMissingEngineAndHint(t *testing.T) {
	a := doctorApp(t, engines.NewRegistry(engines.Builtin{}, ffmpeg.New("/nonexistent/ffmpeg")))
	var buf bytes.Buffer
	if ok := a.PrintDoctor(&buf); ok {
		t.Error("PrintDoctor reported success with a missing engine")
	}

	out := buf.String()
	if !strings.Contains(out, "missing") {
		t.Errorf("doctor output has no missing status:\n%s", out)
	}
	if !strings.Contains(out, "ffmpeg:") {
		t.Errorf("doctor output has no ffmpeg hint:\n%s", out)
	}
}

func TestPrintDoctorIncludesConfigWarnings(t *testing.T) {
	a := doctorApp(t, engines.NewRegistry(engines.Builtin{}))
	a.Warnings = []string{"general.theme is nonsense"}
	var buf bytes.Buffer
	a.PrintDoctor(&buf)

	if !strings.Contains(buf.String(), "general.theme is nonsense") {
		t.Errorf("doctor output dropped the config warning:\n%s", buf.String())
	}
}
