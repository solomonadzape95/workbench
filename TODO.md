# TODO

## AI-text detector

Implemented as `doc.detect_ai` + `internal/detect`. See
[`docs/ai-detector.md`](docs/ai-detector.md).

The detector is diagnostic, not a gate. It scores prose-only sentence rhythm,
burstiness, vocabulary, contractions, starter diversity, repetition,
punctuation, specificity, and assistant phrase fingerprints. It reports the
weighted signals, flagged sentences, a likely model family, confidence, and
warnings. The TUI opens a dedicated result screen with a score gauge, top
signals and paragraphs, and a Humanize action.

Still worth improving:

- [x] Build a labelled calibration set of human and AI academic/business prose.
  Starter corpus: `internal/detect/testdata/calibration` (5 human, 2 AI, 1
  humanized; manifest records genre and first language). Expand it.
- [x] Add a held-out calibration report so changes to weights are measured, not
  guessed: `make calibrate` prints the table and rewrites
  `internal/detect/testdata/calibration/report.md`.
- [x] Formal-L2 false positive: added a research-register counter-signal and
  recalibrated phrase/connector saturation. `human_academic_l2_formal.txt` now
  scores 33 (was 82), human false positives are 0/5, and AI detection remains
  2/2 at 91–96%. `make calibrate` records the result.
- [x] Optional local log-probability scorer hook: `detect.logprob_command`
  receives document text on stdin and returns a 0–1 or 0–100 score (plain or
  JSON). DetectGPT/Fast-DetectGPT/Binoculars wrappers can plug in without
  changing the offline stylometric default.
- [x] Track false-positive rates by genre and by first language. The calibration
  report groups human false positives both ways.
- [x] Add a regression check that humanized output scores lower than its source.
  The manifest pairs `humanized_academic.txt` with `ai_academic.txt`.

## Next up (recommended order)

1. **Persistent history + session restore** (M) — implement `internal/history` per
   [`docs/persistence.md`](docs/persistence.md): append one JSON record per job,
   read recent records at boot, merge them into Home/Jobs, disable `x` on
   historical rows, and let `r`/`R` still work. Add `state.json` for the last
   input, output dir, selected job, and unsaved Compose draft.
   Done: history is loaded at boot and `internal/state` restores the selected
   job and Compose draft.
2. **Headless CLI** (M) — `wb run`, `wb watch`, `wb history`, `wb ops --json`,
   `wb engines --json`, `wb pipeline export|import`. Reuse `app.Boot()` and the
   existing runner; no second execution path.
   Started: `wb run` (`--op`/`--pipeline`), `wb ops|engines|doctor|history
   --json`, and `wb pipeline list|export|import` are in place. Remaining:
   `wb watch`, shell completions, dry-run, notifications, and recursive globs.
3. **Complete the file loop** (S each) — `file.extract`, `file.verify`,
   `file.rename`. All internal, no new engines.
   Started: `file.verify` is implemented (manifest parsing, per-entry
   ok/mismatch/missing, JSON report). `file.extract` and `file.rename` remain.
4. **Document pack** (M–L) — `doc.ocr`, `doc.compress`, `doc.merge`,
   `doc.split`. PDFs are the most common real-world input; `doc.merge` needs
   aggregate support (see below).
5. **Watch folders / recipes** (M) — bind a folder + pipeline and auto-run on
   new or changed files, with debounce, rename-stability checks, and a dry-run
   preview.
   Started: `wb watch` polls a folder, waits for a settle window, batches
   changes, skips hidden files and the output dir, and can exit after one batch
   with `--once`. Recipes, dry-run preview, and a TUI surface remain.
6. **Agent API polish** (S–M) — `wb mcp`, SSE job events, artifact download,
   webhook callbacks.

## Persistence and startup hygiene

- [x] `internal/history`: append-only JSONL at
  `~/.local/share/workbench/history.jsonl`, rotate at 5 MB, `Recent(n)` on boot,
  `Clear()` for `wb history clear`.
- [x] `state.json`: last input text, last output dir, last selected job ID,
  unsaved Compose draft. Loaded at boot; saved on quit, job start, and job done.
- [x] Startup cleanup: remove cache job dirs older than 24 h.
- [x] Engine probe cache at `~/.cache/workbench/engines.json` (24 h TTL): show
  cached results immediately, refresh in the background.
- [x] Log retention: prune job logs by age or count (30 days / 500 files).
- [x] Retention settings: `logs.max_age_days` and `logs.max_files` are wired.
- [x] Document that history contains input/output paths. See `docs/persistence.md`.

## CLI and automation

- [x] `wb run --pipeline <name> --input <path> [--output <dir>]` and
  `--op <id> --param key=value`.
- [x] `wb watch --pipeline <name> --dir <path> [--recursive] [--settle 2s]`.
  Also supports `--output` and `--once`; skips hidden files and the output dir.
- [x] `wb history [--json]`, `wb history clear`.
- [x] `wb ops [--json]`, `wb engines [--json]`, `wb doctor --json`.
- [x] `wb pipeline export|import` for sharing saved chains.
- [x] Shell completions (bash/zsh/fish). `wb completion <shell>`.
- [x] Dry-run/plan mode: `wb run --dry-run` prints resolved inputs, final output
  names, template/collision policy, and missing required engines. Estimated
  savings still needs media inspection; the TUI uses the real compose preview.
- [x] Notifications: wire `general.notify` to desktop notifications
  (osascript/notify-send) and an optional `[notify] webhook` on completion.
- [x] Recursive input globs (`**`) plus `include`/`exclude`, `min_size`,
  `max_size`, `newer_than`, and skip-hidden. `**` walks the tree; a plain glob
  or directory walks recursively with `--recursive`.

## Architectural enabler: aggregate operations

The runner is strictly per-item: `runItem` passes `[]Artifact{it.current}`, so
many→one jobs are impossible. Add a scope to `core.Spec` (for example
`Mode: PerItem | Aggregate`, optionally with `GroupBy`) and teach `runStep` to
call `Run` once with all items for aggregate ops.

**Implemented:** `core.Spec.Mode` (`per_item` / `aggregate`), a runner
`runAggregate` path, `JobView.Aggregate` outputs, history persistence, TUI/API
rendering, and a CLI aggregate line. Merge PDFs, audio join, video concat,
contact sheets, manifests, and duplicate reports now use it.

Unlocks:

- `doc.merge`, `video.concat`, `audio.join`
- `file.dedupe`, `file.manifest`
- `image.contact_sheet`
- a single archive of a folder (`file.archive` currently writes one archive per
  input file, which is usually not what "archive these files" means)
- transcript bundles and report-style outputs

## New operations

### Files

- [x] `file.extract` — unzip/untar/tar.gz/gzip/zstd into a subfolder with
  optional flatten; rejects path traversal and resolves collisions.
- [x] `file.verify` — check files against a `.sha256`/`.sha1`/`.md5` manifest;
  report mismatches and missing files. Pairs with `file.hash`. Supports GNU
  `sha256sum` and BSD `SHA256 (file) = digest` formats; writes a JSON report.
- [x] `file.rename` — pattern rename with `{stem}`, `{ext}`, `{counter}`,
  `{date}`, `{width}`, `{height}` in direct mode, with the target collision
  policy applied before the move.
- [x] `file.move` — move into the output directory instead of copying.
- [x] `file.trash` — safe delete into a timestamped trash folder, with a
  configurable destination and recoverable output paths.
- [x] `file.dedupe` — hash-based duplicates report (aggregate mode). Moving
  extras to trash remains an opt-in follow-up.
- [x] `file.split` — split large files into numbered parts. `file.join` still
  needs aggregate mode.
- [x] `file.manifest` — CSV/JSON inventory (path, size, kind, optional SHA-256).
  Image/video dimension probing can be added later.

### Documents

- [x] `doc.ocr` — images → text or searchable PDF, PDFs → text via
  tesseract + poppler rasterization. PDF→searchable PDF still needs page merge.
- [x] `doc.compress` — Ghostscript PDF compression with `/ebook`, `/screen`,
  and `/prepress` presets.
- [x] `doc.merge` — merge PDFs with Ghostscript (aggregate mode).
- [x] `doc.split` — split PDFs by every N pages or explicit page ranges with
  pdfseparate/pdfunite.
- [x] `doc.extract_text` — PDF/EPUB/DOCX/Markdown/HTML → txt/md through the
  existing converter.
- [x] `doc.translate` / `doc.summarize` / `doc.rewrite` — OpenCode-backed
  transforms for TXT/Markdown, with chunking, model fallback, parallel workers,
  and protected code/URL tokens. Summaries use a map-reduce pass. DOCX/PDF use
  `doc.convert` first.
- [x] `doc.strip_meta` — remove DOCX/ODT author, title, and property parts
  (PDF metadata stripping still needs a dedicated backend).
- [x] `doc.edit` — text/Markdown `$EDITOR` handoff and save; DOCX/PDF use
  `doc.convert` first. Best run from `wb run` so the editor can attach to the TTY.

### Image

- [x] `image.watermark` — text watermark with position, opacity, and margin.
- [x] `image.contact_sheet` — N images → one labeled grid (aggregate mode).
- [x] `image.to_ico` — multi-size favicon output (`16,32,64,128,256`).
- [x] `image.strip_meta` — EXIF/GPS removal.
- [x] `image.rotate` — angle and flip (SPEC "Later").
- [x] `image.upscale` — 2×/3×/4× with the optional Real-ESRGAN ncnn Vulkan
  binary; outputs PNG, JPEG, or WebP with configurable tile size.
- [x] `image.enhance` — sharpen, denoise, auto-contrast (SPEC v1.1).

### Audio

- [x] `audio.normalize` — `loudnorm` with podcast (-16 LUFS) and music
  (-14 LUFS) targets.
- [x] `audio.trim` / `audio.split` — start/end trim; fixed-interval split.
  Silence trimming is `audio.silence_trim`.
- [x] `audio.join` — concatenate with optional crossfade (aggregate mode).
- [x] `audio.fade` — fade in/out.
- [x] `audio.enhance` — denoise (`afftdn`), normalize, voice/podcast/music
  presets (SPEC v1.1).
- [x] `audio.extract_vocals` / `audio.extract_instrumental` — isolate or remove
  vocals with optional Demucs, with an ffmpeg center-channel fallback.
- [x] `audio.silence_trim` — strip leading/trailing silence.

### Video

- [x] `video.trim` — start/end, precise or keyframe-fast (SPEC "Later").
- [x] `video.thumbnail` — one frame at a timestamp, optional width.
- [x] `video.to_gif` — trim, fps, width, dither.
- [x] `video.burn_subs` — burn SRT/VTT into the video. Pairs with
  `audio.transcribe` and `video.transcript`.
- [x] `video.remux` — change container without re-encoding.
- [x] `video.concat` — join clips (aggregate mode).
- [x] `video.watermark` — image overlay with position, opacity, and margin.
- [x] `video.transcribe` — local whisper transcription directly from a video
  (extracts 16 kHz mono audio, then runs whisper.cpp).

## TUI and workflow

- [x] Output review (`a` on Jobs): toggle individual outputs, show input and
  output sizes, keep accepted files, and move rejected files to the undo trash.
- [x] Undo job (`u` on Jobs): move outputs to the undo trash and restore
  overwritten files that the overwrite policy backed up at commit time.
  Direct-mode moves are only trashed because they bypass the commit step.
- [x] Job search, sort, and filter. `/` filters by text; `s` cycles newest, name,
  and status order.
- [x] Stats view: totals, saved bytes by pipeline, 14-day sparkline, and status
  counts (`v` from Home or `> Statistics`).
  `Metrics.BytesIn`/`BytesOut` already carry the data.
- [x] Pipeline editor: load, edit, save in place, rename, and duplicate saved
  pipelines from the command palette.
- [x] Keymap registry from [`docs/tui-keymap-audit.md`](docs/tui-keymap-audit.md):
  footer, help, and tests share `keymap.go`; the audit conflicts (`q`, detector
  `enter`, confirmations, overloaded `d`, stale help) are fixed.
- [x] Contextual `?` help per screen. Help shows Global plus the active screen context.
- [x] Filtered file picker for inputs: type-ahead prefix filtering, directory
  browsing, aliases, and recursive globs all share the same completion path.
- [x] Golden frames for Home, Compose, Jobs, and Editor at 80×24 and 100×32,
  stored under `internal/tui/testdata/golden`; refresh with `UPDATE_GOLDEN=1`.

## New preset jobs

- [x] Screenshot cleanup — `image.strip_meta` → `image.resize` → `image.convert`.
- [x] Podcast package — `video.extract_audio` → `audio.normalize` →
  `audio.compress` → `audio.transcribe`.
- [x] Lecture notes — `video.transcribe` → `doc.summarize`.
- [x] Scanned PDF — `doc.ocr` → `doc.convert` → `doc.humanize` preset.
- [x] Social clip — `video.trim` → `video.burn_subs` → `video.compress`.
- [x] Dataset prep — `image.resize` → `image.convert` → `file.hash`.
- [x] Archive handoff — `file.rename` → `file.archive` → `file.hash`.
  `file.rename` is a chainable direct step; later failures leave the source
  recoverable at the renamed target.

## Agent API

- [x] `wb mcp` — stdio MCP server exposing every operation as a tool
  (`initialize`, `tools/list`, `tools/call`).
- [x] SSE events: `GET /v1/jobs/{id}/events` streaming job snapshots,
  including terminal `done`/`failed` states.
- [x] Artifact endpoints: list and download outputs (`/artifacts`, `/artifacts/{n}`).
- [x] Webhook callbacks (`callback_url` on job creation).
- [x] OpenAPI 3.1 spec at `docs/openapi.yaml` plus the typed Go client in
  `pkg/apiclient`.
- [x] Include persisted history in `GET /v1/jobs`. Idempotency keys remain.
- [x] Token/permission note: the API is local-only by default; `docs/agent-api.md`
  documents the risk of binding to a non-loopback address.

## Quality and release

- [x] CI: run `make check` (gofmt, vet, `go test -race`) on push; add
  `golangci-lint`. `.github/workflows/ci.yml` covers both.
- [x] Finish the GoReleaser setup and add a Homebrew tap. `.goreleaser.yaml`
  publishes `wb` and a Homebrew formula (set `HOMEBREW_TAP_GITHUB_TOKEN`).
- [x] Windows path handling: `%APPDATA%`/`%LOCALAPPDATA%` instead of `~/.config`
  (see `docs/persistence.md`). XDG overrides still win.
- [x] Calibration/regression work is tracked by `make calibrate`, `report.md`,
  genre/language false-positive breakdowns, and the humanized-vs-source check.

> Scope guard: `SPEC.md` §16 names scope creep as the top risk and the §6
> inventory as the contract. Pick at most 2–3 of these per milestone.

