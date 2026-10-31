package tui

import (
	"fmt"
	"os"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/atotto/clipboard"

	"github.com/solomonadzape95/workbench/internal/core"
)

type logState struct {
	jobID     string
	title     string
	hint      string
	status    core.Status
	lines     []string
	offset    int
	searching bool
	query     string
	lastMatch int
}

func (m App) openLogView() (App, tea.Cmd) {
	job, ok := m.selectedJobView()
	if !ok {
		return m, nil
	}
	lines := append([]string(nil), job.Logs...)
	if job.Historical && len(lines) == 0 && job.LogPath != "" {
		if data, err := os.ReadFile(job.LogPath); err == nil {
			lines = strings.Split(strings.TrimRight(string(data), "\n"), "\n")
		}
	}
	m.logView = &logState{
		jobID:  job.ID,
		title:  job.Name,
		hint:   core.HintOf(job.Err),
		status: job.Status,
		lines:  lines,
	}
	return m, nil
}

func (m App) handleLogKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	l := m.logView
	if l == nil {
		return m, nil
	}
	key := msg.String()

	if l.searching {
		switch key {
		case "esc":
			l.searching = false
			l.query = ""
		case "enter":
			l.searching = false
			m.findLogMatch(1)
		case "backspace":
			runes := []rune(l.query)
			if len(runes) > 0 {
				l.query = string(runes[:len(runes)-1])
			}
		case "ctrl+u":
			l.query = ""
		default:
			if msg.Text != "" {
				l.query += singleLine(msg.Text)
			}
		}
		return m, nil
	}

	switch key {
	case "esc", "q":
		m.logView = nil
	case "j", "down":
		l.offset++
	case "k", "up":
		l.offset--
	case "pgdown", "ctrl+d":
		l.offset += 10
	case "pgup", "ctrl+u":
		l.offset -= 10
	case "g", "home":
		l.offset = 0
	case "G", "end":
		l.offset = max(len(l.lines)-1, 0)
	case "/":
		l.searching = true
		l.query = ""
		l.lastMatch = l.offset
	case "n":
		m.findLogMatch(1)
	case "N":
		m.findLogMatch(-1)
	case "c", "y":
		text := strings.Join(l.lines, "\n")
		if l.hint != "" {
			text = "Hint: " + l.hint + "\n" + text
		}
		if err := clipboard.WriteAll(text); err != nil {
			m.toast = "clipboard: " + err.Error()
			m.toastLevel = core.LevelError
		} else {
			m.toast = "job log copied"
			m.toastLevel = core.LevelInfo
		}
	}
	return m, nil
}

func (m *App) findLogMatch(dir int) {
	l := m.logView
	if l == nil || l.query == "" || len(l.lines) == 0 {
		return
	}
	q := strings.ToLower(l.query)
	start := clamp(l.offset, 0, len(l.lines)-1)
	for step := 1; step <= len(l.lines); step++ {
		line := (start + dir*step + len(l.lines)*2) % len(l.lines)
		if strings.Contains(strings.ToLower(l.lines[line]), q) {
			l.offset = line
			l.lastMatch = line
			return
		}
	}
	m.toast = "no match for " + l.query
	m.toastLevel = core.LevelWarn
}

func (m App) logViewView() string {
	l := m.logView
	if l == nil {
		return ""
	}
	width := min(m.width-4, 120)
	var b strings.Builder
	b.WriteString(m.theme.title.Render("Log · " + l.title))
	b.WriteString("\n")
	b.WriteString(m.theme.muted.Render(fmt.Sprintf("%s · %d lines · %s", l.status, len(l.lines), l.jobID)))
	b.WriteString("\n")
	if l.hint != "" {
		b.WriteString(m.theme.warning.Render(truncate("Hint: "+l.hint, width)))
		b.WriteString("\n")
	}
	if l.searching {
		b.WriteString(m.theme.accent.Render("Search  ") + l.query + m.theme.accent.Render("▏"))
		b.WriteString("\n")
	} else if l.query != "" {
		b.WriteString(m.theme.muted.Render("Search  " + l.query + "  · n/N"))
		b.WriteString("\n")
	}
	b.WriteString(m.theme.border.Render(strings.Repeat("─", width)))
	b.WriteString("\n")

	if len(l.lines) == 0 {
		b.WriteString(m.theme.muted.Render("no log lines"))
		return b.String()
	}

	rows := max(m.height-10, 3)
	offset := clamp(l.offset, 0, max(len(l.lines)-1, 0))
	for i := offset; i < offset+rows && i < len(l.lines); i++ {
		b.WriteString(truncate(l.lines[i], width))
		b.WriteString("\n")
	}
	b.WriteString("\n")
	if l.searching {
		b.WriteString(m.theme.muted.Render("type to search · enter find · esc cancel"))
	} else {
		b.WriteString(m.theme.muted.Render("j/k scroll · / search · n/N match · c copy · esc close"))
	}
	return b.String()
}

// refreshLogView keeps an open log overlay live while the job emits lines.
func (m *App) refreshLogView() {
	if m.logView == nil || m.opts.Runner == nil {
		return
	}
	job, ok := m.opts.Runner.Job(m.logView.jobID)
	if !ok {
		return
	}
	view := job.Snapshot()
	m.logView.lines = view.Logs
	m.logView.status = view.Status
	m.logView.hint = core.HintOf(view.Err)
}
