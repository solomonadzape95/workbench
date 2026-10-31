# AI detector

`doc.detect_ai` estimates how much of a document reads like machine-written
prose, shows the evidence behind the number, and makes a best-effort guess at
the model family. It is an **explainable stylometric detector**, not a trained
neural classifier and not proof of authorship.

The detector runs offline. It needs no provider key and no OpenCode call. PDF
input needs `pdftotext` from poppler; TXT, Markdown, and DOCX need nothing.
PDF extraction is best-effort: it removes repeated headers, page numbers, and
obvious table rows, but DOCX gives the cleanest prose-only score.

## Use it

In the TUI:

- `a` from Home opens the detector.
- `n` → search `detect` → **Detect AI writing**.
- `ctrl+p` → `> AI detector`.
- The compound preset **Detect AI writing** is in the palette.

After a detector job finishes, the result screen opens automatically:

```
AI                           HUMAN
█████ █████ %                █████ █████ %
...
AI 63% · HUMAN 37% · Likely AI-assisted · medium confidence
```

The screen shows:

- a bold AI score with the human share in one split gauge;
- verdict, confidence, reliability, profile (formal/academic or conversational);
- the strongest weighted signals with raw values;
- the most AI-like paragraphs and prose sentences, with why they were flagged;
- a likely model-family fingerprint with evidence phrases;
- warnings for short text, formal prose, and mixed signals;
- **H** Humanize this text, **O** full in-app report, **C** copy summary.

On wide terminals the first section is a score board: AI and HUMAN percentages
in a five-row solid-block face (the same family as the wordmark), a verdict and
readout column beside them, and a two-row split gauge underneath. Below roughly
96 columns it collapses to a text headline and single-row gauge so the signals
and flagged sentences stay above the fold. Every meter uses the same braille
dot-matrix bar as job progress (`⣿` filled, `⣀` track), and the gauge, signal
bars, and sentence histogram scale with the panel width. The dashboard adds a
sentence-score histogram and a paragraph score list. The full report is
rendered as a structured terminal document with
aligned tables, wrapped prose, source fingerprints, signal details, paragraph
scores, methodology, and references — not the raw Markdown file. The `.md` and
`.json` artifacts in the output folder remain the portable versions for other
tools.

The Humanize button opens a normal compose screen with the source file already
filled in. DOCX/TXT/Markdown go straight to `doc.humanize`; PDF builds a
`doc.convert` → `doc.humanize` chain so the figures and layout survive.

## What is measured

The detector extracts prose only. Headings, tables, figures, references, code,
blockquotes, short labels, and front-matter names are skipped. That mirrors the
prose-only scope used by production detectors such as Turnitin.

### Document signals

| Signal | What it measures | Why it matters |
|---|---|---|
| Sentence rhythm | Coefficient of variation of sentence lengths | Machine prose often clusters around a narrow length range |
| Uniform runs | Sentences in runs of three or more similar lengths | Template-like rhythm |
| Short sentences | Share of sentences with seven words or fewer | Human drafts usually have more short sentences and asides |
| Paragraph variance | Variation in paragraph length | Weak signal of uniform generation |
| AI phrases | Weighted assistant phrases per 1,000 words | Strongest single signal when present |
| Formal connectors | Moreover, furthermore, additionally, however, therefore, in conclusion | Assistant and academic boilerplate |
| Function words | Articles, pronouns, prepositions, conjunctions | Human prose usually carries more |
| Long words | Words with eight or more characters | Weak, style-dependent signal |
| Contractions | Contractions per 1,000 words | Weak; formal human writing can have none |
| Hedging | May, might, often, typically, generally, suggests | Assistant-style cautious prose |
| Pronouns | First- and second-person pronouns | Human prose often uses more |
| Starter diversity | Unique opening words divided by sentences | Uniform openers are machine-like |
| Formal starters | Discourse-marker openers | However, Moreover, Therefore, In conclusion |
| Repeated trigrams | Repeated three-word content sequences | Templates repeat |
| Specificity | Names, places, and numbers | Specific human writing usually carries more |
| Em dashes / semicolons | Punctuation rates | Style-dependent, weak |

Each signal is normalised to an AI-likeness score from 0 to 100, weighted,
and reported with its raw value. The headline is a blend of the document-level
stylometric probability and the share of prose sentences above the sentence
threshold.

### Sentence scoring

Each sentence gets a local score from:

- assistant phrase hits;
- how close its length is to the document average;
- formal openers;
- function-word density;
- long-word density;
- hedging;
- whether its length is close to its neighbours;
- absence of contractions.

The final sentence score mixes local evidence with the document prior. A
sentence is flagged when it crosses the threshold (default `0.55`). The UI
lists the strongest flagged sentences with reasons, so you can see whether the
signal is a real template or just a short academic sentence.

### Formal/academic profile

When a document has almost no contractions or first/second-person pronouns, the
detector switches to a formal profile. Formal prose is not automatically AI, but
it has fewer of the human cues that casual writing provides, so the rhythm and
structure signals carry more weight and the phrase layer carries less. The
report always prints the profile and warns that formal human writing can look
AI-like.

## Verdict and confidence

| AI score | Verdict |
|---:|---|
| 80–100 | Likely AI-generated |
| 60–79 | Likely AI-assisted |
| 40–59 | Mixed or uncertain |
| 20–39 | Mostly human, with AI-like passages |
| 1–19 | Likely human-written |

Confidence is separate from the score:

- **low** under 150 words or reliability below 40;
- **medium** under 300 words or reliability below 70;
- **high** at 300+ words with agreeing signals.

Short text is always a weak signal. OpenAI's own classifier reported that it was
very unreliable below 1,000 characters, and Turnitin requires at least 300 words
of prose. This detector still scores short samples but says so loudly.

## Model attribution

The **Likely source** section is a phrase/style fingerprint:

- **ChatGPT / GPT family** — classic assistant vocabulary such as “delve”,
  “tapestry”, “multifaceted”, “pivotal”, “robust”, “seamless”, “leverage”,
  “moreover”, “furthermore”, “in conclusion”, “it is important to note”.
- **Claude / Anthropic family** — “that said”, “at its core”, “the key insight”,
  “in practice”, “rather than”, “nuanced”, “I'd be happy to”, “let me know”.
- **Gemini / Google family** — “as a large language model”, “great question”,
  “absolutely”, “I'm happy to help”, “I hope this helps”.
- **Llama, Mistral, DeepSeek, Qwen, Grok** — assistant disclaimers and shared
  open-model phrasing such as “as an AI”, “I don't have personal”, “I cannot”.

If no family clears the evidence threshold, the report says **Generic LLM style
/ unknown** rather than inventing a model. Even when one family leads, models
imitate each other and humanizers erase fingerprints. Treat attribution as a
lead, not an identification.

## Output

Each run writes:

- `<stem>.ai-detection.md` — human-readable report with tables, flagged
  sentences, methodology, limitations, and references.
- `<stem>.ai-detection.json` — the same result as structured data. Disable it
  with the advanced `json` parameter.

The Markdown artifact carries summary metadata (`ai_percent`, `human_percent`,
`verdict`, `confidence`, `profile`, `top_model`) so the job list and API can
show the headline without reading the file. The job item's `outputs` list gives
the final Markdown and JSON paths.

## Agent API

```sh
curl -s -X POST http://127.0.0.1:8765/v1/jobs \
  -H 'Authorization: Bearer dev-secret' \
  -H 'Content-Type: application/json' \
  -d '{
    "operation": "doc.detect_ai",
    "inputs": ["~/Documents/report.docx"],
    "params": {"threshold": 0.55, "json": true},
    "target": {"dir": "/tmp/detected"}
  }'
```

The result item has two outputs and summary metadata. The JSON report is the
stable schema for scripts; the Markdown report is for people.

## Parameters

| Parameter | Default | Meaning |
|---|---|---|
| `threshold` | `0.55` | Sentence AI threshold; lower flags more text |
| `min_words` | `5` | Ignore shorter fragments |
| `json` | `true` | Write the machine-readable sidecar |

## Calibration

`internal/detect/testdata/calibration` is the starter labelled corpus. Each
file is one document, paragraphs are separated by blank lines, and
`manifest.json` records the label (`human`, `ai`, or `humanized`), genre, first
language, and — for rewrites — the source file. It is intentionally small and
plain-English; add cases instead of tuning the weights to a single sample.

Run the held-out report:

```sh
make calibrate
# or:
WORKBENCH_WRITE_CALIBRATION=1 go test ./internal/detect -run TestCalibrationReport -v
```

The report prints every document's score, the human false-positive rate at the
AI-likely threshold (60), the unsure rate (30–59), and false positives grouped
by genre and first language. It writes the table to
`internal/detect/testdata/calibration/report.md`.

The starter corpus caught a formal-L2 false positive: a human academic sample
scored 82, mostly from formal connectors and template-like phrasing. The
research-register counter-signal and less-saturated phrase/connector curves
bring it to 33 without losing AI detection (2/2 at 91–96%). The current human
false-positive rate is 0/5; re-run `make calibrate` after any weight change.

The pass/fail guard is deliberately broad: the human mean must stay below the AI
mean, and `humanized_academic.txt` must score lower than its paired source
`ai_academic.txt`. Corpus-level accuracy is reported, not gated, while the set
is this small. Short sample documents report `low` confidence for the same
reason real short documents do; that is expected, not a calibration failure.

## What this detector is not

- It is not a provenance system. Text can be AI-written and read human, or
  human-written and read AI.
- It does not bundle a log-probability model. If you already have a local
  DetectGPT, Fast-DetectGPT, or Binoculars wrapper, set
  `detect.logprob_command` in the config. The command receives the document on
  stdin and prints a 0–1 or 0–100 score (plain text or JSON with `score`/`ai`),
  and its signal is mixed into the report. Without that command, the detector
  uses explainable statistical proxies only.

  ```toml
  [detect]
  logprob_command = "python3 ~/bin/binoculars_score.py"
  logprob_weight = 0.15
  ```
- It is calibrated for English prose. Code, poetry, transcripts, lists, and
  non-English text are out of scope.
- It can be evaded by paraphrasing, translation, and humanizing. RAID showed
  that detectors are easily fooled by changes in sampling, attacks, and unseen
  models.
- It must not be the sole basis for misconduct, grading, or employment action.

## Research notes

The design follows the public methods and caveats in:

- **DetectGPT** — machine text occupies negative-curvature regions of a model's
  log-probability function; zero-shot detection can use probability curvature.
  [arXiv:2301.11305](https://arxiv.org/abs/2301.11305)
- **GLTR** — token-rank statistics and visualization help humans detect
  generated text. [arXiv:1906.04043](https://arxiv.org/abs/1906.04043)
- **Binoculars** — contrasting two related language models separates machine
  and human text without training. [arXiv:2401.12070](https://arxiv.org/abs/2401.12070)
- **Fast-DetectGPT** — conditional probability curvature is a cheaper zero-shot
  detector. [arXiv:2310.05130](https://arxiv.org/abs/2310.05130)
- **RAID** — detectors are brittle under adversarial attacks, sampling changes,
  and unseen models. [arXiv:2405.07940](https://arxiv.org/abs/2405.07940)
- **GPTZero** — the original statistical layer used perplexity and burstiness;
  the current production model is deep-learning based. [GPTZero: perplexity and
  burstiness](https://gptzero.me/news/perplexity-and-burstiness-what-is-it/)
- **OpenAI classifier** — the 2023 classifier was withdrawn after a low accuracy
  rate; OpenAI warned that detectors are unreliable on short text, non-English
  text, and edited AI text. [OpenAI: New AI classifier for indicating AI-written
  text](https://openai.com/index/new-ai-classifier-for-indicating-ai-written-text/)
- **Turnitin** — uses sentence-level scores and reports the percentage of prose
  sentences above a threshold; excludes headers, references, tables, and code;
  requires 300+ words; suppresses scores below 20%. [Turnitin AI writing
  detection FAQ](https://guides.turnitin.com/hc/en-us/articles/28477544839821-Turnitin-s-AI-writing-detection-capabilities-FAQs)

The detector is diagnostic by design: the point is to show which sentences and
signals feel machine-like, then let the writer fix them or run the humanizer.
