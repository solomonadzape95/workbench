package tui

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
)

func (m App) aliasesView(height int) string {
	width := min(m.width-4, 120)
	if width >= 70 {
		leftW := max(width*46/100, 44)
		rightW := width - leftW - 1
		left := m.panel("Built-in", m.aliasesBuiltin(leftW-4), leftW, height, true)
		right := m.panel("Configured", m.aliasesConfigured(rightW-4), rightW, height, false)
		return lipgloss.JoinHorizontal(lipgloss.Top, left, " ", right)
	}

	b := m.aliasesBuiltin(width-4) + "\n\n" + m.aliasesConfigured(width-4)
	return m.panel("Aliases", b, width, height, true)
}

func (m App) aliasesBuiltin(width int) string {
	var b strings.Builder
	if m.aliasFiltering || m.aliasQuery != "" {
		suffix := "▏"
		if !m.aliasFiltering {
			suffix = ""
		}
		b.WriteString(m.theme.accent.Render("Filter  ") + m.aliasQuery + suffix)
		b.WriteString("\n\n")
	}
	b.WriteString(m.theme.muted.Render("Always available."))
	b.WriteString("\n\n")
	query := strings.ToLower(strings.TrimSpace(m.aliasQuery))
	shown := 0
	for _, row := range [][2]string{
		{"@last", "inputs from the most recent job"},
		{"@clipboard", "paths from the system clipboard"},
		{"@cwd", "files in the current directory"},
		{"@home", "your home directory"},
		{"@downloads", "~/Downloads"},
		{"@documents", "~/Documents"},
		{"@development", "~/Documents/Development"},
		{"@projects", "same as @development"},
		{"@pictures", "~/Pictures"},
		{"@videos", "~/Videos or ~/Movies"},
		{"@music", "~/Music"},
		{"@desktop", "~/Desktop"},
		{"@screenshots", "~/Pictures/Screenshots"},
	} {
		if query != "" && !strings.Contains(strings.ToLower(row[0]+" "+row[1]), query) {
			continue
		}
		b.WriteString(m.theme.accent.Render(padRight(row[0], 16)))
		b.WriteString(m.theme.muted.Render(truncate(row[1], max(width-18, 10))))
		b.WriteString("\n")
		shown++
	}
	if shown == 0 {
		b.WriteString(m.theme.muted.Render("No built-ins match."))
		b.WriteString("\n")
	}
	return b.String()
}

func (m App) aliasesConfigured(width int) string {
	var b strings.Builder
	b.WriteString(m.theme.muted.Render("Edit config.toml."))
	b.WriteString("\n\n")
	if len(m.opts.Config.Aliases) == 0 {
		b.WriteString(m.theme.muted.Render("No aliases yet."))
		b.WriteString("\n")
		return b.String()
	}
	query := strings.ToLower(strings.TrimSpace(m.aliasQuery))
	shown := 0
	for _, alias := range m.opts.Config.Aliases {
		target := alias.Path
		if alias.Glob != "" {
			target = strings.TrimSuffix(target, "/") + "/" + alias.Glob
		}
		if len(alias.Files) > 0 {
			target = fmt.Sprintf("%d fixed files", len(alias.Files))
		}
		if query != "" && !strings.Contains(strings.ToLower("@"+alias.Name+" "+target), query) {
			continue
		}
		b.WriteString(m.theme.accent.Render(padRight("@"+alias.Name, 16)))
		b.WriteString(m.theme.muted.Render(truncate(target, max(width-18, 10))))
		b.WriteString("\n")
		shown++
	}
	if shown == 0 {
		b.WriteString(m.theme.muted.Render("No configured aliases match."))
		b.WriteString("\n")
	}
	return b.String()
}
