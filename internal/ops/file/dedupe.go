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

var dedupeSpec = core.Spec{
	ID:      "file.dedupe",
	Title:   "Find duplicates",
	Summary: "Hash files and report duplicate groups",
	Domain:  core.File,
	Accepts: nil,
	Params: []core.ParamSpec{
		{
			Key: "format", Label: "Format", Type: core.ParamEnum, Default: "json",
			Options: []core.Option{{Value: "json", Label: "JSON"}, {Value: "csv", Label: "CSV"}},
		},
	},
	Output:  core.OutputSpec{FormatParam: "format", Kind: core.File},
	Engines: []core.EngineID{engines.Internal},
	Mode:    core.ModeAggregate,
	Cost:    core.CostMedium,
}

type Dedupe struct{}

func (Dedupe) Spec() core.Spec { return dedupeSpec }

func (Dedupe) Validate(ctx context.Context, req core.Request) error {
	if err := core.Validate(dedupeSpec.Params, req.Params); err != nil {
		return err
	}
	if len(req.Items) == 0 {
		return core.NewError(core.CodeUsage, dedupeSpec.ID, "", "no files to compare", "")
	}
	return nil
}

type duplicateGroup struct {
	SHA256 string   `json:"sha256"`
	Size   int64    `json:"size"`
	Files  []string `json:"files"`
}

type duplicateReport struct {
	Groups         []duplicateGroup `json:"groups"`
	DuplicateFiles int              `json:"duplicate_files"`
	WastedBytes    int64            `json:"wasted_bytes"`
}

func (Dedupe) Run(ctx context.Context, req core.Request, rep core.Reporter) (core.Result, error) {
	start := time.Now()
	format := req.Params.String("format")
	if format == "" {
		format = "json"
	}
	groups := map[string]*duplicateGroup{}
	order := make([]string, 0)
	rep.Stage("hashing files")
	for i, item := range req.Items {
		if err := ctx.Err(); err != nil {
			return core.Result{}, err
		}
		digest, err := hashFile(ctx, item.Path, "sha256")
		if err != nil {
			return core.Result{}, core.Wrap(core.CodeInput, dedupeSpec.ID, item.Path, err, "")
		}
		group, ok := groups[digest]
		if !ok {
			group = &duplicateGroup{SHA256: digest, Size: item.Size}
			groups[digest] = group
			order = append(order, digest)
		}
		group.Files = append(group.Files, item.Path)
		rep.Progress(i+1, len(req.Items))
	}
	report := duplicateReport{}
	for _, digest := range order {
		group := groups[digest]
		if len(group.Files) < 2 {
			continue
		}
		report.Groups = append(report.Groups, *group)
		report.DuplicateFiles += len(group.Files) - 1
		report.WastedBytes += int64(len(group.Files)-1) * group.Size
	}

	name := core.SanitizeStem(req.Items[0].Stem()) + "-duplicates." + format
	output := filepath.Join(req.Target.Dir, name)
	var err error
	switch format {
	case "csv":
		err = writeDuplicatesCSV(output, report.Groups)
	case "json":
		data, marshalErr := json.MarshalIndent(report, "", "  ")
		if marshalErr != nil {
			return core.Result{}, marshalErr
		}
		data = append(data, '\n')
		err = os.WriteFile(output, data, 0o644)
	default:
		return core.Result{}, core.NewError(core.CodeUsage, dedupeSpec.ID, "",
			fmt.Sprintf("unsupported format %q", format), "choose json or csv")
	}
	if err != nil {
		return core.Result{}, core.Wrap(core.CodeOutput, dedupeSpec.ID, output, err, "")
	}
	info, err := os.Stat(output)
	if err != nil {
		return core.Result{}, core.Wrap(core.CodeOutput, dedupeSpec.ID, output, err, "")
	}
	var bytesIn int64
	for _, item := range req.Items {
		bytesIn += item.Size
	}
	artifact := core.Artifact{
		Path: output, Kind: core.File, Format: format, Size: info.Size(),
		SuggestedName: name,
		Meta: map[string]any{
			"groups":          len(report.Groups),
			"duplicate_files": report.DuplicateFiles,
			"wasted_bytes":    report.WastedBytes,
		},
	}
	return core.Result{
		Artifacts: []core.Artifact{artifact},
		Metrics: core.Metrics{
			BytesIn: bytesIn, BytesOut: info.Size(), Duration: time.Since(start), Engine: engines.Internal,
		},
	}, nil
}

func writeDuplicatesCSV(path string, groups []duplicateGroup) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	w := csv.NewWriter(f)
	if err := w.Write([]string{"sha256", "size", "path"}); err != nil {
		f.Close()
		return err
	}
	for _, group := range groups {
		for _, file := range group.Files {
			if err := w.Write([]string{group.SHA256, strconv.FormatInt(group.Size, 10), file}); err != nil {
				f.Close()
				return err
			}
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
