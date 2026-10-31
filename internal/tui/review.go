package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/solomonadzape95/workbench/internal/core"
	"github.com/solomonadzape95/workbench/internal/runner"
)

type reviewEntry struct {
	artifact core.Artifact
	before   int64
	keep     bool
}

type reviewState struct {
	jobID   string
	jobName string
	entries []reviewEntry
	cursor  int
}

func (m App) openReview() App {
	job, ok := m.selectedJobView()
	if !ok {
		m.toast = "select a job first"
		m.toastLevel = core.LevelWarn
		return m
	}
	if isActive(job.Status) {
		m.toast = "wait for the job to finish before reviewing outputs"
		m.toastLevel = core.LevelWarn
		return m
	}
	var entries []reviewEntry
	if len(job.Aggregate) > 0 {
		var before int64
		for _, item := range job.Items {
			before += item.Input.Size
		}
		for _, art := range job.Aggregate {
			entries = append(entries, reviewEntry{artifact: art, before: before, keep: true})
		}
	} else {
		for _, item := range job.Items {
			for _, art := range item.Outputs {
				entries = append(entries, reviewEntry{artifact: art, before: item.Input.Size, keep: true})
			}
		}
	}
	if len(entries) == 0 {
		m.toast = "this job has no outputs to review"
		m.toastLevel = core.LevelWarn
		return m
	}
	m.review = &reviewState{jobID: job.ID, jobName: job.Name, entries: entries}
	m.screen = screenReview
	return m
}

func (m App) applyReview() App {
	r := m.review
	if r == nil {
		return m
	}
	var reject []string
	for _, entry := range r.entries {
		if !entry.keep {
			reject = append(reject, entry.artifact.Path)
		}
	}
	if len(reject) == 0 {
		m.review = nil
		m.screen = screenJobs
		m.toast = fmt.Sprintf("kept all %d output(s)", len(r.entries))
		m.toastLevel = core.LevelInfo
		return m
	}
	dir := filepath.Join(m.opts.Paths.DataDir, "undo", "review", core.SanitizeFilename(r.jobID))
	moved, err := runner.TrashFiles(reject, dir)
	if err != nil {
		m.toast = "review: " + err.Error()
		m.toastLevel = core.LevelError
		return m
	}
	m.review = nil
	m.screen = screenJobs
	m.toast = fmt.Sprintf("kept %d, moved %d rejected output(s) to trash", len(r.entries)-moved, moved)
	m.toastLevel = core.LevelInfo
	return m
}

func (m App) reviewView(height int) string {
	r := m.review
	if r == nil {
		return ""
	}
	width := max(m.width-4, 20)
	inner := max(width-4, 18)
	lines := []string{
		m.theme.title.Render(r.jobName) + m.theme.muted.Render(fmt.Sprintf("  ·  %d output(s)", len(r.entries))),
		"",
		m.section("OUTPUTS", inner),
	}
	maxRows := max(min(len(r.entries), height-9), 1)
	start := 0
	if r.cursor >= maxRows {
		start = r.cursor - maxRows + 1
	}
	for i := start; i < start+maxRows && i < len(r.entries); i++ {
		entry := r.entries[i]
		marker := "  "
		if i == r.cursor {
			marker = m.theme.accent.Render("▸ ")
		}
		check := m.theme.danger.Render("[ ]")
		if entry.keep {
			check = m.theme.success.Render("[x]")
		}
		path := entry.artifact.Path
		missing := ""
		if _, err := os.Stat(path); err != nil {
			missing = m.theme.warning.Render("  missing")
		}
		label := truncateTail(path, max(inner-32, 12))
		size := m.theme.muted.Render(humanBytes(entry.before)) + m.theme.muted.Render(" → ") + humanBytes(entry.artifact.Size)
		lines = append(lines, marker+check+"  "+label+"  "+size+missing)
	}
	if len(r.entries) > maxRows {
		lines = append(lines, m.theme.muted.Render(fmt.Sprintf("  %d–%d of %d", start+1, min(start+maxRows, len(r.entries)), len(r.entries))))
	}
	kept, rejected := 0, 0
	for _, entry := range r.entries {
		if entry.keep {
			kept++
		} else {
			rejected++
		}
	}
	lines = append(lines, "", m.section("SUMMARY", inner),
		fmt.Sprintf("%s kept · %s rejected", m.theme.success.Render(fmt.Sprint(kept)), m.theme.warning.Render(fmt.Sprint(rejected))),
		"",
		m.theme.muted.Render("space toggle · a keep all · d reject selected · enter apply · esc cancel"))
	contentHeight := max(height-2, 4)
	if len(lines) > contentHeight {
		lines = lines[:contentHeight]
	}
	return m.panel("Review outputs", strings.Join(lines, "\n"), width, height, true)
}

func (m App) handleReviewKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	r := m.review
	if r == nil {
		m.screen = screenJobs
		return m, nil
	}
	switch msg.String() {
	case "esc":
		m.review = nil
		m.screen = screenJobs
	case "j", "down":
		r.cursor = clamp(r.cursor+1, 0, len(r.entries)-1)
	case "k", "up":
		r.cursor = clamp(r.cursor-1, 0, len(r.entries)-1)
	case "g", "home":
		r.cursor = 0
	case "G", "end":
		r.cursor = len(r.entries) - 1
	case " ", "space":
		r.entries[r.cursor].keep = !r.entries[r.cursor].keep
	case "a":
		for i := range r.entries {
			r.entries[i].keep = true
		}
	case "d":
		r.entries[r.cursor].keep = false
	case "enter":
		return m.applyReview(), nil
	}
	return m, nil
}
