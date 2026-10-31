package tui

import (
	"strings"
	"testing"
	"time"

	"github.com/solomonadzape95/workbench/internal/config"
	"github.com/solomonadzape95/workbench/internal/core"
	"github.com/solomonadzape95/workbench/internal/runner"
)

func TestStatsViewShowsSavedBytes(t *testing.T) {
	m := sized(t, Options{Config: config.Defaults()})
	m.jobs = []runner.JobView{
		{ID: "1", Name: "Resize", Status: core.StatusDone, QueuedAt: time.Now().Add(-time.Hour), Metrics: core.Metrics{BytesIn: 1000, BytesOut: 400}},
		{ID: "2", Name: "Convert", Status: core.StatusDone, QueuedAt: time.Now(), Metrics: core.Metrics{BytesIn: 500, BytesOut: 700}},
	}
	view := m.statsView(24)
	for _, want := range []string{"Statistics", "SAVED BY PIPELINE", "Resize", "Convert", "net saved"} {
		if !strings.Contains(view, want) {
			t.Errorf("stats view missing %q:\n%s", want, view)
		}
	}
}
