package file

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/solomonadzape95/workbench/internal/core"
)

func writeZip(t *testing.T, path string, entries map[string]string) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(f)
	for name, body := range entries {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestExtractZipPreservesFolders(t *testing.T) {
	dir := t.TempDir()
	archive := filepath.Join(dir, "bundle.zip")
	writeZip(t, archive, map[string]string{
		"a.txt":     "alpha",
		"sub/b.txt": "beta",
	})
	art, err := core.Probe(archive)
	if err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, "out")
	if err := os.MkdirAll(out, 0o755); err != nil {
		t.Fatal(err)
	}
	req := core.Request{Items: []core.Artifact{art}, Target: core.Target{Dir: out, Collision: core.CollisionSuffix}}
	if err := (Extract{}).Validate(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	result, err := (Extract{}).Run(context.Background(), req, nopReporter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Artifacts) != 2 {
		t.Fatalf("artifacts = %+v", result.Artifacts)
	}
	root := filepath.Join(out, "bundle")
	for name, want := range map[string]string{"a.txt": "alpha", "sub/b.txt": "beta"} {
		data, err := os.ReadFile(filepath.Join(root, name))
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if string(data) != want {
			t.Errorf("%s = %q, want %q", name, data, want)
		}
	}
	if _, err := os.Stat(archive); err != nil {
		t.Errorf("extract removed the archive: %v", err)
	}
}

func TestExtractFlattensAndSuffixes(t *testing.T) {
	dir := t.TempDir()
	archive := filepath.Join(dir, "flat.zip")
	writeZip(t, archive, map[string]string{
		"one/a.txt": "one",
		"two/a.txt": "two",
	})
	art, err := core.Probe(archive)
	if err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, "out")
	if err := os.MkdirAll(out, 0o755); err != nil {
		t.Fatal(err)
	}
	result, err := (Extract{}).Run(context.Background(), core.Request{
		Items:  []core.Artifact{art},
		Params: core.Values{"flatten": true},
		Target: core.Target{Dir: out, Collision: core.CollisionSuffix},
	}, nopReporter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Artifacts) != 2 {
		t.Fatalf("artifacts = %+v", result.Artifacts)
	}
	for _, art := range result.Artifacts {
		if filepath.Dir(art.Path) != out {
			t.Errorf("flattened file %q is not directly in %q", art.Path, out)
		}
	}
	if _, err := os.Stat(filepath.Join(out, "a.txt")); err != nil {
		t.Error(err)
	}
	if _, err := os.Stat(filepath.Join(out, "a (1).txt")); err != nil {
		t.Error(err)
	}
}

func TestExtractTar(t *testing.T) {
	dir := t.TempDir()
	archive := filepath.Join(dir, "bundle.tar")
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	for _, entry := range []struct{ name, body string }{{"x.txt", "x"}, {"nested/y.txt", "y"}} {
		if err := tw.WriteHeader(&tar.Header{Name: entry.name, Mode: 0o644, Size: int64(len(entry.body))}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte(entry.body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(archive, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	art, err := core.Probe(archive)
	if err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, "out")
	result, err := (Extract{}).Run(context.Background(), core.Request{
		Items:  []core.Artifact{art},
		Target: core.Target{Dir: out, Collision: core.CollisionSuffix},
	}, nopReporter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Artifacts) != 2 {
		t.Fatalf("artifacts = %+v", result.Artifacts)
	}
	if data, err := os.ReadFile(filepath.Join(out, "bundle", "nested", "y.txt")); err != nil || string(data) != "y" {
		t.Fatalf("tar extraction = %q, %v", data, err)
	}
}

func TestExtractRejectsPathTraversal(t *testing.T) {
	dir := t.TempDir()
	archive := filepath.Join(dir, "evil.zip")
	writeZip(t, archive, map[string]string{"../evil.txt": "bad"})
	art, err := core.Probe(archive)
	if err != nil {
		t.Fatal(err)
	}
	_, err = (Extract{}).Run(context.Background(), core.Request{
		Items:  []core.Artifact{art},
		Target: core.Target{Dir: filepath.Join(dir, "out"), Collision: core.CollisionSuffix},
	}, nopReporter{})
	if err == nil {
		t.Fatal("path traversal was accepted")
	}
	if _, statErr := os.Stat(filepath.Join(dir, "evil.txt")); !os.IsNotExist(statErr) {
		t.Fatalf("traversal wrote outside the output directory: %v", statErr)
	}
}

func TestExtractRejectsUnknownFormat(t *testing.T) {
	err := (Extract{}).Validate(context.Background(), core.Request{
		Items:  []core.Artifact{{Path: "notes.txt"}},
		Target: core.Target{Dir: t.TempDir()},
	})
	if err == nil {
		t.Fatal("unsupported archive passed validation")
	}
}
