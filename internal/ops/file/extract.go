package file

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/klauspost/compress/zstd"

	"github.com/solomonadzape95/workbench/internal/core"
	"github.com/solomonadzape95/workbench/internal/engines"
)

var extractSpec = core.Spec{
	ID:      "file.extract",
	Title:   "Extract archive",
	Summary: "Unpack a ZIP, tar, tar.gz, gzip, or Zstandard archive into a folder",
	Domain:  core.File,
	Accepts: []core.Accept{{Kind: core.Archive, Formats: []string{"zip", "tar", "tgz", "tar.gz", "gz", "zst"}}},
	Params: []core.ParamSpec{
		{
			Key: "flatten", Label: "Flatten folders", Type: core.ParamBool, Default: false,
			Help: "Put every extracted file directly in the output directory instead of preserving archive paths.",
		},
	},
	Output:  core.OutputSpec{Kind: core.File},
	Engines: []core.EngineID{engines.Internal},
	Mode:    core.ModeDirect,
	Cost:    core.CostFast,
}

type Extract struct{}

func (Extract) Spec() core.Spec { return extractSpec }

func (Extract) Validate(ctx context.Context, req core.Request) error {
	if err := core.Validate(extractSpec.Params, req.Params); err != nil {
		return err
	}
	if len(req.Items) == 0 {
		return core.NewError(core.CodeUsage, extractSpec.ID, "", "no archives to extract", "")
	}
	if req.Target.Dir == "" {
		return core.NewError(core.CodeUsage, extractSpec.ID, "", "no output directory", "")
	}
	for _, item := range req.Items {
		if _, err := archiveKind(item.Path); err != nil {
			return core.Wrap(core.CodeInput, extractSpec.ID, item.Path, err,
				"supported formats: zip, tar, tar.gz/tgz, gz, and zst")
		}
	}
	return nil
}

func (Extract) Run(ctx context.Context, req core.Request, rep core.Reporter) (core.Result, error) {
	start := time.Now()
	flatten := req.Params.Bool("flatten")
	if err := os.MkdirAll(req.Target.Dir, 0o755); err != nil {
		return core.Result{}, core.Wrap(core.CodeOutput, extractSpec.ID, req.Target.Dir, err,
			"check permissions on the output directory")
	}

	var out []core.Artifact
	var bytesIn, bytesOut int64
	for i, item := range req.Items {
		if err := ctx.Err(); err != nil {
			return core.Result{}, err
		}
		kind, err := archiveKind(item.Path)
		if err != nil {
			return core.Result{}, core.Wrap(core.CodeInput, extractSpec.ID, item.Path, err, "")
		}
		root := req.Target.Dir
		if !flatten {
			stem := archiveStem(item.Name(), kind)
			proposed := filepath.Join(req.Target.Dir, core.SanitizeStem(stem))
			root, err = core.UniquePath(proposed, req.Target.Collision)
			if err != nil {
				return core.Result{}, core.Wrap(core.CodeOutput, extractSpec.ID, proposed, err,
					"choose a different output directory or collision policy")
			}
		}
		if err := os.MkdirAll(root, 0o755); err != nil {
			return core.Result{}, core.Wrap(core.CodeOutput, extractSpec.ID, root, err, "")
		}

		rep.Stage("extracting " + item.Name())
		rep.Progress(0, int(item.Size))
		paths, err := extractOne(ctx, item, kind, root, flatten, req.Target.Collision, rep)
		if err != nil {
			if ctx.Err() != nil {
				return core.Result{}, ctx.Err()
			}
			return core.Result{}, core.Wrap(core.CodeOutput, extractSpec.ID, item.Path, err,
				"check that the archive is valid and the output directory is writable")
		}
		if len(paths) == 0 {
			return core.Result{}, core.NewError(core.CodeOutput, extractSpec.ID, item.Path,
				"archive contains no extractable files", "")
		}
		for _, extracted := range paths {
			art, probeErr := core.Probe(extracted)
			if probeErr != nil {
				info, statErr := os.Stat(extracted)
				if statErr != nil {
					return core.Result{}, core.Wrap(core.CodeOutput, extractSpec.ID, extracted, statErr, "")
				}
				art = core.Artifact{Path: extracted, Kind: core.File, Size: info.Size()}
			}
			art.SuggestedName = filepath.Base(extracted)
			if art.Meta == nil {
				art.Meta = map[string]any{}
			}
			art.Meta["source"] = item.Path
			art.Meta["archive"] = true
			out = append(out, art)
			bytesOut += art.Size
		}
		bytesIn += item.Size
		rep.Progress(int(item.Size), int(item.Size))
		rep.Progress(i+1, len(req.Items))
	}

	return core.Result{
		Artifacts: out,
		Metrics: core.Metrics{
			BytesIn: bytesIn, BytesOut: bytesOut, Duration: time.Since(start), Engine: engines.Internal,
		},
	}, nil
}

type archiveType string

const (
	archiveZip archiveType = "zip"
	archiveTar archiveType = "tar"
	archiveTGZ archiveType = "tgz"
	archiveGZ  archiveType = "gz"
	archiveZST archiveType = "zst"
)

func archiveKind(name string) (archiveType, error) {
	lower := strings.ToLower(name)
	switch {
	case strings.HasSuffix(lower, ".tar.gz"), strings.HasSuffix(lower, ".tgz"):
		return archiveTGZ, nil
	case strings.HasSuffix(lower, ".tar"):
		return archiveTar, nil
	case strings.HasSuffix(lower, ".zip"):
		return archiveZip, nil
	case strings.HasSuffix(lower, ".zst"):
		return archiveZST, nil
	case strings.HasSuffix(lower, ".gz"):
		return archiveGZ, nil
	default:
		return "", fmt.Errorf("unsupported archive extension %q", filepath.Ext(lower))
	}
}

func archiveStem(name string, kind archiveType) string {
	lower := strings.ToLower(name)
	var suffix string
	switch kind {
	case archiveTGZ:
		if strings.HasSuffix(lower, ".tar.gz") {
			suffix = ".tar.gz"
		} else {
			suffix = ".tgz"
		}
	case archiveTar:
		suffix = ".tar"
	case archiveZip:
		suffix = ".zip"
	case archiveZST:
		suffix = ".zst"
	case archiveGZ:
		suffix = ".gz"
	}
	if suffix != "" && strings.HasSuffix(lower, suffix) {
		return name[:len(name)-len(suffix)]
	}
	return strings.TrimSuffix(name, filepath.Ext(name))
}

func extractOne(
	ctx context.Context,
	item core.Artifact,
	kind archiveType,
	root string,
	flatten bool,
	collision core.Collision,
	rep core.Reporter,
) ([]string, error) {
	f, err := os.Open(item.Path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var paths []string
	switch kind {
	case archiveZip:
		info, err := f.Stat()
		if err != nil {
			return nil, err
		}
		zr, err := zip.NewReader(f, info.Size())
		if err != nil {
			return nil, err
		}
		for _, entry := range zr.File {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			name, err := safeArchiveName(entry.Name)
			if err != nil {
				return nil, err
			}
			if name == "" {
				continue
			}
			if entry.FileInfo().IsDir() || strings.HasSuffix(entry.Name, "/") {
				if !flatten {
					if err := os.MkdirAll(safeJoin(root, name), 0o755); err != nil {
						return nil, err
					}
				}
				continue
			}
			if flatten {
				name = filepath.Base(name)
			}
			dest, err := entryPath(root, name, collision)
			if err != nil {
				if errors.Is(err, core.ErrSkip) {
					continue
				}
				return nil, err
			}
			rc, err := entry.Open()
			if err != nil {
				return nil, err
			}
			written, copyErr := writeExtracted(dest, rc, entry.Mode().Perm(), collision)
			closeErr := rc.Close()
			if copyErr != nil {
				return nil, copyErr
			}
			if closeErr != nil {
				return nil, closeErr
			}
			paths = append(paths, written)
		}

	case archiveTar, archiveTGZ:
		var reader io.Reader = f
		if kind == archiveTGZ {
			gz, err := gzip.NewReader(f)
			if err != nil {
				return nil, err
			}
			defer gz.Close()
			reader = gz
		}
		tr := tar.NewReader(reader)
		for {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			header, err := tr.Next()
			if errors.Is(err, io.EOF) {
				break
			}
			if err != nil {
				return nil, err
			}
			name, err := safeArchiveName(header.Name)
			if err != nil {
				return nil, err
			}
			switch header.Typeflag {
			case tar.TypeDir:
				if name != "" && !flatten {
					if err := os.MkdirAll(safeJoin(root, name), 0o755); err != nil {
						return nil, err
					}
				}
				continue
			case tar.TypeReg:
			default:
				continue
			}
			if name == "" {
				continue
			}
			if flatten {
				name = filepath.Base(name)
			}
			dest, err := entryPath(root, name, collision)
			if err != nil {
				if errors.Is(err, core.ErrSkip) {
					continue
				}
				return nil, err
			}
			mode := os.FileMode(header.Mode).Perm()
			written, err := writeExtracted(dest, tr, mode, collision)
			if err != nil {
				return nil, err
			}
			paths = append(paths, written)
		}

	case archiveGZ, archiveZST:
		var reader io.Reader
		if kind == archiveGZ {
			gz, err := gzip.NewReader(f)
			if err != nil {
				return nil, err
			}
			defer gz.Close()
			reader = gz
		} else {
			zr, err := zstd.NewReader(f)
			if err != nil {
				return nil, err
			}
			defer zr.Close()
			reader = zr
		}
		name := archiveStem(item.Name(), kind)
		if flatten {
			name = filepath.Base(name)
		}
		dest, err := entryPath(root, name, collision)
		if err != nil {
			if errors.Is(err, core.ErrSkip) {
				return nil, nil
			}
			return nil, err
		}
		written, err := writeExtracted(dest, reader, 0o644, collision)
		if err != nil {
			return nil, err
		}
		paths = append(paths, written)
	}
	return paths, nil
}

func entryPath(root, name string, collision core.Collision) (string, error) {
	dest := safeJoin(root, name)
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return "", err
	}
	return core.UniquePath(dest, collision)
}

func writeExtracted(dest string, src io.Reader, mode os.FileMode, collision core.Collision) (string, error) {
	if mode == 0 {
		mode = 0o644
	}
	part := dest + ".part"
	f, err := os.OpenFile(part, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode)
	if err != nil {
		return "", err
	}
	_, copyErr := io.Copy(f, src)
	closeErr := f.Close()
	if copyErr != nil {
		_ = os.Remove(part)
		return "", copyErr
	}
	if closeErr != nil {
		_ = os.Remove(part)
		return "", closeErr
	}
	if collision == core.CollisionOverwrite {
		_ = os.Remove(dest)
	}
	if err := os.Rename(part, dest); err != nil {
		_ = os.Remove(part)
		return "", err
	}
	return dest, nil
}

func safeArchiveName(name string) (string, error) {
	name = strings.ReplaceAll(name, "\\", "/")
	clean := path.Clean(name)
	if clean == "." || clean == "" {
		return "", nil
	}
	if path.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, "../") {
		return "", fmt.Errorf("archive entry %q escapes the output directory", name)
	}
	local := filepath.FromSlash(clean)
	if filepath.VolumeName(local) != "" {
		return "", fmt.Errorf("archive entry %q has an absolute volume", name)
	}
	return local, nil
}

func safeJoin(root, name string) string {
	dest := filepath.Join(root, name)
	rel, err := filepath.Rel(root, dest)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return filepath.Join(root, core.SanitizeFilename(name))
	}
	return dest
}
