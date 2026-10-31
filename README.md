<p align="center">
  <img src="assets/logo.svg" width="112" alt="Workbench logo">
</p>

<h1 align="center">Workbench</h1>

A keyboard-driven terminal workbench for file operations: convert, compress,
resize, transform, and transcribe local media and documents, in batches, in
parallel, with pipelines you can save and re-run.

**Status:** M8 — an organized TUI shell with a simple home screen, a built-in
text editor, eight theme presets with previews, and a local agent API. The product and
architecture spec lives in [`SPEC.md`](./SPEC.md); the TUI structure plan is in
[`docs/tui-architecture.md`](./docs/tui-architecture.md). The humanizer guide is
in [`docs/humanizer.md`](./docs/humanizer.md), and the detector methodology is in
[`docs/ai-detector.md`](./docs/ai-detector.md).

## Install

```sh
curl -fsSL https://raw.githubusercontent.com/solomonadzape95/workbench/main/install.sh | sh
```

See [`docs/install.md`](./docs/install.md) for release archives, `go install`,
Homebrew, and private-checkout instructions.

## Build

```sh
make build
./bin/wb
```

Or run it directly:

```sh
go run ./cmd/workbench
```

## Use it

```sh
./bin/wb            # dashboard
./bin/wb run --op image.convert --input '~/Pictures/*.png' --output ~/Out
./bin/wb watch --pipeline my-chain --dir ~/Inbox --output ~/Out
./bin/wb ops        # list operations; --json for machines
./bin/wb engines    # probe engines; --json for machines
./bin/wb pipeline   # list, export, or import saved pipelines
./bin/wb serve      # local agent API on 127.0.0.1:8765
./bin/wb mcp        # Model Context Protocol server on stdio
./bin/wb doctor     # engine + config report
./bin/wb history    # list or clear saved job history
./bin/wb version
```

`wb run` accepts either `--op <id> --param key=value` or `--pipeline <name>`.
Inputs use the same path, glob, and `@alias` syntax as the TUI. It exits non-zero
when the job fails, which makes it safe in scripts and CI. `wb ops --json`,
`wb engines --json`, `wb doctor --json`, and `wb history --json` provide
machine-readable output; `wb pipeline export|import` moves saved chains between
machines. `wb watch` waits for a settle window after the last write, batches the
changed files into one job, skips hidden files and its own output directory, and
stops with `--once` after the first completed batch. `wb run` accepts `**` globs
plus `--recursive`, `--include`, `--exclude`, `--min-size`, `--max-size`,
`--newer-than`, and `--skip-hidden`. `wb completion bash|zsh|fish` prints shell
completions.

On first launch, a setup screen scans engines and lets you pick starter packs
(Media, Documents). Finish with `enter`; the config is written to
`~/.config/workbench/config.toml`.

From the dashboard:

1. `ctrl+p` opens the palette (or `n` opens the operation picker).
2. Pick an operation to start a chain. `Download video` switches the input to a URL field; add more steps with `n` on the steps list.
3. `tab` / `shift+tab` moves focus between input, output, and steps; `ctrl+→`/`alt+→` still work when you are inside a text field. `enter` edits the selected step. In the input and output fields, `tab` completes the selected path/alias suggestion.
   Type an input path, glob, or alias (`~/Pictures/*.png`, `@shots`, `@last`,
   `@clipboard`). For `Download video`, type one or more `http(s)` URLs instead.
4. `ctrl+r` runs the chain. Progress and per-item results appear on the jobs screen.
5. `ctrl+s` saves the chain as a named pipeline; load it later from `ctrl+p` → `#`.
6. `x` cancels the selected job after a confirmation; cancelled jobs leave no partial output.
7. `e` opens the built-in editor for a file; `s` opens Settings (output folder, theme, max jobs); `?` (or `F1`) opens contextual help. From Aliases, `e` opens `config.toml` in the same editor.
8. Jobs persist. `l` opens a log, `enter` opens an AI report when one exists (otherwise the log), `r` reruns, `R` retries failed items, and `o` reveals the output folder. `wb history list` shows the same history from the shell.

## What works

- `image.convert` — PNG/JPEG/GIF re-encoding, with JPEG quality.
- `image.compress` — JPEG quality reduction and maximum PNG compression.
- `image.resize` — fit, fill-and-crop, or exact scaling with smooth/nearest filters.
- `image.crop` — width/height with nine anchors and offsets.
- `image.remove_bg` — cut out the subject and write a transparent PNG through
  rembg (BRIA by default; u2net, u2netp, and ISNet options; optional edge refinement).
- `image.rotate` — 90/180/270 rotation with horizontal or vertical flip.
- `image.strip_meta` — decode/re-encode to remove EXIF, GPS, PNG text chunks, and
  other ancillary metadata.
- `image.to_ico` — multi-size favicon output (16, 32, 64, 128, 256).
- `image.enhance` — deterministic contrast, brightness, and sharpen pass.
- `image.upscale` — 2×/3×/4× Real-ESRGAN upscaling (optional binary).
- `image.watermark` — text watermark with position, opacity, and margin.
- `audio.convert` / `audio.compress` / `audio.speed` — MP3, M4A, WAV, FLAC,
  Ogg, and Opus, with pitch-preserving speed changes.
- `audio.transcribe` — local speech-to-text through whisper.cpp; outputs
  plain text, SRT, WebVTT, or JSON, with model and language selection.
- `audio.normalize` / `audio.enhance` — loudnorm podcast/music targets and
  voice/podcast/music cleanup chains.
- `audio.trim` / `audio.split` / `audio.fade` / `audio.silence_trim` — cut,
  segment, fade, and strip silence.
- `audio.extract_vocals` / `audio.extract_instrumental` — isolate the vocal
  stem or remove it and keep the instrumental, with optional Demucs or the
  ffmpeg center-channel fallback.
- `video.convert` / `video.compress` / `video.speed` — MP4, MKV, WebM, MOV,
  H.264/H.265/VP9, CRF control, resolution caps, and synced audio.
- `video.extract_audio` — pull an MP3/M4A/WAV/FLAC/Ogg track out of a video.
- `video.download` — fetch a video from a URL with yt-dlp (quality/container/
  playlist options); downloaded titles carry through chained steps.
- `video.transcript` — fetch manual or automatic captions from a URL and save
  a plain-text transcript (language, auto-caption, timestamp options).
- `video.info` — read normalized metadata from a URL and save JSON (title,
  channel, duration, views, formats, caption languages, playlist entries).
- `video.trim` / `video.remux` / `video.watermark` — cut, change container
  without re-encoding, and overlay an image.
- `video.thumbnail` / `video.to_gif` — one frame as PNG/JPEG, or a palette-based
  GIF with fps/width controls.
- `video.burn_subs` — burn SRT/VTT subtitles into the video.
- `video.transcribe` — local whisper.cpp transcription directly from a video.
- `doc.convert` — PDF, DOCX, ODT, Markdown, HTML, EPUB, and text. PDF→DOCX
  prefers pdf2docx for tables and images, with LibreOffice as fallback;
  other formats use LibreOffice or pandoc.
- `doc.compress` — Ghostscript PDF compression with screen, ebook, and prepress
  presets.
- `doc.ocr` — Tesseract OCR for images and scanned PDF pages; text output, or
  a searchable PDF for image inputs.
- `doc.extract_text` — text-only conversion for PDF, DOCX, HTML, EPUB, and
  Markdown through the same converter.
- `doc.strip_meta` — remove DOCX/ODT author, title, and property metadata.
- `doc.translate` / `doc.summarize` / `doc.rewrite` — OpenCode-backed text
  transforms with chunking, model fallback, and protected code/URL blocks.
- `doc.edit` — open a text or Markdown file in `$EDITOR` and save the result.
- `doc.merge` / `doc.split` — merge PDFs or split them by page groups.
- `doc.humanize` — rewrite TXT, Markdown, or DOCX text in a natural human
  voice through your local OpenCode service. Six style modules, optional voice
  sample, automatic free-model fallback, and chunked rewriting for long
  documents. DOCX output keeps the original figures, tables, media, headers,
  and styles; only body paragraphs are replaced. Markdown/text output keeps
  code, tables, quotes, and links intact.
- `doc.detect_ai` — score TXT, Markdown, DOCX, or PDF prose for AI-written
  share, show the sentence and document signals behind the number, and estimate
  the likely model family. Runs offline, needs no API key, writes a Markdown
  report plus a JSON sidecar, and opens a dedicated result screen with a
  one-key Humanize action.
- `file.archive` — ZIP, tar.gz, and Zstandard.
- `file.copy` — byte-for-byte copies with progress.
- `file.hash` — SHA-256, SHA-1, or MD5 manifest per file.
- `file.extract` — unpack ZIP, tar, tar.gz, gzip, and Zstandard archives.
- `file.move` / `file.rename` — move or pattern-rename files into the output
  directory, with collision handling.
- `file.trash` — move files into a recoverable, timestamped trash folder.
- `file.split` — split large files into numbered parts.
- `file.verify` — verify a directory against a GNU or BSD checksum manifest;
  writes a JSON report with ok, mismatch, missing, and error entries.
- Compound presets — searchable chains for PDF→DOCX, web images, podcast
  audio, and share-ready video.
- New job search — type directly on the operation picker to filter; arrows move,
  enter opens.
- Built-in editor — line numbers, current-line highlight, undo/redo, search,
  atomic save, and config editing; no external `$EDITOR` required.
- Themes — eight built-in presets (opencode, tokyonight, catppuccin, gruvbox,
  nord, monokai, vscode, rosepine) with live previews and config persistence.
- Settings — edit output folder and max jobs in place, choose a theme, browse
  aliases, or open `config.toml` in the built-in editor.
- Path completion — typing in the input field suggests aliases and matching
  files/directories; use ↑/↓ then enter to insert.
- Step editor — enum arrows, numeric `←/→` adjustment, explicit field hints,
  and a clear Format/Quality summary in the chain.
- Home — centered dither mark above the ANSI Shadow wordmark, primary actions, recent jobs, and environment details on wide terminals.
- Command palette — literal-first search with fuzzy abbreviations, `>` commands,
  `@` aliases, `#` pipelines, disabled-engine and incompatible-step hints; the
  card shortens to fit the terminal and scrolls with `↑/↓`.
- Pipelines — linear chains, step parameter forms, `ctrl+s` save, `#` reload and
  re-run, `ctrl+d` soft delete to `.trash/` after confirmation.
- Input resolution — paths, globs, directories, comma-separated lists, config
  aliases, and built-ins: `@last`, `@clipboard`, `@cwd`, `@home`, `@downloads`,
  `@documents`, `@development`, `@projects`, `@pictures`, `@videos`, `@music`,
  `@desktop`, `@screenshots`. Edit them from Settings → Aliases or config.toml.
- Job runner — concurrent jobs, per-engine limits, per-item and aggregate
  progress, temp workspaces, atomic commits, collision suffixing, panic
  containment, and process-group cancellation for ffmpeg.
- Previews — truecolor half-block images, text/PDF text, and hex fallbacks
  (`ctrl+o` from compose or jobs; audio/video metadata previews come with the
  next polish pass).
- Job detail — `l` logs, `enter` report or log, `R` retry failed, `r` re-run, `o` reveal output, `c`/`y` copy error; size-saved badge and progress sparkline.
- Job logs — `l` opens a timestamped log overlay, `/` searches, `n/N` jumps between matches, `c`/`y` copies, and every job persists a log under `~/.local/share/workbench/logs/`.
- Job history — `~/.local/share/workbench/history.jsonl`, loaded on startup, rotated at 5 MB, exposed by `wb history list|clear` and the local API. Rerun and retry work from history.
- MCP — `wb mcp` exposes every operation as a tool for agents over stdio.
- OpenAPI — `docs/openapi.yaml` and the typed client in `pkg/apiclient`.
- Session restore — `state.json` restores the selected job, last output, and an unsaved Compose draft after a restart.
- Onboarding — engine scan and starter packs on first launch; the Engines
  screen (`ctrl+p` → `> Engines`) re-scans on demand.
- TUI — btop-style framed panels, dashboard columns, dense job/engine rows,
  generated forms, help overlay, quit confirmation while jobs are running.

Audio/video needs ffmpeg; URL downloads, transcripts, and metadata need yt-dlp.
Vocal separation uses Demucs when available (`python3 -m pip install demucs`) and
falls back to ffmpeg center isolation. PDF compression needs Ghostscript.
Background removal uses rembg and downloads its model on first use
(`python3 -m pip install "rembg[cli,cpu]"`). Upscaling uses the optional
`realesrgan-ncnn-vulkan` binary from the Real-ESRGAN releases. Better PDF editing uses
pdf2docx (`python3 -m pip install pdf2docx`); LibreOffice remains the fallback.
Audio transcription uses whisper.cpp (`brew install whisper-cpp`) with a ggml
model such as `~/.local/share/whisper-models/ggml-small.en.bin`. Document conversion
uses pdf2docx, LibreOffice, and/or pandoc. PDF→DOCX prefers pdf2docx because it
reconstructs tables and images into an editable Word layout; complex or scanned
PDFs can still need manual cleanup. The built-in editor is text-only, so edit
DOCX in Word, Pages, or LibreOffice. Run `./bin/wb doctor` to see what is
available; missing engines never block unrelated operations.

`doc.humanize` needs [OpenCode](https://opencode.ai) installed and signed in. It
uses the models already configured in OpenCode, so there is no second API key to
manage. The default `auto` model tries free text models in order and falls back
when one is rate-limited or unavailable. DOCX files are humanized directly: the
operation edits body paragraphs in `word/document.xml` and writes the original
package back, so figures, tables, media, headers, and styles stay in place.
Unstyled headings, names, signature lines, and list markers are left alone.
Each rewrite is checked for meta replies, new names or numbers, and subject
drift; anything unsafe keeps the original paragraph. For PDFs, use the
`Humanize a PDF` preset to convert to DOCX first. A long
Markdown/text document is split at paragraph boundaries; code blocks, tables,
blockquotes, and links are protected and restored verbatim.

`doc.detect_ai` is the other side of the loop. It reads prose only, scores
sentence rhythm, burstiness, vocabulary, assistant phrases, repetition, and
punctuation, then shows the weighted signals instead of a single opaque number.
The result screen leads with large AI and HUMAN scores in the Workbench block
face plus a split gauge, lists the strongest signals, top paragraphs, flagged
sentences, and likely model-family fingerprints, and puts **Humanize this text**
one key away. It is intentionally diagnostic: formal academic prose, non-native
English, and heavily edited AI text can land in the uncertain middle. Read
[`docs/ai-detector.md`](./docs/ai-detector.md) before treating a score as proof.

## Agent API

`wb serve` starts a local JSON API over the same operations and runner as the
TUI, so an agent or script can list tools, start jobs, poll them, and inspect
outputs. It binds to `127.0.0.1:8765` by default.

```sh
./bin/wb serve --token dev-secret
curl -s -H 'Authorization: Bearer dev-secret' http://127.0.0.1:8765/v1/operations
```

Endpoints: `GET /v1/operations`, `GET /v1/engines`, `GET /v1/pipelines`,
`POST /v1/jobs`, `GET /v1/jobs`, `GET /v1/jobs/{id}?wait=30`,
`POST /v1/jobs/{id}/cancel|retry|rerun`. See
[`docs/agent-api.md`](./docs/agent-api.md) for request and response examples.

## Configuration

`~/.config/workbench/config.toml` (created lazily; defaults are used when absent).
The generated file starts with an editing guide and commented `[[alias]]`
examples. Saved pipelines live in `~/.local/share/workbench/pipelines/*.toml`,
finished jobs in `~/.local/share/workbench/history.jsonl`, and per-job logs in
`~/.local/share/workbench/logs/`. `wb history list` / `wb history clear` manage
the history file. Engine probes are cached for 24 hours in
`~/.cache/workbench/engines.json`.
If you already have a config, add `"opencode"` to `engines.enabled` to turn on
the humanizer.

```toml
[general]
theme_name    = "opencode" # opencode | tokyonight | catppuccin | gruvbox | nord | monokai | vscode | rosepine
max_jobs      = 4

[output]
dir       = "~/Workbench/{date}"
collision = "suffix"
template  = "{stem}.{ext}"

[[alias]]
name = "shots"
path = "~/Pictures/Screenshots"
glob = "*.png"

# Optional: point the humanizer at a specific OpenCode service.
[opencode]
url      = ""  # empty = auto-discover from `opencode service status`
password = ""  # empty = read ~/.config/opencode/service.json or OPENCODE_SERVICE_PASSWORD
```

## Development

```sh
make check   # gofmt, go vet, go test -race
make lint    # golangci-lint, if installed
make doctor
```

## Keys

Global: `esc` back/close · `q` quit · `ctrl+c` quit/interrupt · `?` or `F1` contextual help · `ctrl+p` palette · `:` command mode · `/` search where a list has a filter.

| Key | Action |
|---|---|
| `ctrl+p` / `:` | Command palette / command mode |
| `ctrl+r` | Run the composed chain |
| `ctrl+s` | Save the chain as a pipeline |
| `ctrl+o` / `v` | Preview an input or output |
| `ctrl+d` | Delete a selected saved pipeline (palette, after confirmation) |
| `n` | New operation / append a step |
| `a` | Open the AI detector from Home |
| `j` / `k` | Move through lists (search-first screens use `ctrl+j`/`ctrl+k`) |
| `g` / `G`, `home` / `end` | Top / bottom of a list |
| `ctrl+d` / `ctrl+u` | Half-page down / up in a list or viewer |
| `enter` | Select / edit step / open an AI report |
| `d` | Remove the selected step (after confirmation) |
| `alt+↑` / `alt+↓` | Move the selected step up/down (`J`/`K` also work) |
| `R` / `r` | Retry only failed items / re-run every input |
| `l` | Open the selected job log |
| `o` | Reveal the output directory (or open the full AI report on the detector) |
| `e` | Open the editor (config editing from Aliases) |
| `s` | Open Settings from Home |
| `t` | Open the theme picker |
| `v` | Open Statistics from Home |
| `tab` | Complete a path/alias suggestion inside a text field; otherwise move focus |
| `shift+tab` | Previous field or pane |
| `ctrl+→` / `alt+→`, `ctrl+←` / `alt+←` | Next / previous field inside a text field |
| `ctrl+v` | Paste from the system clipboard (Cmd+V also works) |
| `←` / `→` | Move the field cursor; adjust a numeric or enum field |
| `x` | Cancel the selected job (after confirmation) |
| `u` / `a` | On Jobs: undo outputs / review outputs |
| `h` | Humanize the source from the AI detector screen |
| `enter` / `o` | Open the full AI report from the detector screen |
| `c` / `y` | Copy the AI summary, job error, or log |
| `/` | Search/filter jobs, logs, aliases, and engines |
| `n` / `N` | Next / previous match in the log viewer |
| `?` / `F1` | Toggle contextual help |
| `esc` | Back / close / dismiss |
| `q` | Quit (confirms while jobs are running) |

On macOS, Mission Control may capture `ctrl+←`/`ctrl+→` before Workbench sees them. Use `alt+←`/`alt+→`, or disable those Mission Control shortcuts in System Settings → Keyboard → Keyboard Shortcuts. `ctrl+l` and `ctrl+h` are no longer used for focus because terminals reserve them for redraw/backspace.

In the editor: `ctrl+s` save, `ctrl+z`/`ctrl+y` undo/redo, `ctrl+f` find,
`ctrl+n`/`ctrl+p` (or `F3`/`Shift+F3`) next/previous match, `ctrl+←`/`ctrl+→`
word movement, `ctrl+backspace` delete a word, `ctrl+u` delete to line start,
`esc`/`ctrl+q` close (dirty buffers ask to save or discard).
