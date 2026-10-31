package poppler

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/solomonadzape95/workbench/internal/core"
	"github.com/solomonadzape95/workbench/internal/engines/executil"
)

type Client struct {
	Path string
}

func NewClient(path string) *Client { return &Client{Path: strings.TrimSpace(path)} }

func (c *Client) ResolvePdfToPpm() (string, error) {
	name := "pdftoppm"
	if c.Path != "" && filepath.Dir(c.Path) != "." {
		name = filepath.Join(filepath.Dir(c.Path), "pdftoppm")
	}
	resolved, err := exec.LookPath(name)
	if err != nil {
		return "", core.NewError(core.CodeEngineMissing, "doc.ocr", "",
			"pdftoppm is not installed", InstallHint())
	}
	return resolved, nil
}

// RenderPages rasterizes a PDF into PNG page images and returns them in order.
func (c *Client) RenderPages(ctx context.Context, input, outDir string, dpi int, rep core.Reporter) ([]string, error) {
	binary, err := c.ResolvePdfToPpm()
	if err != nil {
		return nil, err
	}
	if dpi <= 0 {
		dpi = 300
	}
	prefix := filepath.Join(outDir, "page")
	cmd := exec.CommandContext(ctx, binary, "-r", strconv.Itoa(dpi), "-png", input, prefix)
	executil.Configure(cmd)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if rep != nil {
		rep.Stage("rasterizing PDF")
	}
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, core.Wrap(core.CodeEngineFailed, "doc.ocr", input,
			execErr(err, stderr.String()), "check that the PDF is readable")
	}
	pages, err := filepath.Glob(prefix + "-*.png")
	if err != nil {
		return nil, err
	}
	sort.Strings(pages)
	if len(pages) == 0 {
		return nil, core.NewError(core.CodeEngineFailed, "doc.ocr", input,
			"pdftoppm produced no pages", "check that the PDF has pages")
	}
	return pages, nil
}

func execErr(err error, stderr string) error {
	msg := strings.TrimSpace(stderr)
	if msg == "" {
		return err
	}
	if len(msg) > 400 {
		msg = msg[len(msg)-400:]
	}
	return &popplerError{err: err, detail: msg}
}

type popplerError struct {
	err    error
	detail string
}

func (e *popplerError) Error() string { return e.err.Error() + ": " + e.detail }
func (e *popplerError) Unwrap() error { return e.err }

func (c *Client) ResolvePdfSeparate() (string, error) {
	name := "pdfseparate"
	if c.Path != "" && filepath.Dir(c.Path) != "." {
		name = filepath.Join(filepath.Dir(c.Path), "pdfseparate")
	}
	resolved, err := exec.LookPath(name)
	if err != nil {
		return "", core.NewError(core.CodeEngineMissing, "doc.split", "",
			"pdfseparate is not installed", InstallHint())
	}
	return resolved, nil
}

func (c *Client) ResolvePdfUnite() (string, error) {
	name := "pdfunite"
	if c.Path != "" && filepath.Dir(c.Path) != "." {
		name = filepath.Join(filepath.Dir(c.Path), "pdfunite")
	}
	resolved, err := exec.LookPath(name)
	if err != nil {
		return "", core.NewError(core.CodeEngineMissing, "doc.split", "",
			"pdfunite is not installed", InstallHint())
	}
	return resolved, nil
}

// SplitPages writes page ranges into targetDir. every groups pages; ranges
// overrides every with 1-based groups such as "1-3,5,7-9".
func (c *Client) SplitPages(ctx context.Context, input, targetDir, prefix string, every int, ranges string, rep core.Reporter) ([]string, error) {
	separate, err := c.ResolvePdfSeparate()
	if err != nil {
		return nil, err
	}
	temp, err := os.MkdirTemp(targetDir, ".split-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(temp)
	pattern := filepath.Join(temp, "page-%d.pdf")
	cmd := exec.CommandContext(ctx, separate, input, pattern)
	executil.Configure(cmd)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if rep != nil {
		rep.Stage("splitting PDF")
	}
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, core.Wrap(core.CodeEngineFailed, "doc.split", input, execErr(err, stderr.String()), "")
	}
	pages, err := filepath.Glob(filepath.Join(temp, "page-*.pdf"))
	if err != nil {
		return nil, err
	}
	sort.Slice(pages, func(i, j int) bool { return pageNumber(pages[i]) < pageNumber(pages[j]) })
	if len(pages) == 0 {
		return nil, core.NewError(core.CodeEngineFailed, "doc.split", input, "pdfseparate produced no pages", "")
	}
	groups, err := pageGroups(pages, every, ranges)
	if err != nil {
		return nil, err
	}
	var out []string
	for i, group := range groups {
		name := fmt.Sprintf("%s-page%03d.pdf", prefix, i+1)
		if len(group) > 1 || ranges != "" {
			name = fmt.Sprintf("%s-part%03d.pdf", prefix, i+1)
		}
		path := filepath.Join(targetDir, name)
		if len(group) == 1 {
			if err := copyFile(group[0], path); err != nil {
				return nil, err
			}
		} else {
			unite, err := c.ResolvePdfUnite()
			if err != nil {
				return nil, err
			}
			args := append(append([]string{}, group...), path)
			ucmd := exec.CommandContext(ctx, unite, args...)
			executil.Configure(ucmd)
			var ustderr bytes.Buffer
			ucmd.Stderr = &ustderr
			if err := ucmd.Run(); err != nil {
				if ctx.Err() != nil {
					return nil, ctx.Err()
				}
				return nil, core.Wrap(core.CodeEngineFailed, "doc.split", input, execErr(err, ustderr.String()), "")
			}
		}
		out = append(out, path)
		if rep != nil {
			rep.Progress(i+1, len(groups))
		}
	}
	return out, nil
}

func pageGroups(pages []string, every int, ranges string) ([][]string, error) {
	total := len(pages)
	if strings.TrimSpace(ranges) != "" {
		var groups [][]string
		for _, part := range strings.Split(ranges, ",") {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			from, to := 0, 0
			if strings.Contains(part, "-") {
				if _, err := fmt.Sscanf(part, "%d-%d", &from, &to); err != nil {
					return nil, fmt.Errorf("invalid page range %q", part)
				}
			} else {
				if _, err := fmt.Sscanf(part, "%d", &from); err != nil {
					return nil, fmt.Errorf("invalid page range %q", part)
				}
				to = from
			}
			if from < 1 || to < from || to > total {
				return nil, fmt.Errorf("page range %q is outside 1-%d", part, total)
			}
			groups = append(groups, pages[from-1:to])
		}
		return groups, nil
	}
	if every <= 0 {
		every = 1
	}
	var groups [][]string
	for start := 0; start < total; start += every {
		end := min(start+every, total)
		groups = append(groups, pages[start:end])
	}
	return groups, nil
}

func pageNumber(path string) int {
	base := filepath.Base(path)
	base = strings.TrimSuffix(strings.TrimPrefix(base, "page-"), ".pdf")
	n, _ := strconv.Atoi(base)
	return n
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(out, in)
	closeErr := out.Close()
	if copyErr != nil {
		return copyErr
	}
	return closeErr
}
