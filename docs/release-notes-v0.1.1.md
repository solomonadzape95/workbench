# Workbench v0.1.1

A local-first terminal workbench for files: 65 operations across images, audio,
video, documents, and general files. One binary, optional engines, and four ways
to drive it — the TUI, the CLI, a local HTTP API, and an MCP server.

## Install

```sh
brew install solomonadzape95/tap/workbench
```

Or use the installer:

```sh
curl -fsSL https://raw.githubusercontent.com/solomonadzape95/workbench/main/install.sh | sh
```

Release archives for macOS and Linux (arm64 and amd64) and `checksums.txt` are
below. Go users can run
`go install github.com/solomonadzape95/workbench/cmd/workbench@latest`.

## Highlights

- **65 operations** across `image.*`, `audio.*`, `video.*`, `doc.*`, and
  `file.*`, including background removal, local Whisper transcription, PDF OCR,
  PDF compression/merge/split, image-to-PDF, watermarking, deduplication, and
  checksum verification.
- **Pipelines** — build a chain in the TUI, save it with `ctrl+s`, and re-run it
  from the palette or with `wb run --pipeline`.
- **Scriptable CLI** — `wb run`, `wb watch`, `wb ops`, `wb engines`,
  `wb pipeline`, `wb history`, `wb doctor`, and shell completions. Machine-readable
  output with `--json`, and a non-zero exit when a job fails.
- **Agent-ready** — `wb serve` exposes the same runner over HTTP with SSE
  progress and artifact downloads; `wb mcp` exposes every operation as a typed
  MCP tool over stdio.
- **Local by default** — no uploads, accounts, or second API key. ffmpeg,
  yt-dlp, whisper.cpp, LibreOffice, pandoc, Tesseract, Ghostscript, pdf2docx,
  poppler, rembg, Real-ESRGAN, and OpenCode are detected only when needed.
- **TUI** — palette, path/alias completion, parallel jobs, persistent history,
  logs, output review/undo, previews, a built-in editor, eight themes, settings,
  and contextual help.

## Notes

- macOS and Linux binaries. Windows is not supported yet.
- Optional engines enable their operations; `wb doctor` shows what is ready and
  how to install the rest.
- `doc.detect_ai` is diagnostic, not proof. Read
  [`docs/ai-detector.md`](https://github.com/solomonadzape95/workbench/blob/main/docs/ai-detector.md)
  before treating a score as evidence.
- The Homebrew formula is maintained in
  [`solomonadzape95/homebrew-tap`](https://github.com/solomonadzape95/homebrew-tap).

## Thanks

If Workbench saves you a batch job, star the repo and tell me which operation
you want next.
