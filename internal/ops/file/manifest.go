package file

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/solomonadzape95/workbench/internal/core"
	"github.com/solomonadzape95/workbench/internal/engines"
)

var manifestSpec = core.Spec{
	ID:      "file.manifest",
	Title:   "File manifest",
	Summary: "Write a CSV or JSON inventory of the selected files",
	Domain:  core.File,
	Accepts: nil, // any file kind
	Params: []core.ParamSpec{
		{
			Key: "format", Label: "Format", Type: core.ParamEnum, Default: "json",
			Options: []core.Option{{Value: "json", Label: "JSON"}, {Value: "csv", Label: "CSV"}},
		},
		{
			Key: "hash", Label: "Include SHA-256", Type: core.ParamBool, Default: false,
			Help: "Hash every file while building the manifest. Slower on large sets.",
		},
	},
	Output:  core.OutputSpec{FormatParam: "format", Kind: core.File},
	Engines: []core.EngineID{engines.Internal},
	Mode:    core.ModeAggregate,
	Cost:    core.CostMedium,
}

type Manifest struct{}

func (Manifest) Spec() core.Spec { return manifestSpec }

func (Manifest) Validate(ctx context.Context, req core.Request) error {
	if err := core.Validate(manifestSpec.Params, req.Params); err != nil {
		return err
	}
	if len(req.Items) == 0 {
		return core.NewError(core.CodeUsage, manifestSpec.ID, "", "no files to inventory", "")
	}
	return nil
}

type manifestEntry struct {
	Path   string `json:"path"`
	Name   string `json:"name"`
	Size   int64  `json:"size"`
	Kind   string `json:"kind"`
	Format string `json:"format,omitempty"`
	SHA256 string `json:"sha256,omitempty"`
}

func (Manifest) Run(ctx context.Context, req core.Request, rep core.Reporter) (core.Result, error) {
	start := time.Now()
	format := req.Params.String("format")
	if format == "" {
		format = "json"
	}
	withHash := req.Params.Bool("hash")

	entries := make([]manifestEntry, 0, len(req.Items))
	var bytesIn int64
	rep.Stage("inventorying files")
	for i, item := range req.Items {
		if err := ctx.Err(); err != nil {
			return core.Result{}, err
		}
		entry := manifestEntry{
			Path: item.Path, Name: item.Name(), Size: item.Size,
			Kind: string(item.Kind), Format: item.Format,
		}
		if withHash {
			digest, err := hashFile(ctx, item.Path, "sha256")
			if err != nil {
				return core.Result{}, core.Wrap(core.CodeInput, manifestSpec.ID, item.Path, err,
					"check that the file is readable")
			}
			entry.SHA256 = digest
		}
		entries = append(entries, entry)
		bytesIn += item.Size
		rep.Progress(i+1, len(req.Items))
	}

	name := core.SanitizeStem(req.Items[0].Stem()) + "-manifest." + format
	output := filepath.Join(req.Target.Dir, name)
	var err error
	switch format {
	case "csv":
		err = writeManifestCSV(output, entries)
	case "json":
		err = writeManifestJSON(output, entries)
	default:
		return core.Result{}, core.NewError(core.CodeUsage, manifestSpec.ID, "",
			fmt.Sprintf("unsupported manifest format %q", format), "choose json or csv")
	}
	if err != nil {
		return core.Result{}, core.Wrap(core.CodeOutput, manifestSpec.ID, output, err,
			"check permissions on the output directory")
	}
	info, err := os.Stat(output)
	if err != nil {
		return core.Result{}, core.Wrap(core.CodeOutput, manifestSpec.ID, output, err, "")
	}
	artifact := core.Artifact{
		Path: output, Kind: core.File, Format: format, Size: info.Size(),
		SuggestedName: name,
		Meta: map[string]any{
			"files":    len(entries),
			"hashed":   withHash,
			"manifest": true,
		},
	}
	return core.Result{
		Artifacts: []core.Artifact{artifact},
		Metrics: core.Metrics{
			BytesIn: bytesIn, BytesOut: info.Size(), Duration: time.Since(start), Engine: engines.Internal,
		},
	}, nil
}

func writeManifestJSON(path string, entries []manifestEntry) error {
	data, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0o644)
}

func writeManifestCSV(path string, entries []manifestEntry) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	w := csv.NewWriter(f)
	header := []string{"path", "name", "size", "kind", "format", "sha256"}
	if err := w.Write(header); err != nil {
		f.Close()
		return err
	}
	for _, entry := range entries {
		row := []string{entry.Path, entry.Name, strconv.FormatInt(entry.Size, 10), entry.Kind, entry.Format, entry.SHA256}
		if err := w.Write(row); err != nil {
			f.Close()
			return err
		}
	}
	w.Flush()
	flushErr := w.Error()
	closeErr := f.Close()
	if flushErr != nil {
		return flushErr
	}
	return closeErr
}
