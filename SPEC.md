# Workbench — Product & Technical Specification

**Status:** v0.1 draft — canonical source of truth. `workbench-deets.md` is the raw idea dump; this document supersedes it wherever they disagree.
**Last updated:** 2026-10-20
**Target:** MVP by 2026-10-31.

---

## 0. How to use this doc

- **Product section (§1–§4)** — what we're building and why. Read before designing screens.
- **Model section (§5–§6)** — the vocabulary. Every package, screen, and config key uses these terms.
- **Architecture (§7)** — contracts and package boundaries. This is the part that is expensive to change later; challenge it now, not in week 2.
- **Feature specs (§8–§9)** — per-feature behavior and TUI design.
- **Delivery (§10–§17)** — stack, tests, milestones, risks, build tracker.
- **Appendices** — package tree, config, pipeline file, operation catalog, end-to-end scenario, research spikes.
- **Build state** — §17 is the live checklist.

Decisions that were open in the raw notes are resolved in §13 and tagged `D1…Dn`. Open questions that must be answered by a spike are in §14 and tagged `OQ1…On`. If you change a decision, edit this file in the same commit as the code.

---

## 1. What Workbench is

**Workbench is a keyboard-driven terminal workbench for file operations: convert, compress, resize, transform, and transcribe local media and documents, in batches, in parallel, with pipelines you can save and re-run.**

One-liner: *ffmpeg, ImageMagick, and a PDF converter, but as a single TUI with a queue, previews, and reusable recipes.*

It is not a file manager and not an editor. It is the thing you open when you have 40 PNGs to shrink, a lecture recording to transcribe, and a PDF that needs to become a DOCX — and you want to see all of it happening at once instead of juggling three shell one-liners.

```
┌ Workbench ─────────────────────────────── 2 running · ffmpeg ✓ · 3 queued ┐
│                                                                          │
│  ⠋ png-to-jpg · 10 files                                    8/10  1m12s  │
│    └─ remove-bg    ████████████████████░░░░░░   74%   frame 8 of 10      │
│                                                                          │
│  ⠋ pdf-to-docx · contract.pdf                                   running  │
│                                                                          │
│  ● compress lecture.mp4                                         queued   │
└──────────────────────────────────────────────────────────────────────────┘
```

---

## 2. Goals and non-goals

### Goals (v1)

1. **One place for all local file transforms.** Image, audio, video, documents, archives — same interaction model for all.
2. **Parallelism is visible.** Multiple jobs run at once, each with per-item progress, and can be cancelled individually.
3. **Pipelines.** Chain operations (convert → remove background) and save the chain as a named recipe that can be re-run on new inputs.
4. **Never leave a broken file.** Atomic outputs; cancelled or failed items never appear as valid results in the output directory.
5. **Fast to operate.** Command palette, aliases, keyboard-only navigation, fuzzy file picking. No mouse required.
6. **Honest errors.** Every failure says what failed, on which file, why, and what to do next.

### Non-goals (v1)

- Not a GUI or web app. Terminal only.
- Not a media library/DAM: no catalog, tags, search across history, or cloud sync.
- Not a background daemon: Workbench only runs while a process is open (TUI or headless `wb run`).
- Not a rich document editor: editing means extract → `$EDITOR` → re-render (§8.10).
- Not a video editor: no timeline, cutting, compositing.
- Not responsible for installing engines; it detects and guides (§8.8).

### Success criteria (definition of done for MVP)

The §4 scenario (John) runs end-to-end on a machine with only `ffmpeg` installed:

- 10 PNGs → JPEG with a second chained step (background removal when `rembg` is installed, resize otherwise), and `contract.pdf` → DOCX, **both jobs running simultaneously**.
- Per-file progress for both, individually cancellable; cancelled job leaves no partial files.
- Pipeline saved and re-runnable by name from the palette.
- `go test -race ./...` green; app starts in < 300 ms; 80×24 terminal is usable.

---

## 3. Users and jobs-to-be-done

**Primary user:** the author — a developer who lives in the terminal, works with screenshots, recordings, PDFs, and datasets, and already knows ffmpeg but not by heart.

| Job to be done | Today | With Workbench |
|---|---|---|
| Shrink a folder of screenshots | `for f in *.png; do ...; done` | Palette → *Compress image* → `@shots` → run |
| Convert a recording to MP3 | recall ffmpeg flags | Palette → *Extract audio* → alias → run |
| Transcribe a lecture | whisper CLI + model flags | Palette → *Transcribe audio* → pick model → run in background |
| PDF ↔ DOCX | open a web converter | Palette → *Convert document* → run locally |
| Repeat a 3-step recipe monthly | shell history archaeology | Saved pipeline, one keystroke |
| Find out why a job failed | read 200 lines of stderr | Error panel: file, cause, hint, full log behind `d` |

---

## 4. Golden path (north-star scenario)

> **John** opens Workbench to convert 10 PNGs to JPGs and remove their backgrounds, then convert a PDF to DOCX and edit it.

1. `wb` → Dashboard.
2. `ctrl+p` → types `png jpg` → **Convert image**.
3. `ctrl+→` to the steps list, `enter` to edit step 1: `format = jpeg`, `quality = 85`; `enter` commits.
4. `ctrl+←` back to Input, types `@shots` and presses `tab` to complete; alias resolves to 10 files with a count and total size.
5. `ctrl+→` to steps, `n` → palette → **Remove background** (when rembg is installed). It is appended to step 1's output and opens its parameter editor.
6. `ctrl+r` → the chain starts; the compose screen closes and the jobs screen opens.
7. `ctrl+p` → **Convert document** → input `@pdfs/contract.pdf` → target `docx` → `ctrl+r`. Job **pdf-to-docx** starts **in parallel**.
8. `esc` twice → Dashboard shows both jobs with live progress.
9. Conversion job finishes → toast + bell; artifacts listed under the job and in `~/Workbench/2026-10-20/`.
10. `enter` on the finished document job → *Open in editor* (`$EDITOR` on the extracted markdown) → save → Workbench offers *Re-render to DOCX* (§8.10, v1.1).
11. Reopen the chain, `ctrl+s` → name `shots-clean` → available in the palette under `#` forever after.

Every feature in v1 must either appear in this path or directly support it.

---

## 5. Domain model

| Term | Meaning | Lives in |
|---|---|---|
| **Artifact** | A single concrete file (or a future non-file output) with path, kind, format, size, and probed metadata. | `internal/core` |
| **Operation** | A reusable capability with a typed parameter schema, e.g. `image.convert`. Knows how to validate and run itself on a batch. | `internal/core` (contract), `internal/ops/*` (implementations) |
| **Step** | An operation + parameter values + input binding. A node in a pipeline. | `internal/core` |
| **Pipeline** | An ordered (v1) or DAG (v2) set of steps, optionally named and persisted. A pipeline with one step is an ad-hoc job. | `internal/core`, `internal/pipeline` |
| **Job** | A live execution of a pipeline against a concrete input set and target. Has status, steps, items, metrics, logs. | `internal/runner` |
| **Item** | One input artifact flowing through a job. The unit of progress, retry, and per-file error. | `internal/runner` |
| **Run** | Synonym for Job when talking about history ("previous runs"). Persisted to history. | `internal/history` |
| **Engine** | An external binary or in-process implementation that performs work, e.g. `ffmpeg`, `internal`, `rembg`. | `internal/engines` |
| **Alias** | A named shortcut for a path, glob, or list, e.g. `@shots` → `~/Pictures/Screenshots/*.png`. | `internal/alias` |
| **Target** | Where outputs go: directory + filename template + collision policy. | `internal/core` |
| **Preview** | Best-effort visual/textual representation of an artifact in the terminal. Never blocks a job. | `internal/preview` |
| **Param / ParamSpec** | A typed, described input to an operation, used to generate UI, validate, and serialize. | `internal/core` |

**Invariants**

- An Operation never decides where final outputs live; it writes to the paths the runner gives it.
- A Job never writes directly to the user's target directory. It commits atomically (D5).
- Intermediates (non-terminal step outputs) live in a per-job temp directory and are deleted when the job ends.
- Every state change in a Job emits an event (§7.6). The TUI renders only from events; the runner has no UI imports.

---

## 6. What it contains — feature inventory

Legend: **MVP** = ship by 2026-10-31 · **v1.1** = first follow-up · **Later** = backlog.

### 6.1 Image

| Operation | ID | Params (highlights) | Engine | Scope |
|---|---|---|---|---|
| Convert | `image.convert` | `format`, `quality` | internal → ImageMagick | MVP |
| Compress | `image.compress` | `quality` (PNG uses maximum compression) | internal | MVP |
| Resize | `image.resize` | `width`, `height`, `mode` (fit/fill/exact/scale), `filter` | internal | MVP |
| Crop | `image.crop` | `width`, `height`, `gravity`, `x`, `y` | internal | MVP |
| Remove background | `image.remove_bg` | `model` (u2net/u2netp/isnet/bria), `alpha_matting` | rembg | MVP |
| Upscale (AI) | `image.upscale` | `scale` (2/4), `model` | realesrgan-ncnn-vulkan | v1.1 |
| Enhance | `image.enhance` | `sharpen`, `denoise`, `auto_contrast` | ImageMagick/internal | v1.1 |
| Strip metadata | `image.strip_meta` | — | internal | v1.1 |
| Rotate/flip | `image.rotate` | `angle`, `flip` | internal | Later |

Formats: the internal engine reads and writes PNG, JPEG, and GIF. WebP/BMP/TIFF/HEIC/AVIF arrive via ImageMagick/ffmpeg when present (OQ4).

### 6.2 Audio

| Operation | ID | Params | Engine | Scope |
|---|---|---|---|---|
| Convert | `audio.convert` | `format`, `bitrate`, `sample_rate`, `channels` | ffmpeg | MVP |
| Compress | `audio.compress` | `bitrate` (same container) | ffmpeg | MVP |
| Enhance | `audio.enhance` | `preset` (voice/podcast/music), `normalize`, `denoise` | ffmpeg (`loudnorm`, `afftdn`, `dynaudnorm`) | v1.1 |
| Speed | `audio.speed` | `factor` (0.25–4), `preserve_pitch` | ffmpeg (`atempo` chain) | MVP |
| Transcribe | `audio.transcribe` | `model` (tiny/base/small, .en), `language`, `format` (txt/srt/vtt/json) | whisper.cpp | Implemented |

### 6.3 Video

| Operation | ID | Params | Engine | Scope |
|---|---|---|---|---|
| Download | `video.download` | `quality`, `container`, `playlist` (URL inputs) | yt-dlp + ffmpeg | MVP |
| Transcript | `video.transcript` | `language`, `auto`, `timestamps` (URL inputs) | yt-dlp | M7 |
| Metadata | `video.info` | `playlist` (URL inputs) | yt-dlp | M7 |
| Convert | `video.convert` | `format`, `codec`, `crf`, `preset` | ffmpeg | MVP |
| Compress | `video.compress` | `crf`, `preset`, `max_bitrate`, `resolution` | ffmpeg | MVP |
| Extract audio | `video.extract_audio` | `format`, `bitrate` | ffmpeg | MVP |
| Speed | `video.speed` | `factor`, `with_audio` | ffmpeg (`setpts` + `atempo`) | MVP |
| Transcribe | `video.transcribe` | inherits `audio.transcribe` | ffmpeg + whisper.cpp | v1.1 |
| Thumbnail | `video.thumbnail` | `at`, `count`, `size` | ffmpeg | Later |
| Trim | `video.trim` | `start`, `end` | ffmpeg | Later |

### 6.4 Files & documents

| Operation | ID | Params | Engine | Scope |
|---|---|---|---|---|
| Convert document | `doc.convert` | `format`, `engine` (auto/pdf2docx/libreoffice/pandoc) | pdf2docx, LibreOffice, or pandoc | MVP |
| Humanize text | `doc.humanize` | `style`, `model`, `voice`, `chunk_words`, `workers`, `rules` | free models via OpenCode; TXT/MD/DOCX | New |
| Detect AI writing | `doc.detect_ai` | `threshold`, `min_words`, `json` | explainable stylometry; TXT/MD/DOCX/PDF | New |
| Copy | `file.copy` | — | internal | MVP |
| Compress/archive | `file.archive` | `format` (zip/tgz/zst), `level` | internal / Zstandard | MVP |
| Hash | `file.hash` | `algo` (sha256/sha1/md5) | internal | MVP |
| Verify | `file.verify` | `manifest`, `algo` | internal | v1.1 |
| Extract text | `doc.extract_text` | `format` (txt/md) | poppler/pandoc | v1.1 |
| Edit | `doc.edit` | `editor`, `re-render` | `$EDITOR` + engine | v1.1 |

`doc.edit` is a **handoff**, not an in-TUI editor: extract to Markdown/plain text, suspend the TUI (`tea.ExecProcess`), let `$EDITOR` do its thing, then re-render to the original format on save. Rich document editing is explicitly out of scope.

### 6.5 General / cross-cutting

| Feature | Scope | Notes |
|---|---|---|
| Previews | MVP | Images (truecolor half-blocks), text, PDF text, hex; audio/video/archive previews arrive with their engines |
| Agent API | M7 | Local HTTP/JSON over the same runner; `wb serve`, optional bearer token, §8.12 |
| Editor | M8 | Built-in full-screen text editor for text, Markdown, JSON, TOML, configs, §9.6 |
| Settings | M8 | Output folder, max jobs, theme, config handoff |
| Themes | M8 | Eight built-in palettes, previews, config persistence, §9.7 |
| Home + chrome | M8 | Branded home, header breadcrumb, footer rule/status, logo, §9.1/§9.3 |
| Batch jobs | MVP | All operations accept N artifacts |
| Pipelines (create/save/run) | MVP linear, v1.1 DAG | §8.7 |
| Aliases | MVP | §8.4 |
| Keyboard navigation | MVP | Vim-ish + `ctrl` chords, §9.4 |
| Visualization | MVP | Progress bars, throughput, before/after size, job timeline |
| Proper TUI | MVP | Bubble Tea, responsive 80×24 → large |
| Cancellation | MVP | Per job, kills process group, cleans temp |
| Error messaging | MVP | Typed errors + hints + log panel |
| Onboarding | MVP | Engine detection + starter packs, §8.8 |
| Plugin-like extension | v1.1 | Registry + config-defined external ops, §8.11 |
| Online/remote execution | Later | OQ2 |

---

## 7. Architecture

### 7.1 Layers and the dependency rule

```
        ┌────────────────────────────────────────────┐
        │  tui/  (Bubble Tea: screens, components)   │
        └───────────────▲────────────────────────────┘
                        │ reads events, calls runner
        ┌───────────────┴────────────────────────────┐
        │  app/  (wiring, config, registries, DI)    │
        └───────────────▲────────────────────────────┘
        ┌───────────────┴────────────────────────────┐
        │  runner/  (jobs, scheduler, workers, fs)   │
        └───────────────▲────────────────────────────┘
        ┌───────────────┴────────────────────────────┐
        │  ops/  (image, audio, video, doc, file)    │
        └───────────────▲────────────────────────────┘
        ┌───────────────┴────────────────────────────┐
        │  core/  (types, contracts, validation)     │
        └────────────────────────────────────────────┘
              engines/  preview/  alias/  config/  history/
                    (leaf adapters; depend on core only)
```

Rules:

1. `core` imports nothing from the project except stdlib.
2. `ops` and `engines` may import `core`; never `tui`, `app`, or `runner`.
3. `runner` imports `core` and `engines`; never `tui` or concrete `ops`.
4. `tui` imports `core`, `runner` (types/events), `config`, `alias`, `preview`. It never imports `ops` or `engines` directly — the registry from `app` exposes what it needs.
5. `api` imports `core`, `runner`, `config`, `alias`, `engines`, and `pipeline`; it never imports `tui` or concrete operations. The TUI and API share the same composition root.
6. All I/O is context-aware and cancellable.

This keeps the runner testable headlessly and makes a future `wb run` CLI (§7.10) free.

### 7.2 Package layout

```
cmd/workbench/main.go        # flags, app.Boot(), tea.NewProgram
internal/
  core/                      # Artifact, Spec, Param, Values, Request, Result,
                             # Operation, Step, Pipeline, Target, errors, naming
  runner/                    # Runner, Job, scheduler, worker pools, progress,
                             # atomic commit, temp workspace, cancellation
  ops/
    image/  audio/  video/  doc/  file/
  engines/
    registry.go  detect.go  models.go
    ffmpeg/  imagemagick/  libreoffice/  pandoc/  rembg/  whisper/  realesrgan/
  preview/
  alias/
  pipeline/                  # encode/decode, library CRUD
  config/                    # TOML load/validate/defaults, XDG paths
  api/                       # local HTTP agent API over the same runner
  editor/                    # text buffer, undo/redo, atomic save (no TUI)
  history/                   # run log (JSONL)
  app/                       # composition root: wires config+engines+ops+runner
  tui/
    app.go  router.go  keys.go  theme.go
    screens/   components/   styles/
testdata/
  fakebin/                   # fake ffmpeg/etc. for tests
  golden/
docs/
```

**Deliberate change from the raw notes:** no `internal/media/` tree. Domain knowledge lives next to the operations that use it (`ops/image`), and engine-specific code lives in `engines/*`. Two parallel hierarchies would drift.

### 7.3 Core contracts (Go)

```go
package core

type Kind string // "image", "audio", "video", "document", "archive", "unknown"

type Artifact struct {
    Path   string
    Kind   Kind
    Format string            // "jpeg", "mp3", "pdf", "docx"
    Size   int64
    Meta   map[string]any    // width, height, duration_ms, pages, codec…
}

type Operation interface {
    Spec() Spec
    Validate(ctx context.Context, req Request) error
    Run(ctx context.Context, req Request, rep Reporter) (Result, error)
}

type Spec struct {
    ID        string      // "image.convert" — stable, used in pipelines
    Title     string      // "Convert image"
    Summary   string
    Domain    Kind
    Accepts   []Accept    // kind + formats
    Params    []ParamSpec
    Output    OutputSpec  // how to derive the produced format
    Engines   []EngineID  // preference order, first available wins
    Batchable bool
    Cost      Cost        // Fast | Medium | Slow — used for scheduling hints
}

type ParamSpec struct {
    Key      string
    Label    string
    Type     ParamType   // enum, string, int, float, bool, duration, path, format
    Default  any
    Options  []Option
    Min, Max *float64
    Required bool
    Advanced bool        // hidden behind "advanced" in the form
    Help     string
}

type Request struct {
    Items  []Artifact
    Params Values        // typed getters: String/Int/Bool/Duration
    Target Target        // dir + template + collision policy
    DryRun bool
}

type Result struct {
    Artifacts []Artifact
    Skipped   []Skip
    Metrics   Metrics    // bytes in/out, wall time, engine, model
}

// Step is the serializable form of "operation + params + input binding".
type Step struct {
    ID      string
    Op      string
    Params  Values
    Input   StepInput   // v1: Previous(); v1.1: From(stepID)
    OnError OnError     // Continue | Stop
}

// Pipeline is what gets saved to ~/.local/share/workbench/pipelines/*.toml.
type Pipeline struct {
    Name        string
    Version     int
    Description string
    Steps       []Step
}

type OutputSpec struct {
    FormatParam  string // param key that decides the output format, e.g. "format"
    StaticFormat string // used when the format cannot change
}

type Reporter interface {
    Stage(name string)                    // "decoding", "encoding"
    Progress(done, total int)             // item-level
    Detail(item Artifact, msg string)
    Log(level Level, format string, args ...any)
}
```

**Why a declarative `Spec` instead of just `Run`:** the same spec drives (a) the auto-generated compose form, (b) validation, (c) pipeline serialization, (d) palette search keywords, (e) `wb ops list`, and (f) the engine resolver. One source of truth, six consumers. This is the single most important design choice in the project.

Example spec:

```go
var convertSpec = core.Spec{
    ID: "image.convert", Title: "Convert image", Domain: core.Image,
    Accepts: []core.Accept{{Kind: core.Image}},
    Params: []core.ParamSpec{
        {Key: "format", Type: core.ParamEnum, Required: true,
         Options: []core.Option{{"png", "PNG"}, {"jpeg", "JPEG"}, {"webp", "WebP"}}},
        {Key: "quality", Type: core.ParamInt, Default: 85, Min: ptr(1.0), Max: ptr(100.0),
         Help: "Encoder quality; ignored by PNG", Advanced: true},
    },
    Engines: []core.EngineID{"internal", "imagemagick", "ffmpeg"},
    Batchable: true, Cost: core.Fast,
}
```

### Adding an operation (checklist)

1. `Spec()` with ID, accepts, params (typed, with defaults and help), output format rule, engine preference order.
2. `Validate()` for cross-param rules (e.g. JPEG + alpha request) with actionable messages.
3. `Run()` reads only from `req.Items`, writes **only** to runner-provided temp paths, reports progress and stage names.
4. Unit test with a small fixture and a golden output; engine-arg test if it shells out.
5. Fallback/hint path tested: engine missing → disabled op with install hint.
6. Register in `app`; verify it appears in the palette and survives a pipeline save/load round-trip.

If a step in this list feels like overhead, fix the framework, not the operation.

### 7.4 Runner and concurrency

```
Start(pipeline, inputs, target)
        │
        ├── Job (goroutine, own context)
        │     ├── step 1 ── item workers (errgroup, engine-capped)
        │     ├── step 2 ── starts when deps complete
        │     └── commit terminal artifacts per item, atomically
        │
        └── Scheduler ── global semaphores per engine
```

- **Jobs run concurrently**, up to `general.max_jobs` (default 4). Each `Runner.Start` returns immediately with a `*Job`; execution happens in a goroutine owned by the runner. Jobs are dispatched FIFO; there is no priority queue in v1.
- **Statuses.** Job: `queued`, `running`, `cancelling`, `done`, `partial`, `failed`, `cancelled`. Item: `pending`, `running`, `done`, `failed`, `skipped`, `cancelled`. Only `failed` and `partial` are errors in the UI; `cancelled` is neutral.
- **Workers per step:** `errgroup.Group` with `SetLimit(...)`. Batch items fan out. Operations are item-oriented: the runner gives each worker exactly one item, and an intermediate step must produce exactly one artifact to chain. Terminal steps may produce several.
- **Global engine semaphores** prevent 8 ffmpeg jobs from turning the machine into a space heater. Defaults (configurable): `internal = NumCPU`, `ffmpeg = max(1, NumCPU/2)`, `rembg = 2`, `whisper = 1`, `libreoffice = 1`.
- **Failure policy:** per item, the default is continue (`on_error = continue`), so one bad file doesn't waste the batch; `on_error = stop` cancels the remaining items in that step. Across steps, dependents consume only successful outputs; if a step produces **zero** outputs, its dependents are skipped. A job with some successes is `partial`, not `failed`.
- **Cancellation:** `Job.Cancel()` cancels the job context. `exec.CommandContext` is configured with `Setpgid` and a custom `Cancel` that signals the whole process group (`-pgid`), so ffmpeg's children die too. The runner then removes the job temp dir. Terminal artifacts already committed stay (they were atomic).
- **Crash safety:** temp dirs are `~/.cache/workbench/jobs/<job-id>/`. On startup, stale dirs older than 24 h are offered for cleanup in Settings.
- **Idempotency:** outputs are committed via `os.Rename` from the temp dir on the same filesystem. If output and cache are on different volumes, commit falls back to copy-to-`.part`+rename inside the target dir.

```go
func (r *Runner) Start(p pipeline.Pipeline, items []core.Artifact, tgt core.Target) *Job {
    job := newJob(p, items, tgt)
    r.register(job)
    go r.execute(job) // recovers panics → Failed job + log, never crashes the TUI
    return job
}
```

### 7.5 Engine abstraction

```go
type Engine interface {
    ID() core.EngineID
    Probe(ctx context.Context) (Info, error) // path, version, caps, hints
    Caps() Caps                              // formats, codecs, filters
}
```

- Ops declare `Engines: []EngineID` in preference order. `app` resolves each op to a **bound operation** at startup: the first engine whose `Probe` succeeded and whose `Caps` cover the requested formats.
- If no engine is available, the op is **registered but disabled**, with an install hint. The palette shows it greyed with `requires ffmpeg`.
- Engine probing is async and cached (`~/.cache/workbench/engines.json`, 24 h TTL); the dashboard shows a condensed health line.
- Commands are always `exec.Command` with argv arrays — never a shell. Paths with spaces, unicode, and `$` are safe by construction.

Engine matrix (detected, never bundled — D6):

| Engine | Binary | Min | Unlocks |
|---|---|---|---|
| internal | — | — | image convert/compress/resize/crop, archives, hashes |
| ffmpeg | `ffmpeg`, `ffprobe` | 6.0 | all audio/video, thumbnails, fallback image |
| ImageMagick | `magick` / `convert` | 7.0 | HEIC/AVIF, enhance, exotic formats |
| LibreOffice | `soffice` | 7.5 | PDF→DOCX fallback, DOCX/ODT/PPTX/XLSX conversions |
| pandoc | `pandoc` | 3.0 | MD/HTML/EPUB/DOCX text pipelines |
| pdf2docx | `pdf2docx` | 0.5 | PDF→DOCX with table/image layout reconstruction |
| poppler | `pdftotext`, `pdftoppm` | 22.0 | PDF text/preview |
| rembg | `rembg` | 2.0 | background removal (PNG output; model downloads on first use) |
| whisper.cpp | `whisper-cli` | — | audio transcription (ggml model in ~/.local/share/whisper-models) |
| yt-dlp | `yt-dlp` | 2025+ | URL downloads, transcripts, metadata (ffmpeg merges streams) |
| Real-ESRGAN | `realesrgan-ncnn-vulkan` | — | AI upscale |
| 7-Zip | `7z` | 22.0 | 7z archives (optional) |

### 7.6 Events

The runner emits immutable events on a buffered channel; the TUI's `Update` consumes them as `tea.Msg`. This is the only runner→UI coupling, and it makes the runner usable headlessly.

```go
type Event interface{ isEvent() }

type JobQueued     struct{ JobID, Name string; Items int }
type JobStarted    struct{ JobID string; At time.Time }
type StepStarted   struct{ JobID, StepID, Op string }
type ItemStarted   struct{ JobID, ItemID string; Artifact core.Artifact }
type ItemProgress  struct{ JobID, ItemID string; Done, Total int; Stage string }
type ItemDone      struct{ JobID, ItemID string; Out []core.Artifact; Err error }
type StepDone      struct{ JobID, StepID string; Err error }
type JobDone       struct{ JobID string; Status core.Status; Metrics core.Metrics }
type JobLog        struct{ JobID string; Level core.Level; Line string }
```

Event ordering per job is total; ordering across jobs is not guaranteed. The TUI keeps a per-job state machine and must tolerate out-of-order timestamps across jobs.

### 7.7 Output, naming, and targets

```go
type Target struct {
    Dir       string      // template, default: ~/Workbench/{date}
    Template  string      // default: "{stem}.{ext}"
    Collision Collision   // suffix | overwrite | skip | fail  (default: suffix)
}
```

- Template variables: `{stem}`, `{ext}`, `{name}`, `{date}`, `{time}`, `{op}`, `{index}`, `{alias}`.
- **Intermediates never land in `Dir`.** Step N reads step N-1's temp outputs. Only artifacts from terminal steps are committed.
- **Format-aware defaults:** when chaining, the compose screen auto-adjusts conflicting params (JPEG → PNG when a later step needs alpha) and shows a one-line note explaining why.
- Collision check happens at commit time under a lock, so parallel jobs writing to the same directory cannot clobber each other.
- Directory inputs: non-recursive by default; `recursive` param available per step. Symlinks to files are followed; symlinked directories are skipped unless `follow_symlinks = true` (loop-guarded).
- `overwrite` is never the default and requires an explicit confirmation on run.

### 7.8 Persistence and paths

| What | Where | Format |
|---|---|---|
| Config | `$XDG_CONFIG_HOME/workbench/config.toml` (default `~/.config`, resolved by `adrg/xdg`) | TOML |
| Pipelines | `~/.local/share/workbench/pipelines/*.toml` | TOML |
| Run history | `~/.local/share/workbench/history.jsonl` | JSONL, rotated at 5 MB |
| Job temp | `~/.cache/workbench/jobs/<id>/` | files |
| Engine cache | `~/.cache/workbench/engines.json` | JSON |
| Models | `~/.local/share/workbench/models/` | downloaded, checksummed |
| Logs | `~/.local/share/workbench/logs/<job-id>.log` | plain text |

`WORKBENCH_CONFIG_DIR`, `WORKBENCH_DATA_DIR`, `WORKBENCH_CACHE_DIR` override for testing. Paths are resolved by the small internal XDG resolver (`internal/config/paths.go`): on every platform the defaults are `~/.config`, `~/.local/share`, and `~/.cache`. macOS Library paths are deliberately not used — developer tools belong in XDG directories. Invalid config values never prevent startup: the app falls back to the default for that key and shows a dismissible warning naming the file and key.

### 7.9 Error model

```go
type Error struct {
    Code    Code    // usage | input | engine_missing | engine_failed | output | internal | canceled
    Op      string
    Path    string
    Cause   error
    Hint    string  // actionable next step, shown in the UI
}
```

- Errors are wrapped with `%w`; the UI displays `Hint`, the log file gets `Cause` plus engine stderr.
- Every job failure produces one line a human can act on: `contract.pdf: LibreOffice exited 1 — unsupported PDF encryption. Hint: decrypt the PDF first (qpdf --decrypt). [d]etails`
- No panics escape a job goroutine (`recover` → `internal` error + job failure). The TUI never crashes because an operation did.
- `canceled` is not a failure in the UI; it renders as `cancelled`.

### 7.10 CLI surface (cheap, and it tests the runner)

```
wb                      # TUI
wb serve [--addr 127.0.0.1:8765] [--token TOKEN]   # local agent API (M7)
wb run <pipeline> -i @shots [-o DIR] [--dry-run]   # headless (v1.1)
wb ops list [--json]                               # catalog (v1.1)
wb doctor                # engine detection table (MVP)
wb version
```

Building the runner UI-agnostic from day one makes `wb run` a ~100-line addition and gives CI a way to test real pipelines.

---

## 8. Feature specifications

### 8.1 Home

- Header: a small dither row of three accent shades, wordmark, breadcrumb (not on Home), and active-job percentage; no engine list or idle `ready` label.
- Body: one calm, centered composition (no outer border) with the full ANSI Shadow `WORKBENCH` wordmark, five primary actions (`n` new job, `e` editor, `j` jobs, `s` settings, `?` help), recent jobs, and environment details where space allows.
- Footer: context keys on the left; toast on the right, above a full-width rule.
- Empty state: “No jobs yet. Press n to start.” No duplicate job table or charts on Home; those live on Jobs.
- `j` opens Jobs; `enter` on a job from Jobs opens detail; `R`/`r` retry/re-run; `x` cancels; `d` opens logs.
- Completion notifications: terminal bell + OSC 9 desktop notification when a job finishes while the app is unfocused. Configurable.

### 8.2 Command palette

- `ctrl+p` from anywhere. Fuzzy search over: operations (title, ID, keywords, accepted formats), saved pipelines, aliases, screens, settings.
- Grouped results: **Operations**, **Pipelines**, **Aliases**, **Go to**, **Settings**.
- Shows disabled ops with the missing engine and an install hint (§7.5).
- Typing `>` restricts to commands, `@` to aliases, `#` to pipelines (like editors' quick-open prefixes).
- `enter` opens the compose screen pre-filled with that operation, or appends it when the palette was opened from an existing chain (`n` on the steps list).
- Chain-incompatible operations are marked disabled with the reason (for example, `audio` after an `image` step), and a missing engine shows its install hint.
- Running is always a deliberate action on the compose screen (`ctrl+r`).

### 8.3 Compose screen

The compose screen is a chain view plus a modal step editor.

1. **Chain view** — `Input`, `Output`, and the ordered steps. Inputs accept paths,
   globs, comma-separated lists, `@alias`, `@last`, and `@clipboard`. Steps show
   their key parameters (`1. Convert image · Format JPEG · JPEG quality 85`).
   Keys: `ctrl+→`/`ctrl+←` cycles input → output → steps; on the steps list
   `j/k` selects, `enter` edits, `n` appends a step from the palette, `d`
   removes, `J/K` reorders, `ctrl+r` runs, `ctrl+s` saves. In text fields,
   `tab` completes the selected path/alias suggestion instead of changing focus.
2. **Step editor** — generated from `ParamSpec` (enum → select, int/float →
   numeric field, bool → toggle, duration → `10s`). `ctrl+→`/`ctrl+←` moves
   between fields, `←/→` changes a choice, `space` toggles, `enter` commits,
   `esc` discards.
3. **Output** — target directory from config, filename template, collision
   policy. A live size estimate is not implemented yet.
4. **URL mode** — when the first step is a URL-source operation (video download, transcript, or metadata),
   the input field accepts comma-separated `http(s)` URLs instead of files. URLs
   are runtime inputs and are not written into saved pipelines.

Validation is inline: missing inputs, incompatible step outputs, unavailable
operations, and engine errors appear under the chain. Reordering or deleting a
step re-runs the static compatibility checks on the next run/save.

### 8.4 Aliases

```toml
[[alias]]
name  = "shots"
path  = "~/Pictures/Screenshots"
glob  = "*.png"
# or a fixed list:
# files = ["~/a.pdf", "~/b.pdf"]
```

- Resolved lazily at input time; the compose screen shows count and total size after the debounce.
- Built-ins: `@cwd`, `@last` (newest job's inputs), `@clipboard` (paths from the system clipboard), `@downloads`, `@documents`, `@development`/`@projects`, `@pictures`, `@videos`, `@music`, `@desktop`, `@screenshots`.
- Aliases appear in the palette under `@`; selecting one inserts it into the active compose input.
- `@alias/<relative>` resolves relative to the alias root, so `@pdfs/contract.pdf` works even when the alias has a glob.
- URL-source operations use a different input mode: the compose field accepts `http(s)` URLs and creates synthetic URL artifacts; those URLs are runtime inputs and are not saved into the pipeline.
- The Aliases screen is read-only. `e` opens `config.toml` in `$EDITOR`; changes apply on the next launch. Editing the file directly is equally supported — it is the same TOML.
- Unknown aliases are reported inline with the failing pattern.

### 8.5 Jobs screen and detail

- **List:** one job per row with status, name, aggregate percent, settled counts, duration.
- **Selected job:** step name, byte traffic, per-item rows with stage, percent, and the first error/hint.
- **Actions:** `x` cancel, `ctrl+o` preview an output, `d` open the log overlay, `R` retry failed items, `r` re-run the job, `o` reveal the output directory, `c` copy the error.
- **Logs:** `d` shows timestamped runner + engine lines; `j/k` scrolls, `c` copies the whole log, `esc` closes. Logs are also written to `$XDG_DATA_HOME/workbench/logs/<job-id>.log`.
- **Visualization:** aggregate and per-item progress, a saved-size badge, and a progress sparkline.
- A job that finishes while you're on another screen produces a toast.

### 8.6 Previews

`internal/preview` returns a value the TUI renders according to terminal capability:

```go
type Preview struct {
    Kind  PreviewKind // image | video | audio | text | pdf | listing | hex | none
    Text  string      // fallback text (always populated)
    ANSI  string      // truecolor half-block render, if image-like
    Meta  []string    // dimensions, duration, codec, pages
}
```

- **Images:** decode, downscale to the pane, render with `▀` half-blocks and 24-bit color. Fallback: 16-color ASCII art. Kitty/iTerm2 graphics protocols are a v1.1 enhancement (OQ5).
- **Video:** ffmpeg thumbnail at 10% duration + metadata; `enter` opens it in the OS default player.
- **Audio:** waveform from ffmpeg-decoded PCM, bucketed to pane width, rendered with block glyphs; duration/bitrate/codec.
- **PDF:** `pdftoppm` page 1 if present, else extracted text head.
- **Documents/text:** first ~200 lines, syntax-free; archives: top-level listing; unknown: size + magic bytes.
- Previews run as `tea.Cmd`s with a 2 s timeout and are cached by `(path, mtime, size)`. They are best-effort and never block the runner.

### 8.7 Pipelines

```toml
name    = "shots-clean"
version = 1
description = "Screenshots → JPEG → background removed"

[[step]]
op = "image.convert"
[step.params]
format  = "jpeg"
quality = 85

[[step]]
op = "image.remove_bg"
[step.params]
model = "isnet-general-use"
```

- **MVP is linear.** Step N consumes all successful outputs of step N-1. Explicit DAG edges (`from = "step.1"`, multiple parents) are v1.1; the file format already reserves `[step.input] from = ...` so no migration is needed.
- `version` is checked on load; an unknown newer version is refused with a hint to upgrade, never silently misread.
- Steps run sequentially by default; independent DAG branches may run concurrently once DAG support lands.
- Storage: `$XDG_DATA_HOME/workbench/pipelines/<name>.toml`. Writes are atomic, names are slugs, and files are human-editable.
- Saving: `ctrl+s` on compose opens a name prompt. Saved pipelines appear in the palette under `#`.
- Running: pick a `#` pipeline → the chain loads into compose → choose inputs/output → `ctrl+r`. Step parameters can still be edited before running.
- Editing: load the pipeline into compose, or edit the TOML directly.
- Deleting: `ctrl+d` on a `#` entry moves the file to `<pipelines>/.trash/`.
- Compatibility: static checks reject chains whose next step cannot consume the previous step's kind/format. `image.remove_bg` produces PNG, so JPEG→PNG alpha coercion is naturally satisfied for cutout chains.

### 8.8 Onboarding

Shown when `config.toml` does not exist, and re-runnable from the palette (`> Run setup`):

- Welcome, engine scan (found/missing with version or install hint), starter packs, and the output directory on one screen.
- *Essentials* is always on. *Media* toggles ffmpeg. *Documents* toggles LibreOffice, pandoc, and poppler.
- `j/k` moves, `space` toggles a pack, `enter` saves and finishes, `esc` skips (writes the current selection).
- The config is written atomically. Engine-set changes take effect on the next launch; the current screen shows a toast saying so.
- Missing engines never block the app; unrelated operations remain usable.

### 8.9 Engines screen

- Reachable from `ctrl+p` → `> Engines`.
- Rows show engine ID, ready/missing, version or install hint, and binary path.
- `r` re-probes; `esc` returns to the dashboard.
- Config and aliases are editable through the Aliases screen (`e`) or `> Open config`; both suspend the TUI and run `$EDITOR`.
- A full tabbed Settings screen (theme, max jobs, log level, cleanup) is future work.

### 8.10 Document edit handoff

Workbench has two editing paths:

- **Built-in editor (M8):** text, Markdown, JSON, TOML, configs, and small plain-text documents open in the full-screen editor in §9.6. `e` from home asks for a path; `e` from Aliases opens `config.toml`.
- **Document handoff (v1.1):** `doc.edit` on a PDF/DOCX/EPUB still extracts to Markdown/plain text, suspends the TUI for `$EDITOR` or the built-in editor, then re-renders. Rich document editing remains out of scope.

1. `doc.extract_text` to Markdown (or plain text for EPUB) in the job temp dir.
2. `tea.ExecProcess($EDITOR)` — TUI suspends, editor takes over.
3. On exit: diff check; if unchanged, done (no pointless reconversion).
4. If changed: `doc.convert` back to the original format, commit atomically next to the source with `-edited` suffix (collision policy applies).

This is honest about scope: terminals are not document editors, but the round-trip is genuinely useful for text-heavy PDFs.

### 8.11 Plugin-like extensibility

v1: **registry-based** — operations are Go types registered in `app`; adding one is a new file in `internal/ops/*` plus one registration line.

v1.1: **external operations in config**, no Go required:

```toml
[[external_op]]
id        = "audio.denoise_strong"
title     = "Denoise (strong)"
domain    = "audio"
accepts   = ["audio"]
produces  = "inherit"
command   = ["my-denoise", "--strength", "{param:strength}", "-o", "{out}", "{in}"]
params    = [{ key = "strength", type = "int", default = 7, min = 1, max = 10 }]
```

Template placeholders are substituted per item (`{in}`, `{out}`, `{dir}`, `{param:key}`). Runs through the same runner, so progress is indeterminate but cancellation, errors, and output commit behave identically.

v2 (not planned): out-of-process plugin protocol over stdio JSON-RPC. The `Operation` interface is the seam; no work needed now beyond keeping `core` dependency-free.

**Do not use Go's `plugin` package** — macOS support is fragile and it constrains builds to identical toolchains (D7).

### 8.12 Agent API

`wb serve` exposes the same operation catalog and runner as the TUI over local HTTP/JSON. It is headless: no TUI, no daemon state, and no second execution engine. The API is for agents, scripts, and CI, not for remote multi-user hosting.

- **Binding and auth:** `127.0.0.1:8765` by default. Optional bearer token via `--token` or `WORKBENCH_API_TOKEN`; if other processes can reach the port, use it. `GET /healthz` is public; `/v1/*` requires the token when configured.
- **Discovery:** `GET /v1/operations` returns the same `Spec` data the form uses: source (`files`/`urls`), params, defaults, engines, output. `GET /v1/engines` probes availability. `GET /v1/pipelines` lists saved recipes.
- **Execution:** `POST /v1/jobs` accepts exactly one of `operation`, `pipeline`, or `steps`. File inputs resolve through the normal alias/glob/directory path; URL operations take `http(s)` URLs. `target` overrides output directory/template/collision; defaults come from config.
- **Observation:** `GET /v1/jobs` lists recent jobs; `GET /v1/jobs/{id}?wait=30` long-polls until terminal state or timeout. Snapshots carry items, outputs, counts, metrics, logs, and typed errors (`code`, `message`, `hint`, `path`).
- **Control:** `POST /v1/jobs/{id}/cancel`, `/retry`, `/rerun` mirror the TUI keys and return `202` with the new job where applicable.
- **Errors:** stable JSON envelope; HTTP codes distinguish usage/input (400), auth (401), missing (404), state/engine (409), output (422), engine failure (502), internal (500).
- **Why now:** the runner was already UI-agnostic (D9). The API is a thin adapter over it, so the TUI and agents cannot drift on behavior, atomicity, cancellation, or naming.

The full request/response contract lives in [`docs/agent-api.md`](./docs/agent-api.md).

### 8.13 Compound jobs

Compound presets are searchable in the palette (`compound`): PDF→DOCX, Web image, Podcast audio, and Share video. Selecting one opens a normal Compose screen with the steps already assembled, so every parameter remains editable.

PDF editing is deliberately honest: PDF→DOCX is viable through LibreOffice's Writer PDF import, but PDF import preserves layout as positioned text/frames rather than a clean flow. The preset prepares the DOCX for Word/Pages/LibreOffice; the built-in editor is plain text and cannot edit DOCX. A rich in-TUI DOCX/PDF editor, and reliable PDF→Markdown extraction, are not in scope.

### 8.14 Settings

- `s` from Home, or `> Settings` from the palette, opens a Settings panel: output folder, theme, max jobs, aliases, and config file.
- `enter` edits the selected value in a prompt; output folder and max jobs save atomically to `config.toml`. `t` opens the theme picker; `e` opens `config.toml` in the built-in editor.
- The panel shows current values and a short preview of where changes are written. Advanced options stay in the file editor rather than expanding the Settings screen.

---

## 9. TUI specification

### 9.1 Structure

```
app
├── header        (dither mark, wordmark, breadcrumb, active jobs)
├── body          (one active screen + optional side pane)
├── statusline    (contextual keys, persistent error chip, toasts)
└── overlays      (palette, help, confirm, form modal, preview)
```

Screens: `home`, `operations`, `compose`, `jobs`, `job-detail`, `preview`, `aliases`, `engines`, `themes`, `editor`, `onboarding`. A simple router with a return screen supports `esc` (pop) and direct jumps from the palette. Overlays are modal but never lose underlying state.

### 9.2 Layout rules

- Minimum 80×24. Below that, show a single-line message with the required size (don't try to reflow everything).
- Widths: inputs 30% / steps 45% / output 25% at ≥120 cols; stacked vertically at <100.
- All lists are virtualized (`bubbles/viewport` semantics) — 5,000 inputs must not freeze the UI.
- No layout depends on mouse; mouse support is opt-in via config and off by default.

### 9.3 Visual language

- Lipgloss styles in one theme file (`tui/theme.go`) with semantic roles and eight built-in palettes.
- Semantic colors only: `accent`, `success`, `warning`, `danger`, `muted`, `border`, `gutter`, `currentLine`. No ad-hoc colors in screens.
- Identity: a compact dither row of three accent shades in the header; Home shows a larger two-row rendition centered above the full ANSI Shadow `WORKBENCH` wordmark, with a blank line between. `assets/logo.svg` is the matching dither mark.
- Header: dither row of three accent shades + `Workbench` + breadcrumb (not on Home); active-job percentage on the right; one rule below.
- Footer: one rule, context keys on the left, toast or theme/status on the right.
- Icons are ASCII/Unicode with no Nerd Font dependency for core flows; Nerd Font glyphs are decorative only.
- Honors `NO_COLOR`, `CLICOLOR_FORCE`, and dumb terminals (degrade to plain text, no progress animation).
- Animation: one spinner tick per 100 ms, shared; progress bars only re-render on change. Nothing animates while the app is idle (battery).
- Framed layout: titled panels (`╭─ Jobs ─╮`) with accent borders on the focused pane. Home is one centered panel with a wide Recent|Environment split; Jobs splits into full-width list/detail; Compose splits Pipeline/Steps. Sparklines use `▁▂▃▄▅▆▇█`; progress bars use `⣿`/`⣀` dots.
- Density: aligned label/value columns, status glyphs, compact engine rows, and a statusline with key groups rather than prose.

### 9.4 Keymap

Global:

| Key | Action |
|---|---|
| `ctrl+p` | Command palette (toggle) |
| `ctrl+r` | Run the composed pipeline |
| `ctrl+s` | Save the pipeline (compose) |
| `ctrl+o` | Preview input/output |
| `ctrl+d` | Delete the selected saved pipeline (palette) |
| `?` | Help / full keymap (not while typing) |
| `esc` | Back / close overlay / skip setup |
| `q` | Quit (not while typing in a field; confirms if jobs are running) |
| `ctrl+c` | Quit (confirms if jobs are running) |
| `tab` | Complete the selected path/alias suggestion |
| `ctrl+→` / `ctrl+←` | Next/previous field or pane |
| `ctrl+v` | Paste from the system clipboard (bracketed Cmd+V paste is handled too) |
| `j` `k` / arrows | Move through lists; on the steps list, select |
| `enter` | Open / confirm / edit the selected step / finish setup |
| `space` | Toggle a boolean / cycle an enum / toggle a starter pack |
| `n` | New operation (home) / append a step (compose steps) |
| `e` | Open the built-in editor (home); edit `config.toml` from Aliases; edit the previewed file |
| `t` | Open the theme picker |
| `s` | Open Settings from Home |
| `d` | Remove the selected step / open the selected job log |
| `R` | Retry failed items (jobs screen) |
| `r` | Re-run the selected job (jobs screen) |
| `o` | Reveal the output directory |
| `J` `K` | Move the selected step down/up |
| `r` | Re-scan engines (Engines screen) |
| `x` | Cancel the selected job |
| `←` `→` | Change an enum or toggle |

Screen-specific keys are listed in the statusline, so the app teaches itself.

Editor keys:

| Key | Action |
|---|---|
| `ctrl+s` | Save atomically |
| `ctrl+z` / `ctrl+y` | Undo / redo |
| `ctrl+f` | Forward search (enter finds; esc cancels) |
| `esc` / `ctrl+q` | Close; dirty buffer asks save/discard |
| `s` / `d` | Save / discard when the dirty prompt is open |
| arrows, `home`, `end`, `pgup`, `pgdn` | Move the cursor |
| `tab` | Insert two spaces |

Theme picker: type to filter, arrow keys move, `enter` applies and saves, `esc` cancels.

### 9.5 State and responsiveness

- One `tea.Model` per screen; shared `AppState` (config, jobs, registry, alias store) passed by pointer and only mutated in `Update`.
- All blocking work happens in `tea.Cmd` or the runner goroutine; `Update` never does I/O. A 5,000-file glob must not hitch a keystroke.
- Input debounce 80 ms for glob expansion; expansion runs in a `tea.Cmd` with cancellation of the previous expansion.
- Progress events are coalesced (max 30 UI updates/sec/job) to keep rendering under 16 ms.

### 9.6 Editor

The built-in editor is for text, Markdown, JSON, TOML, configs, and small plain-text documents. It is intentionally insert-oriented, not a Vim emulation.

- Full content area with a line-number gutter, current-line surface, horizontal/vertical scroll, and the real terminal cursor via `tea.View.Cursor`.
- `internal/editor.Buffer` owns lines, rune cursor, undo/redo, search, and atomic save; the TUI owns rendering, scrolling, and key translation. This keeps save and cursor behavior testable without a pty.
- Open from Home (`e`), Aliases (`e` for `config.toml`), palette (`> Open editor` / `> Open config`), or Preview (`e`).
- Saving `config.toml` is safe but engine and alias changes require restart; the status toast says so.
- Not included: syntax highlighting, multi-buffer tabs, selections, block editing, and binary files. Rich DOCX/PDF editing remains the §8.10 handoff.

### 9.7 Themes

- Eight palettes live in `tui/palettes.go`: opencode, tokyonight, catppuccin, gruvbox, nord, monokai, vscode, rosepine. Each defines background, foreground, cursor, and the 16 ANSI colors.
- The picker shows every theme, filters by name, previews the selected palette, and writes `general.theme_name` atomically to `config.toml`.
- A missing theme name falls back to `opencode` and shows a notice.
- Semantic mapping: accent = palette 6 or an explicit theme accent (VS Code uses #007acc), success = green, warning = yellow, danger = red, muted/border = palette 8, current line = a blend of background/foreground. This keeps every screen readable even when a theme has an unusual palette.

---

## 10. Non-functional requirements

| Area | Requirement |
|---|---|
| Startup | < 300 ms to first frame with cached engine probe; full probe async |
| Keystroke latency | < 16 ms p95, 5k-item lists |
| Memory | < 100 MB idle; previews capped at 64 MB decoded |
| Concurrency | 2+ jobs, 100+ items per job without UI degradation |
| Atomicity | No partial/corrupt terminal artifact ever appears in output dir |
| Cancellation | < 1 s from keypress to process-group death, temp cleaned |
| Portability | macOS (arm64) primary, Linux (amd64/arm64) v1.1; Windows later |
| Failure containment | No panic reaches the TUI; job failure never loses other jobs |
| Accessibility | Usable at 80×24, monochrome-safe, no font dependency for core flows |
| Privacy | No network except explicit model downloads; no telemetry |

---

## 11. Tech stack

| Concern | Choice | Why |
|---|---|---|
| Language | Go 1.26+ | Single binary, great concurrency/exec story, the learning goal (D1) |
| TUI | `charm.land/bubbletea/v2` + `charm.land/lipgloss/v2` | v2 is stable (Oct 2026) and this is greenfield; starting on v1 would just buy a future migration. Bubbles v2 when components are needed |
| Forms | Generated from `ParamSpec`; `bubbles/textinput`, `list`, custom selects | `huh` is nice for standalone forms but fights embedding in modal workflows; revisit if form code exceeds ~400 LOC |
| Concurrency | `golang.org/x/sync/errgroup` | Worker limits and error collection without hand-rolling |
| Config | TOML via `pelletier/go-toml/v2` | Human-editable, comments, no YAML indentation traps (D3) |
| Pipelines | TOML | Same reason, same library |
| Paths | Internal XDG resolver (`internal/config/paths.go`) | macOS deliberately uses `~/.config`, not `~/Library`; keeps docs and user expectations identical across platforms |
| CLI | stdlib `flag` for v1; `alecthomas/kong` if subcommands grow | Avoid dependency until it earns its place |
| Image | stdlib `image/*` + `golang.org/x/image/draw` | Pure Go, fast enough, no install; Catmull-Rom and nearest-neighbor scaling |
| Compression | stdlib `archive/*`, `compress/gzip` + `klauspost/compress/zstd` | Pure Go |
| Hashing | stdlib `crypto/sha256`; `zeebo/blake3` for BLAKE3 | Pure Go |
| IDs | `oklog/ulid/v2` or `crypto/rand` hex | Sortable job IDs for logs/temp dirs |
| Logging | `log/slog` → file; engine stderr captured per item | Stdlib, structured, no UI coupling |
| Testing | stdlib `testing` + `teatest` for TUI | Minimal |
| Clipboard | `atotto/clipboard` | `@clipboard` without a platform-specific implementation |
| Lint | `golangci-lint` (`errcheck`, `gosec`, `revive`, `staticcheck`) | Catch Go footguns early; the author is new to Go |
| Release | `goreleaser` + Homebrew tap (v1.1) | Single binary distribution |

**Engine licensing:** engines are detected, not bundled. Document each engine's license in `docs/engines.md` and never ship a GPL binary inside the Workbench archive (D6).

---

## 12. Testing strategy

| Layer | What | How |
|---|---|---|
| `core` | Param validation, target naming, collision policies, pipeline encode/decode round-trip | Table tests; golden TOML |
| `ops/*` | Image transforms, background removal argument construction, hash correctness, archive contents, ffmpeg arg construction | Unit tests; pixel/hash assertions with tolerance; fake rembg/ffmpeg; golden files in `testdata/golden` |
| `engines/*` | Detection, version parsing, error mapping, command building | Fake binaries in `testdata/fakebin` (shell scripts that emit `-progress` output) |
| `runner` | Lifecycle, parallelism, cancellation, atomic commit, partial failures | Fake operations; `-race`; deterministic event assertions; temp dirs via `t.TempDir()` |
| `alias`, `config` | Expansion, globbing, permissions, precedence | Table tests with `t.Setenv` and fixture homes |
| `preview` | Fallback rendering, timeouts, cache keys | Golden ANSI/plain text at fixed sizes |
| `pipeline` | TOML round-trip, slug/version guards, atomic save, soft delete | Temp store; malformed-file listing |
| `ops/doc` | Engine selection, PDF→DOCX (pdf2docx tables/images), Markdown→HTML, missing-engine hints | Fake binaries plus real LibreOffice/pdf2docx integration tests that skip when absent |
| `ops/file` | ZIP/tar.gz/Zstd contents, hashing, copying | Read archives back and compare bytes |
| `ops/audio` | Real speech transcription and M4A preparation | whisper.cpp + macOS `say`; skips when the model/tool is absent |
| `runner` logs | In-memory lines, persisted job log, error detail | Fake operation logging + failed item |
| `api` | Catalog, job creation/long-poll, auth, steps and saved pipelines | `httptest` server; `file.copy` as the deterministic operation; no network |
| `editor` | Insert/join/undo/redo/find, dirty state, atomic save and mode preservation | Buffer tests; no pty |
| themes | Eight palettes, semantic mapping, previews, persistence | TUI model tests; no terminal |
| Onboarding | Pack toggles, config write, first-run routing | TUI model test with a temp config path |
| TUI | Palette→chain→run flow, fuzzy filter, step editing, save prompt, panels, resize, key handling | Direct model tests at 80×24/120×40; golden frames when the layout settles |
| Integration | Real ffmpeg conversions and cancellation | Tests skip automatically when ffmpeg is absent; generation uses `lavfi` sine/testsrc sources |

Rules:

- No network in unit tests. Model downloads are tested with a local `httptest` server.
- Every user-visible error path gets a test that asserts the **hint**, not just the error.
- `make check` = `gofmt -l`, `go vet ./...`, `golangci-lint run`, `go test -race ./...`. CI runs it on every push.
- TUI golden files are intentionally small (a few frames), not full-screen dumps, to avoid churn.

### Conventions

- Packages are named for what they provide (`runner`, `preview`), not what they contain (`utils`, `helpers`).
- Errors are wrapped with `%w` and converted to `core.Error` at the operation/engine boundary, where the hint is known.
- No package-level mutable state except the operation registry, which is built once in `app`.
- Tests live next to the code; fixtures in `testdata/`; no test-only branches in production code.
- Commits: `type(scope): imperative summary`, one logical change per commit (matches the repo's git conventions).
- Exported types get doc comments; unexported helpers get comments only when the *why* is non-obvious.

---

## 13. Decisions log

| # | Decision | Rationale |
|---|---|---|
| D1 | Go + Bubble Tea, one binary, no daemon | Matches the goal (learn Go), concurrency fits batch jobs, distribution is trivial |
| D2 | Pin Bubble Tea v2 (`charm.land/bubbletea/v2` v2.1.0) | v2 is stable as of October 2026 and this is a greenfield project; v1 would only defer a migration |
| D3 | TOML for config and pipelines | Editable by hand, comment-friendly, good Go library; JSON is for machine state only |
| D4 | Declarative `Spec` + `ParamSpec` drive UI, validation, serialization | One source of truth; adding an operation doesn't require new UI code |
| D5 | Temp dir per job; terminal artifacts committed atomically via rename | No broken outputs on cancel/crash; enables chain intermediates cheaply |
| D6 | Engines are detected, not bundled | Avoids licensing/size/distribution pain; `doctor` makes setup honest |
| D7 | No Go `plugin` package; external ops via config-defined commands | Go plugins are fragile on macOS; config commands give 80% of the value for 5% of the risk |
| D8 | Linear pipelines for MVP; DAG format reserved | John's scenario is linear; ship the format without migration debt |
| D9 | Runner is UI-agnostic and emits events | Testability, headless CLI later, TUI stays thin |
| D10 | Previews are best-effort and cached; ANSI half-blocks first | Works in every terminal; graphics protocols are an enhancement, not a dependency |
| D11 | Default output `~/Workbench/YYYY-MM-DD/`, collision `suffix` | Predictable, never destructive by default |
| D12 | Global max-jobs cap plus per-engine limits | ffmpeg saturates CPU; more jobs ≠ more throughput |
| D13 | Doc editing = extract → `$EDITOR` → re-render | Honest scope; a TUI rich-text editor is a different project |
| D14 | Minimal aliases in MVP (`@alias`, `@clipboard`, `@last`; list in Settings, edit via config/`$EDITOR`) | High value, low build cost; a dedicated alias form UI can wait |
| D15 | URL downloads are URL-source operations backed by yt-dlp | URLs are runtime inputs, not pipeline params; keeps saved recipes reusable with new links |
| D16 | Stay on Go for the product; native/C++ only for hot engines later | The bottleneck is external media tools, not the orchestrator; Go's concurrency, single binary, and TUI ecosystem outweigh a C++ rewrite |
| D17 | Agent surface is a local HTTP/JSON API, not an embedded LLM or a remote daemon | Agents already speak HTTP; sharing the runner keeps TUI and agent behavior identical; localhost + optional token keeps the trust boundary honest |
| D18 | Transcript and metadata are separate URL operations with stable text/JSON outputs | Downloading media, fetching captions, and reading metadata have different costs and failure modes; agents can call only what they need |
| D19 | Eight curated built-in palettes instead of importing a terminal theme dump | Predictable previews, no filesystem scanning, and a coherent UI across all eight choices |
| D20 | Configs and text documents use the built-in editor; `$EDITOR` remains an escape hatch for rich document handoff | Keeps editing inside the TUI, preserves context, and avoids a suspend/resume cycle for a one-line alias change |

## 14. Open questions and research spikes

| # | Question | Default if unresolved | Spike |
|---|---|---|---|
| OQ1 | Background removal engine: `rembg` CLI vs a Go-native ONNX path? | `rembg` optional engine (BRIA default, u2net/u2netp/ISNet options); op disabled if absent | Implemented and verified on a real photo; rough CPU cost is ~30s per 1000×667 image with BRIA |
| OQ2 | Remote/online operations (call an API for heavy models)? | Not in MVP; `Engine` interface is the seam | None now; design review before v2 |
| OQ3 | Transcription runtime: whisper.cpp binary vs faster-whisper vs Go bindings? | whisper.cpp binary; `small.en` default, other .en/multilingual models selectable | Implemented; real speech transcribed locally (~1s for 5s audio after model load) |
| OQ4 | WebP encode: pure-Go (`gen2brain/webp`/wazero) vs `cwebp` vs ImageMagick? | Decode internal; encode via ImageMagick if present, else unsupported with hint | Build a 5-line encode test with the wazero lib |
| OQ5 | Terminal image protocol support (kitty/iTerm2/sixel) in v1? | ANSI half-blocks only | Prototype capability detection behind a flag |
| OQ6 | PDF→DOCX fidelity: LibreOffice vs `pdf2docx` (Python)? | pdf2docx for PDF→DOCX (tables/images), LibreOffice fallback | Implemented; pdf2docx is preferred and its DOCX retains table elements |
| OQ7 | Should models auto-download during onboarding or only on first use? | On first use, with size/license consent | UX call; default is privacy-preserving |
| OQ8 | Windows support timeline? | Post-v1; avoid Unix-only assumptions in `core` | None |
| OQ9 | History retention and privacy (paths in `history.jsonl`)? | Local only, 5 MB rotation, `wb history clear` | Trivial, decide at implementation |
| OQ10 | Saved pipeline portability/share format? | Single-file TOML, `wb pipeline export` later | None |

---

## 15. Milestones (2026-10-20 → 2026-10-31)

Assumes ~4–5 focused hours/day alongside exams/interview. Each milestone ends with something demoable.

### M0 — Walking skeleton (Oct 20–21)
- `go mod init`, package tree, `Makefile` with `make check` (no CI yet).
- `cmd/workbench` boots Bubble Tea; header/statusline/theme; dashboard stub; `q` and `?` work.
- Config load with defaults + XDG paths; `wb doctor` prints engine table (detect ffmpeg).
- **Demo:** app starts, resizes cleanly, tells you ffmpeg exists.

### M1 — Operation core + runner (Oct 22–23)
- `core`: Artifact, Spec, ParamSpec, Values, Target, errors, operation registry.
- `runner`: jobs, events, one worker per item, temp dirs, atomic commit, cancellation, panic recovery.
- Three real utility ops registered — `file.copy`, `file.hash`, `image.convert` — instead of fakes, so none of the demo work is thrown away.
- Compose screen auto-generates a form from `ParamSpec`; jobs screen shows per-item progress and cancels the selected job.
- **Demo:** 20 PNGs → JPEGs with visible per-item progress; cancel a hash of an 8 GB file and confirm no output remains.

### M2 — Real image ops (Oct 24–25)
- `image.compress`, `.resize`, `.crop` via the internal engine; PNG/JPEG/GIF encode matrix; naming/collision. (`image.convert` and alias/glob input resolution landed early in M1.)
- Preview screen for images (truecolor half blocks) and text/PDF, reachable with `ctrl+o` from compose and jobs.
- **Demo:** 10 real PNGs → JPEGs from the TUI, preview before and after, output dir correct.

### M3 — ffmpeg + parallel jobs (Oct 26–27)
- `engines/ffmpeg`: binary and ffprobe detection, media metadata, `-progress` parsing, process-group cancellation, and error-to-hint mapping.
- `audio.convert`, `audio.compress`, `audio.speed`, `video.convert`, `video.compress`, `video.extract_audio`, `video.speed`.
- Engine semaphores keep ffmpeg at the configured limit while other jobs wait; the dashboard showed two video jobs running at once.
- **Demo:** convert audio while a video is being compressed; cancel the video at 42% and confirm no output or ffmpeg process remains.

### M4 — Pipelines + palette + aliases (Oct 28)
- Command palette with fuzzy search, `>`/`@`/`#` prefixes, and engine/chain disabled hints.
- Chain composer: add/edit/remove/reorder steps, generated parameter forms, `ctrl+r` run, `ctrl+s` save.
- TOML pipeline library at `$XDG_DATA_HOME/workbench/pipelines`; load/run from `#`; soft delete with `ctrl+d`.
- Aliases: config aliases, `@last`, `@clipboard`, `@downloads`, `@desktop`, `@screenshots`; read-only Aliases screen with `$EDITOR` handoff.
- Static chain compatibility checks. `image.remove_bg` produces a transparent PNG that can feed convert/compress/resize steps.
- **Demo:** a two-step convert→compress chain saved as `convert-image-chain`, reloaded from the palette, and run against a PNG.

### M5 — Docs, errors, onboarding (Oct 29)
- `doc.convert` via pdf2docx, LibreOffice, or pandoc with automatic engine selection, compatibility checks, and install hints; `file.archive` for ZIP, tar.gz, and Zstandard.
- Per-job logs: runner + engine lines in memory, `d` overlay with copy (`c`), and persistent `<job-id>.log` files.
- First-run onboarding with engine scan and starter packs; Engines screen with re-scan.
- **Demo:** a document conversion runs alongside an image job (LibreOffice/pandoc adapters are covered by fake binaries and real integration tests); a deliberately broken file produces a hint and a copyable log.

### M6 — Polish, downloads, and UI (Oct 30–31)
- Job detail actions: `R` retry failed, `r` re-run, `o` reveal output, `c` copy error; size-saved badge and progress sparkline.
- btop-style panel layout: framed Jobs/Engines/Activity columns, Jobs/detail split, Pipeline/Steps split, dense engine and job rows.
- `video.download` via yt-dlp with URL input mode, quality/container/playlist parameters, progress parsing, and suggested-name carry-through across chained steps.
- Goreleaser stub, panel/layout tests, and a real 1 MB URL download verified end-to-end.
- `image.remove_bg` uses rembg when installed, with a transparent PNG output and install/model-download hints.
- **Demo:** a direct MP4 URL downloaded from the palette, then re-run/retried from the jobs screen with a visible saved-size badge and sparkline.

### M7 — YouTube intelligence and agent API (extension)
- `video.transcript`: yt-dlp caption fetch, manual/auto selection, VTT-to-text parsing with rolling-caption deduplication, optional timestamps.
- `video.info`: normalized JSON metadata (title, channel, duration, views, formats, caption languages, playlist entries).
- `wb serve`: local HTTP/JSON API for discovery, job creation, long-poll status, cancel/retry/rerun, saved pipelines, and engine probes; optional bearer token.
- Docs: `docs/agent-api.md`; tests for catalog, job lifecycle, auth, transcript parsing, and metadata normalization.
- **Demo:** an agent lists operations, starts a transcript job and a metadata job from a YouTube URL, long-polls both, and reads the resulting `.txt`/`.json`.

### M8 — TUI shell, editor, and themes (extension)
- TUI structure plan in `docs/tui-architecture.md`; split home/editor/themes/logo into focused files and extract `internal/editor`; themes live in `tui/palettes.go`.
- Centered Home with full ANSI Shadow `WORKBENCH`, the dither mark above the wordmark and in the header, five primary actions (including Settings and Help), recent jobs, and environment details where space allows; full-width header/footer rules replace the old all-in-one dashboard.
- Built-in full-screen editor: line numbers, current line, real terminal cursor, undo/redo, find, atomic save, dirty-close confirmation; config editing no longer requires `$EDITOR`.
- Eight built-in palettes (opencode, tokyonight, catppuccin, gruvbox, nord, monokai, vscode, rosepine), semantic color mapping, live previews, and `general.theme_name` persistence.
- Settings for output folder, max jobs, theme, and config editing; path suggestions in compose; clearer step-editor hints and numeric arrow editing.
- **Demo:** open `config.toml` from Aliases, edit an alias, save; switch to `tokyonight` from Settings → Theme and see the UI recolor immediately.

**Not yet in M8:** component extraction into subpackages, golden frames, syntax highlighting, and a file picker. The plan orders these after the shell settles.

**If the schedule slips, cut in this order:** AI ops (bg removal, transcribe, upscale) → doc.convert breadth (ship PDF→DOCX only) → previews beyond images/text → sparklines/visualization extras → pipeline saving (keep ad-hoc chains). Never cut the runner's atomicity, cancellation, or error hints.

### Go ramp-up, mapped to this build

Budget ~2–3 hours before M0 and 30 minutes/day after. Learn only what the current milestone needs.

| Concept | Where it shows up | Exercise |
|---|---|---|
| structs, slices, maps | `Artifact`, `Spec`, `Values` | model an image artifact with metadata |
| interfaces | `Operation`, `Engine`, `Reporter` | write two implementations of a fake op |
| errors / wrapping | `core.Error`, `%w`, `errors.As` | map an `exec.ExitError` into a hinted error |
| goroutines, channels, `context` | `runner`, cancellation, events | run 3 fake ops concurrently and cancel one |
| `os/exec` | `engines/ffmpeg` | wrap `ffmpeg -i` and parse stderr |
| `testing`, `t.TempDir`, `-race` | every package | golden test for a resize |
| `encoding/json`, TOML tags | config, pipelines | round-trip a pipeline file |

Sources: *A Tour of Go*, *Go by Example*, *Effective Go*, and *Learn Go with Tests* (chapters 1–8). Skip generics, reflection, and `unsafe` entirely for this project.

Rule: prototype each engine interaction as a 50-line `main.go` in `/tmp` before moving it into `internal/`. Debugging shell-outs is much easier without a TUI in the way.

### v1.1 (post-MVP, unordered)
Transcription + models, AI upscale/enhance, DAG pipelines, external ops in config, `wb run` headless, `file.verify`, `audio.enhance`, PDF preview rendering, kitty graphics, Linux builds, Homebrew tap.

---

## 16. Risks

| Risk | Likelihood | Impact | Mitigation |
|---|---|---|---|
| First real Go project + TUI + concurrency at once | High | Schedule | M0/M1 are deliberately small and demoable; lean on `core` contracts, no framework sprawl |
| Engine install friction (rembg, whisper, LibreOffice) | High | Feature gaps | Internal engine covers images/files; every disabled op explains its install; onboarding scan |
| Terminal image previews look bad / vary | Medium | Polish | ANSI fallback first, protocols behind flags; previews never block work |
| Scope creep from "files can do everything" | High | Slip | Inventory table (§6) is the contract; v1.1 list is explicit |
| ffmpeg output parsing fragile across versions | Medium | Progress wrong | Probe version, parse `-progress` key-values defensively, fall back to indeterminate |
| macOS process-group cancellation quirks | Medium | Zombie ffmpeg | Explicit `Setpgid` + kill `-pgid`, integration test that asserts child death |
| Bubble Tea v2 migration churn | Low | Rework | Pin v1 (D2) |
| Exam/interview crunch | Certain | Time | Milestones sized at half-days; cut list pre-agreed |

---

## 17. Progress tracker

Update this list as you build; it is the only place that tracks build state.

- [x] M0 — walking skeleton (Oct 20–21)
- [x] M1 — operation core + runner (Oct 22–23)
- [x] M2 — real image ops (Oct 24–25)
- [x] M3 — ffmpeg + parallel jobs (Oct 26–27)
- [x] M4 — pipelines + palette + aliases (Oct 28)  (alpha coercion deferred)
- [x] M5 — docs, errors, onboarding (Oct 29)
- [x] M6 — polish, downloads, and UI (Oct 30–31)  (remove_bg ships with rembg)
- [x] M7 — YouTube intelligence and agent API (extension)
- [~] M8 — TUI shell, editor, and themes (home/themes/editor done; extraction and golden frames pending)

**Current focus:** M8 polish. **Blocking spike:** none. LibreOffice 26.8.1 and pandoc 3.12.1 are installed locally; all nine engines report ready, and a real PDF→DOCX integration test passes.

---

## Appendix A — Config example

```toml
# $XDG_CONFIG_HOME/workbench/config.toml  (see §7.8)
[general]
theme_name    = "opencode"  # opencode | tokyonight | catppuccin | gruvbox | nord | monokai | vscode | rosepine
notify        = true         # bell + OSC 9 on job completion
max_jobs     = 4             # simultaneous jobs; 0 = unlimited (not recommended)

[output]
dir        = "~/Workbench/{date}"
collision  = "suffix"        # suffix | overwrite | skip | fail
template   = "{stem}.{ext}"

[engines]
enabled = ["internal", "ffmpeg", "ytdlp", "imagemagick", "libreoffice", "pandoc", "pdf2docx", "poppler", "rembg", "whisper"]

[engines.paths]
# ffmpeg = "/opt/homebrew/bin/ffmpeg"

[engines.limits]
ffmpeg     = 2
rembg      = 2
whisper    = 1
libreoffice = 1

[[alias]]
name = "shots"
path = "~/Pictures/Screenshots"
glob = "*.png"

[[alias]]
name  = "pdfs"
path  = "~/Documents/PDFs"
glob  = "*.pdf"
```

## Appendix B — Pipeline file example

```toml
# $XDG_DATA_HOME/workbench/pipelines/shots-clean.toml  (see §7.8)
name        = "shots-clean"
version     = 1
description = "Screenshots → JPEG 85 → background removed"

[[step]]
op = "image.convert"
[step.params]
format  = "jpeg"
quality = 85

[[step]]
op = "image.remove_bg"
[step.params]
model = "isnet-general-use"
# output forced to png because JPEG cannot carry alpha
```

## Appendix C — Operation catalog (v1 targets)

| ID | Inputs | Key params | Terminal output | Engine |
|---|---|---|---|---|
| `image.convert` | image | format, quality | derived ext | internal |
| `image.compress` | image | quality | same ext | internal |
| `image.resize` | image | width, height, mode, filter | same ext | internal |
| `image.crop` | image | width, height, gravity, x, y | same ext | internal |
| `image.remove_bg` | image | model, alpha_matting | png | rembg |
| `audio.convert` | audio | format, bitrate, sample_rate, channels | derived ext | ffmpeg |
| `audio.compress` | audio | bitrate | same container | ffmpeg |
| `audio.speed` | audio | factor, preserve_pitch | same ext | ffmpeg |
| `video.download` | URL | quality, container, playlist | suggested video file | yt-dlp + ffmpeg |
| `video.transcript` | URL | language, auto, timestamps | .txt | yt-dlp |
| `video.info` | URL | playlist | .json | yt-dlp |
| `video.convert` | video | format, codec, crf, preset | derived ext | ffmpeg |
| `video.compress` | video | crf, preset, max_bitrate, resolution | same ext | ffmpeg |
| `video.extract_audio` | video | format, bitrate | derived ext | ffmpeg |
| `video.speed` | video | factor, with_audio | same ext | ffmpeg |
| `doc.convert` | document | format, engine | derived ext | pdf2docx/libreoffice/pandoc |
| `doc.humanize` | document | style, model, voice, chunk_words, workers, rules | same ext (DOCX keeps figures/tables) | opencode |
| `doc.detect_ai` | document | threshold, min_words, json | Markdown + JSON report | internal |
| `file.copy` | any | — | same ext | internal |
| `file.archive` | any | format, level | .zip/.tgz/.zst | internal |
| `file.hash` | any | algo | .sha256/.sha1/.md5 | internal |

## Appendix D — Package tree (canonical)

```
workbench/
├── cmd/workbench/main.go
├── internal/
│   ├── core/        artifact.go spec.go param.go values.go operation.go
│   │                step.go pipeline.go target.go naming.go errors.go events.go
│   ├── runner/      runner.go job.go item.go scheduler.go commit.go progress.go
│   ├── ops/
│   │   ├── image/   convert.go compress.go resize.go crop.go removebg.go
│   │   ├── audio/   convert.go compress.go speed.go transcribe.go
│   │   ├── video/   download.go transcript.go info.go convert.go compress.go extract.go speed.go
│   │   ├── doc/     convert.go extract.go edit.go
│   │   ├── humanize/ humanize.go chunk.go rules.go rules.md polish.go
│   │   ├── file/    copy.go archive.go hash.go verify.go
│   │   ├── media/   media.go
│   │   └── oputil/  oputil.go
│   ├── engines/
│   │   ├── registry.go detect.go models.go
│   │   ├── executil/ executil.go executil_unix.go
│   │   ├── ffmpeg/  ffmpeg.go client.go media.go
│   │   ├── libreoffice/ libreoffice.go
│   │   ├── pandoc/  pandoc.go
│   │   ├── poppler/ poppler.go
│   │   ├── opencode/ opencode.go
│   │   ├── ytdlp/   ytdlp.go
│   │   ├── imagemagick/
│   │   ├── pdf2docx/  rembg/  whisper/  realesrgan/
│   ├── preview/     preview.go image.go video.go audio.go doc.go cache.go
│   ├── alias/       alias.go resolve.go builtin.go
│   ├── pipeline/    store.go
│   ├── config/      config.go paths.go defaults.go
│   ├── api/         server.go handlers.go dto.go
│   ├── editor/      buffer.go
│   ├── history/     history.go
│   ├── version/     version.go
│   ├── app/         app.go registry.go wiring.go doctor.go
│   └── tui/
│       ├── app.go view.go home.go theme.go themes.go logo.go editor.go compose.go screens.go palette.go prompt.go aliases.go onboarding.go logs.go panels.go
│       ├── screens/ dashboard.go compose.go jobs.go jobdetail.go
│       │            pipelines.go settings.go onboarding.go
│       ├── components/ palette.go form.go inputpicker.go progress.go
│       │              preview.go toast.go confirm.go
│       └── styles/  layout.go
├── testdata/
│   ├── fakebin/     ffmpeg, ffprobe, soffice (shell scripts)
│   └── golden/
├── assets/          logo.svg
├── docs/            tui-architecture.md, agent-api.md, engines.md, demo.md
├── Makefile
├── go.mod
├── SPEC.md          ← this file
└── README.md        ← pitch + GIF + install + 5-minute tour
```

Some files shown (transcribe, verify, realesrgan, whisper) are v1.1; the tree is the target shape, not the MVP checklist.

## Appendix E — Golden path, screen by screen

```
1. wb
   Dashboard.  [ctrl+p] palette

2. palette: "png jpg"
   ▸ image.convert   Convert image            ← enter
   ▸ image.compress  Compress image
     audio.convert   Convert audio   (requires ffmpeg)

3. compose
   Inputs: @shots            → chip "10 files · 42.1 MB"
   Steps : image.convert     → format=jpeg, quality=85
   Output: ~/Workbench/2026-10-20/ · suffix collisions
   Estimate: 10 outputs · ~4.2 MB → ~1.1 MB · ~12 s

4. [ctrl+→][ctrl+→] → steps, [n] → palette → image.compress (or remove_bg when rembg is installed)
   Step 2 opens its parameter editor; [enter] commits.

5. [ctrl+r] → job "Convert image chain" starts
   Compose closes; statusline shows 1 running

6. [ctrl+p] → "pdf docx" → doc.convert
   Inputs: @pdfs/contract.pdf → Output: docx → [ctrl+r]
   Statusline: 2 running

7. [esc] → dashboard
   ⠋ image.convert + remove_bg   10 items   8/10
   ⠋ doc.convert                1 item     running

8. jobs finish → toast "10 files · saved 74% · 1m38s"
   [enter] on the doc job → [o] open output, [e] edit via extract→$EDITOR→re-render

9. Reopen the chain or build it again, then [ctrl+s] → save as "shots-clean"
   Available forever at ctrl+p → #shots-clean
```

## Appendix F — Research spikes (do these before/while building M2–M5)

1. **`rembg` on this machine** (OQ1): implemented with the `rembg` CLI; BRIA is the default, with u2net/u2netp/ISNet options. Verified on a real photo (~30s per 1000×667 image on CPU); model downloads on first use.
2. **whisper.cpp** (OQ3): installed and verified with `say`-generated speech using `small.en`; default is `small.en`, with base/tiny/multilingual options. Model files live in `~/.local/share/whisper-models`.
3. **WebP encode** (OQ4): test `gen2brain/webp` (wazero) on a 12 MP photo; if < 300 ms and pure Go, adopt internal WebP and drop the ImageMagick dependency for it.
4. **LibreOffice PDF→DOCX** (OQ6): convert 3 layouts (single column, two column, scanned); timeline output quality. If bad, cap v1 at DOCX/ODT/MD and de-emphasize PDF→DOCX.
5. **ffmpeg progress + cancel** (resolved in M3): `-progress pipe:1` parsing and process-group cancellation are covered by unit and real-ffmpeg tests on macOS.
6. **Bubble Tea v2 status** (D2): if stable and documented, re-evaluate; otherwise pin v1 and move on within an hour.

