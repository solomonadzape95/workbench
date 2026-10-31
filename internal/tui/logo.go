package tui

import "charm.land/lipgloss/v2"

// Full ANSI Shadow wordmark. It fits an 80-column terminal (77 columns).
var logoWide = []string{
	"██╗    ██╗ ██████╗ ██████╗ ██╗  ██╗██████╗ ███████╗███╗   ██╗ ██████╗██╗  ██╗",
	"██║    ██║██╔═══██╗██╔══██╗██║ ██╔╝██╔══██╗██╔════╝████╗  ██║██╔════╝██║  ██║",
	"██║ █╗ ██║██║   ██║██████╔╝█████╔╝ ██████╔╝█████╗  ██╔██╗ ██║██║     ███████║",
	"██║███╗██║██║   ██║██╔══██╗██╔═██╗ ██╔══██╗██╔══╝  ██║╚██╗██║██║     ██╔══██║",
	"╚███╔███╔╝╚██████╔╝██║  ██║██║  ██╗██████╔╝███████╗██║ ╚████║╚██████╗██║  ██║",
	" ╚══╝╚══╝  ╚═════╝ ╚═╝  ╚═╝╚═╝  ╚═╝╚═════╝ ╚══════╝╚═╝  ╚═══╝ ╚═════╝╚═╝  ╚═╝",
}

func (m App) logoForHeader() []string {
	shades := m.logoShades()
	return []string{shades[0].Render("▓") + shades[1].Render("▓") + shades[2].Render("▓")}
}

func (m App) logoForHome() string {
	shades := m.logoShades()
	line := shades[0].Render("▓▓▓") + shades[1].Render("▓▓▓") + shades[2].Render("▓▓▓")
	return line + "\n" + line
}

func (m App) logoShades() []lipgloss.Style {
	if m.theme.accentHex == "" {
		return []lipgloss.Style{m.theme.accent, m.theme.accent, m.theme.accent}
	}
	return []lipgloss.Style{
		lipgloss.NewStyle().Foreground(lipgloss.Color(m.theme.accentHex)),
		lipgloss.NewStyle().Foreground(lipgloss.Color(mixHex(m.theme.accentHex, m.theme.bg, 0.35))),
		lipgloss.NewStyle().Foreground(lipgloss.Color(mixHex(m.theme.accentHex, m.theme.bg, 0.6))),
	}
}

func (m App) logoBlock(width int) string {
	width = max(width, 12)
	mark := m.logoForHome()
	// The full ANSI Shadow wordmark is 77 columns wide. On narrow terminals
	// keep the dither mark and set the name in normal type instead of letting
	// the wordmark clip.
	if width < lipgloss.Width(logoWide[0])+2 {
		return lipgloss.PlaceHorizontal(width, lipgloss.Center, mark) + "\n\n" +
			lipgloss.PlaceHorizontal(width, lipgloss.Center, m.theme.title.Render("Workbench"))
	}
	lines := make([]string, 0, len(logoWide))
	for _, line := range logoWide {
		lines = append(lines, m.theme.accent.Render(line))
	}
	return lipgloss.PlaceHorizontal(width, lipgloss.Center, mark) + "\n\n" +
		lipgloss.PlaceHorizontal(width, lipgloss.Center, lipgloss.JoinVertical(lipgloss.Left, lines...))
}
