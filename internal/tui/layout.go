package tui

import (
	"strings"

	"charm.land/lipgloss/v2"
	xansi "github.com/charmbracelet/x/ansi"
)

// padLeft right-aligns plain text in a fixed cell. Styled text should be
// padded before styling, not after.
func padLeft(s string, width int) string {
	if n := width - lipgloss.Width(s); n > 0 {
		return strings.Repeat(" ", n) + s
	}
	return s
}

// listRow is the shared selection treatment for every navigable list. The
// accent gutter is reserved on unselected rows too, so the content never
// shifts sideways as the cursor moves. Selected rows paint the full width.
func (m App) listRow(content string, width int, selected bool) string {
	gutter := "  "
	if selected {
		gutter = m.theme.accent.Render("▌ ")
	}
	line := gutter + content
	line = xansi.Truncate(line, width, "…")
	if selected {
		return m.selectedRow(line, width)
	}
	if n := width - lipgloss.Width(line); n > 0 {
		line += strings.Repeat(" ", n)
	}
	return line
}

// numericCell right-aligns a plain numeric value in a table cell.
func numericCell(value string, width int) string {
	if width <= 0 {
		return ""
	}
	return padLeft(truncate(value, width), width)
}
