// Package engines discovers and describes the tools Workbench can delegate
// work to. An engine is either built in (internal) or an external binary.
package engines

import (
	"context"
	"sync"

	"github.com/solomonadzape95/workbench/internal/core"
)

// ID is the stable identifier used in config and pipelines. It aliases the
// core type so operations and engines agree without a dependency cycle.
type ID = core.EngineID

const (
	Internal    ID = "internal"
	FFmpeg      ID = "ffmpeg"
	ImageMagick ID = "imagemagick"
	LibreOffice ID = "libreoffice"
	Pandoc      ID = "pandoc"
	Tesseract   ID = "tesseract"
	Ghostscript ID = "ghostscript"
	PDF2DOCX    ID = "pdf2docx"
	Poppler     ID = "poppler"
	Rembg       ID = "rembg"
	Demucs      ID = "demucs"
	Whisper     ID = "whisper"
	RealESRGAN  ID = "realesrgan"
	YtDlp       ID = "ytdlp"
	OpenCode    ID = "opencode"
	SevenZip    ID = "7z"
)

// Info is a snapshot of an engine's availability. It is safe to copy and to
// send across goroutines, which is why the probe error is kept as a value.
type Info struct {
	ID        ID
	Name      string
	Path      string
	Version   string
	Available bool
	Detail    string // extra context, e.g. "ffprobe: /usr/bin/ffprobe"
	Hint      string // actionable install hint when unavailable
	Err       error
}

// Engine probes itself. Probe must respect ctx and should not take longer
// than a few seconds; it runs concurrently with other engines.
type Engine interface {
	ID() ID
	Probe(ctx context.Context) Info
}

// Registry holds the engines enabled for this run.
type Registry struct {
	engines []Engine
}

func NewRegistry(list ...Engine) *Registry {
	return &Registry{engines: list}
}

func (r *Registry) Len() int {
	if r == nil {
		return 0
	}
	return len(r.engines)
}

// Engines returns the registered engines in display order.
func (r *Registry) Engines() []Engine {
	if r == nil {
		return nil
	}
	return r.engines
}

// ProbeAll probes every engine concurrently and returns results in
// registration order, so the UI is stable regardless of completion order.
func (r *Registry) ProbeAll(ctx context.Context) []Info {
	if r == nil || len(r.engines) == 0 {
		return nil
	}

	infos := make([]Info, len(r.engines))
	var wg sync.WaitGroup
	for i, e := range r.engines {
		wg.Add(1)
		go func() {
			defer wg.Done()
			infos[i] = e.Probe(ctx)
		}()
	}
	wg.Wait()
	return infos
}
