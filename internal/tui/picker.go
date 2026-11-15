package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/solomonadzape95/workbench/internal/alias"
	"github.com/solomonadzape95/workbench/internal/config"
	"github.com/solomonadzape95/workbench/internal/core"
)

type pickerEntry struct {
	name string
	path string
	dir  bool
	size int64
}

// filePicker is a multi-select browser for the compose input step. It keeps
// selection order so aggregate operations such as Merge PDFs see the order the
// user picked.
type filePicker struct {
	dir      string
	display  string
	entries  []pickerEntry
	cursor   int
	selected map[string]bool
	order    []string
	allowed  map[string]bool
	err      string
}

func (m App) newFilePicker(c *compose) *filePicker {
	p := &filePicker{
		selected: map[string]bool{},
		allowed:  pickerFormats(c),
	}
	start := m.pickerStartDir(c)
	p.load(start)
	return p
}

func (m App) pickerStartDir(c *compose) string {
	if dir := c.commonInputDir(); dir != "" {
		return dir
	}
	raw := strings.TrimSpace(c.input)
	if i := strings.LastIndexAny(raw, ",\n"); i >= 0 {
		raw = strings.TrimSpace(raw[i+1:])
	}
	if raw != "" {
		if strings.HasPrefix(raw, "@") {
			if base, ok := alias.BasePath(raw, m.opts.Config.Aliases); ok {
				return config.ExpandHome(base)
			}
		}
		path := config.ExpandHome(raw)
		if info, err := os.Stat(path); err == nil {
			if info.IsDir() {
				return path
			}
			return filepath.Dir(path)
		}
		return filepath.Dir(path)
	}
	if cwd, err := os.Getwd(); err == nil {
		return cwd
	}
	return "."
}

func pickerFormats(c *compose) map[string]bool {
	if len(c.steps) == 0 {
		return nil
	}
	var formats []string
	for _, accept := range c.steps[0].spec.Accepts {
		formats = append(formats, accept.Formats...)
	}
	if len(formats) == 0 {
		return nil
	}
	out := make(map[string]bool, len(formats))
	for _, format := range formats {
		format = strings.ToLower(strings.TrimPrefix(strings.TrimSpace(format), "."))
		if format != "" {
			out["."+format] = true
		}
	}
	return out
}

func (p *filePicker) load(dir string) {
	dir = filepath.Clean(dir)
	p.dir = dir
	p.display = displayPath(dir)
	p.entries = nil
	p.cursor = 0
	entries, err := os.ReadDir(dir)
	if err != nil {
		p.err = fmt.Sprintf("cannot open %s", displayPath(dir))
		return
	}
	dirs := make([]pickerEntry, 0, len(entries))
	files := make([]pickerEntry, 0, len(entries))
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		if entry.IsDir() {
			dirs = append(dirs, pickerEntry{name: entry.Name(), path: path, dir: true})
			continue
		}
		ext := strings.ToLower(filepath.Ext(entry.Name()))
		if p.allowed != nil && !p.allowed[ext] {
			continue
		}
		info, statErr := entry.Info()
		if statErr != nil {
			continue
		}
		files = append(files, pickerEntry{name: entry.Name(), path: path, size: info.Size()})
	}
	sort.Slice(dirs, func(i, j int) bool {
		return strings.ToLower(dirs[i].name) < strings.ToLower(dirs[j].name)
	})
	sort.Slice(files, func(i, j int) bool {
		return strings.ToLower(files[i].name) < strings.ToLower(files[j].name)
	})
	p.entries = append(dirs, files...)
	p.err = ""
}

func (p *filePicker) current() (pickerEntry, bool) {
	if p.cursor < 0 || p.cursor >= len(p.entries) {
		return pickerEntry{}, false
	}
	return p.entries[p.cursor], true
}

func (p *filePicker) toggleCurrent() {
	entry, ok := p.current()
	if !ok || entry.dir {
		return
	}
	if p.selected[entry.path] {
		delete(p.selected, entry.path)
		for i, path := range p.order {
			if path == entry.path {
				p.order = append(p.order[:i], p.order[i+1:]...)
				break
			}
		}
		return
	}
	p.selected[entry.path] = true
	p.order = append(p.order, entry.path)
	p.err = ""
}

func (p *filePicker) openCurrent() {
	entry, ok := p.current()
	if !ok || !entry.dir {
		return
	}
	p.load(entry.path)
}

func (p *filePicker) parent() {
	parent := filepath.Dir(p.dir)
	if parent == p.dir {
		return
	}
	p.load(parent)
}

func (p *filePicker) selectAll() {
	for _, entry := range p.entries {
		if entry.dir || p.selected[entry.path] {
			continue
		}
		p.selected[entry.path] = true
		p.order = append(p.order, entry.path)
	}
	p.err = ""
}

func (p *filePicker) clear() {
	p.selected = map[string]bool{}
	p.order = nil
	p.err = ""
}

func shortPath(path string) string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return path
	}
	if path == home {
		return "~"
	}
	if strings.HasPrefix(path, home+string(filepath.Separator)) {
		return "~/" + strings.TrimPrefix(path, home+string(filepath.Separator))
	}
	return path
}

func displayPath(path string) string {
	path = shortPath(path)
	if path == "~" {
		return "~/"
	}
	if !strings.HasSuffix(path, "/") {
		path += "/"
	}
	return path
}

func (m App) handlePickerKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	p := m.picker
	if p == nil {
		return m, nil
	}
	switch msg.String() {
	case "esc":
		m.picker = nil
	case "j", "down":
		p.cursor = clamp(p.cursor+1, 0, max(len(p.entries)-1, 0))
	case "k", "up":
		p.cursor = clamp(p.cursor-1, 0, max(len(p.entries)-1, 0))
	case "g", "home":
		p.cursor = 0
	case "G", "end":
		p.cursor = max(len(p.entries)-1, 0)
	case " ", "space":
		p.toggleCurrent()
	case "enter":
		if entry, ok := p.current(); ok && entry.dir {
			p.openCurrent()
		} else {
			p.toggleCurrent()
		}
	case "right", "l":
		p.openCurrent()
	case "left", "h", "backspace":
		p.parent()
	case "a":
		p.selectAll()
	case "A":
		p.clear()
	case "ctrl+s":
		return m.applyFilePicker()
	}
	return m, nil
}

func (m App) applyFilePicker() (tea.Model, tea.Cmd) {
	p := m.picker
	c := m.compose
	if p == nil || c == nil {
		m.picker = nil
		return m, nil
	}
	if len(p.order) == 0 {
		p.err = "pick at least one file"
		return m, nil
	}
	if c.pipelineMode() == core.ModeAggregate && len(p.order) < 2 {
		p.err = "this operation needs at least two files"
		return m, nil
	}
	selected := make([]string, 0, len(p.order))
	for _, path := range p.order {
		selected = append(selected, shortPath(path))
	}
	c.input = strings.Join(selected, ", ")
	c.dirty = true
	c.focus = focusInput
	c.formErr = nil
	m.picker = nil
	return m, c.afterTextChange(m.opts.Config)
}

func (m App) pickerView(height int) string {
	p := m.picker
	if p == nil {
		return ""
	}
	width := min(m.width-8, 86)
	inner := max(width-6, 24)
	rows := min(len(p.entries), max(height-12, 4))
	start := 0
	if p.cursor >= rows {
		start = p.cursor - rows + 1
	}
	end := min(start+rows, len(p.entries))

	var lines []string
	lines = append(lines, m.theme.accent.Render("Pick files")+
		m.theme.muted.Render("  "+truncate(p.display, max(inner-12, 12))))
	if p.err != "" {
		lines = append(lines, m.theme.danger.Render(truncate(p.err, inner)))
	}
	if len(p.entries) == 0 {
		lines = append(lines, m.theme.muted.Render("  no matching files"))
	}
	nameW := max(inner-14, 16)
	for i := start; i < end; i++ {
		entry := p.entries[i]
		check := m.theme.danger.Render("[ ]")
		if entry.dir {
			check = "   "
		} else if p.selected[entry.path] {
			check = m.theme.success.Render("[x]")
		}
		name := entry.name
		style := m.theme.muted
		if entry.dir {
			name += "/"
			style = m.theme.accent
		}
		size := ""
		if !entry.dir {
			size = humanBytes(entry.size)
		}
		line := check + " " + style.Render(padRight(truncate(name, nameW), nameW)) +
			" " + m.theme.muted.Render(numericCell(size, 9))
		lines = append(lines, m.listRow(line, inner, i == p.cursor))
	}
	if len(p.entries) > rows {
		lines = append(lines, m.theme.muted.Render(fmt.Sprintf("  … %d more", len(p.entries)-end)))
	}
	lines = append(lines, "", m.theme.muted.Render(fmt.Sprintf(
		"  %d selected · space/enter pick · → open · ctrl+s use · esc cancel", len(p.order))))
	return m.theme.card.Width(width).Render(strings.Join(lines, "\n"))
}
