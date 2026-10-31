package editor

import (
	"os"
	"path/filepath"
	"testing"
)

func TestInsertNewlineBackspaceAndUndo(t *testing.T) {
	b := New("hello\nworld", "note.txt")
	b.MoveTo(0, 5)
	b.Newline()
	b.Insert("there")
	if got := b.Text(); got != "hello\nthere\nworld" {
		t.Fatalf("text = %q", got)
	}
	if line, col := b.Cursor(); line != 1 || col != 5 {
		t.Errorf("cursor = %d:%d, want 1:5", line, col)
	}
	b.Backspace()
	if got := b.Text(); got != "hello\nther\nworld" {
		t.Fatalf("after backspace = %q", got)
	}
	b.Undo()
	if got := b.Text(); got != "hello\nthere\nworld" {
		t.Fatalf("after undo = %q", got)
	}
	b.Redo()
	if got := b.Text(); got != "hello\nther\nworld" {
		t.Fatalf("after redo = %q", got)
	}
}

func TestBackspaceJoinsLines(t *testing.T) {
	b := New("one\ntwo", "")
	b.MoveTo(1, 0)
	b.Backspace()
	if got := b.Text(); got != "onetwo" {
		t.Errorf("text = %q", got)
	}
	if line, col := b.Cursor(); line != 0 || col != 3 {
		t.Errorf("cursor = %d:%d, want 0:3", line, col)
	}
}

func TestSaveIsAtomicAndReloadable(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "cfg.toml")
	if err := os.WriteFile(path, []byte("a = 1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	b, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	b.MoveTo(0, 5)
	b.Insert("2")
	if err := b.Save(); err != nil {
		t.Fatal(err)
	}
	if b.Dirty() {
		t.Error("buffer still dirty after save")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "a = 12\n" {
		t.Errorf("saved = %q", data)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("mode = %o, want 600", info.Mode().Perm())
	}
	reloaded, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.Text() != "a = 12\n" {
		t.Errorf("reloaded = %q", reloaded.Text())
	}
}

func TestFind(t *testing.T) {
	b := New("alpha beta\nGamma beta", "")
	line, col, ok := b.Find("beta", 0, 0)
	if !ok || line != 0 || col != 6 {
		t.Errorf("first find = %d:%d:%v", line, col, ok)
	}
	line, col, ok = b.Find("beta", line, col+1)
	if !ok || line != 1 || col != 6 {
		t.Errorf("second find = %d:%d:%v", line, col, ok)
	}
}

func TestWordMovementAndDeletion(t *testing.T) {
	b := New("alpha beta gamma", "")
	b.MoveTo(0, len([]rune("alpha beta gamma")))
	b.MoveWordLeft()
	if _, col := b.Cursor(); col != len([]rune("alpha beta ")) {
		t.Fatalf("MoveWordLeft col = %d", col)
	}
	b.DeleteWordBackward()
	if got := b.Line(0); got != "alpha gamma" {
		t.Fatalf("DeleteWordBackward = %q", got)
	}
	b.MoveWordRight()
	if _, col := b.Cursor(); col != len([]rune("alpha gamma")) {
		t.Fatalf("MoveWordRight col = %d", col)
	}
	b.MoveTo(0, len([]rune("alpha gamma")))
	b.DeleteToLineStart()
	if got := b.Line(0); got != "" {
		t.Fatalf("DeleteToLineStart = %q", got)
	}
}

func TestFindPrevWraps(t *testing.T) {
	b := New("one two one two", "")
	line, col, ok := b.FindPrev("one", 0, 11)
	if !ok || line != 0 || col != 8 {
		t.Fatalf("FindPrev = %d,%d,%v", line, col, ok)
	}
}
