package file

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/solomonadzape95/workbench/internal/config"
	"github.com/solomonadzape95/workbench/internal/core"
	"github.com/solomonadzape95/workbench/internal/engines"
)

var verifySpec = core.Spec{
	ID:      "file.verify",
	Title:   "Verify checksums",
	Summary: "Check a directory against a SHA-256, SHA-1, or MD5 manifest",
	Domain:  core.File,
	Accepts: nil, // the input is the manifest file
	Params: []core.ParamSpec{
		{
			Key: "base", Label: "Base directory", Type: core.ParamPath,
			Help: "Resolve relative manifest entries against this directory. Defaults to the manifest's directory.",
		},
		{
			Key: "algo", Label: "Algorithm", Type: core.ParamEnum, Default: "auto",
			Options: []core.Option{
				{Value: "auto", Label: "Detect from digest"},
				{Value: "sha256", Label: "SHA-256"},
				{Value: "sha1", Label: "SHA-1"},
				{Value: "md5", Label: "MD5"},
			},
		},
	},
	Output:    core.OutputSpec{StaticFormat: "json"},
	Engines:   []core.EngineID{engines.Internal},
	Batchable: true,
	Cost:      core.CostFast,
}

type Verify struct{}

func (Verify) Spec() core.Spec { return verifySpec }

func (Verify) Validate(ctx context.Context, req core.Request) error {
	if err := core.Validate(verifySpec.Params, req.Params); err != nil {
		return err
	}
	if len(req.Items) == 0 {
		return core.NewError(core.CodeUsage, verifySpec.ID, "", "no manifest files", "")
	}
	return nil
}

type verifyEntry struct {
	Path      string `json:"path"`
	Name      string `json:"name"`
	Expected  string `json:"expected"`
	Actual    string `json:"actual,omitempty"`
	Algorithm string `json:"algorithm"`
	Size      int64  `json:"size,omitempty"`
	Status    string `json:"status"` // ok, mismatch, missing, error
	Error     string `json:"error,omitempty"`
}

type verifyReport struct {
	Manifest  string        `json:"manifest"`
	Base      string        `json:"base"`
	Algorithm string        `json:"algorithm"`
	CheckedAt time.Time     `json:"checked_at"`
	Total     int           `json:"total"`
	OK        int           `json:"ok"`
	Mismatch  int           `json:"mismatch"`
	Missing   int           `json:"missing"`
	Errors    int           `json:"errors"`
	Entries   []verifyEntry `json:"entries"`
}

func (Verify) Run(ctx context.Context, req core.Request, rep core.Reporter) (core.Result, error) {
	start := time.Now()
	chosen := req.Params.String("algo")
	if chosen == "" {
		chosen = "auto"
	}
	baseOverride := config.ExpandHome(req.Params.String("base"))

	var out []core.Artifact
	var bytesIn, bytesOut int64
	for _, item := range req.Items {
		if err := ctx.Err(); err != nil {
			return core.Result{}, err
		}
		data, err := os.ReadFile(item.Path)
		if err != nil {
			return core.Result{}, core.Wrap(core.CodeInput, verifySpec.ID, item.Path, err,
				"check that the manifest is readable")
		}
		entries, err := parseChecksumManifest(string(data))
		if err != nil {
			return core.Result{}, core.Wrap(core.CodeInput, verifySpec.ID, item.Path, err,
				"expected sha256sum-style lines or SHA256 (file) = digest lines")
		}
		base := baseOverride
		if base == "" {
			base = filepath.Dir(item.Path)
		}
		report := verifyReport{
			Manifest:  item.Path,
			Base:      base,
			Algorithm: chosen,
			CheckedAt: time.Now().UTC(),
			Total:     len(entries),
			Entries:   make([]verifyEntry, 0, len(entries)),
		}

		rep.Stage("verifying")
		rep.Progress(0, max(len(entries), 1))
		for i, entry := range entries {
			if err := ctx.Err(); err != nil {
				return core.Result{}, err
			}
			result := verifyOne(ctx, base, entry, chosen)
			switch result.Status {
			case "ok":
				report.OK++
			case "mismatch":
				report.Mismatch++
			case "missing":
				report.Missing++
			default:
				report.Errors++
			}
			report.Entries = append(report.Entries, result)
			rep.Progress(i+1, max(len(entries), 1))
		}

		outPath := filepath.Join(req.Target.Dir, core.SanitizeStem(item.Stem())+".verify.json")
		encoded, err := json.MarshalIndent(report, "", "  ")
		if err != nil {
			return core.Result{}, core.Wrap(core.CodeInternal, verifySpec.ID, item.Path, err, "")
		}
		encoded = append(encoded, '\n')
		if err := os.WriteFile(outPath, encoded, 0o644); err != nil {
			return core.Result{}, core.Wrap(core.CodeOutput, verifySpec.ID, outPath, err,
				"check permissions on the output directory")
		}
		info, err := os.Stat(outPath)
		if err != nil {
			return core.Result{}, core.Wrap(core.CodeOutput, verifySpec.ID, outPath, err, "")
		}
		out = append(out, core.Artifact{
			Path:   outPath,
			Kind:   core.File,
			Format: "json",
			Size:   info.Size(),
			Meta: map[string]any{
				"kind":      "file-verify",
				"manifest":  item.Path,
				"base":      base,
				"total":     report.Total,
				"ok":        report.OK,
				"mismatch":  report.Mismatch,
				"missing":   report.Missing,
				"errors":    report.Errors,
				"algorithm": report.Algorithm,
			},
		})
		bytesIn += int64(len(data))
		bytesOut += info.Size()
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

func verifyOne(ctx context.Context, base string, entry checksumEntry, chosen string) verifyEntry {
	result := verifyEntry{
		Name:      entry.Name,
		Expected:  strings.ToLower(entry.Digest),
		Algorithm: chosen,
	}
	path := entry.Name
	if !filepath.IsAbs(path) {
		path = filepath.Join(base, path)
	}
	result.Path = path
	if result.Algorithm == "auto" {
		result.Algorithm = inferAlgorithm(entry.Digest)
	}
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		result.Status = "missing"
		return result
	}
	if err != nil {
		result.Status = "error"
		result.Error = err.Error()
		return result
	}
	if info.IsDir() {
		result.Status = "error"
		result.Error = "path is a directory"
		return result
	}
	result.Size = info.Size()
	if result.Algorithm == "" {
		result.Status = "error"
		result.Error = "cannot infer hash algorithm from digest"
		return result
	}
	actual, err := hashFile(ctx, path, result.Algorithm)
	if err != nil {
		result.Status = "error"
		result.Error = err.Error()
		return result
	}
	result.Actual = actual
	if strings.EqualFold(actual, entry.Digest) {
		result.Status = "ok"
	} else {
		result.Status = "mismatch"
	}
	return result
}

func hashFile(ctx context.Context, path, algo string) (string, error) {
	hasher, err := newHasher(algo)
	if err != nil {
		return "", err
	}
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	buf := make([]byte, 128*1024)
	for {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		n, readErr := f.Read(buf)
		if n > 0 {
			if _, err := hasher.Write(buf[:n]); err != nil {
				return "", err
			}
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return "", readErr
		}
	}
	return hex.EncodeToString(hasher.Sum(nil)), nil
}

func inferAlgorithm(digest string) string {
	switch len(strings.TrimSpace(digest)) {
	case 64:
		return "sha256"
	case 40:
		return "sha1"
	case 32:
		return "md5"
	default:
		return ""
	}
}

type checksumEntry struct {
	Digest string
	Name   string
}

func parseChecksumManifest(text string) ([]checksumEntry, error) {
	var entries []checksumEntry
	for lineNo, raw := range strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		entry, err := parseChecksumLine(line)
		if err != nil {
			return nil, fmt.Errorf("line %d: %w", lineNo+1, err)
		}
		if entry.Digest != "" && entry.Name != "" {
			entries = append(entries, entry)
		}
	}
	if len(entries) == 0 {
		return nil, fmt.Errorf("manifest has no checksum entries")
	}
	return entries, nil
}

func parseChecksumLine(line string) (checksumEntry, error) {
	// BSD style: SHA256 (file name) = digest
	if open := strings.Index(line, "("); open > 0 {
		if close := strings.Index(line[open:], ")"); close > 0 {
			name := strings.TrimSpace(line[open+1 : open+close])
			rest := strings.TrimSpace(line[open+close+1:])
			if strings.HasPrefix(rest, "=") {
				digest := strings.TrimSpace(strings.TrimPrefix(rest, "="))
				if !isHexDigest(digest) {
					return checksumEntry{}, fmt.Errorf("invalid digest %q", digest)
				}
				return checksumEntry{Digest: digest, Name: name}, nil
			}
		}
	}
	// GNU style: digest<space><space or *>file name
	cut := strings.IndexAny(line, " \t")
	if cut <= 0 {
		return checksumEntry{}, fmt.Errorf("expected '<digest>  <file>'")
	}
	digest := strings.TrimSpace(line[:cut])
	name := strings.TrimLeft(line[cut:], " \t*")
	if digest == "" || name == "" {
		return checksumEntry{}, fmt.Errorf("expected '<digest>  <file>'")
	}
	if !isHexDigest(digest) {
		return checksumEntry{}, fmt.Errorf("invalid digest %q", digest)
	}
	return checksumEntry{Digest: digest, Name: name}, nil
}

func isHexDigest(digest string) bool {
	if len(digest) != 32 && len(digest) != 40 && len(digest) != 64 {
		return false
	}
	_, err := hex.DecodeString(digest)
	return err == nil
}
