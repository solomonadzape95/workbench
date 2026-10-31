package file

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/solomonadzape95/workbench/internal/core"
)

func sha256Of(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func TestVerifyReportsOKMismatchAndMissing(t *testing.T) {
	dir := t.TempDir()
	good := filepath.Join(dir, "good.txt")
	bad := filepath.Join(dir, "bad.txt")
	if err := os.WriteFile(good, []byte("good"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(bad, []byte("changed"), 0o644); err != nil {
		t.Fatal(err)
	}
	goodHash := sha256Of(t, good)
	manifest := filepath.Join(dir, "checks.sha256")
	body := goodHash + "  good.txt\n" +
		"0000000000000000000000000000000000000000000000000000000000000000  bad.txt\n" +
		"1111111111111111111111111111111111111111111111111111111111111111  missing.txt\n"
	if err := os.WriteFile(manifest, []byte("# generated\n"+body), 0o644); err != nil {
		t.Fatal(err)
	}

	work := filepath.Join(dir, "work")
	if err := os.Mkdir(work, 0o755); err != nil {
		t.Fatal(err)
	}
	artifact, err := core.Probe(manifest)
	if err != nil {
		t.Fatal(err)
	}
	op := Verify{}
	req := core.Request{
		Items:  []core.Artifact{artifact},
		Params: core.Values{"algo": "auto"},
		Target: core.Target{Dir: work},
	}
	if err := op.Validate(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	result, err := op.Run(context.Background(), req, nopReporter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Artifacts) != 1 || result.Artifacts[0].Format != "json" {
		t.Fatalf("artifacts = %+v", result.Artifacts)
	}
	if result.Artifacts[0].Meta["kind"] != "file-verify" {
		t.Errorf("meta = %+v", result.Artifacts[0].Meta)
	}
	data, err := os.ReadFile(result.Artifacts[0].Path)
	if err != nil {
		t.Fatal(err)
	}
	var report verifyReport
	if err := json.Unmarshal(data, &report); err != nil {
		t.Fatalf("report: %v\n%s", err, data)
	}
	if report.Total != 3 || report.OK != 1 || report.Mismatch != 1 || report.Missing != 1 {
		t.Fatalf("counts = %+v", report)
	}
	if report.Algorithm != "auto" {
		t.Errorf("algorithm = %q", report.Algorithm)
	}
	if report.Entries[0].Status != "ok" || report.Entries[1].Status != "mismatch" || report.Entries[2].Status != "missing" {
		t.Errorf("statuses = %+v", report.Entries)
	}
	if report.Entries[0].Algorithm != "sha256" {
		t.Errorf("entry algorithm = %q", report.Entries[0].Algorithm)
	}
}

func TestVerifyAcceptsBSDManifestAndBase(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "docs")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(sub, "paper.txt")
	if err := os.WriteFile(file, []byte("paper"), 0o644); err != nil {
		t.Fatal(err)
	}
	digest := sha256Of(t, file)
	manifest := filepath.Join(dir, "checks.txt")
	if err := os.WriteFile(manifest, []byte("SHA256 (paper.txt) = "+digest+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	work := filepath.Join(dir, "work")
	if err := os.Mkdir(work, 0o755); err != nil {
		t.Fatal(err)
	}
	artifact, err := core.Probe(manifest)
	if err != nil {
		t.Fatal(err)
	}
	result, err := (Verify{}).Run(context.Background(), core.Request{
		Items:  []core.Artifact{artifact},
		Params: core.Values{"algo": "sha256", "base": sub},
		Target: core.Target{Dir: work},
	}, nopReporter{})
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(result.Artifacts[0].Path)
	if err != nil {
		t.Fatal(err)
	}
	var report verifyReport
	if err := json.Unmarshal(data, &report); err != nil {
		t.Fatal(err)
	}
	if report.OK != 1 || report.Mismatch != 0 || report.Missing != 0 {
		t.Fatalf("report = %+v", report)
	}
	if report.Entries[0].Path != file {
		t.Errorf("path = %q, want %q", report.Entries[0].Path, file)
	}
}

func TestParseChecksumManifestRejectsBadLines(t *testing.T) {
	if _, err := parseChecksumManifest("not a manifest\n"); err == nil {
		t.Fatal("bad manifest parsed without error")
	}
	entries, err := parseChecksumManifest("2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824 *hello world.txt\n")
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name != "hello world.txt" {
		t.Fatalf("entries = %+v", entries)
	}
	if got := inferAlgorithm(entries[0].Digest); got != "sha256" {
		t.Fatalf("algorithm = %q", got)
	}
}

func TestVerifyValidateRejectsBadAlgorithm(t *testing.T) {
	err := (Verify{}).Validate(context.Background(), core.Request{
		Items:  []core.Artifact{{Path: "/tmp/m"}},
		Params: core.Values{"algo": "crc32"},
	})
	if err == nil {
		t.Fatal("bad algorithm passed validation")
	}
}
