package doc

import (
	"archive/zip"
	"context"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/solomonadzape95/workbench/internal/core"
	"github.com/solomonadzape95/workbench/internal/engines/libreoffice"
	"github.com/solomonadzape95/workbench/internal/engines/pdf2docx"
)

// TestRealPDFToDOCX guards the Writer PDF import filter. Without it LibreOffice
// opens PDFs in Draw and refuses to export DOCX, which is exactly how the first
// real PDF job failed.
func TestRealPDFToDOCX(t *testing.T) {
	soffice, err := exec.LookPath("soffice")
	if err != nil {
		t.Skip("LibreOffice is not installed")
	}
	dir := t.TempDir()
	html := filepath.Join(dir, "source.html")
	if err := os.WriteFile(html, []byte("<html><body><h1>Round trip</h1><p>editable text</p></body></html>"), 0o644); err != nil {
		t.Fatal(err)
	}
	profile, err := os.MkdirTemp("", "workbench-lo-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(profile)
	profileURL := (&url.URL{Scheme: "file", Path: profile}).String()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, soffice, "--headless", "--norestore", "--invisible",
		"-env:UserInstallation="+profileURL, "--convert-to", "pdf", "--outdir", dir, html)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("creating test PDF: %v\n%s", err, out)
	}
	pdf := filepath.Join(dir, "source.pdf")
	art, err := core.Probe(pdf)
	if err != nil {
		t.Fatal(err)
	}
	op := Convert{LibreOffice: libreoffice.NewClient("")}
	req := core.Request{
		Items:  []core.Artifact{art},
		Params: core.Values{"format": "docx", "engine": "auto"},
		Target: core.Target{Dir: t.TempDir()},
	}
	if err := op.Validate(ctx, req); err != nil {
		t.Fatal(err)
	}
	result, err := op.Run(ctx, req, nopReporter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Artifacts) != 1 || result.Artifacts[0].Format != "docx" {
		t.Fatalf("artifacts = %+v", result.Artifacts)
	}
	if info, err := os.Stat(result.Artifacts[0].Path); err != nil || info.Size() == 0 {
		t.Fatalf("output = %v, %v", info, err)
	}
}

func TestRealPDF2DOCXPreservesTable(t *testing.T) {
	if _, err := exec.LookPath("pdf2docx"); err != nil {
		t.Skip("pdf2docx is not installed")
	}
	if _, err := exec.LookPath("soffice"); err != nil {
		t.Skip("LibreOffice is not installed")
	}
	dir := t.TempDir()
	html := filepath.Join(dir, "table.html")
	if err := os.WriteFile(html, []byte(`<html><body><h1>Table</h1><table border="1"><tr><th>Name</th><th>Value</th></tr><tr><td>alpha</td><td>one</td></tr></table></body></html>`), 0o644); err != nil {
		t.Fatal(err)
	}
	profile, err := os.MkdirTemp("", "workbench-lo-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(profile)
	profileURL := (&url.URL{Scheme: "file", Path: profile}).String()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "soffice", "--headless", "--norestore", "--invisible",
		"-env:UserInstallation="+profileURL, "--convert-to", "pdf", "--outdir", dir, html)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("creating test PDF: %v\n%s", err, out)
	}
	pdf := filepath.Join(dir, "table.pdf")
	art, err := core.Probe(pdf)
	if err != nil {
		t.Fatal(err)
	}
	op := Convert{PDF2DOCX: pdf2docx.NewClient("")}
	req := core.Request{
		Items:  []core.Artifact{art},
		Params: core.Values{"format": "docx", "engine": "auto"},
		Target: core.Target{Dir: t.TempDir()},
	}
	if err := op.Validate(ctx, req); err != nil {
		t.Fatal(err)
	}
	result, err := op.Run(ctx, req, nopReporter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Artifacts) != 1 {
		t.Fatalf("artifacts = %+v", result.Artifacts)
	}
	r, err := zip.OpenReader(result.Artifacts[0].Path)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	var xml string
	for _, f := range r.File {
		if f.Name == "word/document.xml" {
			rc, err := f.Open()
			if err != nil {
				t.Fatal(err)
			}
			data, _ := io.ReadAll(rc)
			rc.Close()
			xml = string(data)
		}
	}
	if !strings.Contains(xml, "<w:tbl>") {
		t.Fatalf("pdf2docx DOCX has no table element")
	}
}
