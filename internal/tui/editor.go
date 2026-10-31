package tui

import (
	"fmt"
	"os"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/solomonadzape95/workbench/internal/alias"
	"github.com/solomonadzape95/workbench/internal/config"
	"github.com/solomonadzape95/workbench/internal/core"
	"github.com/solomonadzape95/workbench/internal/editor"
)

type editorState struct {
	buf          *editor.Buffer
	start        string
	scrollY      int
	scrollX      int
	pendingClose bool
	searching    bool
	query        string
	searchErr    string
}

func (m App) resolveEditorPath(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", fmt.Errorf("enter a file path")
	}
	if strings.HasPrefix(raw, "@") {
		arts, err := alias.Resolve(raw, m.opts.Config.Aliases)
		if err != nil {
			return "", err
		}
		if len(arts) == 0 {
			return "", fmt.Errorf("%s matched no files", raw)
		}
		if len(arts) > 1 {
			return "", fmt.Errorf("%s matches %d files; choose one under the alias", raw, len(arts))
		}
		return arts[0].Path, nil
	}
	return config.ExpandHome(raw), nil
}

func (m App) openEditor(path string, returnTo screen) (tea.Model, tea.Cmd) {
	resolved, err := m.resolveEditorPath(path)
	if err != nil {
		m.toast = "editor: " + err.Error()
		m.toastLevel = core.LevelError
		return m, nil
	}
	if info, statErr := os.Stat(resolved); statErr == nil && info.IsDir() {
		m.toast = "editor: that is a directory; enter a file path"
		m.toastLevel = core.LevelWarn
		return m, nil
	}
	path = resolved
	buf, err := editor.Open(path)
	if err != nil {
		m.toast = "editor: " + err.Error()
		m.toastLevel = core.LevelError
		return m, nil
	}
	if path == m.opts.Paths.ConfigFile && !strings.Contains(buf.Text(), "# Workbench configuration") {
		buf = editor.New(config.Header+"\n"+buf.Text(), path)
	}
	start, _ := os.Getwd()
	m.editor = &editorState{buf: buf, start: start}
	m.editorReturn = returnTo
	m.prompt = nil
	m.screen = screenEditor
	return m, nil
}

func (m App) handleEditorKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	e := m.editor
	if e == nil {
		m.screen = screenHome
		return m, nil
	}
	key := msg.String()

	if e.pendingClose {
		switch key {
		case "s", "ctrl+s":
			if m.saveEditor() {
				return m.closeEditor(), nil
			}
		case "d", "q", "ctrl+q":
			return m.closeEditor(), nil
		case "esc":
			e.pendingClose = false
		}
		return m, nil
	}

	if e.searching {
		switch key {
		case "esc":
			e.searching = false
			e.searchErr = ""
		case "enter":
			m.findInEditor()
		case "backspace":
			runes := []rune(e.query)
			if len(runes) > 0 {
				e.query = string(runes[:len(runes)-1])
			}
		case "ctrl+u":
			e.query = ""
		default:
			if msg.Text != "" {
				e.query += msg.Text
			}
		}
		return m, nil
	}

	switch key {
	case "esc", "ctrl+q":
		if e.buf.Dirty() {
			e.pendingClose = true
			return m, nil
		}
		return m.closeEditor(), nil
	case "ctrl+s":
		m.saveEditor()
		return m, nil
	case "ctrl+z":
		e.buf.Undo()
		m.scrollEditor()
		return m, nil
	case "ctrl+y":
		e.buf.Redo()
		m.scrollEditor()
		return m, nil
	case "ctrl+f":
		e.searching = true
		e.query = ""
		e.searchErr = ""
		return m, nil
	case "ctrl+n", "f3":
		if e.query != "" {
			m.findInEditorAt(true)
		}
		return m, nil
	case "ctrl+p", "shift+f3":
		if e.query != "" {
			m.findInEditorAt(false)
		}
		return m, nil
	case "ctrl+left", "alt+left":
		e.buf.MoveWordLeft()
	case "ctrl+right", "alt+right":
		e.buf.MoveWordRight()
	case "ctrl+backspace", "ctrl+w":
		e.buf.DeleteWordBackward()
	case "ctrl+u":
		e.buf.DeleteToLineStart()
	case "up":
		e.buf.MoveUp()
	case "down":
		e.buf.MoveDown()
	case "left":
		e.buf.MoveLeft()
	case "right":
		e.buf.MoveRight()
	case "home", "ctrl+a":
		e.buf.Home()
	case "end", "ctrl+e":
		e.buf.End()
	case "pgup":
		e.buf.MovePage(-1, m.editorRows())
	case "pgdown":
		e.buf.MovePage(1, m.editorRows())
	case "ctrl+home":
		e.buf.MoveTo(0, 0)
	case "ctrl+end":
		e.buf.MoveTo(e.buf.LineCount()-1, 0)
		e.buf.End()
	case "tab":
		e.buf.Insert("  ")
	case "enter":
		e.buf.Newline()
	case "backspace":
		e.buf.Backspace()
	case "delete":
		e.buf.Delete()
	default:
		if msg.Text != "" {
			e.buf.Insert(msg.Text)
		}
	}
	m.scrollEditor()
	return m, nil
}

func (m App) saveEditor() bool {
	e := m.editor
	if e == nil {
		return false
	}
	if err := e.buf.Save(); err != nil {
		m.toast = "save: " + err.Error()
		m.toastLevel = core.LevelError
		return false
	}
	e.pendingClose = false
	if e.buf.Path() == m.opts.Paths.ConfigFile {
		m.toast = "saved config — restart to apply engine/alias changes"
	} else {
		m.toast = "saved " + e.buf.Path()
	}
	m.toastLevel = core.LevelInfo
	return true
}

func (m App) closeEditor() App {
	m.editor = nil
	m.screen = m.editorReturn
	return m
}

func (m App) findInEditor() {
	e := m.editor
	if e == nil || strings.TrimSpace(e.query) == "" {
		return
	}
	line, col := e.buf.Cursor()
	line, col, ok := e.buf.Find(e.query, line, col+1)
	if !ok {
		e.searchErr = "not found: " + e.query
		return
	}
	e.buf.MoveTo(line, col)
	e.searching = false
	e.searchErr = ""
	m.scrollEditor()
}

func (m App) findInEditorAt(forward bool) {
	e := m.editor
	if e == nil || strings.TrimSpace(e.query) == "" {
		return
	}
	line, col := e.buf.Cursor()
	var ok bool
	if forward {
		line, col, ok = e.buf.Find(e.query, line, col+1)
	} else {
		line, col, ok = e.buf.FindPrev(e.query, line, col)
	}
	if !ok {
		e.searchErr = "not found: " + e.query
		return
	}
	e.buf.MoveTo(line, col)
	e.searchErr = ""
	m.scrollEditor()
}

func (m App) scrollEditor() {
	e := m.editor
	if e == nil {
		return
	}
	line, col := e.buf.Cursor()
	rows := m.editorRows()
	if line < e.scrollY {
		e.scrollY = line
	}
	if line >= e.scrollY+rows {
		e.scrollY = line - rows + 1
	}
	width := m.editorTextWidth()
	if col < e.scrollX {
		e.scrollX = col
	}
	if col >= e.scrollX+width {
		e.scrollX = col - width + 1
	}
	if e.scrollY < 0 {
		e.scrollY = 0
	}
	if e.scrollX < 0 {
		e.scrollX = 0
	}
}

func (m App) editorRows() int {
	header := lipgloss.Height(m.headerView())
	status := lipgloss.Height(m.statusView())
	return max(m.height-header-status-5, 3)
}

func (m App) editorGutter() int {
	if m.editor == nil {
		return 5
	}
	width := len(fmt.Sprint(max(m.editor.buf.LineCount(), 1)))
	return max(width, 2) + 3
}

func (m App) editorTextWidth() int {
	return max(m.width-6-m.editorGutter(), 8)
}

func (m App) editorView(height int) string {
	e := m.editor
	if e == nil {
		return ""
	}
	e.scrollY = clamp(e.scrollY, 0, max(e.buf.LineCount()-1, 0))
	m.scrollEditor()

	width := m.width - 4
	contentWidth := max(width-4, 20)
	gutter := m.editorGutter()
	textWidth := max(contentWidth-gutter, 8)
	rows := max(height-5, 3)
	if e.searching {
		rows = max(rows-1, 2)
	}

	var b strings.Builder
	title := e.buf.Path()
	if title == "" {
		title = "untitled"
	}
	b.WriteString(m.theme.accent.Render("▌ ") + m.theme.title.Render(truncateTail(title, max(contentWidth-14, 10))))
	if e.buf.Dirty() {
		b.WriteString(m.theme.warning.Render("  modified"))
	}
	if e.searching {
		b.WriteString("\n")
		b.WriteString(m.theme.accent.Render("find: "))
		b.WriteString(truncate(e.query, max(textWidth-8, 6)))
		b.WriteString(m.theme.accent.Render("▏"))
		if e.searchErr != "" {
			b.WriteString("  " + m.theme.danger.Render(e.searchErr))
		}
	}
	b.WriteString("\n")
	if e.start != "" {
		b.WriteString(m.theme.muted.Render("start: " + truncate(e.start, max(contentWidth-7, 8))))
		b.WriteString("\n")
	}
	b.WriteString(m.theme.border.Render(strings.Repeat("─", contentWidth)))
	b.WriteString("\n")

	lineCount := e.buf.LineCount()
	end := min(e.scrollY+rows, lineCount)
	for i := e.scrollY; i < end; i++ {
		line := strings.ReplaceAll(e.buf.Line(i), "\t", "    ")
		runes := []rune(line)
		start := min(e.scrollX, len(runes))
		segment := string(runes[start:])
		segment = truncate(segment, textWidth)
		prefix := fmt.Sprintf("%*d │ ", gutter-3, i+1)
		row := prefix + segment
		if i == cursorLine(e) {
			row = m.theme.currentLine.Render(padRight(row, contentWidth))
		}
		b.WriteString(row)
		b.WriteString("\n")
	}
	if lineCount == 0 {
		b.WriteString(m.theme.muted.Render("  empty file"))
		b.WriteString("\n")
	}
	for i := end - e.scrollY; i < rows; i++ {
		b.WriteString("\n")
	}
	if e.pendingClose {
		b.WriteString(m.theme.warning.Render("unsaved changes · s save · d discard · esc cancel"))
	} else {
		line, col := e.buf.Cursor()
		b.WriteString(m.theme.muted.Render(fmt.Sprintf("Ln %d, Col %d", line+1, col+1)))
	}
	return m.panel("Editor", b.String(), width, height, true)
}

func cursorLine(e *editorState) int {
	line, _ := e.buf.Cursor()
	return line
}

func (m App) editorCursor() (x, y int, ok bool) {
	e := m.editor
	if e == nil || m.screen != screenEditor || m.palette != nil || m.prompt != nil || m.showHelp || m.logView != nil || e.pendingClose || e.searching {
		return 0, 0, false
	}
	line, col := e.buf.Cursor()
	if line < e.scrollY || line >= e.scrollY+m.editorRows() {
		return 0, 0, false
	}
	text := strings.ReplaceAll(e.buf.Line(line), "\t", "    ")
	runes := []rune(text)
	if col > len(runes) {
		col = len(runes)
	}
	prefix := string(runes[:col])
	visual := lipgloss.Width(prefix) - e.scrollX
	if visual < 0 {
		return 0, 0, false
	}
	header := lipgloss.Height(m.headerView())
	// screen x: body left padding (2) + panel border/space (2) + gutter.
	x = 4 + m.editorGutter() + visual
	// screen y: header + panel top + title + optional start line + rule. The
	// previous +3 put the cursor one row above the character being edited.
	prelude := 3
	if e.start != "" {
		prelude++
	}
	y = header + prelude + (line - e.scrollY)
	return x, y, true
}
