package doc

import (
	"context"
	"fmt"
	"strings"

	"github.com/solomonadzape95/workbench/internal/core"
	"github.com/solomonadzape95/workbench/internal/engines"
)

// ExtractText is the discoverable text-only entry point. It delegates to the
// same converter the general document op uses, so there is no second
// conversion path to drift.
var extractTextSpec = func() core.Spec {
	spec := convertSpec
	spec.ID = "doc.extract_text"
	spec.Title = "Extract document text"
	spec.Summary = "Convert PDF, DOCX, HTML, EPUB, or Markdown to plain text"
	spec.Params = []core.ParamSpec{
		{
			Key: "format", Label: "Output", Type: core.ParamEnum, Default: "txt",
			Options: []core.Option{
				{Value: "txt", Label: "Plain text"},
				{Value: "md", Label: "Markdown"},
			},
		},
		convertSpec.Params[1],
	}
	spec.Output = core.OutputSpec{FormatParam: "format", Kind: core.Document}
	spec.Engines = []core.EngineID{engines.LibreOffice, engines.Pandoc}
	return spec
}()

type ExtractText struct {
	Convert Convert
}

func (e ExtractText) Spec() core.Spec { return extractTextSpec }

func (e ExtractText) Validate(ctx context.Context, req core.Request) error {
	if err := core.Validate(extractTextSpec.Params, req.Params); err != nil {
		return err
	}
	format := strings.ToLower(req.Params.String("format"))
	if format != "txt" && format != "md" {
		return core.NewError(core.CodeUsage, extractTextSpec.ID, "",
			fmt.Sprintf("unsupported text format %q", format), "choose txt or md")
	}
	inner := req
	inner.Params = core.Values{"format": format, "engine": req.Params.String("engine")}
	if err := e.Convert.Validate(ctx, inner); err != nil {
		return err
	}
	return nil
}

func (e ExtractText) Run(ctx context.Context, req core.Request, rep core.Reporter) (core.Result, error) {
	format := strings.ToLower(req.Params.String("format"))
	inner := req
	inner.Params = core.Values{"format": format, "engine": req.Params.String("engine")}
	return e.Convert.Run(ctx, inner, rep)
}
