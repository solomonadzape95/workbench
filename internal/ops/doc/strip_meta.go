package doc

import (
	"archive/zip"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/solomonadzape95/workbench/internal/core"
	"github.com/solomonadzape95/workbench/internal/engines"
)

var stripMetaSpec = core.Spec{
	ID:        "doc.strip_meta",
	Title:     "Strip document metadata",
	Summary:   "Remove author, title, and tracking properties from DOCX or ODT",
	Domain:    core.Document,
	Accepts:   []core.Accept{{Kind: core.Document, Formats: []string{"docx", "odt"}}},
	Params:    nil,
	Output:    core.OutputSpec{StaticFormat: "", Kind: core.Document},
	Engines:   []core.EngineID{engines.Internal},
	Batchable: true,
	Cost:      core.CostFast,
}

type StripMeta struct{}

func (StripMeta) Spec() core.Spec { return stripMetaSpec }

func (StripMeta) Validate(ctx context.Context, req core.Request) error {
	if len(req.Items) == 0 {
		return core.NewError(core.CodeUsage, stripMetaSpec.ID, "", "no documents", "")
	}
	for _, item := range req.Items {
		if !stripMetaSpec.AcceptsArtifact(item) {
			return core.NewError(core.CodeInput, stripMetaSpec.ID, item.Path,
				"only DOCX and ODT files can be stripped", "choose a DOCX or ODT")
		}
	}
	return nil
}

func (StripMeta) Run(ctx context.Context, req core.Request, rep core.Reporter) (core.Result, error) {
	start := time.Now()
	var out []core.Artifact
	var bytesIn, bytesOut int64
	for _, item := range req.Items {
		if err := ctx.Err(); err != nil {
			return core.Result{}, err
		}
		ext := item.Ext()
		output := filepath.Join(req.Target.Dir, core.SanitizeStem(item.Stem())+"."+ext)
		rep.Stage("stripping metadata")
		if err := stripZipMetadata(ctx, item.Path, output); err != nil {
			return core.Result{}, core.Wrap(core.CodeOutput, stripMetaSpec.ID, item.Path, err,
				"check that the document is a valid DOCX or ODT")
		}
		info, err := os.Stat(output)
		if err != nil {
			return core.Result{}, core.Wrap(core.CodeOutput, stripMetaSpec.ID, output, err, "")
		}
		out = append(out, core.Artifact{
			Path: output, Kind: core.Document, Format: ext, Size: info.Size(),
			SuggestedName: filepath.Base(output),
			Meta: map[string]any{
				"source": item.Path, "metadata_stripped": true,
			},
		})
		bytesIn += item.Size
		bytesOut += info.Size()
		rep.Progress(1, 1)
	}
	return core.Result{
		Artifacts: out,
		Metrics: core.Metrics{
			BytesIn: bytesIn, BytesOut: bytesOut, Duration: time.Since(start), Engine: engines.Internal,
		},
	}, nil
}

func stripZipMetadata(ctx context.Context, input, output string) error {
	r, err := zip.OpenReader(input)
	if err != nil {
		return err
	}
	defer r.Close()
	if err := os.MkdirAll(filepath.Dir(output), 0o755); err != nil {
		return err
	}
	f, err := os.Create(output)
	if err != nil {
		return err
	}
	zw := zip.NewWriter(f)
	var copyErr error
	for _, entry := range r.File {
		if err := ctx.Err(); err != nil {
			copyErr = err
			break
		}
		if dropMetadata(entry.Name) {
			continue
		}
		header := entry.FileHeader
		header.Name = entry.Name
		w, err := zw.CreateHeader(&header)
		if err != nil {
			copyErr = err
			break
		}
		src, err := entry.Open()
		if err != nil {
			copyErr = err
			break
		}
		_, err = io.Copy(w, src)
		closeErr := src.Close()
		if err != nil {
			copyErr = err
			break
		}
		if closeErr != nil {
			copyErr = closeErr
			break
		}
	}
	closeZipErr := zw.Close()
	closeFileErr := f.Close()
	if copyErr != nil {
		_ = os.Remove(output)
		return copyErr
	}
	if closeZipErr != nil {
		_ = os.Remove(output)
		return closeZipErr
	}
	return closeFileErr
}

func dropMetadata(name string) bool {
	clean := strings.ToLower(strings.TrimPrefix(name, "/"))
	switch clean {
	case "docprops/core.xml", "docprops/app.xml", "docprops/custom.xml",
		"docprops/thumbnail.jpeg", "docprops/thumbnail.jpg",
		"meta.xml", "mimetype":
		return clean != "mimetype" // ODT needs mimetype as the first entry
	}
	return false
}
