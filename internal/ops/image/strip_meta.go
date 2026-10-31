package image

import (
	"context"
	"fmt"
	"time"

	"github.com/solomonadzape95/workbench/internal/core"
	"github.com/solomonadzape95/workbench/internal/engines"
)

var stripMetaSpec = core.Spec{
	ID:      "image.strip_meta",
	Title:   "Strip metadata",
	Summary: "Re-encode an image so EXIF and other ancillary metadata are dropped",
	Domain:  core.Image,
	Accepts: []core.Accept{{Kind: core.Image, Formats: []string{"png", "jpg", "jpeg", "gif"}}},
	Params: []core.ParamSpec{
		{
			Key: "format", Label: "Format", Type: core.ParamEnum, Default: "auto",
			Options: []core.Option{
				{Value: "auto", Label: "Keep source format"},
				{Value: "png", Label: "PNG"},
				{Value: "jpeg", Label: "JPEG"},
				{Value: "gif", Label: "GIF"},
			},
			Help: "The image is decoded and re-encoded, which drops EXIF, ICC, and text chunks.",
		},
		{
			Key: "quality", Label: "JPEG quality", Type: core.ParamInt, Default: 90,
			Min: floatPtr(1), Max: floatPtr(100), Advanced: true,
			Help: "1–100, only used for JPEG output",
		},
	},
	Engines:   []core.EngineID{engines.Internal},
	Batchable: true,
	Cost:      core.CostFast,
}

type StripMeta struct{}

func (StripMeta) Spec() core.Spec { return stripMetaSpec }

func (StripMeta) Validate(ctx context.Context, req core.Request) error {
	if err := core.Validate(stripMetaSpec.Params, req.Params); err != nil {
		return err
	}
	if len(req.Items) == 0 {
		return core.NewError(core.CodeUsage, stripMetaSpec.ID, "", "no input files", "")
	}
	format := normalizeFormat(req.Params.String("format"))
	if format != "auto" && !supportedFormat(format) {
		return core.NewError(core.CodeUsage, stripMetaSpec.ID, "",
			fmt.Sprintf("unsupported output format %q", req.Params.String("format")),
			"choose auto, png, jpeg, or gif")
	}
	for _, item := range req.Items {
		if !stripMetaSpec.AcceptsArtifact(item) {
			return core.NewError(core.CodeInput, stripMetaSpec.ID, item.Path,
				fmt.Sprintf("%s is not a supported image format", item.Format),
				"supported inputs: PNG, JPEG, GIF")
		}
	}
	return nil
}

func (StripMeta) Run(ctx context.Context, req core.Request, rep core.Reporter) (core.Result, error) {
	start := time.Now()
	requested := normalizeFormat(req.Params.String("format"))
	quality := req.Params.Int("quality")
	if quality == 0 {
		quality = 90
	}

	var out []core.Artifact
	var bytesIn, bytesOut int64
	for _, item := range req.Items {
		if err := ctx.Err(); err != nil {
			return core.Result{}, err
		}

		rep.Stage("decoding")
		rep.Progress(0, 1)
		img, _, err := decode(item.Path)
		if err != nil {
			return core.Result{}, core.Wrap(core.CodeInput, stripMetaSpec.ID, item.Path, err,
				"the file may be corrupt or use an unsupported image encoding")
		}

		format := requested
		if format == "" || format == "auto" {
			format = normalizeFormat(item.Format)
		}
		if !supportedFormat(format) {
			format = "png"
		}

		rep.Stage("re-encoding")
		art, err := writeOutput(req, item, img, format, quality)
		if err != nil {
			return core.Result{}, err
		}
		art.Meta["metadata_stripped"] = true
		out = append(out, art)
		bytesIn += item.Size
		bytesOut += art.Size
		rep.Progress(1, 1)
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
