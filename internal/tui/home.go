package tui

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/solomonadzape95/workbench/internal/engines"
)

func (m App) homeView(height int) string {
	width := min(m.width, 220)
	contentWidth := max(width, 20)
	if height < 16 {
		return m.homeCompact(contentWidth)
	}
	identity := m.homeIdentity(contentWidth)
	topPad := 0
	if height >= 40 {
		topPad = 4
	} else if height >= 28 {
		topPad = 2
	}
	if topPad > 0 {
		identity = strings.Repeat("\n", topPad) + identity
	}

	if width >= 120 {
		identityLines := strings.Count(identity, "\n") + 1
		lowerH := height - 2 - identityLines - 2
		if lowerH >= 8 {
			leftW := max((contentWidth-1)/2, 50)
			rightW := contentWidth - leftW - 1
			lowerLeft := m.homeRecent(leftW-4) + "\n" + m.homePipelines(leftW-4)
			lower := lipgloss.JoinHorizontal(lipgloss.Top,
				m.panel("Recent", lowerLeft, leftW, lowerH, false),
				" ",
				m.panel("Environment", m.homeEnvironment(rightW-4), rightW, lowerH, false))
			return identity + "\n\n" + lower
		}
	}

	// The larger logo plus recent jobs can exceed a 24-row body. Show fewer
	// recent rows rather than letting lipgloss clip the last job silently.
	limit := 4
	var content string
	for {
		footer := m.homeRecentLimit(contentWidth, limit)
		content = identity + "\n" + lipgloss.PlaceHorizontal(contentWidth, lipgloss.Center, footer)
		if lipgloss.Height(content) <= height || limit == 0 {
			break
		}
		limit--
	}
	return content
}

// homeCompact keeps the identity and the four most useful actions when the
// body is too short for the full logo, descriptions, and recent work.
func (m App) homeCompact(width int) string {
	var b strings.Builder
	b.WriteString(lipgloss.PlaceHorizontal(width, lipgloss.Center, m.logoForHome()))
	b.WriteString("\n")
	b.WriteString(lipgloss.PlaceHorizontal(width, lipgloss.Center, m.theme.title.Render("Workbench")))
	b.WriteString("\n\n")
	var actions strings.Builder
	for _, action := range [][2]string{
		{"n", "New job"},
		{"a", "AI detector"},
		{"j", "Jobs"},
		{"?", "Help"},
	} {
		actions.WriteString(m.theme.accent.Render(padRight(action[0], 3)))
		actions.WriteString(m.theme.title.Render(action[1]))
		actions.WriteString("\n")
	}
	b.WriteString(lipgloss.PlaceHorizontal(width, lipgloss.Center, strings.TrimSuffix(actions.String(), "\n")))
	return b.String()
}

func (m App) homeIdentity(width int) string {
	var b strings.Builder
	b.WriteString(m.logoBlock(width))
	b.WriteString("\n\n")

	var actions strings.Builder
	descWidth := clamp(width-20, 24, 48)
	compact := width < 58
	for _, action := range [][3]string{
		{"n", "New job", "convert, compress, or transform files"},
		{"a", "AI detector", "estimate AI-written share and likely model"},
		{"e", "Open editor", "edit text, Markdown, JSON, TOML, and configs"},
		{"j", "Jobs", "running and finished work"},
		{"s", "Settings", "output, theme, and config"},
		{"?", "Help", "keys and screen guide"},
	} {
		actions.WriteString(m.theme.accent.Render(padRight(action[0], 3)))
		actions.WriteString(m.theme.title.Render(padRight(action[1], 14)))
		if !compact {
			actions.WriteString(m.theme.muted.Render(padRight(truncate(action[2], descWidth), descWidth)))
		}
		actions.WriteString("\n")
	}
	b.WriteString(lipgloss.PlaceHorizontal(width, lipgloss.Center, strings.TrimSuffix(actions.String(), "\n")))
	b.WriteString("\n")
	return b.String()
}

func (m App) homeRecent(width int) string {
	return m.homeRecentLimit(width, 4)
}

func (m App) homeRecentLimit(width, limit int) string {
	var b strings.Builder
	b.WriteString(m.theme.accent.Render("Recent jobs"))
	b.WriteString("\n")
	if len(m.jobs) == 0 {
		b.WriteString(m.theme.muted.Render("  No jobs yet. Press n to start."))
		b.WriteString("\n")
		return b.String()
	}
	limit = min(len(m.jobs), limit)
	for i := 0; i < limit; i++ {
		job := m.jobs[i]
		glyph, style := m.jobGlyph(job.Status)
		b.WriteString("  ")
		b.WriteString(style.Render(glyph))
		b.WriteString(" ")
		b.WriteString(truncate(job.Name, max(width-18, 8)))
		b.WriteString(m.theme.muted.Render("  " + humanDuration(job.Duration())))
		b.WriteString("\n")
	}
	if len(m.jobs) > limit {
		b.WriteString(m.theme.muted.Render(fmt.Sprintf("  … %d more", len(m.jobs)-limit)))
		b.WriteString("\n")
	}
	return b.String()
}

func (m App) homePipelines(width int) string {
	var b strings.Builder
	b.WriteString("\n")
	b.WriteString(m.theme.accent.Render("Pipelines"))
	b.WriteString("\n")
	metas := m.pipelineMetas
	if len(metas) == 0 {
		b.WriteString(m.theme.muted.Render("  No saved pipelines. ctrl+s on compose."))
		b.WriteString("\n")
		return b.String()
	}
	for i, meta := range metas {
		if i >= 5 {
			b.WriteString(m.theme.muted.Render(fmt.Sprintf("  … %d more", len(metas)-i)))
			b.WriteString("\n")
			break
		}
		b.WriteString("  ")
		b.WriteString(m.theme.accent.Render("#" + truncate(meta.Name, max(width-14, 8))))
		b.WriteString(m.theme.muted.Render(fmt.Sprintf("  %d steps", meta.Steps)))
		b.WriteString("\n")
	}
	return b.String()
}

func (m App) homeEnvironment(width int) string {
	var b strings.Builder
	b.WriteString(m.theme.accent.Render("Engines"))
	b.WriteString("\n")
	if m.probing {
		b.WriteString(m.theme.muted.Render("  scanning…"))
		b.WriteString("\n")
	} else {
		for _, info := range m.engines {
			mark := m.theme.success.Render("●")
			detail := info.Version
			if !info.Available {
				mark = m.theme.warning.Render("○")
				detail = info.Hint
			}
			if detail == "" {
				detail = info.Detail
			}
			b.WriteString("  ")
			b.WriteString(mark)
			b.WriteString(" ")
			b.WriteString(m.theme.accent.Render(padRight(truncate(string(info.ID), 12), 12)))
			b.WriteString(m.theme.muted.Render(truncate(detail, max(width-17, 8))))
			b.WriteString("\n")
		}
	}
	b.WriteString("\n")
	b.WriteString(m.theme.accent.Render("Output"))
	b.WriteString("\n")
	b.WriteString(m.theme.muted.Render("  " + truncate(defaultOutputDir(m.opts.Config), width-2)))
	b.WriteString("\n\n")
	b.WriteString(m.theme.muted.Render("  Press ? for the full keymap."))
	b.WriteString("\n")
	return b.String()
}

func (m App) homeEngineLine(width int) string {
	if m.probing {
		return "scanning engines…"
	}
	var parts []string
	missingFFmpeg := false
	for _, info := range m.engines {
		if info.ID == engines.Internal {
			continue
		}
		mark := "✓"
		if !info.Available {
			mark = "✗"
			if info.ID == engines.FFmpeg {
				missingFFmpeg = true
			}
		}
		parts = append(parts, string(info.ID)+" "+mark)
	}
	if len(parts) == 0 {
		return "no external engines · run wb doctor"
	}
	line := strings.Join(parts, " · ")
	if missingFFmpeg {
		line = "ffmpeg not found — run wb doctor"
	}
	if len([]rune(line)) > width {
		line = truncate(line, width)
	}
	return line
}
