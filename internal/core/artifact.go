// Package core contains the contracts every part of Workbench shares:
// artifacts, operations, parameters, pipelines, targets, events, and errors.
// It depends only on the standard library.
package core

import (
	"path/filepath"
	"strings"
)

// Kind is the broad class of a file.
type Kind string

const (
	Image    Kind = "image"
	Audio    Kind = "audio"
	Video    Kind = "video"
	Document Kind = "document"
	Archive  Kind = "archive"
	File     Kind = "file"
	Unknown  Kind = "unknown"
)

// Artifact is one concrete file flowing through a job.
type Artifact struct {
	Path          string
	Kind          Kind
	Format        string // "jpeg", "mp3", "pdf"; never includes the dot
	Size          int64
	Label         string         // friendly display name; URLs use this
	SuggestedName string         // output filename hint for the runner's commit step
	Meta          map[string]any // width, height, duration_ms, pages, codec…
}

// Name returns the artifact's friendly name. URLs and synthetic inputs carry a
// Label so the UI does not print a full query string as a filename.
func (a Artifact) Name() string {
	if a.Label != "" {
		return a.Label
	}
	return filepath.Base(a.Path)
}

// Stem returns the base name without its final extension.
func (a Artifact) Stem() string {
	name := a.Name()
	return strings.TrimSuffix(name, filepath.Ext(name))
}

// Ext returns the extension without a dot, preferring Format when present.
func (a Artifact) Ext() string {
	if a.Format != "" {
		return strings.TrimPrefix(strings.ToLower(a.Format), ".")
	}
	return strings.TrimPrefix(strings.ToLower(filepath.Ext(a.Path)), ".")
}

// WithPath returns a copy of the artifact pointing at a new file.
func (a Artifact) WithPath(path string) Artifact {
	a.Path = path
	return a
}
