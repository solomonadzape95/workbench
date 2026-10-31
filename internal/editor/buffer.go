// Package editor contains the text model behind Workbench's built-in editor.
// It has no terminal or Bubble Tea dependencies, so saving and cursor behavior
// can be tested without a pty.
package editor

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

var ErrNoPath = errors.New("editor: no file path")

type state struct {
	lines []string
	line  int
	col   int
}

// Buffer is a small, deliberately boring text buffer: lines, a rune cursor,
// undo/redo, atomic save. Rendering belongs to the TUI.
type Buffer struct {
	path  string
	lines []string
	line  int
	col   int
	dirty bool

	undo []state
	redo []state
}

func New(text, path string) *Buffer {
	lines := splitLines(text)
	return &Buffer{path: path, lines: lines}
}

// Open reads a text file. A missing file starts an empty buffer with that path
// so the editor can create it on the first save.
func Open(path string) (*Buffer, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return New("", path), nil
	}
	if err != nil {
		return nil, err
	}
	if strings.IndexByte(string(data), 0) >= 0 {
		return nil, fmt.Errorf("%s looks like a binary file", path)
	}
	return New(normalize(string(data)), path), nil
}

func NewFromLines(lines []string, path string) *Buffer {
	if len(lines) == 0 {
		lines = []string{""}
	}
	return &Buffer{path: path, lines: append([]string(nil), lines...)}
}

func (b *Buffer) Path() string     { return b.path }
func (b *Buffer) SetPath(p string) { b.path = p }
func (b *Buffer) Dirty() bool      { return b.dirty }
func (b *Buffer) LineCount() int   { return len(b.lines) }

func (b *Buffer) Cursor() (line, col int) { return b.line, b.col }

func (b *Buffer) Line(n int) string {
	if n < 0 || n >= len(b.lines) {
		return ""
	}
	return b.lines[n]
}

func (b *Buffer) Text() string { return strings.Join(b.lines, "\n") }

func (b *Buffer) Insert(text string) {
	if text == "" {
		return
	}
	b.record()
	parts := strings.Split(normalize(text), "\n")
	current := b.lines[b.line]
	prefix := string([]rune(current)[:b.col])
	suffix := string([]rune(current)[b.col:])
	if len(parts) == 1 {
		b.lines[b.line] = prefix + parts[0] + suffix
		b.col += len([]rune(parts[0]))
	} else {
		newLines := make([]string, 0, len(b.lines)+len(parts)-1)
		newLines = append(newLines, b.lines[:b.line]...)
		newLines = append(newLines, prefix+parts[0])
		newLines = append(newLines, parts[1:len(parts)-1]...)
		last := parts[len(parts)-1]
		newLines = append(newLines, last+suffix)
		newLines = append(newLines, b.lines[b.line+1:]...)
		b.lines = newLines
		b.line += len(parts) - 1
		b.col = len([]rune(last))
	}
	b.dirty = true
}

func (b *Buffer) Newline() { b.Insert("\n") }

func (b *Buffer) Backspace() {
	if b.col > 0 {
		b.record()
		runes := []rune(b.lines[b.line])
		b.lines[b.line] = string(runes[:b.col-1]) + string(runes[b.col:])
		b.col--
		b.dirty = true
		return
	}
	if b.line == 0 {
		return
	}
	b.record()
	prev := b.lines[b.line-1]
	cur := b.lines[b.line]
	b.lines[b.line-1] = prev + cur
	b.lines = append(b.lines[:b.line], b.lines[b.line+1:]...)
	b.line--
	b.col = len([]rune(prev))
	b.dirty = true
}

func (b *Buffer) Delete() {
	runes := []rune(b.lines[b.line])
	if b.col < len(runes) {
		b.record()
		b.lines[b.line] = string(runes[:b.col]) + string(runes[b.col+1:])
		b.dirty = true
		return
	}
	if b.line >= len(b.lines)-1 {
		return
	}
	b.record()
	b.lines[b.line] += b.lines[b.line+1]
	b.lines = append(b.lines[:b.line+1], b.lines[b.line+2:]...)
	b.dirty = true
}

func (b *Buffer) MoveLeft() {
	if b.col > 0 {
		b.col--
	} else if b.line > 0 {
		b.line--
		b.col = len([]rune(b.lines[b.line]))
	}
}

func (b *Buffer) MoveRight() {
	if b.col < len([]rune(b.lines[b.line])) {
		b.col++
	} else if b.line < len(b.lines)-1 {
		b.line++
		b.col = 0
	}
}

func (b *Buffer) MoveWordLeft() {
	runes := []rune(b.lines[b.line])
	pos := b.col
	if pos == 0 {
		if b.line == 0 {
			return
		}
		b.line--
		b.col = len([]rune(b.lines[b.line]))
		return
	}
	for pos > 0 && !isWordRune(runes[pos-1]) {
		pos--
	}
	for pos > 0 && isWordRune(runes[pos-1]) {
		pos--
	}
	b.col = pos
}

func (b *Buffer) MoveWordRight() {
	runes := []rune(b.lines[b.line])
	pos := b.col
	if pos >= len(runes) {
		if b.line >= len(b.lines)-1 {
			return
		}
		b.line++
		b.col = 0
		return
	}
	for pos < len(runes) && isWordRune(runes[pos]) {
		pos++
	}
	for pos < len(runes) && !isWordRune(runes[pos]) {
		pos++
	}
	b.col = pos
}

func (b *Buffer) DeleteWordBackward() {
	if b.col == 0 {
		b.Backspace()
		return
	}
	runes := []rune(b.lines[b.line])
	pos := b.col
	for pos > 0 && !isWordRune(runes[pos-1]) {
		pos--
	}
	for pos > 0 && isWordRune(runes[pos-1]) {
		pos--
	}
	if pos == b.col {
		return
	}
	b.record()
	b.lines[b.line] = string(runes[:pos]) + string(runes[b.col:])
	b.col = pos
	b.dirty = true
}

func (b *Buffer) DeleteToLineStart() {
	if b.col == 0 {
		return
	}
	b.record()
	runes := []rune(b.lines[b.line])
	b.lines[b.line] = string(runes[b.col:])
	b.col = 0
	b.dirty = true
}

func isWordRune(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
		(r >= '0' && r <= '9') || r == '_'
}

func (b *Buffer) MoveUp() {
	if b.line > 0 {
		b.line--
		b.clampCol()
	}
}

func (b *Buffer) MoveDown() {
	if b.line < len(b.lines)-1 {
		b.line++
		b.clampCol()
	}
}

func (b *Buffer) MovePage(delta, rows int) {
	if rows < 1 {
		rows = 1
	}
	b.line = clamp(b.line+delta*rows, 0, len(b.lines)-1)
	b.clampCol()
}

func (b *Buffer) Home() { b.col = 0 }

func (b *Buffer) End() { b.col = len([]rune(b.lines[b.line])) }

func (b *Buffer) MoveTo(line, col int) {
	b.line = clamp(line, 0, len(b.lines)-1)
	b.col = clamp(col, 0, len([]rune(b.lines[b.line])))
}

func (b *Buffer) Find(query string, fromLine, fromCol int) (int, int, bool) {
	if query == "" {
		return 0, 0, false
	}
	q := strings.ToLower(query)
	for offset := 0; offset < len(b.lines); offset++ {
		line := (fromLine + offset) % len(b.lines)
		runes := []rune(b.lines[line])
		start := 0
		if offset == 0 {
			start = clamp(fromCol, 0, len(runes))
		}
		segment := string(runes[start:])
		if idx := strings.Index(strings.ToLower(segment), q); idx >= 0 {
			return line, start + len([]rune(segment[:idx])), true
		}
	}
	return 0, 0, false
}

// FindPrev searches backwards from (fromLine, fromCol), wrapping at the top.
func (b *Buffer) FindPrev(query string, fromLine, fromCol int) (int, int, bool) {
	if query == "" {
		return 0, 0, false
	}
	q := strings.ToLower(query)
	for offset := 0; offset < len(b.lines); offset++ {
		line := (fromLine - offset + len(b.lines)*2) % len(b.lines)
		runes := []rune(b.lines[line])
		end := len(runes)
		if offset == 0 {
			end = clamp(fromCol, 0, len(runes))
		}
		segment := string(runes[:end])
		if idx := strings.LastIndex(strings.ToLower(segment), q); idx >= 0 {
			return line, len([]rune(segment[:idx])), true
		}
	}
	return 0, 0, false
}

func (b *Buffer) Undo() {
	if len(b.undo) == 0 {
		return
	}
	prev := b.undo[len(b.undo)-1]
	b.undo = b.undo[:len(b.undo)-1]
	b.redo = append(b.redo, b.snapshot())
	b.lines, b.line, b.col = append([]string(nil), prev.lines...), prev.line, prev.col
	b.dirty = true
}

func (b *Buffer) Redo() {
	if len(b.redo) == 0 {
		return
	}
	next := b.redo[len(b.redo)-1]
	b.redo = b.redo[:len(b.redo)-1]
	b.undo = append(b.undo, b.snapshot())
	b.lines, b.line, b.col = append([]string(nil), next.lines...), next.line, next.col
	b.dirty = true
}

// Save writes atomically, preserving the existing file mode.
func (b *Buffer) Save() error {
	if strings.TrimSpace(b.path) == "" {
		return ErrNoPath
	}
	if err := os.MkdirAll(filepath.Dir(b.path), 0o755); err != nil {
		return err
	}
	mode := os.FileMode(0o644)
	if info, err := os.Stat(b.path); err == nil {
		mode = info.Mode().Perm()
	}
	tmp, err := os.CreateTemp(filepath.Dir(b.path), "."+filepath.Base(b.path)+".tmp-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if _, err := tmp.WriteString(b.Text()); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmpName, mode); err != nil {
		return err
	}
	if err := os.Rename(tmpName, b.path); err != nil {
		return err
	}
	b.dirty = false
	return nil
}

func (b *Buffer) record() {
	b.undo = append(b.undo, b.snapshot())
	if len(b.undo) > 200 {
		b.undo = b.undo[len(b.undo)-200:]
	}
	b.redo = nil
}

func (b *Buffer) snapshot() state {
	return state{
		lines: append([]string(nil), b.lines...),
		line:  b.line,
		col:   b.col,
	}
}

func (b *Buffer) clampCol() {
	b.col = clamp(b.col, 0, len([]rune(b.lines[b.line])))
}

func splitLines(text string) []string {
	text = normalize(text)
	if text == "" {
		return []string{""}
	}
	return strings.Split(text, "\n")
}

func normalize(text string) string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	return strings.ReplaceAll(text, "\r", "\n")
}

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
