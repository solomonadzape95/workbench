package core

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type Collision string

const (
	CollisionSuffix    Collision = "suffix"
	CollisionOverwrite Collision = "overwrite"
	CollisionSkip      Collision = "skip"
	CollisionFail      Collision = "fail"
)

// ErrSkip lets the runner distinguish "intentionally not written" from a real
// output failure.
var ErrSkip = errors.New("output already exists")

type Target struct {
	Dir       string
	Template  string
	Collision Collision
}

// OutputPath renders the target template for one artifact. The collision policy
// is applied separately, under the runner's commit lock.
func (t Target) OutputPath(in Artifact, outFormat, opID string, index int, now time.Time) string {
	ext := ExtFor(outFormat, in)
	template := t.Template
	if strings.TrimSpace(template) == "" {
		template = "{stem}.{ext}"
	}
	stem := SanitizeStem(in.Stem())
	width, height := "", ""
	if in.Meta != nil {
		if v, ok := in.Meta["width"]; ok && v != nil {
			width = fmt.Sprint(v)
		}
		if v, ok := in.Meta["height"]; ok && v != nil {
			height = fmt.Sprint(v)
		}
	}
	name := strings.NewReplacer(
		"{stem}", stem,
		"{ext}", ext,
		"{name}", SanitizeFilename(in.Name()),
		"{date}", now.Format("2006-01-02"),
		"{time}", now.Format("15-04-05"),
		"{op}", strings.ReplaceAll(opID, "/", "-"),
		"{index}", strconv.Itoa(index+1),
		"{counter}", strconv.Itoa(index+1),
		"{width}", width,
		"{height}", height,
		"{alias}", "",
	).Replace(template)
	name = SanitizeFilename(name)
	return filepath.Join(t.Dir, name)
}

// UniquePath applies the collision policy to a proposed path. A race between the
// stat and the later rename is prevented by the caller holding a lock around
// both this call and the commit.
func UniquePath(path string, policy Collision) (string, error) {
	_, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return path, nil
	}
	if err != nil {
		return "", Wrap(CodeOutput, "", path, err, "check permissions on the output directory")
	}

	switch policy {
	case CollisionOverwrite:
		return path, nil
	case CollisionSkip:
		return "", ErrSkip
	case CollisionFail:
		return "", NewError(CodeOutput, "", path, "output already exists",
			"choose the suffix or overwrite collision policy")
	}

	ext := filepath.Ext(path)
	stem := strings.TrimSuffix(path, ext)
	for i := 1; i < 10_000; i++ {
		candidate := fmt.Sprintf("%s (%d)%s", stem, i, ext)
		if _, err := os.Lstat(candidate); errors.Is(err, os.ErrNotExist) {
			return candidate, nil
		}
	}
	return "", NewError(CodeOutput, "", path, "could not find a free output name",
		"clean up the output directory")
}

// ExtFor maps a format name to a filename extension and falls back to the
// input artifact's extension.
func ExtFor(format string, in Artifact) string {
	ext := strings.TrimPrefix(strings.ToLower(strings.TrimSpace(format)), ".")
	if ext == "" {
		ext = in.Ext()
	}
	switch ext {
	case "jpeg":
		ext = "jpg"
	case "tiff":
		ext = "tif"
	case "mpeg":
		ext = "mpg"
	}
	var b strings.Builder
	for _, r := range ext {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		}
	}
	if b.Len() == 0 {
		return "bin"
	}
	return b.String()
}

// SanitizeStem makes a filename stem safe to embed in a template result.
func SanitizeStem(stem string) string {
	stem = strings.TrimSpace(stem)
	stem = strings.ReplaceAll(stem, string(filepath.Separator), "_")
	stem = strings.ReplaceAll(stem, "\x00", "")
	if stem == "" || stem == "." || stem == ".." {
		return "output"
	}
	return stem
}

// SanitizeFilename prevents template output from escaping the target directory.
func SanitizeFilename(name string) string {
	name = filepath.Base(strings.ReplaceAll(name, "\x00", ""))
	if name == "" || name == "." || name == ".." {
		return "output"
	}
	return name
}
