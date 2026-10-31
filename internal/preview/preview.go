// Package preview renders best-effort terminal representations of files.
// It never mutates the file and is always safe to run in a background command.
package preview

import (
	"bytes"
	"context"
	"encoding/hex"
	"fmt"
	"image"
	"image/color"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	xdraw "golang.org/x/image/draw"
)

type Kind string

const (
	KindImage    Kind = "image"
	KindText     Kind = "text"
	KindPDF      Kind = "pdf"
	KindBinary   Kind = "binary"
	KindMetadata Kind = "metadata"
)

type Preview struct {
	Kind Kind
	Text string
	ANSI string
	Meta []string
}

// Generate builds a preview sized for width columns and height rows.
func Generate(path string, width, height int) (Preview, error) {
	if width < 10 {
		width = 10
	}
	if height < 3 {
		height = 3
	}

	info, err := os.Stat(path)
	if err != nil {
		return Preview{}, err
	}
	if info.IsDir() {
		return Preview{Kind: KindMetadata, Text: "directory", Meta: []string{path}}, nil
	}

	ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(path), "."))
	meta := []string{fmt.Sprintf("%s · %s", strings.ToUpper(ext), humanBytes(info.Size()))}

	switch ext {
	case "png", "jpg", "jpeg", "gif", "bmp", "tif", "tiff":
		if p, err := imagePreview(path, width, height, meta); err == nil {
			return p, nil
		}
		// Fall through to a text/hex preview for mislabeled images.
	case "pdf":
		return pdfPreview(path, height, meta), nil
	}

	data, err := readHead(path, 64<<10)
	if err != nil {
		return Preview{}, err
	}
	if isTextData(ext, data) {
		return textPreview(data, width, height, meta), nil
	}
	return hexPreview(data, width, height, meta), nil
}

func imagePreview(path string, width, height int, meta []string) (Preview, error) {
	f, err := os.Open(path)
	if err != nil {
		return Preview{}, err
	}
	defer f.Close()

	img, format, err := image.Decode(f)
	if err != nil {
		return Preview{}, err
	}
	bounds := img.Bounds()
	srcW, srcH := bounds.Dx(), bounds.Dy()
	meta = append(meta, fmt.Sprintf("%s %dx%d", strings.ToUpper(format), srcW, srcH))
	if srcW == 0 || srcH == 0 {
		return Preview{Kind: KindImage, Text: meta[0], Meta: meta}, nil
	}

	// Two vertical pixels per terminal cell, using half blocks.
	maxH := height * 2
	scale := minFloat(float64(width)/float64(srcW), float64(maxH)/float64(srcH))
	targetW := max(1, int(float64(srcW)*scale+0.5))
	targetH := max(1, int(float64(srcH)*scale+0.5))
	targetW = min(targetW, width)
	targetH = min(targetH, maxH)

	dst := image.NewRGBA(image.Rect(0, 0, targetW, targetH))
	xdraw.CatmullRom.Scale(dst, dst.Bounds(), img, bounds, xdraw.Src, nil)

	var b strings.Builder
	for y := 0; y < targetH; y += 2 {
		for x := 0; x < targetW; x++ {
			top := toRGBA(dst.At(x, y))
			bottom := top
			if y+1 < targetH {
				bottom = toRGBA(dst.At(x, y+1))
			}
			fmt.Fprintf(&b, "\x1b[38;2;%d;%d;%dm\x1b[48;2;%d;%d;%dm▀",
				top.R, top.G, top.B, bottom.R, bottom.G, bottom.B)
		}
		b.WriteString("\x1b[0m\n")
	}

	return Preview{
		Kind: KindImage,
		Text: strings.Join(meta, " · "),
		ANSI: strings.TrimRight(b.String(), "\n"),
		Meta: meta,
	}, nil
}

func pdfPreview(path string, height int, meta []string) Preview {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	out, err := exec.CommandContext(ctx, "pdftotext", "-f", "1", "-l", "2", path, "-").Output()
	if err != nil {
		meta = append(meta, "pdftotext not available — install poppler for PDF text previews")
		return Preview{Kind: KindPDF, Text: strings.Join(meta, "\n"), Meta: meta}
	}
	text := truncateLines(string(out), 120, max(height-2, 2))
	return Preview{Kind: KindPDF, Text: text, Meta: meta}
}

func textPreview(data []byte, width, height int, meta []string) Preview {
	text := truncateLines(string(data), width, max(height-2, 2))
	meta = append(meta, fmt.Sprintf("%d lines shown", strings.Count(text, "\n")+1))
	return Preview{Kind: KindText, Text: text, Meta: meta}
}

func hexPreview(data []byte, width, height int, meta []string) Preview {
	head := data
	if len(head) > 512 {
		head = head[:512]
	}
	dump := hex.Dump(head)
	text := truncateLines(dump, width, max(height-2, 2))
	meta = append(meta, "binary · first 512 bytes")
	return Preview{Kind: KindBinary, Text: text, Meta: meta}
}

func readHead(path string, limit int64) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	buf := make([]byte, limit)
	n, err := f.Read(buf)
	if err != nil && n == 0 {
		return nil, err
	}
	return buf[:n], nil
}

func isTextData(ext string, data []byte) bool {
	switch ext {
	case "txt", "md", "markdown", "log", "json", "toml", "yaml", "yml", "xml", "csv",
		"go", "rs", "py", "js", "ts", "tsx", "jsx", "css", "html", "htm", "sh", "zsh",
		"c", "h", "cpp", "hpp", "java", "kt", "swift", "rb", "php", "sql", "mod", "sum",
		"ini", "conf", "cfg", "env", "srt", "vtt":
		return true
	}
	if bytes.IndexByte(data, 0) >= 0 {
		return false
	}
	return utf8.Valid(data)
}

func truncateLines(text string, width, maxLines int) string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	lines := strings.Split(text, "\n")
	if len(lines) > maxLines {
		lines = lines[:maxLines]
		lines = append(lines, "…")
	}
	for i, line := range lines {
		line = strings.Map(func(r rune) rune {
			if r == '\t' {
				return ' '
			}
			if r < 32 {
				return -1
			}
			return r
		}, line)
		if len([]rune(line)) > width {
			line = string([]rune(line)[:max(width-1, 0)]) + "…"
		}
		lines[i] = line
	}
	return strings.Join(lines, "\n")
}

func toRGBA(c color.Color) color.RGBA {
	r, g, b, _ := c.RGBA()
	return color.RGBA{R: uint8(r >> 8), G: uint8(g >> 8), B: uint8(b >> 8), A: 255}
}

func humanBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	value := float64(n)
	for _, suffix := range []string{"KB", "MB", "GB"} {
		value /= unit
		if value < unit {
			return fmt.Sprintf("%.1f %s", value, suffix)
		}
	}
	return fmt.Sprintf("%.1f TB", value/unit)
}

func minFloat(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}
