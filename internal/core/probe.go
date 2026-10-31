package core

import (
	"os"
	"path/filepath"
	"strings"
)

// Probe builds an Artifact from a path using the extension as the format hint.
// Content sniffing comes later; operations validate what they actually read.
func Probe(path string) (Artifact, error) {
	info, err := os.Stat(path)
	if err != nil {
		return Artifact{}, Wrap(CodeInput, "", path, err, "check that the file exists and is readable")
	}
	if info.IsDir() {
		return Artifact{}, NewError(CodeInput, "", path, "is a directory", "choose files, a glob, or an alias that resolves to files")
	}
	ext := strings.TrimPrefix(strings.ToLower(filepath.Ext(path)), ".")
	kind, format := classify(ext)
	return Artifact{
		Path:   path,
		Kind:   kind,
		Format: format,
		Size:   info.Size(),
		Meta:   map[string]any{"mtime": info.ModTime()},
	}, nil
}

func classify(ext string) (Kind, string) {
	switch ext {
	case "png", "jpg", "jpeg", "gif", "webp", "bmp", "tif", "tiff", "heic", "avif", "svg":
		return Image, ext
	case "mp3", "wav", "flac", "ogg", "m4a", "aac", "opus", "wma":
		return Audio, ext
	case "mp4", "mov", "mkv", "webm", "avi", "m4v", "wmv", "flv":
		return Video, ext
	case "pdf", "doc", "docx", "odt", "rtf", "md", "markdown", "txt", "html", "htm", "epub", "xlsx", "pptx":
		return Document, ext
	case "zip", "tar", "gz", "tgz", "zst", "7z", "rar", "bz2", "xz":
		return Archive, ext
	default:
		return File, ext
	}
}
