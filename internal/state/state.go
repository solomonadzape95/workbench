// Package state persists the small amount of TUI session data that makes a
// restart feel continuous: the last input/output, the selected job, and one
// unsaved compose draft. It is intentionally separate from history: history is
// an event log, this is a replaceable snapshot.
package state

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/solomonadzape95/workbench/internal/core"
)

const Version = 1

type Draft struct {
	Pipeline  core.Pipeline `json:"pipeline"`
	Input     string        `json:"input,omitempty"`
	Output    string        `json:"output,omitempty"`
	SavedName string        `json:"saved_name,omitempty"`
	Selected  int           `json:"selected,omitempty"`
}

type State struct {
	Version     int    `json:"version"`
	LastInput   string `json:"last_input,omitempty"`
	LastOutput  string `json:"last_output,omitempty"`
	SelectedJob string `json:"selected_job,omitempty"`
	Draft       *Draft `json:"draft,omitempty"`
}

type Store struct {
	Path string
}

func New(dir string) *Store {
	if dir == "" {
		return &Store{}
	}
	return &Store{Path: filepath.Join(dir, "state.json")}
}

func (s *Store) Load() (State, error) {
	if s == nil || s.Path == "" {
		return State{}, nil
	}
	data, err := os.ReadFile(s.Path)
	if errors.Is(err, fs.ErrNotExist) {
		return State{}, nil
	}
	if err != nil {
		return State{}, err
	}
	var st State
	if err := json.Unmarshal(data, &st); err != nil {
		return State{}, fmt.Errorf("parse %s: %w", s.Path, err)
	}
	return st, nil
}

func (s *Store) Save(st State) error {
	if s == nil || s.Path == "" {
		return nil
	}
	if st.Version <= 0 {
		st.Version = Version
	}
	data, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	dir := filepath.Dir(s.Path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".state-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, s.Path)
}

func (s *Store) Clear() error {
	if s == nil || s.Path == "" {
		return nil
	}
	if err := os.Remove(s.Path); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return nil
}
