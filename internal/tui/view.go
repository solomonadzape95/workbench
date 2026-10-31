package tui

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/solomonadzape95/workbench/internal/core"
	"github.com/solomonadzape95/workbench/internal/runner"
)

func (m App) render() string {
	if m.compactShell() {
		return lipgloss.JoinVertical(lipgloss.Left,
			m.compactHeaderView(),
			m.bodyView(max(m.height-2, 1)),
			m.compactStatusView())
	}
	header := m.headerView()
	status := m.statusView()
	bodyHeight := m.height - lipgloss.Height(header) - lipgloss.Height(status)
	if bodyHeight < 1 {
		bodyHeight = 1
	}
	return lipgloss.JoinVertical(lipgloss.Left, header, m.bodyView(bodyHeight), status)
}

// compactShell trades the two-line header and footer rules for single-line
// chrome. It is what keeps 40×12 and 30×10 terminals usable.
func (m App) compactShell() bool {
	return m.width < 50 || m.height < 16
}

func (m App) compactHeaderView() string {
	title := m.theme.title.Render("Workbench")
	if crumb := m.breadcrumb(); crumb != "" {
		title += m.theme.muted.Render(" › ") +
			m.theme.accent.Render(truncate(crumb, max(m.width-14, 4)))
	}
	return truncateStyled(" "+title, m.width)
}

func (m App) compactStatusView() string {
	line := " " + m.renderHints(m.footerText())
	if m.toast != "" {
		style := m.theme.success
		switch m.toastLevel {
		case core.LevelWarn:
			style = m.theme.warning
		case core.LevelError:
			style = m.theme.danger
		}
		toast := style.Render(truncate(m.toast, max(m.width/2, 8)))
		gap := m.width - lipgloss.Width(line) - lipgloss.Width(toast)
		if gap > 1 {
			line += strings.Repeat(" ", gap) + toast
		}
	}
	return truncateStyled(line, m.width)
}

func (m App) headerView() string {
	rawLogo := m.logoForHeader()
	logo := make([]string, 0, len(rawLogo))
	for _, line := range rawLogo {
		logo = append(logo, line)
	}
	title := m.theme.title.Render("Workbench")
	if crumb := m.breadcrumb(); crumb != "" {
		crumb = truncate(crumb, max(m.width-18, 6))
		title += m.theme.muted.Render(" › ") + m.theme.accent.Render(crumb)
	}
	active := ""
	if m.width >= 64 {
		active = m.headerActive()
	}

	lines := make([]string, 0, 4)
	for i, mark := range logo {
		line := " " + mark
		if i == 0 {
			line += "  " + title
			if active != "" {
				gap := m.width - lipgloss.Width(line) - lipgloss.Width(active) - 1
				if gap > 0 {
					line += strings.Repeat(" ", gap) + m.theme.muted.Render(active)
				}
			}
		}
		lines = append(lines, line)
	}
	if m.notice != "" {
		lines = append(lines, m.theme.warning.Render("! "+truncate(m.notice, m.width-8)+"  esc"))
	}
	lines = append(lines, m.theme.border.Render(strings.Repeat("─", m.width)))
	return lipgloss.JoinVertical(lipgloss.Left, lines...)
}

func (m App) statusView() string {
	rule := m.theme.border.Render(strings.Repeat("─", max(m.width, 1)))
	left := m.footerText()
	left = truncate(left, max(m.width-3, 8))
	leftRendered := " " + m.renderHints(left)
	if m.toast == "" {
		return rule + "\n" + leftRendered
	}

	maxToast := m.width - lipgloss.Width(leftRendered) - 2
	if maxToast < 10 {
		return rule + "\n" + leftRendered
	}
	toast := truncate(m.toast, maxToast)

	style := m.theme.success
	switch m.toastLevel {
	case core.LevelWarn:
		style = m.theme.warning
	case core.LevelError:
		style = m.theme.danger
	}
	rightRendered := style.Render(toast)
	gap := m.width - lipgloss.Width(leftRendered) - lipgloss.Width(rightRendered)
	if gap < 1 {
		return rule + "\n" + leftRendered
	}
	return rule + "\n" + leftRendered + strings.Repeat(" ", gap) + rightRendered
}

func (m App) renderHints(text string) string {
	parts := strings.Split(text, " · ")
	var b strings.Builder
	for i, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if i > 0 {
			b.WriteString(m.theme.muted.Render(" · "))
		}
		key, rest, ok := strings.Cut(part, " ")
		b.WriteString(m.theme.accent.Render(key))
		if ok && rest != "" {
			b.WriteString(" ")
			b.WriteString(m.theme.muted.Render(rest))
		}
	}
	return b.String()
}

func (m App) breadcrumb() string {
	switch m.screen {
	case screenHome:
		return ""
	case screenOperations:
		return "New job"
	case screenCompose:
		return "New job › Pipeline"
	case screenJobs:
		return "Jobs"
	case screenReview:
		return "Review outputs"
	case screenPreview:
		return "Preview"
	case screenAliases:
		return "Aliases"
	case screenEngines:
		return "Engines"
	case screenOnboarding:
		return "Setup"
	case screenThemes:
		return "Themes"
	case screenSettings:
		return "Settings"
	case screenStats:
		return "Statistics"
	case screenEditor:
		return "Editor"
	case screenDetector:
		return "AI detection"
	case screenDetectorReport:
		return "AI report"
	default:
		return ""
	}
}

func (m App) bodyView(height int) string {
	var content string
	center := m.screen == screenHome ||
		m.screen == screenOperations ||
		m.screen == screenAliases ||
		m.screen == screenEngines ||
		m.screen == screenThemes ||
		m.screen == screenSettings ||
		m.screen == screenStats

	switch {
	case m.showHelp:
		content = m.helpView()
		center = true
	case m.confirmQuit:
		content = m.confirmView(m.activeJobCount())
		center = true
	case m.confirm != nil:
		content = m.confirmStateView()
		center = true
	case m.prompt != nil:
		content = m.promptView()
		center = true
	case m.palette != nil:
		content = m.paletteView(height)
		center = true
	case m.logView != nil:
		content = m.logViewView()
	default:
		content = m.screenView(height)
	}

	if m.screen == screenHome && !m.showHelp && !m.confirmQuit && m.confirm == nil && m.prompt == nil && m.palette == nil && m.logView == nil {
		return lipgloss.Place(m.width, height, lipgloss.Center, lipgloss.Center, lipgloss.NewStyle().MaxHeight(height).Render(content))
	}
	content = lipgloss.NewStyle().MaxWidth(m.width - 4).MaxHeight(height).Render(content)
	if center {
		return lipgloss.Place(m.width, height, lipgloss.Center, lipgloss.Center, content)
	}
	return lipgloss.Place(m.width, height, lipgloss.Left, lipgloss.Top,
		lipgloss.NewStyle().Padding(0, 2).Render(content))
}

func (m App) screenView(height int) string {
	switch m.screen {
	case screenOperations:
		return m.operationsView(height)
	case screenCompose:
		return m.composeView(height)
	case screenJobs:
		return m.jobsView(height)
	case screenReview:
		return m.reviewView(height)
	case screenPreview:
		return m.previewView()
	case screenAliases:
		return m.aliasesView(height)
	case screenEngines:
		return m.enginesView(height)
	case screenOnboarding:
		return m.onboardingView(height)
	case screenThemes:
		return m.themePickerView(height)
	case screenEditor:
		return m.editorView(height)
	case screenSettings:
		return m.settingsView(height)
	case screenStats:
		return m.statsView(height)
	case screenDetector:
		return m.detectorView(height)
	case screenDetectorReport:
		return m.detectorReportView(height)
	default:
		return m.homeView(height)
	}
}

func (m App) helpView() string {
	const keyW = 12
	descW := max(min(m.width-18, 64), 18)
	var lines []string
	for i, section := range m.helpSections() {
		if i > 0 {
			lines = append(lines, "")
		}
		lines = append(lines, m.theme.accent.Render(section.Title))
		for _, hint := range section.Hints {
			lines = append(lines, m.theme.title.Render(padRight(hint.Key, keyW))+"  "+
				m.theme.muted.Render(truncate(hint.Label, descW)))
		}
	}

	rows := max(m.height-12, 5)
	maxScroll := max(len(lines)-rows, 0)
	start := clamp(m.helpScroll, 0, maxScroll)
	end := min(start+rows, len(lines))

	var b strings.Builder
	b.WriteString(m.theme.accent.Render("Keys"))
	b.WriteString("\n\n")
	b.WriteString(strings.Join(lines[start:end], "\n"))
	b.WriteString("\n\n")
	if maxScroll > 0 {
		b.WriteString(m.theme.muted.Render(fmt.Sprintf("%d–%d of %d · j/k scroll · esc close", start+1, end, len(lines))))
	} else {
		b.WriteString(m.theme.muted.Render("Any screen: esc back · ? help · ctrl+p palette"))
	}
	return m.theme.card.Render(b.String())
}

func (m App) headerActive() string {
	var running int
	var active *runner.JobView
	for i := range m.jobs {
		job := &m.jobs[i]
		if job.Status == core.StatusRunning || job.Status == core.StatusCancelling {
			running++
			if active == nil {
				active = job
			}
		}
	}
	if running == 0 {
		return ""
	}
	if active == nil {
		return fmt.Sprintf("%d running", running)
	}
	return fmt.Sprintf("%d running · %d%%", running, active.Percent())
}

func (m App) tooSmallView() string {
	width, height := max(m.width, 1), max(m.height, 1)
	msg := fmt.Sprintf("Need %d×%d", minWidth, minHeight)
	return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center,
		m.theme.warning.Render(truncate(msg, width-2)))
}

func truncate(s string, width int) string {
	if width < 1 {
		return ""
	}
	runes := []rune(s)
	if len(runes) <= width {
		return s
	}
	if width == 1 {
		return "…"
	}
	return string(runes[:width-1]) + "…"
}
