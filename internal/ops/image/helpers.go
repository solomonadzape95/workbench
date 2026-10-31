package image

import (
	"fmt"
	"image"
	"os"
	"path/filepath"
	"strings"

	"github.com/solomonadzape95/workbench/internal/core"
)

const (
	maxDimension = 20_000
	maxPixels    = 200_000_000
)

func normalizeFormat(format string) string {
	f := strings.ToLower(strings.TrimSpace(format))
	if f == "jpg" {
		return "jpeg"
	}
	return f
}

func supportedFormat(format string) bool {
	switch normalizeFormat(format) {
	case "png", "jpeg", "gif":
		return true
	default:
		return false
	}
}

// writeOutput encodes img into the request's work directory and returns the
// artifact the runner will validate and commit.
func writeOutput(req core.Request, item core.Artifact, img image.Image, format string, quality int) (core.Artifact, error) {
	format = normalizeFormat(format)
	if !supportedFormat(format) {
		return core.Artifact{}, core.NewError(core.CodeUsage, spec.ID, item.Path,
			fmt.Sprintf("unsupported output format %q", format), "choose png, jpeg, or gif")
	}

	ext := core.ExtFor(format, item)
	path := filepath.Join(req.Target.Dir, core.SanitizeStem(item.Stem())+"."+ext)
	if err := encodeFile(path, img, format, quality); err != nil {
		return core.Artifact{}, core.Wrap(core.CodeOutput, spec.ID, item.Path, err,
			"check that the output directory is writable")
	}
	info, err := os.Stat(path)
	if err != nil {
		return core.Artifact{}, core.Wrap(core.CodeOutput, spec.ID, path, err, "")
	}
	bounds := img.Bounds()
	return core.Artifact{
		Path:   path,
		Kind:   core.Image,
		Format: format,
		Size:   info.Size(),
		Meta: map[string]any{
			"width":  bounds.Dx(),
			"height": bounds.Dy(),
			"source": item.Path,
		},
	}, nil
}

func checkTarget(size int) error {
	if size <= 0 || size > maxDimension {
		return core.NewError(core.CodeUsage, spec.ID, "",
			fmt.Sprintf("target dimension %d is out of range", size),
			fmt.Sprintf("use a value between 1 and %d", maxDimension))
	}
	return nil
}

func checkPixels(width, height int) error {
	if width > 0 && height > 0 && int64(width)*int64(height) > maxPixels {
		return core.NewError(core.CodeUsage, spec.ID, "",
			"target image is too large", "reduce the target dimensions")
	}
	return nil
}
