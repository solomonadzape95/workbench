package file

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/klauspost/compress/zstd"

	"github.com/solomonadzape95/workbench/internal/core"
)

func archiveFixture(t *testing.T) (core.Artifact, string) {
	t.Helper()
	dir := t.TempDir()
	src := filepath.Join(dir, "hello.txt")
	if err := os.WriteFile(src, []byte("hello archive"), 0o644); err != nil {
		t.Fatal(err)
	}
	art, err := core.Probe(src)
	if err != nil {
		t.Fatal(err)
	}
	return art, dir
}

func runArchive(t *testing.T, format string) core.Artifact {
	t.Helper()
	art, dir := archiveFixture(t)
	out := filepath.Join(dir, "out")
	if err := os.Mkdir(out, 0o755); err != nil {
		t.Fatal(err)
	}
	op := Archive{}
	req := core.Request{
		Items:  []core.Artifact{art},
		Params: core.Values{"format": format, "level": 6},
		Target: core.Target{Dir: out},
	}
	if err := op.Validate(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	result, err := op.Run(context.Background(), req, nopReporter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Artifacts) != 1 {
		t.Fatalf("artifacts = %+v", result.Artifacts)
	}
	return result.Artifacts[0]
}

func TestArchiveZIP(t *testing.T) {
	art := runArchive(t, "zip")
	zr, err := zip.OpenReader(art.Path)
	if err != nil {
		t.Fatal(err)
	}
	defer zr.Close()
	if len(zr.File) != 1 || zr.File[0].Name != "hello.txt" {
		t.Fatalf("entries = %+v", zr.File)
	}
	rc, err := zr.File[0].Open()
	if err != nil {
		t.Fatal(err)
	}
	data, _ := io.ReadAll(rc)
	rc.Close()
	if string(data) != "hello archive" {
		t.Errorf("content = %q", data)
	}
}

func TestArchiveTGZ(t *testing.T) {
	art := runArchive(t, "tgz")
	f, err := os.Open(art.Path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		t.Fatal(err)
	}
	tr := tar.NewReader(gz)
	header, err := tr.Next()
	if err != nil {
		t.Fatal(err)
	}
	if header.Name != "hello.txt" {
		t.Errorf("name = %q", header.Name)
	}
	data, _ := io.ReadAll(tr)
	if string(data) != "hello archive" {
		t.Errorf("content = %q", data)
	}
}

func TestArchiveZstd(t *testing.T) {
	art := runArchive(t, "zst")
	f, err := os.Open(art.Path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	zr, err := zstd.NewReader(f)
	if err != nil {
		t.Fatal(err)
	}
	defer zr.Close()
	data, err := io.ReadAll(zr)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "hello archive" {
		t.Errorf("content = %q", data)
	}
}
