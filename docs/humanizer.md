# Humanizer

`doc.humanize` rewrites a TXT or Markdown document so it reads like a person
wrote it, while keeping the meaning, facts, names, numbers, quotes, and
technical terms. It uses the models you already have in
[OpenCode](https://opencode.ai); Workbench does not store provider keys.

## Requirements

- OpenCode installed and signed in (`opencode --version`).
- At least one text model available. That can be a free model (NVIDIA/OpenCode
  tiers) or a provider you connected yourself with `/connect` and `/models`.
- OpenCode normally runs a background service. Workbench starts it on the first
  humanize run if it is not already running.

Run `wb doctor` to confirm the `opencode` engine is ready.

## Use it

In the TUI, press `n` and choose **Humanize text**, or use a writing preset:

- **Humanize text** — a TXT or Markdown file.
- **Humanize a DOCX** — edit the body text in place and keep every figure, table, header, and style.
- **Humanize a PDF** — PDF → DOCX, then humanize the body text and keep the converted layout.

Options:

| Option | Meaning |
|---|---|
| Style | `professional` (default), `casual`, `academic`, `blog`, `social`, `narrative` |
| Model | `auto`, or `provider/model` from a model you connected in OpenCode |
| Voice sample | Optional text file with a sample of your writing |
| Chunk size | Words per model call; default 900 |
| Workers | Chunks rewritten in parallel; default 3 |
| Rules file | Optional replacement for the embedded humanizer rules |

The output keeps the input format: `.txt` stays `.txt`, both `.md` and
`.markdown` produce `.md`, and `.docx` produces `.docx` with the same figures,
tables, styles, and page setup.

## Long documents

For Markdown and text, a 100-page document is split at paragraph boundaries.
Each chunk goes to the model independently, in parallel, and the results are
put back in order. The default threshold is 900 words per call. Lower it if a
free model starts truncating or timing out.

For DOCX, Workbench edits `word/document.xml` directly. It humanizes body
paragraphs one at a time and writes the same package back with the original
tables, figures, media, headers, footers, styles, and page setup untouched.
Headings, captions, references, table cells, and paragraphs with mixed inline
formatting are left as they are. Converted reports often leave headings in the
Normal style, so Workbench also skips short, unpunctuated lines: section
headings, names, signature lines, and title-page lines. List markers such as
`i.`, `iv.`, and `1.` are preserved. Bold label paragraphs such as
`Overview: ...` keep their bold label.

Every rewrite is checked before it is written back. Workbench rejects a
paragraph if the model returns meta commentary or Markdown, drops or changes a
number or name, adds a name, place, or number that was not in the input, or
writes something with almost no overlap with the original. The original
paragraph stays in place and the job warning lists it. This matters most with
small free models, which sometimes answer a fragment with a completely
different paragraph.

These parts are replaced with protected tokens before a Markdown/text chunk is
sent and restored exactly afterwards:

- fenced and indented code blocks
- inline code
- tables
- blockquotes and double-quoted text
- YAML/TOML front matter
- display and inline math
- URLs and link targets
- HTML tags
- heading and list markers

If a model drops a protected token, the result is rejected and another model is
tried. If every model fails for one chunk, the original chunk is kept and a
warning is written to the job log. If every chunk fails, the job fails instead
of writing an unchanged file.

## Models and fallback

`auto` starts with the free models that have been reliable for rewriting, then
walks the rest of the available catalog. A model that reports a limit, a region
block, a privacy restriction, or an auth failure is marked unavailable for the
rest of the run. Timeouts and transient failures try the next model.

The first successful model is reused for later chunks. You can pin a model in
the step editor with `provider/model`; an explicit model is tried first even if
it is paid. With `allow_paid = true`, `auto` also includes paid models from
connected providers, after the curated free models.

Free tiers are rate-limited. A large document can take a while, and a model that
worked yesterday may be unavailable today. The fallback chain is there for that.

## Rules and voice

The rules live in [`humanizer-rules.md`](../humanizer-rules.md). The package
embeds a copy at `internal/ops/humanize/rules.md`; a test keeps the two in sync.

To use a different rules file without rebuilding:

```sh
export WORKBENCH_HUMANIZER_RULES=~/my-humanizer-rules.md
```

or set the **Rules file** field on the step. The file needs the core rules and at
least one `STYLE: NAME` section.

A voice sample is optional. It is capped at 6,000 characters and is used only to
match tone; the model is told not to copy its content.

## Agent API

```sh
curl -s -X POST http://127.0.0.1:8765/v1/jobs \
  -H 'Content-Type: application/json' \
  -d '{
    "operation": "doc.humanize",
    "inputs": ["~/Documents/article.md"],
    "params": {"style": "professional", "model": "auto"},
    "target": {"dir": "/tmp/humanized"}
  }'
```

Poll the returned job ID with `GET /v1/jobs/{id}?wait=30`. The item metadata
records the style, model, chunk count, and how many chunks were left unchanged.

## Configuration

Provider keys belong to OpenCode, not Workbench. Connect them in the OpenCode
TUI with `/connect`, then check the model IDs with `opencode models`. Workbench
never receives or stores those keys.

Auto-discovery reads the URL from `opencode service status` and the password
from OpenCode's `service.json`. To override:

```toml
[opencode]
url      = "http://127.0.0.1:4096"
password = "..."
allow_paid = false
allowed_providers = []
```

Environment variables `OPENCODE_SERVER_URL` and `OPENCODE_SERVICE_PASSWORD` also
work. The password is the OpenCode service password, not a provider API key.

### Bring your own key

The `Model` field accepts `provider/model`:

```text
anthropic/claude-sonnet-5-5
openai/gpt-6.1-sol
openrouter/anthropic/claude-sonnet-5.5
```

An explicit model is tried first, even if it is paid; if it fails, the normal
fallback chain runs (free models, plus paid models when `allow_paid = true`).
This means you can use a key you connected in OpenCode without changing
Workbench's config.

`allow_paid` only controls `auto`:

- `false` (default) — `auto` considers free models only; explicit paid models
  still work.
- `true` — `auto` also considers paid models from connected providers, after
  the curated free models.

`allowed_providers` optionally narrows discovery for both free and paid models.
An explicit `provider/model` bypasses it. Leave it empty to allow every provider
OpenCode exposes.

Changing these values requires a restart.

## Detector notes

External detectors tend to flag uniform sentence lengths, very few short
sentences, low function-word ratio, long words, no contractions, em dashes, and
neutral formal sentence starters. The humanizer now checks those signals
directly and runs up to two revision passes when a chunk looks too even. A
deterministic pass also adds contractions, splits common "is about X, and about
Y" constructions, removes formal connectors and em dashes, and swaps inflated
words for plain ones.

No detector is reliable, and no output should be treated as guaranteed to pass
one. Use the notes as a diagnostic, not a target. Workbench now ships an
explainable in-house detector: `doc.detect_ai`. It scores the same rhythm,
vocabulary, repetition, and punctuation signals, shows the evidence, and opens
a result screen with a one-key Humanize action. See
[`docs/ai-detector.md`](./ai-detector.md).

## Troubleshooting

- **Engine missing / no models** — install OpenCode, connect a provider, and run
  `opencode models` to see what is enabled.
- **Service is not running** — run `opencode service start`. Workbench also tries
  this on the first run.
- **Free usage limit reached** — the free model is throttled. `auto` moves to the
  next model. A pinned model is tried first and then falls back to the free
  chain.
- **Model not available in your country** — that model is region-blocked. Use
  `auto` or pick another free model.
- **Privacy / trains on request data** — OpenCode can require a workspace
  privacy setting for some free Go models. Either allow it in the OpenCode
  dashboard or use a model that does not need it.
- **Output is too short** — removing filler often makes the result shorter than
  the input, which is expected. Lower **Chunk size** so the model has less room
  to merge paragraphs. The humanizer asks for a rewrite within 15% of the input
  length and retries when a chunk loses more than about a third of its length.
