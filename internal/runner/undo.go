package runner

import (
	"errors"
	"os"
	"path/filepath"

	"github.com/solomonadzape95/workbench/internal/core"
)

// UndoResult reports what an undo changed.
type UndoResult struct {
	Files    int // outputs moved to the undo trash
	Restored int // overwritten files put back
}

// UndoJob moves a finished job's outputs into <undoDir>/trash/<jobID>/ and
// restores any overwritten files that the commit step backed up. Direct-mode
// operations do not produce commit backups, so their outputs are only moved to
// the trash.
func UndoJob(view JobView, undoDir string) (UndoResult, error) {
	if undoDir == "" {
		return UndoResult{}, errors.New("undo directory is not configured")
	}
	if isActiveStatus(view.Status) {
		return UndoResult{}, errors.New("job is still running")
	}
	outputs := jobOutputs(view)
	if len(outputs) == 0 {
		return UndoResult{}, errors.New("this job has no outputs to undo")
	}
	trashRoot := filepath.Join(undoDir, "trash", core.SanitizeFilename(view.ID))
	if err := os.MkdirAll(trashRoot, 0o755); err != nil {
		return UndoResult{}, err
	}

	var result UndoResult
	seen := map[string]bool{}
	for _, art := range outputs {
		if art.Path == "" || seen[art.Path] {
			continue
		}
		seen[art.Path] = true
		if _, err := os.Stat(art.Path); err == nil {
			trash, err := core.UniquePath(filepath.Join(trashRoot, filepath.Base(art.Path)), core.CollisionSuffix)
			if err != nil {
				return result, err
			}
			if err := moveFile(art.Path, trash); err != nil {
				return result, err
			}
			result.Files++
		} else if !os.IsNotExist(err) {
			return result, err
		}

		backup, _ := art.Meta["backup"].(string)
		if backup == "" {
			continue
		}
		if _, err := os.Stat(backup); err != nil {
			continue
		}
		if err := os.MkdirAll(filepath.Dir(art.Path), 0o755); err != nil {
			return result, err
		}
		if err := moveFile(backup, art.Path); err != nil {
			return result, err
		}
		result.Restored++
	}
	return result, nil
}

func jobOutputs(view JobView) []core.Artifact {
	if len(view.Aggregate) > 0 {
		return view.Aggregate
	}
	var out []core.Artifact
	for _, item := range view.Items {
		out = append(out, item.Outputs...)
	}
	return out
}

// TrashFiles moves existing files into dir, preserving names with collision
// suffixes. It is used by output review and other user-driven cleanup flows.
func TrashFiles(paths []string, dir string) (int, error) {
	if dir == "" {
		return 0, errors.New("trash directory is not configured")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return 0, err
	}
	count := 0
	seen := map[string]bool{}
	for _, path := range paths {
		if path == "" || seen[path] {
			continue
		}
		seen[path] = true
		if _, err := os.Stat(path); os.IsNotExist(err) {
			continue
		} else if err != nil {
			return count, err
		}
		dest, err := core.UniquePath(filepath.Join(dir, filepath.Base(path)), core.CollisionSuffix)
		if err != nil {
			return count, err
		}
		if err := moveFile(path, dest); err != nil {
			return count, err
		}
		count++
	}
	return count, nil
}
