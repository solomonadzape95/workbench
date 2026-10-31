// Package libreoffice runs headless document conversions with LibreOffice.
package libreoffice

import (
	"bytes"
	"context"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/solomonadzape95/workbench/internal/core"
	"github.com/solomonadzape95/workbench/internal/engines"
	"github.com/solomonadzape95/workbench/internal/engines/executil"
)

type Client struct {
	Path string
}

func NewClient(path string) *Client {
	return &Client{Path: strings.TrimSpace(path)}
}

func (c *Client) ID() engines.ID { return engines.LibreOffice }

func (c *Client) Resolve() (string, error) {
	candidates := []string{c.Path}
	if c.Path == "" {
		candidates = []string{"soffice", "libreoffice"}
	}
	for _, candidate := range candidates {
		if candidate == "" {
			continue
		}
		if resolved, err := exec.LookPath(candidate); err == nil {
			return resolved, nil
		}
	}
	return "", core.NewError(core.CodeEngineMissing, "", "",
		"LibreOffice is not installed", InstallHint())
}

func (c *Client) Check() error {
	_, err := c.Resolve()
	return err
}

func (c *Client) Probe(parent context.Context) engines.Info {
	info := engines.Info{ID: engines.LibreOffice, Name: "libreoffice", Detail: "PDF, DOCX, ODT, EPUB"}
	path, err := c.Resolve()
	if err != nil {
		info.Hint = InstallHint()
		info.Err = err
		return info
	}
	info.Path = path

	// The first launch after install can spend several seconds in Gatekeeper
	// before soffice prints its version. A short timeout falsely reports a
	// working install as missing.
	ctx, cancel := context.WithTimeout(parent, 15*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, path, "--version").CombinedOutput()
	if err != nil {
		info.Hint = InstallHint()
		info.Detail = "found but failed to run"
		info.Err = err
		return info
	}
	info.Available = true
	info.Version = firstLine(out)
	return info
}

// Convert runs soffice headless and returns the artifact path it produced.
func (c *Client) Convert(ctx context.Context, input, outDir, targetExt string, rep core.Reporter) (string, error) {
	binary, err := c.Resolve()
	if err != nil {
		return "", err
	}
	profile, err := os.MkdirTemp("", "workbench-lo-*")
	if err != nil {
		return "", core.Wrap(core.CodeInternal, "", "", err, "cannot create a LibreOffice profile")
	}
	defer os.RemoveAll(profile)

	profileURL := (&url.URL{Scheme: "file", Path: profile}).String()
	args := []string{
		"--headless", "--norestore", "--invisible",
		"-env:UserInstallation=" + profileURL,
	}
	// PDF opens in Draw by default, and Draw cannot write Writer formats such
	// as DOCX. The Writer PDF import filter reconstructs an editable document
	// first, which is what a PDF→DOCX job actually promises.
	if strings.EqualFold(filepath.Ext(input), ".pdf") && !strings.EqualFold(targetExt, "pdf") {
		args = append(args, "--infilter=writer_pdf_import")
	}
	args = append(args,
		"--convert-to", targetExt,
		"--outdir", outDir,
		input,
	)
	cmd := exec.CommandContext(ctx, binary, args...)
	executil.Configure(cmd)
	cmd.WaitDelay = 10 * time.Second

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	rep.Stage("converting with LibreOffice")
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		text := strings.TrimSpace(stderr.String())
		if text == "" {
			text = strings.TrimSpace(stdout.String())
		}
		if text != "" {
			rep.Log(core.LevelError, "libreoffice: %s", text)
		}
		return "", core.Wrap(core.CodeEngineFailed, "", input,
			fmt.Errorf("soffice failed: %s", firstLine([]byte(text))), hintFor(text))
	}

	if expected := filepath.Join(outDir, strings.TrimSuffix(filepath.Base(input), filepath.Ext(input))+"."+targetExt); fileExists(expected) {
		return expected, nil
	}
	if found := newestWithExt(outDir, targetExt); found != "" {
		return found, nil
	}
	return "", core.NewError(core.CodeOutput, "", input,
		"LibreOffice reported success but produced no output",
		"check that the input is a document LibreOffice can open")
}

func firstLine(out []byte) string {
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(lines) == 0 {
		return ""
	}
	return strings.TrimSpace(lines[0])
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func newestWithExt(dir, ext string) string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}
	var newest string
	var newestTime time.Time
	for _, entry := range entries {
		if entry.IsDir() || !strings.EqualFold(filepath.Ext(entry.Name()), "."+ext) {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		if info.ModTime().After(newestTime) {
			newest = filepath.Join(dir, entry.Name())
			newestTime = info.ModTime()
		}
	}
	return newest
}

func hintFor(stderr string) string {
	lower := strings.ToLower(stderr)
	switch {
	case strings.Contains(lower, "password"), strings.Contains(lower, "encrypted"):
		return "the document is encrypted; remove the password first"
	case strings.Contains(lower, "no such file"):
		return "check that the input file still exists"
	case strings.Contains(lower, "permission denied"):
		return "check permissions on the input and output paths"
	default:
		return "try opening the document in LibreOffice once, or choose a different target format"
	}
}

func InstallHint() string {
	switch runtime.GOOS {
	case "darwin":
		return "brew install --cask libreoffice"
	case "linux":
		return "sudo apt install libreoffice   # or your distribution's package manager"
	case "windows":
		return "winget install TheDocumentFoundation.LibreOffice"
	default:
		return "install LibreOffice and ensure soffice is on PATH"
	}
}
