package tui

import (
	"context"
	"fmt"
	"sort"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/solomonadzape95/workbench/internal/core"
	"github.com/solomonadzape95/workbench/internal/engines"
)

type engineInstallEntry struct {
	spec      engines.InstallSpec
	available bool
	selected  bool
	status    string // "", "queued", "installing", "done", "failed"
	output    string
}

type engineInstaller struct {
	entries  []*engineInstallEntry
	cursor   int
	running  bool
	done     bool
	current  int
	pending  []int
	logs     []string
	err      string
	returnTo screen
	ctx      context.Context
	cancel   context.CancelFunc
}

type engineInstallDoneMsg struct {
	id     engines.ID
	output string
	err    error
}

func (m App) openEngineInstaller(returnTo screen, selectAll bool) (tea.Model, tea.Cmd) {
	m.installer = m.newEngineInstaller(returnTo, selectAll)
	m.exitAfterInstaller = false
	return m, nil
}

func (m App) newEngineInstaller(returnTo screen, selectAll bool) *engineInstaller {
	known := make(map[engines.ID]engines.Info, len(m.engines))
	for _, info := range m.engines {
		known[info.ID] = info
	}

	in := &engineInstaller{returnTo: returnTo}
	for _, spec := range engines.InstallCatalog() {
		available := spec.Installed()
		if info, ok := known[spec.ID]; ok {
			available = info.Available
		}
		entry := &engineInstallEntry{spec: spec, available: available}
		if selectAll && !available && spec.Auto() {
			entry.selected = true
		}
		in.entries = append(in.entries, entry)
	}
	sort.SliceStable(in.entries, func(i, j int) bool {
		return engineInstallRank(in.entries[i]) < engineInstallRank(in.entries[j])
	})
	return in
}

// engineInstallRank keeps missing automatic installers first, then manual
// installs, then engines that are already present.
func engineInstallRank(entry *engineInstallEntry) int {
	switch {
	case entry.available:
		return 2
	case !entry.spec.Auto():
		return 1
	default:
		return 0
	}
}

func (m App) handleInstallerKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	in := m.installer
	if in == nil {
		return m, nil
	}
	key := msg.String()

	if in.running {
		if key == "esc" || key == "x" || key == "ctrl+c" {
			if in.cancel != nil {
				in.cancel()
			}
			in.running = false
			in.current = -1
			in.pending = nil
			in.cancel = nil
			in.err = "install cancelled"
			in.logs = append(in.logs, "cancelled")
		}
		return m, nil
	}

	switch key {
	case "esc", "q", "ctrl+c":
		return m.closeEngineInstaller()
	case "j", "down":
		in.cursor = clamp(in.cursor+1, 0, max(len(in.entries)-1, 0))
	case "k", "up":
		in.cursor = clamp(in.cursor-1, 0, max(len(in.entries)-1, 0))
	case "g", "home":
		in.cursor = 0
	case "G", "end":
		in.cursor = max(len(in.entries)-1, 0)
	case " ", "space":
		entry := in.currentEntry()
		if entry == nil {
			return m, nil
		}
		switch {
		case entry.available:
			in.err = entry.spec.Name + " is already installed"
		case !entry.spec.Auto():
			in.err = entry.spec.Name + " needs a manual install"
		default:
			entry.selected = !entry.selected
			in.err = ""
			in.done = false
		}
	case "a":
		for _, entry := range in.entries {
			if !entry.available && entry.spec.Auto() {
				entry.selected = true
			}
		}
		in.err = ""
		in.done = false
	case "A":
		for _, entry := range in.entries {
			entry.selected = false
		}
		in.err = ""
	case "enter":
		if in.done && in.selectedCount() == 0 {
			return m.closeEngineInstaller()
		}
		return m.startEngineInstall()
	}
	return m, nil
}

func (m App) startEngineInstall() (tea.Model, tea.Cmd) {
	in := m.installer
	if in == nil {
		return m, nil
	}
	in.pending = nil
	for i, entry := range in.entries {
		if !entry.selected || entry.available || !entry.spec.Auto() {
			continue
		}
		entry.selected = false
		entry.status = "queued"
		in.pending = append(in.pending, i)
	}
	if len(in.pending) == 0 {
		in.err = "select at least one missing engine"
		return m, nil
	}
	in.running = true
	in.done = false
	in.err = ""
	in.logs = nil
	in.ctx, in.cancel = context.WithCancel(context.Background())
	return m.nextEngineInstall()
}

func (m App) nextEngineInstall() (tea.Model, tea.Cmd) {
	in := m.installer
	if in == nil {
		return m, nil
	}
	if len(in.pending) == 0 {
		in.running = false
		in.done = true
		in.current = -1
		if in.cancel != nil {
			in.cancel()
			in.cancel = nil
		}
		installed, failed := in.counts()
		switch {
		case failed > 0:
			m.toast = fmt.Sprintf("installed %d · %d failed", installed, failed)
			m.toastLevel = core.LevelWarn
		case installed > 0:
			m.toast = fmt.Sprintf("installed %d engine(s) — restart Workbench to use them", installed)
			m.toastLevel = core.LevelInfo
		}
		return m, probeEngines(m.opts.Registry)
	}

	index := in.pending[0]
	in.pending = in.pending[1:]
	in.current = index
	in.entries[index].status = "installing"
	ctx := in.ctx
	spec := in.entries[index].spec
	return m, func() tea.Msg {
		output, err := engines.RunInstall(ctx, spec.Command())
		return engineInstallDoneMsg{id: spec.ID, output: output, err: err}
	}
}

func (m App) handleEngineInstallDone(msg engineInstallDoneMsg) (tea.Model, tea.Cmd) {
	in := m.installer
	if in == nil || !in.running {
		return m, nil
	}
	index := in.current
	if index < 0 || index >= len(in.entries) || in.entries[index].spec.ID != msg.id {
		index = in.indexOf(msg.id)
	}
	if index >= 0 {
		entry := in.entries[index]
		if msg.err != nil {
			entry.status = "failed"
			entry.output = strings.TrimSpace(msg.output + "\n" + msg.err.Error())
			in.err = entry.spec.Name + " failed"
		} else {
			entry.status = "done"
			entry.output = strings.TrimSpace(msg.output)
			entry.available = true
		}
		in.appendLog(entry.spec, msg)
	}
	in.current = -1
	return m.nextEngineInstall()
}

func (m App) closeEngineInstaller() (tea.Model, tea.Cmd) {
	in := m.installer
	if in == nil {
		return m, nil
	}
	if in.cancel != nil {
		in.cancel()
	}
	returnTo := in.returnTo
	m.installer = nil
	if m.exitAfterInstaller {
		return m, tea.Quit
	}
	m.screen = returnTo
	return m, nil
}

func (in *engineInstaller) currentEntry() *engineInstallEntry {
	if in.cursor < 0 || in.cursor >= len(in.entries) {
		return nil
	}
	return in.entries[in.cursor]
}

func (in *engineInstaller) indexOf(id engines.ID) int {
	for i, entry := range in.entries {
		if entry.spec.ID == id {
			return i
		}
	}
	return -1
}

func (in *engineInstaller) summary() (missing, selected, installed int) {
	for _, entry := range in.entries {
		if entry.available {
			installed++
			continue
		}
		missing++
		if entry.selected {
			selected++
		}
	}
	return missing, selected, installed
}

func (in *engineInstaller) selectedCount() int {
	selected := 0
	for _, entry := range in.entries {
		if entry.selected {
			selected++
		}
	}
	return selected
}

func (in *engineInstaller) counts() (installed, failed int) {
	for _, entry := range in.entries {
		switch entry.status {
		case "done":
			installed++
		case "failed":
			failed++
		}
	}
	return installed, failed
}

func (in *engineInstaller) refresh(infos []engines.Info) {
	known := make(map[engines.ID]engines.Info, len(infos))
	for _, info := range infos {
		known[info.ID] = info
	}
	for _, entry := range in.entries {
		if info, ok := known[entry.spec.ID]; ok && info.Available {
			entry.available = true
		}
	}
}

func (in *engineInstaller) appendLog(spec engines.InstallSpec, msg engineInstallDoneMsg) {
	in.logs = append(in.logs, "$ "+spec.Hint())
	lines := strings.Split(strings.TrimSpace(msg.output), "\n")
	if len(lines) > 6 {
		lines = lines[len(lines)-6:]
	}
	for _, line := range lines {
		line = strings.TrimRight(line, " \t")
		if line != "" {
			in.logs = append(in.logs, "  "+line)
		}
	}
	if msg.err != nil {
		in.logs = append(in.logs, "  error: "+msg.err.Error())
	}
	if len(in.logs) > 24 {
		in.logs = in.logs[len(in.logs)-24:]
	}
}

func (m App) installerView(height int) string {
	in := m.installer
	if in == nil {
		return ""
	}
	width := min(m.width-4, 106)
	contentWidth := width - 4
	missing, selected, installed := in.summary()

	var b strings.Builder
	b.WriteString(m.theme.accent.Render("Install optional engines"))
	b.WriteString("\n")
	b.WriteString(m.theme.muted.Render(fmt.Sprintf(
		"%d missing · %d selected · %d installed", missing, selected, installed)))
	b.WriteString("\n\n")

	rows := max(min(len(in.entries), max(height-12, 4)), 1)
	start := 0
	if in.cursor >= rows {
		start = in.cursor - rows + 1
	}
	end := min(start+rows, len(in.entries))

	nameW, statusW := 14, 11
	detailW := max(contentWidth-nameW-statusW-6, 10)
	for i := start; i < end; i++ {
		entry := in.entries[i]
		check := "[ ]"
		switch {
		case entry.available:
			check = m.theme.success.Render("[✓]")
		case !entry.spec.Auto():
			check = m.theme.muted.Render("[–]")
		case entry.selected:
			check = m.theme.accent.Render("[x]")
		}
		status := "missing"
		statusStyle := m.theme.warning
		detail := entry.spec.Hint()
		switch entry.status {
		case "queued":
			status, statusStyle = "queued", m.theme.muted
		case "installing":
			status, statusStyle = "installing", m.theme.accent
		case "done":
			status, statusStyle = "done", m.theme.success
		case "failed":
			status, statusStyle = "failed", m.theme.danger
			detail = entry.output
		}
		if entry.available && entry.status == "" {
			status, statusStyle = "installed", m.theme.success
			detail = "ready"
		}
		if !entry.spec.Auto() && !entry.available && entry.status == "" {
			status, statusStyle = "manual", m.theme.muted
		}
		line := check + " " +
			m.theme.accent.Render(padRight(truncate(entry.spec.Name, nameW), nameW)) + " " +
			statusStyle.Render(padRight(status, statusW)) + " " +
			m.theme.muted.Render(truncate(detail, detailW))
		b.WriteString(m.listRow(line, contentWidth, i == in.cursor))
		b.WriteString("\n")
	}
	if len(in.entries) > rows {
		b.WriteString(m.theme.muted.Render(fmt.Sprintf("  … %d more", len(in.entries)-end)))
		b.WriteString("\n")
	}

	b.WriteString("\n")
	if entry := in.currentEntry(); entry != nil {
		if entry.spec.Summary != "" {
			b.WriteString(m.theme.muted.Render("  " +
				truncate(entry.spec.Summary, max(contentWidth-4, 10))))
			b.WriteString("\n")
		}
		switch {
		case entry.available:
			b.WriteString(m.theme.success.Render("  installed"))
		case !entry.spec.Auto():
			b.WriteString(m.theme.warning.Render("  manual: ") +
				m.theme.muted.Render(truncate(entry.spec.Hint(), max(contentWidth-10, 10))))
		default:
			b.WriteString(m.theme.muted.Render("  $ ") +
				truncate(entry.spec.Hint(), max(contentWidth-4, 10)))
		}
		if entry.spec.PostInstall != "" {
			b.WriteString("\n")
			b.WriteString(m.theme.muted.Render("  after: " +
				truncate(entry.spec.PostInstall, max(contentWidth-9, 10))))
		}
		b.WriteString("\n")
	}
	if in.err != "" {
		b.WriteString(m.theme.danger.Render("  ! " + truncate(in.err, max(contentWidth-4, 10))))
		b.WriteString("\n")
	}
	if in.running || in.done || len(in.logs) > 0 {
		b.WriteString("\n")
		b.WriteString(m.theme.accent.Render("Log"))
		b.WriteString("\n")
		logs := in.logs
		if len(logs) > 6 {
			logs = logs[len(logs)-6:]
		}
		for _, line := range logs {
			b.WriteString(m.theme.muted.Render("  " + truncate(line, max(contentWidth-4, 8))))
			b.WriteString("\n")
		}
	}
	if in.done {
		b.WriteString("\n")
		b.WriteString(m.theme.muted.Render("enter or esc to close"))
	}

	content := b.String()
	panelHeight := min(height, max(16, len(strings.Split(content, "\n"))+2))
	return m.panel("Install engines", content, width, panelHeight, true)
}
