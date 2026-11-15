package tui

import (
	"fmt"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/solomonadzape95/workbench/internal/config"
	"github.com/solomonadzape95/workbench/internal/core"
)

type settingsItem struct {
	label  string
	value  string
	action string
	hint   string
}

func (m App) settingsItems() []settingsItem {
	theme := m.opts.Config.General.ThemeName
	if theme == "" {
		theme = "opencode"
	}
	return []settingsItem{
		{"Output folder", m.opts.Config.Output.Dir, "output", "enter"},
		{"Theme", theme, "theme", "enter/t"},
		{"Max jobs", strconv.Itoa(m.opts.Config.General.MaxJobs), "jobs", "enter"},
		{"Confirm discard", onOffValue(m.opts.Config.General.ConfirmDiscard), "discard", "enter toggle"},
		{"Aliases", fmt.Sprintf("%d configured", len(m.opts.Config.Aliases)), "aliases", "enter browse · e edit"},
		{"OpenCode auto", openCodeAutoValue(m.opts.Config), "opencode", "enter toggle"},
		{"Config file", m.opts.Paths.ConfigFile, "config", "e edit"},
	}
}

func (m App) openSettings(returnTo screen) (tea.Model, tea.Cmd) {
	m.settingsCursor = 0
	m.screen = screenSettings
	m.settingsReturn = returnTo
	return m, nil
}

func (m App) handleSettingsKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	items := m.settingsItems()
	key := msg.String()
	switch key {
	case "esc":
		m.screen = m.settingsReturn
		return m, nil
	case "j", "down":
		m.settingsCursor = clamp(m.settingsCursor+1, 0, len(items)-1)
		return m, nil
	case "k", "up":
		m.settingsCursor = clamp(m.settingsCursor-1, 0, len(items)-1)
		return m, nil
	case "g", "home":
		m.settingsCursor = 0
		return m, nil
	case "G", "end":
		m.settingsCursor = max(len(items)-1, 0)
		return m, nil
	case "pgdown", "ctrl+d":
		m.settingsCursor = clamp(m.settingsCursor+8, 0, len(items)-1)
		return m, nil
	case "pgup", "ctrl+u":
		m.settingsCursor = clamp(m.settingsCursor-8, 0, len(items)-1)
		return m, nil
	case "?":
		m.showHelp = true
		m.helpScroll = 0
		return m, nil
	case "ctrl+p":
		return m.togglePalette()
	case ":":
		return m.openCommandPalette()
	case "q":
		return m.requestQuit()
	case "t":
		return m.openThemePicker(screenSettings)
	case "e":
		return m.openConfigEditor()
	case "enter":
		if m.settingsCursor < 0 || m.settingsCursor >= len(items) {
			return m, nil
		}
		return m.runSettingsItem(items[m.settingsCursor])
	}
	return m, nil
}

func (m App) runSettingsItem(item settingsItem) (tea.Model, tea.Cmd) {
	switch item.action {
	case "output":
		m.prompt = newValuePrompt(promptOutputDir, "Output folder", m.opts.Config.Output.Dir)
	case "theme":
		return m.openThemePicker(screenSettings)
	case "jobs":
		m.prompt = newValuePrompt(promptMaxJobs, "Max simultaneous jobs", strconv.Itoa(m.opts.Config.General.MaxJobs))
	case "discard":
		cfg := m.opts.Config
		cfg.General.ConfirmDiscard = !cfg.General.ConfirmDiscard
		return m.saveSetting(cfg, "discard confirmation "+onOffValue(cfg.General.ConfirmDiscard))
	case "aliases":
		m.screen = screenAliases
		return m, nil
	case "opencode":
		cfg := m.opts.Config
		cfg.OpenCode.AllowPaid = !cfg.OpenCode.AllowPaid
		return m.saveSetting(cfg, "OpenCode auto model policy saved — restart to apply")
	case "config":
		return m.openConfigEditor()
	}
	return m, nil
}

func onOffValue(value bool) string {
	if value {
		return "on"
	}
	return "off"
}

func openCodeAutoValue(cfg config.Config) string {
	if cfg.OpenCode.AllowPaid {
		return "paid allowed"
	}
	return "free only"
}

func (m App) settingsView(height int) string {
	width := min(m.width-4, 120)
	contentWidth := max(width-4, 20)
	var b strings.Builder
	b.WriteString(m.theme.muted.Render("Changes are written to config.toml."))
	b.WriteString("\n\n")

	items := m.settingsItems()
	for i, item := range items {
		selected := i == m.settingsCursor
		style := m.theme.muted
		if selected {
			style = m.theme.accent
		}
		line := style.Render(padRight(item.label, 16)) +
			m.theme.title.Render(padRight(truncate(item.value, max(contentWidth-36, 12)), max(contentWidth-36, 12))) +
			m.theme.muted.Render(item.hint)
		b.WriteString(m.listRow(line, contentWidth, selected))
		b.WriteString("\n")
		if i < len(items)-1 {
			b.WriteString("\n")
		}
	}
	b.WriteString("\n")
	b.WriteString(m.theme.muted.Render("j/k move · enter edit · t theme · e config · esc back"))
	return m.panel("Settings", b.String(), width, height, true)
}

func (m App) saveSetting(cfg config.Config, message string) (tea.Model, tea.Cmd) {
	if path := m.opts.Paths.ConfigFile; path != "" {
		if err := config.Save(path, cfg); err != nil {
			m.toast = "could not save config: " + err.Error()
			m.toastLevel = core.LevelError
			return m, nil
		}
	}
	m.opts.Config = cfg
	m.toast = message
	m.toastLevel = core.LevelInfo
	return m, nil
}
