package tui

import (
	"fmt"
	"sort"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/solomonadzape95/workbench/internal/core"
	"github.com/solomonadzape95/workbench/internal/engines"
	"github.com/solomonadzape95/workbench/internal/pipeline"
)

type paletteTarget int

const (
	paletteNewCompose paletteTarget = iota
	paletteAppendStep
)

type paletteKind int

const (
	paletteOperation paletteKind = iota
	palettePreset
	palettePipeline
	paletteAlias
	paletteCommand
)

type paletteItem struct {
	kind         paletteKind
	title        string
	subtitle     string
	keywords     string
	op           core.Operation
	preset       *jobPreset
	meta         pipeline.Meta
	insert       string
	command      string
	disabled     bool
	disabledHint string
	order        int
	score        int
}

type palette struct {
	query    string
	cursor   int
	all      []paletteItem
	items    []paletteItem
	target   paletteTarget
	returnTo screen
	err      string
}

func (m App) buildPalette(target paletteTarget) *palette {
	p := &palette{target: target, returnTo: m.screen}
	add := func(item paletteItem) {
		item.order = len(p.all)
		p.all = append(p.all, item)
	}

	for _, op := range m.ops {
		spec := op.Spec()
		disabled, hint := m.operationDisabled(spec)
		if target == paletteAppendStep && m.compose != nil {
			if ok, why := m.compose.canAppend(op); !ok {
				disabled = true
				hint = why
			}
		}
		item := paletteItem{
			kind:         paletteOperation,
			title:        spec.Title,
			subtitle:     string(spec.Domain) + " · " + acceptsSummary(spec),
			keywords:     spec.ID + " " + spec.Summary,
			op:           op,
			disabled:     disabled,
			disabledHint: hint,
		}
		add(item)
	}

	for i := range jobPresets {
		preset := &jobPresets[i]
		disabled, hint := m.presetDisabled(preset)
		add(paletteItem{
			kind:         palettePreset,
			title:        "Compound · " + preset.name,
			subtitle:     preset.description,
			keywords:     "compound preset recipe " + preset.name + " " + preset.description,
			preset:       preset,
			disabled:     disabled,
			disabledHint: hint,
		})
	}

	if m.opts.Pipelines != nil {
		metas, err := m.opts.Pipelines.List()
		if err != nil {
			p.err = err.Error()
		}
		for _, meta := range metas {
			disabled := meta.Err != nil
			hint := ""
			if disabled {
				hint = meta.Err.Error()
			} else if m.opts.Pipelines != nil {
				if loaded, err := m.opts.Pipelines.Load(meta.Name); err == nil {
					for _, step := range loaded.Steps {
						if _, ok := m.opts.Operations.Get(step.Op); !ok {
							disabled = true
							hint = "missing operation: " + step.Op
							break
						}
					}
				}
			}
			add(paletteItem{
				kind:         palettePipeline,
				title:        "#" + meta.Name,
				subtitle:     fmt.Sprintf("%d steps · %s", meta.Steps, meta.Description),
				keywords:     meta.Name + " " + meta.Description,
				meta:         meta,
				disabled:     disabled,
				disabledHint: hint,
			})
		}
	}

	for _, a := range m.opts.Config.Aliases {
		subtitle := a.Path
		if a.Glob != "" {
			subtitle = strings.TrimSuffix(subtitle, "/") + "/" + a.Glob
		}
		if len(a.Files) > 0 {
			subtitle = fmt.Sprintf("%d fixed files", len(a.Files))
		}
		add(paletteItem{
			kind:     paletteAlias,
			title:    "@" + a.Name,
			subtitle: subtitle,
			keywords: a.Name + " " + subtitle,
			insert:   "@" + a.Name,
		})
	}
	for _, builtin := range []struct{ name, description string }{
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
		add(paletteItem{
			kind:     paletteAlias,
			title:    builtin.name,
			subtitle: builtin.description,
			keywords: builtin.name + " " + builtin.description,
			insert:   builtin.name,
		})
	}

	commands := []struct{ id, title, subtitle string }{
		{"new", "New operation", "browse all operations"},
		{"detect", "AI detector", "score a document and open the report"},
		{"jobs", "Jobs", "running and finished jobs"},
		{"home", "Home", "logo, actions, recent jobs"},
		{"aliases", "Aliases", "configured input shortcuts"},
		{"pipelines", "Pipelines", "saved pipeline library"},
		{"engines", "Engines", "tool availability and install hints"},
		{"theme", "Change theme", "eight built-in palettes with previews"},
		{"settings", "Settings", "output folder, theme, max jobs, config"},
		{"stats", "Statistics", "saved bytes and job totals"},
		{"editor", "Open editor", "edit text, Markdown, JSON, TOML, and configs"},
		{"setup", "Run setup", "revisit starter packs and engine choices"},
		{"config", "Open config", "edit config.toml in the built-in editor"},
	}
	if m.compose != nil && m.screen == screenCompose && len(m.compose.steps) > 0 {
		commands = append(commands, struct{ id, title, subtitle string }{
			"save", "Save current pipeline", "write the chain to the pipeline library",
		})
		if m.compose.savedName != "" {
			commands = append(commands,
				struct{ id, title, subtitle string }{
					"rename", "Rename pipeline", "save under a new name and retire the old one",
				},
				struct{ id, title, subtitle string }{
					"duplicate", "Duplicate pipeline", "save a copy and keep editing the copy",
				},
			)
		}
	}
	for _, c := range commands {
		add(paletteItem{
			kind:     paletteCommand,
			title:    "> " + c.title,
			subtitle: c.subtitle,
			keywords: c.title + " " + c.subtitle,
			command:  c.id,
		})
	}

	p.refilter()
	return p
}

func (m App) presetDisabled(preset *jobPreset) (bool, string) {
	if preset == nil {
		return true, "invalid preset"
	}
	for _, step := range preset.steps {
		op, ok := m.opts.Operations.Get(step.op)
		if !ok {
			return true, "missing operation: " + step.op
		}
		if disabled, hint := m.operationDisabled(op.Spec()); disabled {
			return true, hint
		}
	}
	return false, ""
}

func (m App) operationDisabled(spec core.Spec) (bool, string) {
	if len(spec.Engines) == 0 {
		return false, ""
	}
	for _, id := range spec.Engines {
		if id == engines.Internal {
			return false, ""
		}
	}
	if m.probing {
		return false, ""
	}

	if spec.RequireEngines {
		for _, id := range spec.Engines {
			if !m.engineEnabled(id) {
				return true, "disabled in setup — run > Run setup"
			}
			found := false
			for _, info := range m.engines {
				if info.ID != id {
					continue
				}
				found = true
				if !info.Available {
					hint := info.Hint
					if hint == "" {
						hint = "run wb doctor for install instructions"
					}
					return true, "requires " + string(id) + " — " + hint
				}
			}
			if !found {
				return true, "requires " + string(id) + " — run wb doctor"
			}
		}
		return false, ""
	}

	anyEnabled := false
	var missingID engines.ID
	for _, id := range spec.Engines {
		if !m.engineEnabled(id) {
			continue
		}
		anyEnabled = true
		for _, info := range m.engines {
			if info.ID == id {
				if info.Available {
					return false, ""
				}
				if missingID == "" {
					missingID = id
				}
				if info.Hint != "" {
					return true, "requires " + string(id) + " — " + info.Hint
				}
			}
		}
	}
	if !anyEnabled {
		return true, "disabled in setup — run > Run setup"
	}
	if missingID != "" {
		return true, "requires " + string(missingID) + " — run wb doctor"
	}
	return true, "engine is not available"
}

func (m App) engineEnabled(id engines.ID) bool {
	if id == engines.Internal {
		return true
	}
	for _, enabled := range m.opts.Config.Engines.Enabled {
		if enabled == string(id) {
			return true
		}
	}
	return false
}

func (p *palette) refilter() {
	mode := ""
	query := p.query
	if len(query) > 0 {
		switch query[0] {
		case '>', '@', '#':
			mode = query[:1]
			query = query[1:]
		}
	}
	query = strings.ToLower(strings.TrimSpace(query))

	items := make([]paletteItem, 0, len(p.all))
	for _, item := range p.all {
		switch mode {
		case ">":
			if item.kind != paletteCommand {
				continue
			}
		case "@":
			if item.kind != paletteAlias {
				continue
			}
		case "#":
			if item.kind != palettePipeline {
				continue
			}
		}
		if query == "" {
			items = append(items, item)
			continue
		}
		haystack := strings.ToLower(item.title + " " + item.subtitle + " " + item.keywords)
		if strings.Contains(haystack, query) {
			title := strings.ToLower(item.title)
			item.score = 100
			if strings.HasPrefix(title, query) {
				item.score += 100
			}
			if strings.Contains(title, query) {
				item.score += 50
			}
			items = append(items, item)
		}
	}

	// Keep fuzzy matching for short abbreviations such as "cv", but only when
	// no literal match exists. Otherwise "crop" matches Open config and half
	// the palette through loose subsequences.
	if len(items) == 0 {
		for _, item := range p.all {
			switch mode {
			case ">":
				if item.kind != paletteCommand {
					continue
				}
			case "@":
				if item.kind != paletteAlias {
					continue
				}
			case "#":
				if item.kind != palettePipeline {
					continue
				}
			}
			haystack := item.title + " " + item.subtitle + " " + item.keywords
			if score, ok := fuzzyScore(query, haystack); ok {
				item.score = score
				items = append(items, item)
			}
		}
	}

	sort.SliceStable(items, func(i, j int) bool {
		if items[i].disabled != items[j].disabled {
			return !items[i].disabled
		}
		if items[i].score != items[j].score {
			return items[i].score > items[j].score
		}
		return items[i].order < items[j].order
	})
	p.items = items
	p.cursor = clamp(p.cursor, 0, max(len(items)-1, 0))
}

func fuzzyScore(query, target string) (int, bool) {
	q := []rune(strings.ToLower(strings.TrimSpace(query)))
	if len(q) == 0 {
		return 0, true
	}
	t := []rune(strings.ToLower(target))
	qi := 0
	score := 0
	last := -2
	wordStart := true

	for i, r := range t {
		if r == q[qi] {
			score += 10
			if i == last+1 {
				score += 6
			}
			if i == 0 || wordStart {
				score += 4
			}
			last = i
			qi++
			if qi == len(q) {
				return score, true
			}
		}
		wordStart = r == ' ' || r == '-' || r == '/' || r == '.' || r == '_'
	}
	return 0, false
}

func (m App) handlePaletteKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	p := m.palette
	if p == nil {
		return m, nil
	}
	key := msg.String()
	switch key {
	case "esc", "ctrl+p":
		m.palette = nil
		return m, nil
	case "up", "ctrl+k":
		p.cursor = clamp(p.cursor-1, 0, max(len(p.items)-1, 0))
		return m, nil
	case "down", "ctrl+j", "ctrl+n":
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
		return m.selectPaletteItem()
	case "ctrl+d":
		return m.deletePalettePipeline()
	case "backspace":
		runes := []rune(p.query)
		if len(runes) > 0 {
			p.query = string(runes[:len(runes)-1])
		}
		p.err = ""
		p.refilter()
		p.cursor = 0
		return m, nil
	case "ctrl+u":
		p.query = ""
		p.err = ""
		p.refilter()
		p.cursor = 0
		return m, nil
	}

	if msg.Text != "" {
		p.query += msg.Text
		p.err = ""
		p.refilter()
		p.cursor = 0
	}
	return m, nil
}

func (m App) selectPaletteItem() (tea.Model, tea.Cmd) {
	p := m.palette
	if p == nil || len(p.items) == 0 {
		return m, nil
	}
	item := p.items[p.cursor]
	if item.disabled {
		p.err = item.disabledHint
		return m, nil
	}

	switch item.kind {
	case paletteOperation:
		if p.target == paletteAppendStep && m.compose != nil {
			m.compose.addStep(item.op)
			m.compose.openStep(len(m.compose.steps) - 1)
		} else {
			m.compose = m.newCompose(item.op)
			m.screen = screenCompose
		}
		m.palette = nil
		return m, nil

	case palettePreset:
		if item.preset == nil {
			p.err = "invalid preset"
			return m, nil
		}
		built, ok := m.presetPipeline(*item.preset)
		if !ok {
			p.err = "this preset needs an operation that is not available"
			return m, nil
		}
		m.compose = m.newComposeFromPipeline(built)
		m.screen = screenCompose
		m.palette = nil
		return m, nil

	case palettePipeline:
		if m.opts.Pipelines == nil {
			p.err = "pipeline storage is unavailable"
			return m, nil
		}
		loaded, err := m.opts.Pipelines.Load(item.meta.Name)
		if err != nil {
			p.err = err.Error()
			return m, nil
		}
		m.compose = m.newComposeFromPipeline(loaded)
		m.screen = screenCompose
		m.palette = nil
		return m, nil

	case paletteAlias:
		if m.compose != nil && m.screen == screenCompose {
			m.compose.focus = focusInput
			m.compose.input = item.insert
			m.compose.inputErr = nil
			m.compose.artifacts = nil
			m.compose.formErr = nil
			m.compose.inputVersion++
			m.compose.resolving = true
			version := m.compose.inputVersion
			m.screen = screenCompose
			m.palette = nil
			return m, tea.Tick(inputDebounce, func(time.Time) tea.Msg {
				return resolveTickMsg{version: version}
			})
		}
		m.screen = screenAliases
		m.palette = nil
		return m, nil

	case paletteCommand:
		m.palette = nil
		switch item.command {
		case "new":
			m.screen = screenOperations
		case "detect":
			if op, ok := m.opts.Operations.Get("doc.detect_ai"); ok {
				m.compose = m.newCompose(op)
				m.screen = screenCompose
			}
		case "jobs":
			m.ensureJobSelection()
			m.screen = screenJobs
		case "home":
			m.screen = screenHome
		case "aliases":
			m.screen = screenAliases
		case "engines":
			m.screen = screenEngines
		case "theme":
			return m.openThemePicker(m.screen)
		case "settings":
			return m.openSettings(m.screen)
		case "stats":
			m.screen = screenStats
		case "editor":
			m.prompt = m.newFilePrompt()
			return m, nil
		case "setup":
			m.onboarding = newOnboarding(m.opts.Config)
			m.screen = screenOnboarding
		case "config":
			return m.openConfigEditor()
		case "rename":
			if m.compose != nil && m.compose.savedName != "" {
				m.prompt = newRenamePrompt(m.compose)
			}
			return m, nil
		case "duplicate":
			if m.compose != nil && m.compose.savedName != "" {
				m.prompt = newDuplicatePrompt(m.compose)
			}
			return m, nil
		case "save":
			if m.compose != nil {
				m.prompt = newSavePrompt(m.compose)
			}
		case "pipelines":
			next := m.buildPalette(paletteNewCompose)
			next.query = "#"
			next.refilter()
			m.palette = next
		}
		return m, nil
	}
	return m, nil
}

func (m App) deletePalettePipeline() (tea.Model, tea.Cmd) {
	p := m.palette
	if p == nil || len(p.items) == 0 || m.opts.Pipelines == nil {
		return m, nil
	}
	item := p.items[p.cursor]
	if item.kind != palettePipeline {
		p.err = "ctrl+d only deletes saved pipelines"
		return m, nil
	}
	m.confirm = &confirmState{
		kind:    confirmDeletePipeline,
		title:   "Delete saved pipeline?",
		body:    "#" + item.meta.Name + " will move to .trash.",
		subject: item.meta.Name,
	}
	return m, nil
}

func (m *App) deletePipelineNow(name string) {
	if m.opts.Pipelines == nil {
		return
	}
	if err := m.opts.Pipelines.Delete(name); err != nil {
		m.toast = err.Error()
		m.toastLevel = core.LevelError
		return
	}
	if metas, err := m.opts.Pipelines.List(); err == nil {
		m.pipelineMetas = metas
	}
	m.toast = "deleted #" + name + " (moved to .trash)"
	m.toastLevel = core.LevelWarn
	if p := m.palette; p != nil {
		next := m.buildPalette(p.target)
		next.query = p.query
		next.refilter()
		m.palette = next
	}
}

func (m App) paletteView(height int) string {
	p := m.palette
	if p == nil {
		return ""
	}
	width := min(m.width-14, 82)
	// Start with every row and shrink until the framed card fits the body.
	// This keeps the entire border on screen instead of letting the card clip
	// its bottom edge at 80×24.
	maxRows := max(len(p.items), 1)
	for {
		card := m.theme.card.Width(width).Render(m.paletteBody(width, maxRows))
		if lipgloss.Height(card) <= height || maxRows <= 1 {
			return card
		}
		maxRows--
	}
}

func (m App) paletteBody(width, maxRows int) string {
	p := m.palette
	if p == nil {
		return ""
	}
	inner := max(width-8, 8)

	var b strings.Builder
	b.WriteString(m.theme.accent.Render("Command palette"))
	b.WriteString("\n")
	b.WriteString("› ")
	b.WriteString(truncate(p.query, inner-2))
	b.WriteString(m.theme.accent.Render("▏"))
	b.WriteString("\n")
	b.WriteString(m.theme.border.Render(strings.Repeat("─", inner)))
	b.WriteString("\n")

	if len(p.items) == 0 {
		b.WriteString(m.theme.muted.Render("no matches"))
		b.WriteString("\n")
	}

	maxRows = max(1, maxRows)
	start := 0
	if p.cursor >= maxRows {
		start = p.cursor - maxRows + 1
	}
	if maxStart := max(len(p.items)-maxRows, 0); start > maxStart {
		start = maxStart
	}
	for i := start; i < start+maxRows && i < len(p.items); i++ {
		item := p.items[i]
		marker := "  "
		if i == p.cursor {
			marker = m.theme.accent.Render("▸ ")
		}
		titleStyle := lipgloss.NewStyle()
		subtitle := item.subtitle
		subtitleStyle := m.theme.muted
		if item.disabled {
			titleStyle = m.theme.muted
			subtitle = item.disabledHint
			subtitleStyle = m.theme.warning
		}
		line := marker + titleStyle.Render(padRight(truncate(item.title, 30), 30)) +
			"  " + subtitleStyle.Render(truncate(subtitle, max(inner-34, 8)))
		if i == p.cursor {
			line = m.selectedRow(line, inner)
		}
		b.WriteString(line)
		b.WriteString("\n")
	}

	if len(p.items) > maxRows {
		b.WriteString(m.theme.muted.Render(fmt.Sprintf("  … %d results · %d/%d", len(p.items), p.cursor+1, len(p.items))))
		b.WriteString("\n")
	}
	if p.err != "" {
		b.WriteString(m.theme.danger.Render(truncate(p.err, inner)))
		b.WriteString("\n")
	}
	b.WriteString(m.theme.muted.Render("> commands · # pipelines · @ aliases"))
	b.WriteString("\n")
	footer := "↑↓/ctrl+j/k move · enter select · esc close"
	if len(p.items) > 0 && p.items[p.cursor].kind == palettePipeline {
		footer += " · ctrl+d delete"
	}
	b.WriteString(m.theme.muted.Render(footer))
	return b.String()
}
