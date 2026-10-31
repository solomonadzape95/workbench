package tui

import (
	"os"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/solomonadzape95/workbench/internal/alias"
	"github.com/solomonadzape95/workbench/internal/core"
	"github.com/solomonadzape95/workbench/internal/pipeline"
)

type promptAction string

const (
	promptSavePipeline      promptAction = "save-pipeline"
	promptRenamePipeline    promptAction = "rename-pipeline"
	promptDuplicatePipeline promptAction = "duplicate-pipeline"
	promptOpenFile          promptAction = "open-file"
	promptOutputDir         promptAction = "output-dir"
	promptMaxJobs           promptAction = "max-jobs"
)

type prompt struct {
	action       promptAction
	title        string
	value        string
	err          string
	suggestions  []string
	suggestIndex int
}

func newOpenFilePrompt(value string) *prompt {
	return &prompt{action: promptOpenFile, title: "Open file", value: value}
}

func newValuePrompt(action promptAction, title, value string) *prompt {
	return &prompt{action: action, title: title, value: value}
}

func (m App) newFilePrompt() *prompt {
	p := newOpenFilePrompt("")
	p.suggestions = m.refreshPromptSuggestionsFor(p)
	return p
}

func (m App) refreshPromptSuggestions() []string {
	return m.refreshPromptSuggestionsFor(m.prompt)
}

func (m App) refreshPromptSuggestionsFor(p *prompt) []string {
	if p == nil {
		return nil
	}
	switch p.action {
	case promptOpenFile:
		all := completeInput(p.value, m.opts.Config.Aliases)
		if strings.TrimSpace(p.value) == "" {
			// Put current-directory entries first; people should not have to
			// discover that completion starts at cwd by typing first. Aliases
			// remain one keystroke away by typing @.
			all = append(completeDirectory("./", ""), all...)
		}
		out := make([]string, 0, len(all))
		for _, suggestion := range all {
			if suggestion == "@last" || suggestion == "@clipboard" {
				continue
			}
			// Make aliases browseable: selecting @documents opens its
			// directory listing instead of trying to edit a directory.
			if strings.HasPrefix(suggestion, "@") && !strings.Contains(suggestion, "/") {
				if _, ok := alias.BasePath(suggestion, m.opts.Config.Aliases); ok {
					suggestion += "/"
				}
			}
			out = append(out, suggestion)
		}
		return out
	case promptOutputDir:
		all := completeInput(p.value, m.opts.Config.Aliases)
		out := all[:0]
		for _, suggestion := range all {
			if !strings.HasPrefix(suggestion, "@") {
				out = append(out, suggestion)
			}
		}
		return out
	default:
		return nil
	}
}

func trimLastWord(s string) string {
	s = strings.TrimRight(s, " ")
	if s == "" {
		return ""
	}
	if i := strings.LastIndexAny(s, " /"); i >= 0 {
		return s[:i+1]
	}
	return ""
}

func newSavePrompt(c *compose) *prompt {
	name := c.savedName
	if name == "" || !pipeline.ValidName(name) {
		name = pipeline.Slug(c.defaultName())
	}
	return &prompt{
		action: promptSavePipeline,
		title:  "Save pipeline",
		value:  name,
	}
}

func newRenamePrompt(c *compose) *prompt {
	return &prompt{
		action: promptRenamePipeline,
		title:  "Rename pipeline",
		value:  c.savedName,
	}
}

func newDuplicatePrompt(c *compose) *prompt {
	name := c.savedName + "-copy"
	return &prompt{
		action: promptDuplicatePipeline,
		title:  "Duplicate pipeline",
		value:  pipeline.Slug(name),
	}
}

func (m App) handlePromptKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	p := m.prompt
	if p == nil {
		return m, nil
	}
	switch msg.String() {
	case "esc":
		m.prompt = nil
		return m, nil
	case "up", "ctrl+p":
		if len(p.suggestions) > 0 {
			p.suggestIndex = clamp(p.suggestIndex-1, 0, len(p.suggestions)-1)
		}
		return m, nil
	case "down", "ctrl+n":
		if len(p.suggestions) > 0 {
			p.suggestIndex = clamp(p.suggestIndex+1, 0, len(p.suggestions)-1)
		}
		return m, nil
	case "ctrl+w":
		p.value = trimLastWord(p.value)
		p.err = ""
		p.suggestions = m.refreshPromptSuggestions()
		p.suggestIndex = 0
		return m, nil
	case "tab":
		if len(p.suggestions) > 0 {
			p.value = p.suggestions[p.suggestIndex]
			p.suggestions = m.refreshPromptSuggestions()
			p.suggestIndex = 0
			p.err = ""
		}
		return m, nil
	case "enter":
		return m.submitPrompt()
	case "backspace":
		runes := []rune(p.value)
		if len(runes) > 0 {
			p.value = string(runes[:len(runes)-1])
		}
		p.err = ""
		p.suggestions = m.refreshPromptSuggestions()
		p.suggestIndex = 0
		return m, nil
	case "ctrl+u":
		p.value = ""
		p.err = ""
		p.suggestions = m.refreshPromptSuggestions()
		p.suggestIndex = 0
		return m, nil
	}
	if msg.Text != "" {
		p.value += msg.Text
		p.err = ""
		p.suggestions = m.refreshPromptSuggestions()
		p.suggestIndex = 0
	}
	return m, nil
}

func (m App) submitOpenFile() (tea.Model, tea.Cmd) {
	p := m.prompt
	raw := strings.TrimSpace(p.value)
	if raw == "" {
		p.err = "enter a file path or @alias"
		return m, nil
	}
	// Enter accepts the highlighted suggestion when the typed value is not
	// already a concrete file. A directory suggestion browses deeper; a file
	// suggestion opens immediately.
	if len(p.suggestions) > 0 {
		selected := p.suggestions[clamp(p.suggestIndex, 0, len(p.suggestions)-1)]
		resolved, err := m.resolveEditorPath(raw)
		if err != nil || !isRegularFile(resolved) {
			p.value = selected
			raw = selected
			if strings.HasSuffix(selected, "/") || strings.HasSuffix(selected, string(os.PathSeparator)) {
				p.suggestions = m.refreshPromptSuggestions()
				p.suggestIndex = 0
				return m, nil
			}
			if _, err := m.resolveEditorPath(selected); err != nil {
				p.err = err.Error()
				return m, nil
			}
		}
	}
	return m.openEditor(raw, m.screen)
}

func isRegularFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func (m App) submitPrompt() (tea.Model, tea.Cmd) {
	p := m.prompt
	if p == nil {
		return m, nil
	}
	switch p.action {
	case promptOpenFile:
		return m.submitOpenFile()
	case promptOutputDir:
		dir := strings.TrimSpace(p.value)
		if dir == "" {
			p.err = "enter an output folder"
			return m, nil
		}
		cfg := m.opts.Config
		cfg.Output.Dir = dir
		m.prompt = nil
		return m.saveSetting(cfg, "output folder saved")
	case promptMaxJobs:
		n, err := strconv.Atoi(strings.TrimSpace(p.value))
		if err != nil || n < 0 {
			p.err = "enter 0 or a positive whole number"
			return m, nil
		}
		cfg := m.opts.Config
		cfg.General.MaxJobs = n
		m.prompt = nil
		return m.saveSetting(cfg, "max jobs saved")
	case promptSavePipeline, promptRenamePipeline, promptDuplicatePipeline:
		// handled below
	default:
		m.prompt = nil
		return m, nil
	}
	c := m.compose
	if c == nil {
		p.err = "there is no pipeline to save"
		return m, nil
	}
	oldName := c.savedName
	name := strings.TrimSpace(p.value)
	if !pipeline.ValidName(name) {
		p.err = "use lowercase letters, digits, - and _ (max 64 chars)"
		return m, nil
	}
	if err := c.validateStatic(m.opts.Operations); err != nil {
		p.err = err.Error()
		return m, nil
	}
	if m.opts.Pipelines == nil {
		p.err = "pipeline storage is unavailable"
		return m, nil
	}

	saved := c.buildPipeline(name)
	if err := m.opts.Pipelines.Save(saved); err != nil {
		p.err = err.Error()
		return m, nil
	}
	if p.action == promptRenamePipeline && oldName != "" && oldName != name {
		if err := m.opts.Pipelines.Delete(oldName); err != nil {
			p.err = "saved the new name, but could not remove the old one: " + err.Error()
			return m, nil
		}
	}
	c.savedName = name
	c.dirty = false
	if metas, err := m.opts.Pipelines.List(); err == nil {
		m.pipelineMetas = metas
	}
	m.prompt = nil
	switch p.action {
	case promptRenamePipeline:
		m.toast = "renamed #" + oldName + " → #" + name
	case promptDuplicatePipeline:
		m.toast = "duplicated #" + oldName + " as #" + name
	default:
		m.toast = "saved #" + name
	}
	m.toastLevel = core.LevelInfo
	return m, nil
}

func (m App) promptView() string {
	p := m.prompt
	if p == nil {
		return ""
	}
	width := min(m.width-10, 60)
	if p.action == promptOpenFile {
		width = min(m.width-8, 96)
	}
	inner := width - 6
	var b strings.Builder
	b.WriteString(m.theme.accent.Render(p.title))
	b.WriteString("\n\n")
	b.WriteString("› ")
	if p.action == promptOpenFile {
		b.WriteString(truncateTail(p.value, max(inner-2, 8)))
	} else {
		b.WriteString(truncate(p.value, inner-2))
	}
	b.WriteString(m.theme.accent.Render("▏"))
	b.WriteString("\n")
	if p.action == promptOpenFile {
		if cwd, err := os.Getwd(); err == nil {
			b.WriteString(m.theme.muted.Render("start: " + truncateTail(cwd, max(inner-7, 8))))
			b.WriteString("\n")
		}
	}
	b.WriteString("\n")
	if p.err != "" {
		b.WriteString(m.theme.danger.Render(truncate(p.err, inner)))
		b.WriteString("\n\n")
	}

	if p.action == promptOpenFile && len(p.suggestions) > 0 {
		maxRows := min(len(p.suggestions), max(3, min(7, m.height-16)))
		start := 0
		if p.suggestIndex >= maxRows {
			start = p.suggestIndex - maxRows + 1
		}
		for i := start; i < start+maxRows && i < len(p.suggestions); i++ {
			marker := "  "
			if i == p.suggestIndex {
				marker = m.theme.accent.Render("▸ ")
			}
			b.WriteString(marker + truncateTail(p.suggestions[i], max(inner-2, 8)))
			b.WriteString("\n")
		}
		if len(p.suggestions) > maxRows {
			b.WriteString(m.theme.muted.Render("  … more matches"))
			b.WriteString("\n")
		}
		b.WriteString(m.theme.muted.Render("↑↓ choose · tab complete · enter open · esc cancel"))
	} else {
		hint := "enter save · esc cancel"
		if p.action == promptOpenFile {
			hint = "enter open · esc cancel"
		} else if p.action == promptOutputDir || p.action == promptMaxJobs {
			hint = "enter save · esc cancel"
		}
		if len(p.suggestions) > 0 {
			choice := p.suggestions[clamp(p.suggestIndex, 0, len(p.suggestions)-1)]
			hint = "↑↓ · tab: " + truncateTail(choice, max(inner-18, 8))
		}
		b.WriteString(m.theme.muted.Render(hint))
	}
	return m.theme.card.Width(width).Render(b.String())
}

func truncateTail(s string, width int) string {
	if width < 2 {
		return truncate(s, max(width, 1))
	}
	runes := []rune(s)
	if len(runes) <= width {
		return s
	}
	return "…" + string(runes[len(runes)-width+1:])
}
