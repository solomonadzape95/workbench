// Package doc implements document conversion through LibreOffice and pandoc.
package doc

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/solomonadzape95/workbench/internal/core"
	"github.com/solomonadzape95/workbench/internal/engines"
	"github.com/solomonadzape95/workbench/internal/engines/libreoffice"
	"github.com/solomonadzape95/workbench/internal/engines/pandoc"
	"github.com/solomonadzape95/workbench/internal/engines/pdf2docx"
)

var convertSpec = core.Spec{
	ID:      "doc.convert",
	Title:   "Convert document",
	Summary: "Convert PDF, DOCX, ODT, Markdown, HTML, EPUB, and text",
	Domain:  core.Document,
	Accepts: []core.Accept{{Kind: core.Document, Formats: []string{
		"pdf", "doc", "docx", "odt", "rtf", "md", "markdown", "html", "htm", "txt", "epub",
	}}},
	Params: []core.ParamSpec{
		{
			Key: "format", Label: "Format", Type: core.ParamEnum, Required: true, Default: "docx",
			Options: []core.Option{
				{Value: "pdf", Label: "PDF"},
				{Value: "docx", Label: "DOCX"},
				{Value: "odt", Label: "ODT"},
				{Value: "md", Label: "Markdown"},
				{Value: "html", Label: "HTML"},
				{Value: "epub", Label: "EPUB"},
				{Value: "txt", Label: "Plain text"},
			},
		},
		{
			Key: "engine", Label: "Engine", Type: core.ParamEnum, Default: "auto", Options: []core.Option{
				{Value: "auto", Label: "Auto"},
				{Value: "libreoffice", Label: "LibreOffice"},
				{Value: "pandoc", Label: "pandoc"},
			}, Advanced: true,
			Help: "Auto prefers pandoc for text formats and LibreOffice for PDF/DOCX.",
		},
	},
	Output:    core.OutputSpec{FormatParam: "format", Kind: core.Document},
	Engines:   []core.EngineID{engines.PDF2DOCX, engines.LibreOffice, engines.Pandoc},
	Batchable: true,
	Cost:      core.CostMedium,
}

type Convert struct {
	LibreOffice *libreoffice.Client
	Pandoc      *pandoc.Client
	PDF2DOCX    *pdf2docx.Client
}

func (c Convert) Spec() core.Spec { return convertSpec }

func (c Convert) Validate(ctx context.Context, req core.Request) error {
	if err := core.Validate(convertSpec.Params, req.Params); err != nil {
		return err
	}
	if len(req.Items) == 0 {
		return core.NewError(core.CodeUsage, convertSpec.ID, "", "no input files", "")
	}
	target := normalizeTarget(req.Params.String("format"))
	preferred := req.Params.String("engine")
	for _, item := range req.Items {
		if !convertSpec.AcceptsArtifact(item) {
			return core.NewError(core.CodeInput, convertSpec.ID, item.Path,
				fmt.Sprintf("%s is not a supported document format", item.Format),
				"supported inputs: PDF, DOC, DOCX, ODT, RTF, Markdown, HTML, TXT, EPUB")
		}
		if _, err := c.choose(item, target, preferred); err != nil {
			return err
		}
	}
	return nil
}

func (c Convert) Run(ctx context.Context, req core.Request, rep core.Reporter) (core.Result, error) {
	start := time.Now()
	target := normalizeTarget(req.Params.String("format"))
	preferred := req.Params.String("engine")

	var out []core.Artifact
	var bytesIn, bytesOut int64
	for _, item := range req.Items {
		if err := ctx.Err(); err != nil {
			return core.Result{}, err
		}
		engine, err := c.choose(item, target, preferred)
		if err != nil {
			return core.Result{}, err
		}

		var output string
		switch engine {
		case "pdf2docx":
			output = filepath.Join(req.Target.Dir, core.SanitizeStem(item.Stem())+"."+core.ExtFor(target, item))
			if err := c.PDF2DOCX.Convert(ctx, item.Path, output, rep); err != nil {
				if ctx.Err() != nil {
					return core.Result{}, ctx.Err()
				}
				return core.Result{}, core.Wrap(core.CodeEngineFailed, convertSpec.ID, item.Path, err, core.HintOf(err))
			}
		case "pandoc":
			output = filepath.Join(req.Target.Dir, core.SanitizeStem(item.Stem())+"."+core.ExtFor(target, item))
			if err := c.Pandoc.Convert(ctx, item.Path, output, rep); err != nil {
				if ctx.Err() != nil {
					return core.Result{}, ctx.Err()
				}
				return core.Result{}, core.Wrap(core.CodeEngineFailed, convertSpec.ID, item.Path, err, core.HintOf(err))
			}
		default:
			output, err = c.LibreOffice.Convert(ctx, item.Path, req.Target.Dir, target, rep)
			if err != nil {
				if ctx.Err() != nil {
					return core.Result{}, ctx.Err()
				}
				return core.Result{}, core.Wrap(core.CodeEngineFailed, convertSpec.ID, item.Path, err, core.HintOf(err))
			}
		}

		info, err := os.Stat(output)
		if err != nil {
			return core.Result{}, core.Wrap(core.CodeOutput, convertSpec.ID, output, err,
				"the converter reported success but produced no output")
		}
		out = append(out, core.Artifact{
			Path:   output,
			Kind:   core.Document,
			Format: target,
			Size:   info.Size(),
			Meta: map[string]any{
				"source": item.Path,
				"engine": engine,
			},
		})
		bytesIn += item.Size
		bytesOut += info.Size()
		rep.Progress(1, 1)
	}

	return core.Result{
		Artifacts: out,
		Metrics: core.Metrics{
			BytesIn:  bytesIn,
			BytesOut: bytesOut,
			Duration: time.Since(start),
		},
	}, nil
}

func (c Convert) choose(item core.Artifact, target, preferred string) (string, error) {
	src := strings.ToLower(strings.TrimPrefix(item.Ext(), "."))
	loAvailable := c.LibreOffice != nil && c.LibreOffice.Check() == nil
	pdAvailable := c.Pandoc != nil && c.Pandoc.Check() == nil
	p2dAvailable := c.PDF2DOCX != nil && c.PDF2DOCX.Check() == nil
	loSupports := supportsLibreOffice(src, target)
	pdSupports := c.Pandoc != nil && c.Pandoc.Supports(src, target)
	p2dSupports := src == "pdf" && target == "docx"

	switch preferred {
	case "pdf2docx":
		if p2dSupports && p2dAvailable {
			return "pdf2docx", nil
		}
		if !p2dSupports {
			return "", c.unsupported(item, src, target, "pdf2docx only converts PDF→DOCX")
		}
		return "", core.NewError(core.CodeEngineMissing, convertSpec.ID, item.Path,
			"pdf2docx is not installed", pdf2docx.InstallHint())
	case "libreoffice":
		if loSupports && loAvailable {
			return "libreoffice", nil
		}
		if !loSupports {
			return "", c.unsupported(item, src, target, "LibreOffice cannot convert this pair")
		}
		return "", core.NewError(core.CodeEngineMissing, convertSpec.ID, item.Path,
			"LibreOffice is not installed", libreoffice.InstallHint())
	case "pandoc":
		if pdSupports && pdAvailable {
			return "pandoc", nil
		}
		if !pdSupports {
			return "", c.unsupported(item, src, target, "pandoc cannot convert this pair")
		}
		return "", core.NewError(core.CodeEngineMissing, convertSpec.ID, item.Path,
			"pandoc is not installed", pandoc.InstallHint())
	}

	// pdf2docx is the better default for editable PDFs: it reconstructs tables
	// and images instead of turning the page into positioned text frames.
	if p2dSupports && p2dAvailable {
		return "pdf2docx", nil
	}
	if pdSupports && pdAvailable {
		return "pandoc", nil
	}
	if loSupports && loAvailable {
		return "libreoffice", nil
	}
	if !loAvailable && !pdAvailable && !p2dAvailable {
		return "", core.NewError(core.CodeEngineMissing, convertSpec.ID, item.Path,
			"no document conversion engine is installed",
			"install LibreOffice ("+libreoffice.InstallHint()+") or pandoc ("+pandoc.InstallHint()+")")
	}
	return "", c.unsupported(item, src, target, "no available engine supports this pair")
}

func (c Convert) unsupported(item core.Artifact, src, target, reason string) error {
	hint := "try a different target format"
	switch {
	case src == "pdf" && target == "md":
		hint = "convert PDF→DOCX with pdf2docx first, then DOCX→Markdown with pandoc"
	case src == "pdf" && (target == "html" || target == "epub" || target == "txt"):
		hint = "install LibreOffice, or convert PDF→DOCX first"
	case target == "pdf" && (src == "md" || src == "markdown"):
		hint = "convert Markdown→DOCX first, then DOCX→PDF with LibreOffice"
	case target == "md" && c.Pandoc == nil:
		hint = "install pandoc for Markdown output"
	}
	return core.NewError(core.CodeUsage, convertSpec.ID, item.Path,
		fmt.Sprintf("%s: %s→%s", reason, src, target), hint)
}

func supportsLibreOffice(src, target string) bool {
	switch target {
	case "pdf", "docx", "odt", "html", "txt", "epub":
	default:
		return false
	}
	switch src {
	case "pdf", "doc", "docx", "odt", "rtf", "txt", "html", "htm", "epub":
		return true
	default:
		return false
	}
}

func normalizeTarget(target string) string {
	target = strings.ToLower(strings.TrimPrefix(target, "."))
	switch target {
	case "markdown":
		return "md"
	case "htm":
		return "html"
	default:
		return target
	}
}
