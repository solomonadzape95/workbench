package tui

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/solomonadzape95/workbench/internal/config"
	"github.com/solomonadzape95/workbench/internal/core"
)

type themeChoice struct {
	name  string
	theme paletteTheme
}

type themePicker struct {
	choices  []themeChoice
	items    []themeChoice
	query    string
	cursor   int
	returnTo screen
	err      string
}

func (m App) openThemePicker(returnTo screen) (tea.Model, tea.Cmd) {
	choices := make([]themeChoice, 0, len(builtinThemes))
	for _, theme := range builtinThemes {
		choices = append(choices, themeChoice{name: theme.Name, theme: theme})
	}
	p := &themePicker{choices: choices, returnTo: returnTo}
	p.cursor = currentThemeChoice(choices, m.opts.Config)
	p.filter()
	m.themePicker = p
	m.palette = nil
	m.screen = screenThemes
	return m, nil
}

func currentThemeChoice(choices []themeChoice, cfg config.Config) int {
	name := strings.TrimSpace(cfg.General.ThemeName)
	for i, choice := range choices {
		if choice.name == name {
			return i
		}
	}
	return 0
}

func (p *themePicker) filter() {
	query := strings.ToLower(strings.TrimSpace(p.query))
	p.items = p.items[:0]
	for _, choice := range p.choices {
		if query == "" || strings.Contains(strings.ToLower(choice.name), query) {
			p.items = append(p.items, choice)
		}
	}
	p.cursor = clamp(p.cursor, 0, max(len(p.items)-1, 0))
}

func (m App) handleThemeKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	p := m.themePicker
	if p == nil {
		return m, nil
	}
	switch msg.String() {
	case "esc":
		m.themePicker = nil
		m.screen = p.returnTo
		return m, nil
	case "up", "ctrl+k":
		p.cursor = clamp(p.cursor-1, 0, max(len(p.items)-1, 0))
		return m, nil
	case "down", "ctrl+j":
		p.cursor = clamp(p.cursor+1, 0, max(len(p.items)-1, 0))
		return m, nil
	case "home":
		p.cursor = 0
		return m, nil
	case "end":
		p.cursor = max(len(p.items)-1, 0)
		return m, nil
	case "pgup":
		p.cursor = clamp(p.cursor-8, 0, max(len(p.items)-1, 0))
		return m, nil
	case "pgdown":
		p.cursor = clamp(p.cursor+8, 0, max(len(p.items)-1, 0))
		return m, nil
	case "enter":
		if len(p.items) == 0 {
			return m, nil
		}
		return m.applyThemeChoice(p.items[p.cursor])
	case "ctrl+p":
		return m.togglePalette()
	case ":":
		return m.openCommandPalette()
	case "backspace":
		runes := []rune(p.query)
		if len(runes) > 0 {
			p.query = string(runes[:len(runes)-1])
		}
		p.cursor = 0
		p.filter()
		return m, nil
	case "ctrl+u":
		p.query = ""
		p.cursor = 0
		p.filter()
		return m, nil
	}
	if msg.Text != "" {
		p.query += msg.Text
		p.cursor = 0
		p.filter()
	}
	return m, nil
}

func (m App) applyThemeChoice(choice themeChoice) (tea.Model, tea.Cmd) {
	cfg := m.opts.Config
	cfg.General.ThemeName = choice.name
	m.opts.Config = cfg
	m.theme = newThemeFromPalette(choice.theme)
	if path := m.opts.Paths.ConfigFile; path != "" {
		if err := config.Save(path, cfg); err != nil {
			m.toast = "theme applied; could not save config: " + err.Error()
			m.toastLevel = core.LevelWarn
		} else {
			m.toast = "theme: " + choice.name
			m.toastLevel = core.LevelInfo
		}
	} else {
		m.toast = "theme: " + choice.name
		m.toastLevel = core.LevelInfo
	}
	m.screen = m.themePicker.returnTo
	m.themePicker = nil
	return m, nil
}

func (m App) themePickerView(height int) string {
	p := m.themePicker
	if p == nil {
		return ""
	}
	width := min(m.width-4, 100)
	contentWidth := width - 4
	var b strings.Builder
	compact := height < 20
	if !compact {
		b.WriteString(m.theme.muted.Render("Choose a theme. The preview updates as you move."))
		b.WriteString("\n")
	}
	b.WriteString(m.theme.accent.Render("› "))
	b.WriteString(truncate(p.query, max(contentWidth-4, 8)))
	b.WriteString(m.theme.accent.Render("▏"))
	b.WriteString("\n")
	if p.err != "" {
		b.WriteString(m.theme.danger.Render(truncate(p.err, contentWidth)))
		b.WriteString("\n")
	}

	for i, choice := range p.items {
		row := m.themeRow(choice, i == p.cursor, contentWidth)
		if i == p.cursor {
			row = m.selectedRow(row, contentWidth)
		}
		b.WriteString(row)
		b.WriteString("\n")
	}
	if len(p.items) == 0 {
		b.WriteString(m.theme.muted.Render("  no themes match"))
		b.WriteString("\n")
	}
	if len(p.items) > 0 {
		choice := p.items[p.cursor]
		preview := newThemeFromPalette(choice.theme)
		b.WriteString("\n")
		if !compact {
			b.WriteString(m.theme.muted.Render("Preview"))
			b.WriteString("\n")
			b.WriteString(m.themePreview(preview, contentWidth))
			b.WriteString("\n")
		} else {
			b.WriteString(m.themePreviewCompact(preview, contentWidth))
			b.WriteString("\n")
		}
	}
	b.WriteString("\n")
	b.WriteString(m.theme.muted.Render("↑↓/ctrl+j/k move · enter apply · type to filter · esc cancel"))
	return m.panel("Themes", b.String(), width, height, true)
}

func (m App) themeRow(choice themeChoice, selected bool, width int) string {
	marker := "  "
	style := m.theme.muted
	if selected {
		marker = m.theme.accent.Render("▸ ")
		style = m.theme.accent
	}
	return marker + style.Render(padRight(truncate(choice.name, max(width-4, 10)), max(width-4, 10)))
}

func (m App) themePreviewCompact(t theme, width int) string {
	var b strings.Builder
	b.WriteString(lipgloss.NewStyle().Background(t.bgColor).Foreground(t.fgColor).Render(" Aa Workbench "))
	b.WriteString("  ")
	for _, hex := range t.palette {
		b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color(hex)).Render("██"))
	}
	return b.String()
}

func (m App) themePreview(t theme, width int) string {
	var b strings.Builder
	label := lipgloss.NewStyle().
		Background(t.bgColor).
		Foreground(t.fgColor).
		Padding(0, 1).
		Render("Aa  Workbench")
	b.WriteString(label)
	b.WriteString("\n")
	for i, hex := range t.palette {
		if i == 8 {
			b.WriteString("\n")
		}
		b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color(hex)).Render("██"))
	}
	b.WriteString("\n")
	b.WriteString(t.accent.Render("accent"))
	b.WriteString("  ")
	b.WriteString(t.success.Render("success"))
	b.WriteString("  ")
	b.WriteString(t.warning.Render("warning"))
	b.WriteString("  ")
	b.WriteString(t.danger.Render("danger"))
	return b.String()
}
