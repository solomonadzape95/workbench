package tui

import (
	"strings"

	"charm.land/lipgloss/v2"
	xansi "github.com/charmbracelet/x/ansi"
)

// panel renders a titled bordered box. Content lines are padded to the inner
// width and clipped to the panel height, so callers can pass a list of rows
// without doing their own geometry.
func (m App) panel(title, content string, width, height int, focused bool) string {
	if width < 8 || height < 3 {
		return content
	}
	inner := width - 4
	if inner < 2 {
		inner = 2
	}
	title = truncate(title, max(width-6, 1))

	border := m.theme.border
	titleStyle := m.theme.muted
	if focused {
		border = m.theme.accent
		titleStyle = m.theme.accent
	}

	dashes := width - lipgloss.Width(title) - 5
	if dashes < 1 {
		dashes = 1
	}
	top := border.Render("╭─ ") + titleStyle.Render(title) + border.Render(" "+strings.Repeat("─", dashes)+"╮")

	rows := strings.Split(content, "\n")
	lines := make([]string, 0, height)
	lines = append(lines, top)
	for i := 0; i < height-2; i++ {
		line := ""
		if i < len(rows) {
			line = xansi.Truncate(rows[i], inner, "…")
		}
		if pad := inner - lipgloss.Width(line); pad > 0 {
			line += strings.Repeat(" ", pad)
		}
		lines = append(lines, border.Render("│ ")+line+border.Render(" │"))
	}
	lines = append(lines, border.Render("╰"+strings.Repeat("─", width-2)+"╯"))
	return lipgloss.JoinVertical(lipgloss.Left, lines...)
}
