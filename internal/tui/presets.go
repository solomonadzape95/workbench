package tui

import "github.com/solomonadzape95/workbench/internal/core"

// jobPreset is a searchable, reusable combination of operations. Presets open
// a normal compose screen, so every step can still be inspected or edited.
type presetStep struct {
	op     string
	params core.Values
}

type jobPreset struct {
	name        string
	description string
	steps       []presetStep
}

var jobPresets = []jobPreset{
	{
		name:        "PDF to editable DOCX",
		description: "Reconstruct tables/images into editable DOCX (pdf2docx, LibreOffice fallback)",
		steps:       []presetStep{{op: "doc.convert", params: core.Values{"format": "docx", "engine": "auto"}}},
	},
	{
		name:        "Web image",
		description: "Convert to JPEG and compress it for the web",
		steps: []presetStep{
			{op: "image.convert", params: core.Values{"format": "jpeg", "quality": 85}},
			{op: "image.compress", params: core.Values{"quality": 78}},
		},
	},
	{
		name:        "Podcast audio",
		description: "Extract the audio from a video and compress it to MP3",
		steps: []presetStep{
			{op: "video.extract_audio", params: core.Values{"format": "mp3", "bitrate": "192k"}},
			{op: "audio.compress", params: core.Values{"bitrate": "128k"}},
		},
	},
	{
		name:        "Detect AI writing",
		description: "Estimate AI-written share, inspect the signals, and see the likely model family",
		steps:       []presetStep{{op: "doc.detect_ai", params: core.Values{"threshold": 0.55, "json": true}}},
	},
	{
		name:        "Humanize text",
		description: "Rewrite a Markdown or text file in a natural voice with a free OpenCode model",
		steps:       []presetStep{{op: "doc.humanize", params: core.Values{"style": "professional", "model": "auto"}}},
	},
	{
		name:        "Humanize a DOCX",
		description: "Humanize the body text and keep every figure, table, and style",
		steps: []presetStep{
			{op: "doc.humanize", params: core.Values{"style": "professional", "model": "auto"}},
		},
	},
	{
		name:        "Humanize a PDF",
		description: "Convert PDF to DOCX, then humanize the text and keep the layout",
		steps: []presetStep{
			{op: "doc.convert", params: core.Values{"format": "docx", "engine": "auto"}},
			{op: "doc.humanize", params: core.Values{"style": "professional", "model": "auto"}},
		},
	},
	{
		name:        "Share video",
		description: "Compress to 1080p and remux as MP4",
		steps: []presetStep{
			{op: "video.compress", params: core.Values{"crf": 28, "preset": "fast", "resolution": "1080p", "max_bitrate": "none"}},
			{op: "video.convert", params: core.Values{"format": "mp4", "codec": "auto", "crf": 28, "preset": "fast"}},
		},
	},
	{
		name:        "Screenshot cleanup",
		description: "Strip metadata, fit to 1600px, and convert to a clean JPEG",
		steps: []presetStep{
			{op: "image.strip_meta", params: core.Values{"format": "auto", "quality": 90}},
			{op: "image.resize", params: core.Values{"width": 1600, "height": 0, "mode": "fit", "filter": "smooth"}},
			{op: "image.convert", params: core.Values{"format": "jpeg", "quality": 85}},
		},
	},
	{
		name:        "Podcast package",
		description: "Extract audio, normalize to -16 LUFS, compress, and transcribe",
		steps: []presetStep{
			{op: "video.extract_audio", params: core.Values{"format": "mp3", "bitrate": "192k"}},
			{op: "audio.normalize", params: core.Values{"preset": "podcast"}},
			{op: "audio.compress", params: core.Values{"bitrate": "128k"}},
			{op: "audio.transcribe", params: core.Values{"model": "small.en", "language": "en", "format": "txt"}},
		},
	},
	{
		name:        "Dataset prep",
		description: "Resize, convert to JPEG, and write a SHA-256 manifest",
		steps: []presetStep{
			{op: "image.resize", params: core.Values{"width": 1024, "height": 0, "mode": "fit", "filter": "smooth"}},
			{op: "image.convert", params: core.Values{"format": "jpeg", "quality": 88}},
			{op: "file.hash", params: core.Values{"algo": "sha256"}},
		},
	},
	{
		name:        "Social clip",
		description: "Trim to 30 seconds, burn a subtitle file, and compress to 1080p (set the subtitle path on step 2)",
		steps: []presetStep{
			{op: "video.trim", params: core.Values{"start": "0s", "end": "30s", "fast": false}},
			{op: "video.burn_subs", params: core.Values{"subtitle": "", "font_size": 24}},
			{op: "video.compress", params: core.Values{"crf": 24, "preset": "fast", "resolution": "1080p", "max_bitrate": "none"}},
		},
	},
	{
		name:        "Vocal stem",
		description: "Isolate the vocal from a song with Demucs when available, or center isolation",
		steps: []presetStep{
			{op: "audio.extract_vocals", params: core.Values{"method": "auto", "model": "htdemucs", "format": "wav", "bitrate": "192k"}},
		},
	},
	{
		name:        "Instrumental",
		description: "Remove the vocal and keep the instrumental mix",
		steps: []presetStep{
			{op: "audio.extract_instrumental", params: core.Values{"method": "auto", "model": "htdemucs", "format": "wav", "bitrate": "192k"}},
		},
	},
	{
		name:        "Video transcript",
		description: "Extract the audio from a video and transcribe it locally",
		steps: []presetStep{
			{op: "video.extract_audio", params: core.Values{"format": "mp3", "bitrate": "192k"}},
			{op: "audio.transcribe", params: core.Values{"model": "small.en", "language": "en", "format": "txt"}},
		},
	},
	{
		name:        "Extract document text",
		description: "Convert PDF, DOCX, HTML, or EPUB to plain text",
		steps: []presetStep{
			{op: "doc.convert", params: core.Values{"format": "txt", "engine": "auto"}},
		},
	},
	{
		name:        "Compress PDF",
		description: "Shrink a PDF with Ghostscript's ebook preset",
		steps: []presetStep{
			{op: "doc.compress", params: core.Values{"preset": "ebook"}},
		},
	},
	{
		name:        "Scanned PDF",
		description: "OCR a scanned PDF, convert the text, then humanize it (set engine/paths as needed)",
		steps: []presetStep{
			{op: "doc.ocr", params: core.Values{"format": "txt", "language": "eng", "dpi": 300}},
			{op: "doc.convert", params: core.Values{"format": "docx", "engine": "auto"}},
			{op: "doc.humanize", params: core.Values{"style": "professional", "model": "auto"}},
		},
	},
	{
		name:        "Merge PDFs",
		description: "Combine several PDFs into one document",
		steps: []presetStep{
			{op: "doc.merge", params: core.Values{"preset": "prepress"}},
		},
	},
	{
		name:        "Join audio",
		description: "Concatenate several audio files into one track",
		steps: []presetStep{
			{op: "audio.join", params: core.Values{"format": "mp3", "bitrate": "192k", "crossfade": "0s"}},
		},
	},
	{
		name:        "Join videos",
		description: "Concatenate several video clips into one file",
		steps: []presetStep{
			{op: "video.concat", params: core.Values{"container": "mp4", "codec": "auto", "crf": 20, "preset": "medium"}},
		},
	},
	{
		name:        "Contact sheet",
		description: "Arrange images into one labeled grid",
		steps: []presetStep{
			{op: "image.contact_sheet", params: core.Values{"columns": 4, "thumb_width": 320, "thumb_height": 240, "gap": 12, "margin": 12, "labels": true, "format": "png"}},
		},
	},
	{
		name:        "Lecture notes",
		description: "Transcribe a video locally, then summarize the transcript",
		steps: []presetStep{
			{op: "video.transcribe", params: core.Values{"model": "small.en", "language": "en", "format": "txt"}},
			{op: "doc.summarize", params: core.Values{"length": "medium", "style": "bullets", "model": "auto", "chunk_words": 900, "workers": 3}},
		},
	},
	{
		name:        "Archive handoff",
		description: "Rename selected files, archive them, and write SHA-256 manifests",
		steps: []presetStep{
			{op: "file.rename", params: core.Values{"template": "{stem}-handoff.{ext}"}},
			{op: "file.archive", params: core.Values{"format": "zip", "level": 6}},
			{op: "file.hash", params: core.Values{"algo": "sha256"}},
		},
	},
	{
		name:        "Translate document",
		description: "Translate a text or Markdown file with a free OpenCode model",
		steps: []presetStep{
			{op: "doc.translate", params: core.Values{"language": "English", "model": "auto", "chunk_words": 900, "workers": 3}},
		},
	},
	{
		name:        "File manifest",
		description: "Write a JSON or CSV inventory of the selected files",
		steps: []presetStep{
			{op: "file.manifest", params: core.Values{"format": "json", "hash": false}},
		},
	},
	{
		name:        "Find duplicates",
		description: "Hash files and report duplicate groups",
		steps: []presetStep{
			{op: "file.dedupe", params: core.Values{"format": "json"}},
		},
	},
	{
		name:        "Favicon",
		description: "Write a multi-size .ico from one image",
		steps: []presetStep{
			{op: "image.to_ico", params: core.Values{"sizes": "16,32,64,128,256"}},
		},
	},
}

func (m App) presetPipeline(p jobPreset) (core.Pipeline, bool) {
	out := core.Pipeline{Name: p.name, Version: core.PipelineVersion}
	for _, step := range p.steps {
		if _, ok := m.opts.Operations.Get(step.op); !ok {
			return core.Pipeline{}, false
		}
		out.Steps = append(out.Steps, core.Step{Op: step.op, Params: step.params})
	}
	return out, len(out.Steps) > 0
}
