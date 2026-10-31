// Package pipeline persists named pipelines as human-editable TOML files.
package pipeline

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/pelletier/go-toml/v2"

	"github.com/solomonadzape95/workbench/internal/core"
)

type Store struct {
	Dir string
}

type Meta struct {
	Name        string
	Description string
	Steps       int
	Err         error
}

func NewStore(dir string) *Store {
	return &Store{Dir: dir}
}

func (s *Store) Path(name string) string {
	return filepath.Join(s.Dir, name+".toml")
}

// List reads pipeline metadata. A malformed file appears as a Meta with Err so
// the palette can show it without losing the rest of the library.
func (s *Store) List() ([]Meta, error) {
	if s == nil || s.Dir == "" {
		return nil, nil
	}
	entries, err := os.ReadDir(s.Dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	var out []Meta
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".toml") {
			continue
		}
		name := strings.TrimSuffix(entry.Name(), ".toml")
		p, err := s.Load(name)
		meta := Meta{Name: name, Steps: len(p.Steps), Description: p.Description}
		if err != nil {
			meta.Err = err
		}
		out = append(out, meta)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

func (s *Store) Load(name string) (core.Pipeline, error) {
	if !ValidName(name) {
		return core.Pipeline{}, fmt.Errorf("invalid pipeline name %q", name)
	}
	data, err := os.ReadFile(s.Path(name))
	if err != nil {
		return core.Pipeline{}, err
	}
	var p core.Pipeline
	if err := toml.Unmarshal(data, &p); err != nil {
		return core.Pipeline{}, fmt.Errorf("parse %s: %w", name, err)
	}
	if p.Name == "" {
		p.Name = name
	}
	if p.Version == 0 {
		p.Version = 1
	}
	if p.Version > core.PipelineVersion {
		return core.Pipeline{}, fmt.Errorf("pipeline %q needs a newer Workbench (version %d)", name, p.Version)
	}
	if len(p.Steps) == 0 {
		return core.Pipeline{}, fmt.Errorf("pipeline %q has no steps", name)
	}
	return p, nil
}

// Save writes atomically so a crash cannot leave a half-written recipe.
func (s *Store) Save(p core.Pipeline) error {
	if p.Version <= 0 {
		p.Version = core.PipelineVersion
	}
	if err := Validate(p); err != nil {
		return err
	}
	if err := os.MkdirAll(s.Dir, 0o755); err != nil {
		return err
	}
	data, err := toml.Marshal(p)
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(s.Dir, ".tmp-*")
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
	if err := os.Chmod(tmpName, 0o644); err != nil {
		return err
	}
	return os.Rename(tmpName, s.Path(p.Name))
}

// Delete is a soft delete: recipes move to .trash so a stray keypress is
// recoverable.
func (s *Store) Delete(name string) error {
	if !ValidName(name) {
		return fmt.Errorf("invalid pipeline name %q", name)
	}
	trash := filepath.Join(s.Dir, ".trash")
	if err := os.MkdirAll(trash, 0o755); err != nil {
		return err
	}
	dst := filepath.Join(trash, fmt.Sprintf("%d-%s.toml", time.Now().UnixNano(), name))
	return os.Rename(s.Path(name), dst)
}

func Validate(p core.Pipeline) error {
	if !ValidName(p.Name) {
		return fmt.Errorf("name %q is invalid: use lowercase letters, digits, - and _", p.Name)
	}
	if p.Version <= 0 {
		p.Version = 1
	}
	if p.Version > core.PipelineVersion {
		return fmt.Errorf("version %d is newer than this build supports", p.Version)
	}
	if len(p.Steps) == 0 {
		return errors.New("a pipeline needs at least one step")
	}
	for i, step := range p.Steps {
		if strings.TrimSpace(step.Op) == "" {
			return fmt.Errorf("step %d has no operation", i+1)
		}
	}
	return nil
}

func ValidName(name string) bool {
	if name == "" || len(name) > 64 {
		return false
	}
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z':
		case r >= '0' && r <= '9':
		case r == '-' || r == '_':
		default:
			return false
		}
	}
	return true
}

// Slug turns a human title into a valid pipeline name.
func Slug(text string) string {
	var b strings.Builder
	lastDash := false
	for _, r := range strings.ToLower(text) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			lastDash = false
		default:
			if !lastDash && b.Len() > 0 {
				b.WriteByte('-')
				lastDash = true
			}
		}
	}
	name := strings.Trim(b.String(), "-")
	if len(name) > 64 {
		name = strings.Trim(name[:64], "-")
	}
	if name == "" {
		name = "pipeline"
	}
	return name
}
