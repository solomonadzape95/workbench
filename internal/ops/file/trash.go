package file

import (
	"context"
	"os"
	"path/filepath"
	"time"

	"github.com/solomonadzape95/workbench/internal/config"
	"github.com/solomonadzape95/workbench/internal/core"
	"github.com/solomonadzape95/workbench/internal/engines"
)

var trashSpec = core.Spec{
	ID:      "file.trash",
	Title:   "Move to trash",
	Summary: "Safely delete files by moving them into a recoverable trash folder",
	Domain:  core.File,
	Accepts: nil,
	Params: []core.ParamSpec{
		{
			Key: "trash_dir", Label: "Trash folder", Type: core.ParamPath,
			Help: "Defaults to the Workbench data directory's trash folder.",
		},
	},
	Output:  core.OutputSpec{Kind: core.File},
	Engines: []core.EngineID{engines.Internal},
	Mode:    core.ModeDirect,
	Cost:    core.CostFast,
}

type Trash struct{}

func (Trash) Spec() core.Spec { return trashSpec }

func (Trash) Validate(ctx context.Context, req core.Request) error {
	if err := core.Validate(trashSpec.Params, req.Params); err != nil {
		return err
	}
	if len(req.Items) == 0 {
		return core.NewError(core.CodeUsage, trashSpec.ID, "", "no files to trash", "")
	}
	return nil
}

func (Trash) Run(ctx context.Context, req core.Request, rep core.Reporter) (core.Result, error) {
	start := time.Now()
	root := config.ExpandHome(req.Params.String("trash_dir"))
	if root == "" {
		root = filepath.Join(config.DefaultPaths().DataDir, "trash")
	}
	batch := filepath.Join(root, time.Now().Format("20060102-150405"))
	if err := os.MkdirAll(batch, 0o755); err != nil {
		return core.Result{}, core.Wrap(core.CodeOutput, trashSpec.ID, batch, err,
			"check permissions on the trash folder")
	}

	var out []core.Artifact
	var bytesIn, bytesOut int64
	for i, item := range req.Items {
		if err := ctx.Err(); err != nil {
			return core.Result{}, err
		}
		proposed := filepath.Join(batch, core.SanitizeFilename(item.Name()))
		final, err := core.UniquePath(proposed, core.CollisionSuffix)
		if err != nil {
			return core.Result{}, core.Wrap(core.CodeOutput, trashSpec.ID, proposed, err, "")
		}
		if sameFilePath(item.Path, final) {
			return core.Result{}, core.NewError(core.CodeOutput, trashSpec.ID, final,
				"source is already in the trash folder", "choose a different trash folder")
		}
		rep.Stage("trashing " + item.Name())
		if err := movePath(item.Path, final); err != nil {
			return core.Result{}, core.Wrap(core.CodeOutput, trashSpec.ID, item.Path, err,
				"check permissions on the source and trash folders")
		}
		info, err := os.Stat(final)
		if err != nil {
			return core.Result{}, core.Wrap(core.CodeOutput, trashSpec.ID, final, err, "")
		}
		out = append(out, core.Artifact{
			Path: final, Kind: item.Kind, Format: item.Format, Size: info.Size(),
			SuggestedName: filepath.Base(final),
			Meta: map[string]any{
				"source":  item.Path,
				"trashed": true,
			},
		})
		bytesIn += item.Size
		bytesOut += info.Size()
		rep.Progress(i+1, len(req.Items))
	}
	return core.Result{
		Artifacts: out,
		Metrics: core.Metrics{
			BytesIn: bytesIn, BytesOut: bytesOut, Duration: time.Since(start), Engine: engines.Internal,
		},
	}, nil
}
