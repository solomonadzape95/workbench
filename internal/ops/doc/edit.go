package doc

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/solomonadzape95/workbench/internal/core"
	"github.com/solomonadzape95/workbench/internal/engines"
)

var editSpec = core.Spec{
	ID:      "doc.edit",
	Title:   "Edit document",
	Summary: "Open a text or Markdown file in $EDITOR and save the edited result",
	Domain:  core.Document,
	Accepts: []core.Accept{{Kind: core.Document, Formats: []string{"txt", "md", "markdown"}}},
	Params: []core.ParamSpec{
		{Key: "editor", Label: "Editor command", Type: core.ParamString,
			Help: "Defaults to $VISUAL, then $EDITOR, then vi. Best used from `wb run`."},
		{Key: "format", Label: "Output", Type: core.ParamEnum, Default: "auto",
			Options: []core.Option{
				{Value: "auto", Label: "Same as input"},
				{Value: "txt", Label: "Plain text"},
				{Value: "md", Label: "Markdown"},
			}},
	},
	Output:    core.OutputSpec{Kind: core.Document},
	Engines:   []core.EngineID{engines.Internal},
	Batchable: true,
	Cost:      core.CostFast,
}

type Edit struct{}

func (Edit) Spec() core.Spec { return editSpec }

func (Edit) Validate(ctx context.Context, req core.Request) error {
	if err := core.Validate(editSpec.Params, req.Params); err != nil {
		return err
	}
	if len(req.Items) == 0 {
		return core.NewError(core.CodeUsage, editSpec.ID, "", "no input files", "")
	}
	editor := resolveEditor(req.Params.String("editor"))
	if editor == "" {
		return core.NewError(core.CodeUsage, editSpec.ID, "",
			"no editor configured", "set $EDITOR or pass an editor command")
	}
	for _, item := range req.Items {
		if !editSpec.AcceptsArtifact(item) {
			return core.NewError(core.CodeInput, editSpec.ID, item.Path,
				fmt.Sprintf("%s is not a text document", item.Format),
				"supported inputs: .txt, .md, .markdown; convert DOCX or PDF to Markdown first")
		}
	}
	return nil
}

func (Edit) Run(ctx context.Context, req core.Request, rep core.Reporter) (core.Result, error) {
	start := time.Now()
	editor := resolveEditor(req.Params.String("editor"))
	if editor == "" {
		return core.Result{}, core.NewError(core.CodeUsage, editSpec.ID, "", "no editor configured", "")
	}
	format := strings.ToLower(req.Params.String("format"))
	var out []core.Artifact
	var bytesIn, bytesOut int64
	for _, item := range req.Items {
		if err := ctx.Err(); err != nil {
			return core.Result{}, err
		}
		ext := outputExtForEdit(item, format)
		temp, err := os.CreateTemp("", "workbench-edit-*."+ext)
		if err != nil {
			return core.Result{}, core.Wrap(core.CodeOutput, editSpec.ID, "", err, "")
		}
		tempPath := temp.Name()
		data, readErr := os.ReadFile(item.Path)
		if readErr == nil {
			_, readErr = temp.Write(data)
		}
		closeErr := temp.Close()
		if readErr != nil {
			_ = os.Remove(tempPath)
			return core.Result{}, core.Wrap(core.CodeInput, editSpec.ID, item.Path, readErr, "")
		}
		if closeErr != nil {
			_ = os.Remove(tempPath)
			return core.Result{}, core.Wrap(core.CodeOutput, editSpec.ID, tempPath, closeErr, "")
		}
		rep.Stage("opening editor for " + item.Name())
		if err := runEditor(ctx, editor, tempPath); err != nil {
			_ = os.Remove(tempPath)
			return core.Result{}, core.Wrap(core.CodeEngineFailed, editSpec.ID, item.Path, err,
				"run the operation from a terminal so the editor can attach to stdin/stdout")
		}
		edited, err := os.ReadFile(tempPath)
		_ = os.Remove(tempPath)
		if err != nil {
			return core.Result{}, core.Wrap(core.CodeOutput, editSpec.ID, tempPath, err, "")
		}
		if strings.TrimSpace(string(edited)) == "" {
			return core.Result{}, core.NewError(core.CodeOutput, editSpec.ID, item.Path,
				"the editor left the file empty", "cancel without saving to keep the original")
		}
		output := filepath.Join(req.Target.Dir, core.SanitizeStem(item.Stem())+".edited."+ext)
		if err := os.MkdirAll(req.Target.Dir, 0o755); err != nil {
			return core.Result{}, core.Wrap(core.CodeOutput, editSpec.ID, req.Target.Dir, err, "")
		}
		if err := os.WriteFile(output, edited, 0o644); err != nil {
			return core.Result{}, core.Wrap(core.CodeOutput, editSpec.ID, output, err, "")
		}
		info, err := os.Stat(output)
		if err != nil {
			return core.Result{}, core.Wrap(core.CodeOutput, editSpec.ID, output, err, "")
		}
		out = append(out, core.Artifact{
			Path: output, Kind: core.Document, Format: ext, Size: info.Size(),
			SuggestedName: filepath.Base(output),
			Meta:          map[string]any{"source": item.Path, "editor": editor},
		})
		bytesIn += item.Size
		bytesOut += info.Size()
		rep.Progress(1, 1)
	}
	return core.Result{Artifacts: out, Metrics: core.Metrics{
		BytesIn: bytesIn, BytesOut: bytesOut, Duration: time.Since(start), Engine: engines.Internal,
	}}, nil
}

func resolveEditor(configured string) string {
	if value := strings.TrimSpace(configured); value != "" {
		return value
	}
	for _, key := range []string{"VISUAL", "EDITOR"} {
		if value := strings.TrimSpace(os.Getenv(key)); value != "" {
			return value
		}
	}
	if runtime.GOOS == "windows" {
		return "notepad"
	}
	return "vi"
}

func outputExtForEdit(item core.Artifact, format string) string {
	switch format {
	case "txt":
		return "txt"
	case "md", "markdown":
		return "md"
	}
	if item.Ext() == "markdown" {
		return "md"
	}
	if item.Ext() == "" {
		return "txt"
	}
	return item.Ext()
}

func runEditor(ctx context.Context, editor, path string) error {
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.CommandContext(ctx, "cmd", "/C", editor+" "+quoteArg(path))
	} else {
		cmd = exec.CommandContext(ctx, "sh", "-c", editor+" "+quoteArg(path))
	}
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func quoteArg(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}
