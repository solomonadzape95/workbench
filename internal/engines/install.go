package engines

import (
	"context"
	"fmt"
	"os/exec"
	"runtime"
	"strings"

	"github.com/solomonadzape95/workbench/internal/engines/executil"
)

// InstallCommand is one package-manager invocation for an engine. NeedsSudo
// marks commands the TUI should not run unattended; the user gets the command
// to run in their own shell instead.
type InstallCommand struct {
	Args      []string
	NeedsSudo bool
}

// InstallSpec describes how to install one optional engine. It is deliberately
// separate from Engine: the installer must list engines that are not enabled in
// the current config, and it also needs a display name for a picker.
type InstallSpec struct {
	ID          ID
	Name        string
	Summary     string
	Binaries    []string
	Commands    map[string]InstallCommand
	ManualURL   string
	PostInstall string
}

// Command returns the install command for the current platform. The zero value
// means the engine has no automatic installer here.
func (s InstallSpec) Command() InstallCommand {
	if s.Commands == nil {
		return InstallCommand{}
	}
	return s.Commands[runtime.GOOS]
}

// Auto reports whether the picker can run the install itself.
func (s InstallSpec) Auto() bool {
	c := s.Command()
	return len(c.Args) > 0 && !c.NeedsSudo
}

// Hint returns a compact command or URL for display.
func (s InstallSpec) Hint() string {
	if c := s.Command(); len(c.Args) > 0 {
		return strings.Join(c.Args, " ")
	}
	if s.ManualURL != "" {
		return s.ManualURL
	}
	return "install manually and add it to PATH"
}

// Installed is a cheap PATH check used when the engine is not in the current
// registry. It is intentionally conservative: every listed binary must exist.
func (s InstallSpec) Installed() bool {
	if len(s.Binaries) == 0 {
		return false
	}
	for _, name := range s.Binaries {
		if _, err := exec.LookPath(name); err != nil {
			return false
		}
	}
	return true
}

// InstallCatalog is the set of optional engines Workbench can use. Keep it in
// sync with the engine constants and the registry in internal/app.
func InstallCatalog() []InstallSpec {
	brew := func(pkg string) InstallCommand {
		return InstallCommand{Args: []string{"brew", "install", pkg}}
	}
	apt := func(pkg string) InstallCommand {
		return InstallCommand{Args: []string{"sudo", "apt-get", "install", "-y", pkg}, NeedsSudo: true}
	}
	pip := func(pkg string) InstallCommand {
		return InstallCommand{Args: []string{"python3", "-m", "pip", "install", "--user", pkg}}
	}

	return []InstallSpec{
		{
			ID: FFmpeg, Name: "FFmpeg",
			Summary:  "audio/video conversion, progress, and metadata",
			Binaries: []string{"ffmpeg", "ffprobe"},
			Commands: map[string]InstallCommand{
				"darwin": brew("ffmpeg"),
				"linux":  apt("ffmpeg"),
			},
		},
		{
			ID: YtDlp, Name: "yt-dlp",
			Summary:  "video downloads, captions, and URL metadata",
			Binaries: []string{"yt-dlp"},
			Commands: map[string]InstallCommand{
				"darwin": brew("yt-dlp"),
				"linux":  pip("yt-dlp"),
			},
		},
		{
			ID: LibreOffice, Name: "LibreOffice",
			Summary:  "document conversion and PDF fallback",
			Binaries: []string{"soffice"},
			Commands: map[string]InstallCommand{
				"darwin": {Args: []string{"brew", "install", "--cask", "libreoffice"}},
				"linux":  apt("libreoffice"),
			},
		},
		{
			ID: Pandoc, Name: "pandoc",
			Summary:  "Markdown, HTML, DOCX, and EPUB conversion",
			Binaries: []string{"pandoc"},
			Commands: map[string]InstallCommand{
				"darwin": brew("pandoc"),
				"linux":  apt("pandoc"),
			},
		},
		{
			ID: Tesseract, Name: "Tesseract",
			Summary:  "OCR for scanned images and PDF pages",
			Binaries: []string{"tesseract"},
			Commands: map[string]InstallCommand{
				"darwin": brew("tesseract"),
				"linux":  apt("tesseract-ocr"),
			},
		},
		{
			ID: Ghostscript, Name: "Ghostscript",
			Summary:  "PDF compression and merging",
			Binaries: []string{"gs"},
			Commands: map[string]InstallCommand{
				"darwin": brew("ghostscript"),
				"linux":  apt("ghostscript"),
			},
		},
		{
			ID: PDF2DOCX, Name: "pdf2docx",
			Summary:  "PDF→DOCX with tables and images",
			Binaries: []string{"pdf2docx"},
			Commands: map[string]InstallCommand{
				"darwin": pip("pdf2docx"),
				"linux":  pip("pdf2docx"),
			},
		},
		{
			ID: Poppler, Name: "Poppler",
			Summary:  "PDF text extraction, splitting, and previews",
			Binaries: []string{"pdftotext"},
			Commands: map[string]InstallCommand{
				"darwin": brew("poppler"),
				"linux":  apt("poppler-utils"),
			},
		},
		{
			ID: Whisper, Name: "whisper.cpp",
			Summary:  "local speech-to-text and subtitles",
			Binaries: []string{"whisper-cli"},
			Commands: map[string]InstallCommand{
				"darwin": brew("whisper-cpp"),
			},
			ManualURL:   "https://github.com/ggml-org/whisper.cpp",
			PostInstall: "download a ggml model, e.g. small.en, into ~/.local/share/whisper-models",
		},
		{
			ID: Rembg, Name: "rembg",
			Summary:  "background removal; the first run downloads a model",
			Binaries: []string{"rembg"},
			Commands: map[string]InstallCommand{
				"darwin": pip(`rembg[cli,cpu]`),
				"linux":  pip(`rembg[cli,cpu]`),
			},
		},
		{
			ID: Demucs, Name: "Demucs",
			Summary:  "vocal and instrumental stem separation",
			Binaries: []string{"demucs"},
			Commands: map[string]InstallCommand{
				"darwin": pip("demucs"),
				"linux":  pip("demucs"),
			},
		},
		{
			ID: RealESRGAN, Name: "Real-ESRGAN",
			Summary:     "AI upscaling with realesrgan-ncnn-vulkan",
			Binaries:    []string{"realesrgan-ncnn-vulkan"},
			ManualURL:   "https://github.com/xinntao/Real-ESRGAN/releases",
			PostInstall: "put realesrgan-ncnn-vulkan on PATH",
		},
		{
			ID: OpenCode, Name: "OpenCode",
			Summary:     "local AI rewriting, translation, and summaries",
			Binaries:    []string{"opencode"},
			ManualURL:   "https://opencode.ai",
			PostInstall: "install OpenCode, sign in, then run `opencode service start`",
		},
	}
}

// RunInstall executes one install command and returns its combined output. The
// output is clipped from the front so the useful tail survives a long brew log.
func RunInstall(ctx context.Context, command InstallCommand) (string, error) {
	if len(command.Args) == 0 {
		return "", fmt.Errorf("no install command for this platform")
	}
	cmd := exec.CommandContext(ctx, command.Args[0], command.Args[1:]...)
	executil.Configure(cmd)
	out, err := cmd.CombinedOutput()
	text := strings.TrimSpace(string(out))
	const maxOutput = 4000
	if len(text) > maxOutput {
		text = "…" + text[len(text)-maxOutput:]
	}
	if err != nil {
		return text, fmt.Errorf("%s: %w", strings.Join(command.Args, " "), err)
	}
	return text, nil
}
