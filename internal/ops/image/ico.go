package image

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"image"
	"image/png"
	"io"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	xdraw "golang.org/x/image/draw"

	"github.com/solomonadzape95/workbench/internal/core"
	"github.com/solomonadzape95/workbench/internal/engines"
)

var toICOSpec = core.Spec{
	ID:      "image.to_ico",
	Title:   "Convert to ICO",
	Summary: "Render a multi-size Windows icon from an image",
	Domain:  core.Image,
	Accepts: []core.Accept{{Kind: core.Image, Formats: []string{"png", "jpg", "jpeg", "gif"}}},
	Params: []core.ParamSpec{
		{
			Key: "sizes", Label: "Sizes", Type: core.ParamString, Default: "16,32,64,128,256",
			Help: "Comma-separated square sizes in pixels; each must be 1–256.",
		},
	},
	Output:    core.OutputSpec{StaticFormat: "ico", Kind: core.Image},
	Engines:   []core.EngineID{engines.Internal},
	Batchable: true,
	Cost:      core.CostMedium,
}

type ToICO struct{}

func (ToICO) Spec() core.Spec { return toICOSpec }

func (ToICO) Validate(ctx context.Context, req core.Request) error {
	if err := core.Validate(toICOSpec.Params, req.Params); err != nil {
		return err
	}
	if len(req.Items) == 0 {
		return core.NewError(core.CodeUsage, toICOSpec.ID, "", "no input files", "")
	}
	if _, err := parseICOSizes(req.Params.String("sizes")); err != nil {
		return err
	}
	for _, item := range req.Items {
		if !toICOSpec.AcceptsArtifact(item) {
			return core.NewError(core.CodeInput, toICOSpec.ID, item.Path,
				fmt.Sprintf("%s is not a supported image format", item.Format),
				"supported inputs: PNG, JPEG, GIF")
		}
	}
	return nil
}

func (ToICO) Run(ctx context.Context, req core.Request, rep core.Reporter) (core.Result, error) {
	start := time.Now()
	sizes, err := parseICOSizes(req.Params.String("sizes"))
	if err != nil {
		return core.Result{}, err
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
			return core.Result{}, core.Wrap(core.CodeInput, toICOSpec.ID, item.Path, err,
				"the file may be corrupt or use an unsupported image encoding")
		}

		rep.Stage("rendering icons")
		icons := make([]*image.RGBA, len(sizes))
		for i, size := range sizes {
			icons[i] = iconFrame(img, size)
		}

		outPath := filepath.Join(req.Target.Dir, core.SanitizeStem(item.Stem())+".ico")
		rep.Stage("writing icon")
		if err := writeICO(outPath, sizes, icons); err != nil {
			return core.Result{}, core.Wrap(core.CodeOutput, toICOSpec.ID, item.Path, err,
				"check that the output directory is writable")
		}

		info, err := os.Stat(outPath)
		if err != nil {
			return core.Result{}, core.Wrap(core.CodeOutput, toICOSpec.ID, outPath, err, "")
		}
		largest := 0
		for _, size := range sizes {
			largest = max(largest, size)
		}
		out = append(out, core.Artifact{
			Path:   outPath,
			Kind:   core.Image,
			Format: "ico",
			Size:   info.Size(),
			Meta: map[string]any{
				"width":  largest,
				"height": largest,
				"sizes":  append([]int(nil), sizes...),
				"source": item.Path,
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
			Engine:   engines.Internal,
		},
	}, nil
}

// parseICOSizes turns "16,32,64" into a sorted, de-duplicated list of icon
// sizes. Each entry must fit ICO's one-byte dimension field.
func parseICOSizes(raw string) ([]int, error) {
	if strings.TrimSpace(raw) == "" {
		raw = "16,32,64,128,256"
	}
	var sizes []int
	seen := make(map[int]bool)
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		n, err := strconv.Atoi(part)
		if err != nil {
			return nil, core.NewError(core.CodeUsage, toICOSpec.ID, "",
				fmt.Sprintf("invalid icon size %q", part), "use comma-separated integers such as 16,32,64")
		}
		if n < 1 || n > 256 {
			return nil, core.NewError(core.CodeUsage, toICOSpec.ID, "",
				fmt.Sprintf("icon size %d is out of range", n), "use sizes between 1 and 256")
		}
		if seen[n] {
			continue
		}
		seen[n] = true
		sizes = append(sizes, n)
	}
	if len(sizes) == 0 {
		return nil, core.NewError(core.CodeUsage, toICOSpec.ID, "",
			"no icon sizes given", "use comma-separated integers such as 16,32,64")
	}
	if len(sizes) > 32 {
		return nil, core.NewError(core.CodeUsage, toICOSpec.ID, "",
			"too many icon sizes", "use at most 32 sizes")
	}
	return sizes, nil
}

// iconFrame scales img to fit inside a transparent size x size square without
// distorting its aspect ratio.
func iconFrame(src image.Image, size int) *image.RGBA {
	bounds := src.Bounds()
	srcW, srcH := bounds.Dx(), bounds.Dy()
	scale := math.Min(float64(size)/float64(srcW), float64(size)/float64(srcH))
	targetW := max(1, int(math.Round(float64(srcW)*scale)))
	targetH := max(1, int(math.Round(float64(srcH)*scale)))

	scaled := image.NewRGBA(image.Rect(0, 0, targetW, targetH))
	xdraw.CatmullRom.Scale(scaled, scaled.Bounds(), src, bounds, xdraw.Src, nil)

	dst := image.NewRGBA(image.Rect(0, 0, size, size))
	offset := image.Pt((size-targetW)/2, (size-targetH)/2)
	rect := image.Rect(offset.X, offset.Y, offset.X+targetW, offset.Y+targetH)
	xdraw.Draw(dst, rect, scaled, image.Point{}, xdraw.Over)
	return dst
}

// writeICO writes an ICONDIR followed by PNG-compressed entries.
func writeICO(path string, sizes []int, icons []*image.RGBA) (err error) {
	if len(sizes) != len(icons) {
		return fmt.Errorf("ico: %d sizes for %d images", len(sizes), len(icons))
	}

	var pngData [][]byte
	for _, icon := range icons {
		var buf bytes.Buffer
		if err := png.Encode(&buf, icon); err != nil {
			return err
		}
		pngData = append(pngData, buf.Bytes())
	}

	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer func() {
		closeErr := f.Close()
		if err == nil {
			err = closeErr
		}
		if err != nil {
			_ = os.Remove(path)
		}
	}()

	if err := writeICOTo(f, sizes, pngData); err != nil {
		return err
	}
	return nil
}

func writeICOTo(w io.Writer, sizes []int, pngData [][]byte) error {
	const dirSize = 6
	const entrySize = 16
	offset := dirSize + entrySize*len(sizes)

	header := make([]byte, dirSize)
	binary.LittleEndian.PutUint16(header[0:], 0) // reserved
	binary.LittleEndian.PutUint16(header[2:], 1) // type: icon
	binary.LittleEndian.PutUint16(header[4:], uint16(len(sizes)))
	if _, err := w.Write(header); err != nil {
		return err
	}

	for i, size := range sizes {
		entry := make([]byte, entrySize)
		entry[0] = icoDimension(size)
		entry[1] = icoDimension(size)
		entry[2] = 0                                 // palette size
		entry[3] = 0                                 // reserved
		binary.LittleEndian.PutUint16(entry[4:], 1)  // color planes
		binary.LittleEndian.PutUint16(entry[6:], 32) // bits per pixel
		binary.LittleEndian.PutUint32(entry[8:], uint32(len(pngData[i])))
		binary.LittleEndian.PutUint32(entry[12:], uint32(offset))
		if _, err := w.Write(entry); err != nil {
			return err
		}
		offset += len(pngData[i])
	}

	for _, data := range pngData {
		if _, err := w.Write(data); err != nil {
			return err
		}
	}
	return nil
}

func icoDimension(size int) byte {
	if size >= 256 {
		return 0
	}
	return byte(size)
}
