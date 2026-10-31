package file

import (
	"context"
	"crypto/md5" // weak on purpose: file integrity and checksum compatibility, not security
	"crypto/sha1"
	"crypto/sha256"
	"fmt"
	"hash"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/solomonadzape95/workbench/internal/core"
	"github.com/solomonadzape95/workbench/internal/engines"
	"github.com/solomonadzape95/workbench/internal/ops/oputil"
)

var hashSpec = core.Spec{
	ID:      "file.hash",
	Title:   "Hash file",
	Summary: "Write a checksum beside each file",
	Domain:  core.File,
	Accepts: nil, // any file kind
	Params: []core.ParamSpec{
		{
			Key: "algo", Label: "Algorithm", Type: core.ParamEnum, Default: "sha256",
			Options: []core.Option{
				{Value: "sha256", Label: "SHA-256"},
				{Value: "sha1", Label: "SHA-1"},
				{Value: "md5", Label: "MD5"},
			},
		},
	},
	Output:    core.OutputSpec{FormatParam: "algo"},
	Engines:   []core.EngineID{engines.Internal},
	Batchable: true,
	Cost:      core.CostFast,
}

type Hash struct{}

func (Hash) Spec() core.Spec { return hashSpec }

func (Hash) Validate(ctx context.Context, req core.Request) error {
	if err := core.Validate(hashSpec.Params, req.Params); err != nil {
		return err
	}
	if len(req.Items) == 0 {
		return core.NewError(core.CodeUsage, hashSpec.ID, "", "no input files", "")
	}
	return nil
}

func (Hash) Run(ctx context.Context, req core.Request, rep core.Reporter) (core.Result, error) {
	start := time.Now()
	algo := req.Params.String("algo")
	var out []core.Artifact
	var bytesIn, bytesOut int64

	for _, item := range req.Items {
		if err := ctx.Err(); err != nil {
			return core.Result{}, err
		}
		if !hashSpec.AcceptsArtifact(item) {
			return core.Result{}, core.NewError(core.CodeInput, hashSpec.ID, item.Path, "not a regular file", "")
		}

		hasher, err := newHasher(algo)
		if err != nil {
			return core.Result{}, core.NewError(core.CodeUsage, hashSpec.ID, "", err.Error(), "")
		}

		rep.Stage("hashing")
		rep.Progress(0, int(item.Size))

		in, err := os.Open(item.Path)
		if err != nil {
			return core.Result{}, core.Wrap(core.CodeInput, hashSpec.ID, item.Path, err, "")
		}
		var lastReport time.Time
		_, copyErr := oputil.Copy(ctx, hasher, in, func(done int64) {
			if time.Since(lastReport) < 100*time.Millisecond && done < item.Size {
				return
			}
			lastReport = time.Now()
			rep.Progress(int(done), int(item.Size))
		})
		closeErr := in.Close()
		if copyErr == nil {
			copyErr = closeErr
		}
		if copyErr != nil {
			return core.Result{}, core.Wrap(core.CodeInput, hashSpec.ID, item.Path, copyErr, "")
		}

		outPath := filepath.Join(req.Target.Dir, core.SanitizeStem(item.Stem())+"."+algo)
		line := fmt.Sprintf("%x  %s\n", hasher.Sum(nil), item.Name())
		if err := os.WriteFile(outPath, []byte(line), 0o644); err != nil {
			return core.Result{}, core.Wrap(core.CodeOutput, hashSpec.ID, outPath, err, "")
		}

		info, err := os.Stat(outPath)
		if err != nil {
			return core.Result{}, core.Wrap(core.CodeOutput, hashSpec.ID, outPath, err, "")
		}
		out = append(out, core.Artifact{
			Path:   outPath,
			Kind:   core.File,
			Format: algo,
			Size:   info.Size(),
			Meta:   map[string]any{"algorithm": strings.ToUpper(algo), "source": item.Path},
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

func newHasher(algo string) (hash.Hash, error) {
	switch algo {
	case "sha256":
		return sha256.New(), nil
	case "sha1":
		return sha1.New(), nil
	case "md5":
		return md5.New(), nil
	default:
		return nil, fmt.Errorf("unknown hash algorithm %q", algo)
	}
}
