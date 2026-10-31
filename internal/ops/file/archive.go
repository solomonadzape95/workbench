package file

import (
	"archive/tar"
	"archive/zip"
	"compress/flate"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/klauspost/compress/zstd"

	"github.com/solomonadzape95/workbench/internal/core"
	"github.com/solomonadzape95/workbench/internal/engines"
	"github.com/solomonadzape95/workbench/internal/ops/oputil"
)

var archiveSpec = core.Spec{
	ID:      "file.archive",
	Title:   "Archive file",
	Summary: "Compress a file into a ZIP, tar.gz, or Zstandard archive",
	Domain:  core.File,
	Accepts: nil,
	Params: []core.ParamSpec{
		{
			Key: "format", Label: "Format", Type: core.ParamEnum, Default: "zip",
			Options: []core.Option{
				{Value: "zip", Label: "ZIP"},
				{Value: "tgz", Label: "tar.gz"},
				{Value: "zst", Label: "Zstandard"},
			},
		},
		{
			Key: "level", Label: "Level", Type: core.ParamInt, Default: 6,
			Min: floatPtr(1), Max: floatPtr(9), Advanced: true,
			Help: "1 is fastest, 9 is smallest.",
		},
	},
	Output:    core.OutputSpec{FormatParam: "format"},
	Engines:   []core.EngineID{engines.Internal},
	Batchable: true,
	Cost:      core.CostFast,
}

type Archive struct{}

func (Archive) Spec() core.Spec { return archiveSpec }

func (Archive) Validate(ctx context.Context, req core.Request) error {
	if err := core.Validate(archiveSpec.Params, req.Params); err != nil {
		return err
	}
	if len(req.Items) == 0 {
		return core.NewError(core.CodeUsage, archiveSpec.ID, "", "no input files", "")
	}
	return nil
}

func (Archive) Run(ctx context.Context, req core.Request, rep core.Reporter) (core.Result, error) {
	start := time.Now()
	format := req.Params.String("format")
	level := req.Params.Int("level")
	if level == 0 {
		level = 6
	}

	var out []core.Artifact
	var bytesIn, bytesOut int64
	for _, item := range req.Items {
		if err := ctx.Err(); err != nil {
			return core.Result{}, err
		}
		output := filepath.Join(req.Target.Dir, core.SanitizeStem(item.Stem())+"."+format)

		rep.Stage("archiving")
		rep.Progress(0, int(item.Size))
		if err := archiveOne(ctx, item, output, format, level, rep); err != nil {
			_ = os.Remove(output)
			if ctx.Err() != nil {
				return core.Result{}, ctx.Err()
			}
			return core.Result{}, core.Wrap(core.CodeOutput, archiveSpec.ID, item.Path, err,
				"check permissions on the input and output directories")
		}
		info, err := os.Stat(output)
		if err != nil {
			return core.Result{}, core.Wrap(core.CodeOutput, archiveSpec.ID, output, err, "")
		}
		out = append(out, core.Artifact{
			Path:   output,
			Kind:   core.Archive,
			Format: format,
			Size:   info.Size(),
			Meta: map[string]any{
				"source":  item.Path,
				"entries": 1,
				"level":   level,
			},
		})
		bytesIn += item.Size
		bytesOut += info.Size()
		rep.Progress(int(item.Size), int(item.Size))
	}

	return core.Result{
		Artifacts: out,
		Metrics: core.Metrics{
			BytesIn:  bytesIn,
			BytesOut: bytesOut,
			Duration: time.Since(start),
			Engine:   engines.Internal,
		},
	}, nil
}

func archiveOne(ctx context.Context, item core.Artifact, output, format string, level int, rep core.Reporter) error {
	info, err := os.Stat(item.Path)
	if err != nil {
		return err
	}
	in, err := os.Open(item.Path)
	if err != nil {
		return err
	}
	defer in.Close()

	report := func(done int64) {
		rep.Progress(int(done), int(item.Size))
	}

	switch format {
	case "zip":
		f, err := os.Create(output)
		if err != nil {
			return err
		}
		zw := zip.NewWriter(f)
		zw.RegisterCompressor(zip.Deflate, func(out io.Writer) (io.WriteCloser, error) {
			return flate.NewWriter(out, level)
		})
		header := &zip.FileHeader{Name: item.Name(), Method: zip.Deflate}
		header.Modified = info.ModTime()
		w, err := zw.CreateHeader(header)
		if err == nil {
			_, err = oputil.Copy(ctx, w, in, report)
		}
		closeZip := zw.Close()
		closeFile := f.Close()
		if err == nil {
			err = closeZip
		}
		if err == nil {
			err = closeFile
		}
		return err

	case "tgz":
		f, err := os.Create(output)
		if err != nil {
			return err
		}
		gz, err := gzip.NewWriterLevel(f, level)
		if err != nil {
			f.Close()
			return err
		}
		tw := tar.NewWriter(gz)
		header := &tar.Header{
			Name:    item.Name(),
			Mode:    int64(info.Mode().Perm()),
			Size:    info.Size(),
			ModTime: info.ModTime(),
		}
		if err := tw.WriteHeader(header); err != nil {
			tw.Close()
			gz.Close()
			f.Close()
			return err
		}
		_, copyErr := oputil.Copy(ctx, tw, in, report)
		closeTar := tw.Close()
		closeGzip := gz.Close()
		closeFile := f.Close()
		if copyErr != nil {
			return copyErr
		}
		if closeTar != nil {
			return closeTar
		}
		if closeGzip != nil {
			return closeGzip
		}
		return closeFile

	case "zst":
		f, err := os.Create(output)
		if err != nil {
			return err
		}
		zw, err := zstd.NewWriter(f, zstd.WithEncoderLevel(zstd.EncoderLevelFromZstd(level)))
		if err != nil {
			f.Close()
			return err
		}
		_, copyErr := oputil.Copy(ctx, zw, in, report)
		closeZstd := zw.Close()
		closeFile := f.Close()
		if copyErr != nil {
			return copyErr
		}
		if closeZstd != nil {
			return closeZstd
		}
		return closeFile

	default:
		return fmt.Errorf("unsupported archive format %q", format)
	}
}

func floatPtr(v float64) *float64 { return &v }
