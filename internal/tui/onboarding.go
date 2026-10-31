package tui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/solomonadzape95/workbench/internal/config"
	"github.com/solomonadzape95/workbench/internal/core"
)

type onboardingPack struct {
	name        string
	description string
	engines     []string
	enabled     bool
	locked      bool
}

type onboarding struct {
	packs  []onboardingPack
	cursor int
	output string
	err    string
}

func newOnboarding(cfg config.Config) *onboarding {
	output := cfg.Output.Dir
	if strings.TrimSpace(output) == "" {
		output = "~/Workbench/{date}"
	}
	enabled := make(map[string]bool, len(cfg.Engines.Enabled))
	for _, id := range cfg.Engines.Enabled {
		enabled[id] = true
	}
	return &onboarding{
		cursor: 1,
		output: output,
		packs: []onboardingPack{
			{name: "Essentials", description: "convert, compress, resize, crop, archive, hash", engines: []string{"internal"}, enabled: true, locked: true},
			{name: "Media", description: "audio/video, downloads, transcription", engines: []string{"ffmpeg", "ytdlp", "whisper"}, enabled: enabled["ffmpeg"] || enabled["ytdlp"] || enabled["whisper"]},
			{name: "Documents", description: "PDF, DOCX, ODT, Markdown, HTML, EPUB", engines: []string{"libreoffice", "pandoc", "pdf2docx", "poppler"}, enabled: enabled["libreoffice"] || enabled["pandoc"] || enabled["pdf2docx"] || enabled["poppler"]},
			{name: "Writing", description: "humanize text with free OpenCode models", engines: []string{"opencode"}, enabled: enabled["opencode"]},
			{name: "Image extras", description: "background removal (rembg, first run downloads a model)", engines: []string{"rembg"}, enabled: enabled["rembg"]},
		},
	}
}

func (m App) handleOnboardingKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	o := m.onboarding
	if o == nil {
		return m, nil
	}
	switch msg.String() {
	case "j", "down":
		o.cursor = clamp(o.cursor+1, 1, len(o.packs)-1)
	case "k", "up":
		o.cursor = clamp(o.cursor-1, 1, len(o.packs)-1)
	case " ", "space", "x":
		if o.cursor > 0 {
			o.packs[o.cursor].enabled = !o.packs[o.cursor].enabled
		}
	case "enter", "f", "ctrl+s":
		return m.finishOnboarding()
	case "esc":
		m.onboarding = nil
		m.screen = screenHome
		m.toast = "setup skipped — run setup from ctrl+p when ready"
		m.toastLevel = core.LevelInfo
	case "?":
		m.showHelp = true
		m.helpScroll = 0
	case "ctrl+p":
		return m.togglePalette()
	case ":":
		return m.openCommandPalette()
	case "q":
		return m.requestQuit()
	}
	return m, nil
}

func (m App) finishOnboarding() (tea.Model, tea.Cmd) {
	o := m.onboarding
	if o == nil {
		return m, nil
	}
	enabled := []string{"internal"}
	for _, pack := range o.packs[1:] {
		if pack.enabled {
			enabled = append(enabled, pack.engines...)
		}
	}
	enabled = dedupeStrings(enabled)

	cfg := m.opts.Config
	cfg.Engines.Enabled = enabled
	cfg.Output.Dir = o.output
	if err := config.Save(m.opts.Paths.ConfigFile, cfg); err != nil {
		o.err = err.Error()
		return m, nil
	}

	m.opts.Config = cfg
	m.onboarding = nil
	m.screen = screenHome
	m.toast = "setup saved — restart to apply engine changes"
	m.toastLevel = core.LevelInfo
	return m, nil
}

func dedupeStrings(in []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(in))
	for _, value := range in {
		if seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}

func (m App) onboardingView(height int) string {
	o := m.onboarding
	if o == nil {
		return ""
	}
	width := min(m.width-4, 120)
	contentWidth := width - 4
	var b strings.Builder
	b.WriteString(m.theme.accent.Render("Welcome to Workbench"))
	b.WriteString("\n")
	b.WriteString(m.theme.muted.Render("Convert, compress, and transform files without leaving the terminal."))
	b.WriteString("\n\n")

	b.WriteString(m.theme.accent.Render("Engines"))
	b.WriteString("\n")
	if m.probing {
		b.WriteString(m.theme.muted.Render("  scanning…"))
		b.WriteString("\n")
	} else if len(m.engines) == 0 {
		b.WriteString(m.theme.muted.Render("  no engines detected"))
		b.WriteString("\n")
	}
	if len(m.engines) > 7 {
		// The full engine table is too tall for 80×24 once Workbench ships
		// nine engines. Keep the summary and only call out missing tools; the
		// Engines screen still has versions and paths.
		ready := 0
		for _, info := range m.engines {
			if info.Available {
				ready++
			}
		}
		b.WriteString("  ")
		b.WriteString(m.theme.success.Render(fmt.Sprintf("%d/%d ready", ready, len(m.engines))))
		b.WriteString(m.theme.muted.Render(" · details in > Engines"))
		b.WriteString("\n")
		for _, info := range m.engines {
			if info.Available {
				continue
			}
			b.WriteString("  ")
			b.WriteString(m.theme.warning.Render("✗ "))
			b.WriteString(m.theme.accent.Render(padRight(string(info.ID), 14)))
			b.WriteString(m.theme.muted.Render(truncate(info.Hint, max(contentWidth-20, 10))))
			b.WriteString("\n")
		}
	} else {
		for _, info := range m.engines {
			mark := m.theme.success.Render("✓")
			detail := info.Version
			if !info.Available {
				mark = m.theme.warning.Render("✗")
				detail = info.Hint
			}
			if detail == "" {
				detail = info.Detail
			}
			b.WriteString("  ")
			b.WriteString(m.theme.accent.Render(padRight(string(info.ID), 14)))
			b.WriteString(mark)
			b.WriteString("  ")
			b.WriteString(m.theme.muted.Render(truncate(detail, max(contentWidth-22, 10))))
			b.WriteString("\n")
		}
	}

	b.WriteString(m.theme.accent.Render("Starter packs"))
	b.WriteString("\n")
	for i, pack := range o.packs {
		marker := "  "
		if i == o.cursor {
			marker = m.theme.accent.Render("▸ ")
		}
		check := "[x]"
		if !pack.enabled {
			check = "[ ]"
		}
		if pack.locked {
			marker = "  "
			check = "[x]"
		}
		b.WriteString(marker)
		b.WriteString(check)
		b.WriteString(" ")
		b.WriteString(m.theme.accent.Render(padRight(pack.name, 12)))
		b.WriteString(" ")
		b.WriteString(m.theme.muted.Render(truncate(pack.description, max(contentWidth-21, 10))))
		b.WriteString("\n")
	}

	b.WriteString(m.theme.muted.Render("Output  "))
	b.WriteString(truncate(o.output, max(contentWidth-10, 10)))
	b.WriteString("\n")
	if o.err != "" {
		b.WriteString(m.theme.danger.Render(truncate("! "+o.err, contentWidth)))
		b.WriteString("\n")
	}
	b.WriteString("\n")
	b.WriteString(m.theme.muted.Render("j/k move · space toggle · enter finish · esc skip"))
	return m.panel("Setup", b.String(), width, height, true)
}
