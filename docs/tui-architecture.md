# TUI architecture

Workbench's TUI is a single Bubble Tea program. The goal is not to build a
framework; it is to keep one shell predictable while screens grow. This is the
organization the current code is moving toward.

## 1. Shell

```
┌ header ───────────────────────────────────────────────────────────────┐
│ ▓ square  Workbench › Breadcrumb                        active jobs   │
├ content ──────────────────────────────────────────────────────────────┤
│ screen or overlay                                                    │
├ footer ───────────────────────────────────────────────────────────────┤
│ context keys                                      status / toast     │
└───────────────────────────────────────────────────────────────────────┘
```

The root model owns geometry, global keys, overlays, and the active screen.
Screens do not paint the header/footer and do not know the terminal size until
they are asked to lay out their content.

Layout breakpoints:

| Width | Behavior |
|---|---|
| 30–49 (or height < 16) | compact shell: one-line header/status, compact home, stacked panels |
| 50–79 | compact single column; lists fill the content area; panels stack |
| 80–99 | one column; lists fill the content area; panels stack |
| 100–139 | two columns where it helps: jobs/list + detail, compose pipeline + steps |
| 140+ | wider gutters and more preview rows; same information architecture |

The supported minimum is 30×10; below that the shell shows a two-word resize
notice. At 40×12 and 60×18 the detector and report reflow their bars, score
headline, and section rows instead of clipping. The editor uses the full
content area; its cursor is
the real terminal cursor, not a glyph inside the text.

## 2. Screen contract

The root keeps a `screen` enum and a return screen. A screen is a state struct
with `update`/`view` methods in the `tui` package; it shares `App` through an
explicit method receiver. This is deliberately lighter than an interface per
screen, because the screens share runner/config state.

Detector results are the exception to the usual list-and-detail layout: the
`screenDetector` view is a dashboard. It loads the JSON sidecar, leads with a
score board on wide terminals: AI and HUMAN percentages in a five-row
solid-block face, a verdict/readout column, and a two-row split gauge. Below
~96 columns it collapses to a text headline and single-row gauge. The dashboard
then shows dot-matrix signal bars, a sentence-score histogram, top paragraph
scores, and flagged sentences. Every meter reuses the
job progress braille bar, and the gauge, bars, and histogram scale with the
panel width. The
**Humanize this text** button is pinned to the bottom. `enter` or `o` opens
`screenDetectorReport`, a styled, scrollable terminal report rather than a raw
Markdown preview. The report is divided into labeled VERDICT, LIKELY SOURCE,
SIGNALS, DISTRIBUTION, PARAGRAPHS, FLAGGED SENTENCES, READ THIS, METHODOLOGY,
LIMITATIONS, and REFERENCES sections, and its title carries the visible row
range. Pressing `h` opens a normal compose screen with the detected
source prefilled; PDFs build a convert→humanize chain.

Every screen provides:

- `title` / breadcrumb
- `keymap` for the footer
- `view(width, height)`
- one `esc` behavior (back, cancel, or close overlay)

Overlays are separate from screens: palette, prompt, help, logs, quit
confirmation, and editor save confirmation. The editor's open-file prompt lists
cwd entries immediately, supports `@alias/` browsing, and shows a live
suggestion list with the current start directory. Only one overlay is visible at a
time. Long overlays shrink to the body height and scroll rather than clipping
their frame: the palette shortens its row viewport, and help is a scrollable
single column generated from `keymap.go`.
The routing order is fixed:

```
confirm → prompt → logs → palette → help → screen
```

Global keys are handled before screen keys: `ctrl+c` quit/interrupt, `ctrl+p`
palette, `:` command mode, `ctrl+v` paste, `?`/`F1` contextual help, `q` quit
when no text field is capturing input, `esc` back/close. Printable keys are
never intercepted while a filter, prompt, palette, or text field has focus;
`capturesText()` in `keymap.go` is the gate.

## 3. File organization

Current shape (same package, one concern per file):

```
internal/tui/
  app.go          root model, Init/Update dispatch, job events
  keymap.go       contexts, footer hints, contextual help, text-capture gate
  view.go         shell: header, body, footer, breakpoints
  home.go         home screen
  screens.go      operations, jobs, engines, aliases, preview
  detector.go     AI result dashboard, score gauge, humanize action
  compose.go      pipeline composer + step editor
  editor.go       editor screen and cursor placement
  themes.go       eight-theme picker with previews
  palette.go      fuzzy command palette
  prompt.go       save/open prompts
  logs.go         log overlay
  onboarding.go   first-run setup
  panels.go       bordered panel primitive
  logo.go         wordmark and compact mark
  theme.go        semantic styles
internal/editor/   text buffer, undo, atomic save (no Bubble Tea)
```

Next extraction, when a file exceeds ~600 lines:

```
tui/components/  panel, status bar, list viewport, toast, form fields
tui/screens/     one file per screen, exported state types
```

Do not extract until the boundary is proven. The editor and theme parser are
already independent because they are testable without a terminal.

## 4. Navigation

```
Home
├─ n  New job → Operations → Compose → Jobs
├─ e  Open editor → Editor
├─ t  Themes → Theme picker
├─ j  Jobs → Job detail
├─ s  Settings → Output / Theme / Jobs / Config
├─ ctrl+p  Palette
│           ├─ > Settings
│           ├─ > Engines
│           ├─ > Aliases
│           ├─ > Change theme
│           ├─ > Open editor
│           └─ > Run setup
└─ ?  Help
```

`esc` always moves one level back. `q` quits from any screen that is not
capturing text; it never means “back”. In a filter or text field it types
normally, and `ctrl+c` is the universal quit/interrupt.

## 5. Home screen

Home answers three questions in one screen:

1. What can I do? Primary actions: `n` new job, `a` AI detector, `e` editor, `j` jobs, `s` settings, `t` theme, `?` help.
2. What happened recently? Recent jobs and saved pipelines.
3. Is the environment healthy? Engine marks, output path, and a path to `wb doctor`.

The hero is one centered ANSI Shadow `WORKBENCH` wordmark with a centered action block. It fits an 80-column terminal (77 columns) and is centered on both axes when Home is the active screen. Wide
terminals add a lower Recent | Environment split; narrow terminals stack it.
No max-width cap is used for Jobs, Compose, or Editor; list-only screens are
centered at a readable width instead of stretching metadata across 200 columns.

No progress bars, no sparkline, no duplicate job table. Those belong on Jobs.

## 6. Chrome and identity

- Wide mark: ANSI Shadow `WORKBENCH` in `logo.go`, with a larger two-row, three-block dither mark centered above it on Home and a blank line between them; the header uses the compact single-row mark. `assets/logo.svg` is the matching mark.
- Header: dither row of three accent shades + wordmark + breadcrumb on the left;
  active-job percentage on the right; a single rule below.
- Footer: one rule, context keys on the left, toast on the right; key tokens are accented.
- Progress bars are dot-matrix (`⣿` completed, `⣀` track), not solid blocks;
  the fill color changes with status (accent, green, red, muted).
- Color is semantic, never ad-hoc: `accent`, `success`, `warning`, `danger`,
  `muted`, `border`, `gutter`, `currentLine`.

## 7. Themes

Eight palettes are bundled in `tui/palettes.go`: opencode, tokyonight,
catppuccin, gruvbox, nord, monokai, vscode, rosepine. `theme.go` maps each to semantic styles;
the picker previews the selected palette and writes `general.theme_name` back
to `config.toml` atomically. A missing name falls back to opencode.

## 8. Editor

The editor is a real in-TUI editor for text, Markdown, JSON, TOML, configs, and
small documents. It is not a rich-text or binary editor.

- Full-screen, line-number gutter, current-line highlight, real terminal cursor.
- Insert-oriented: typing inserts; arrows move; `home`/`end`, page keys.
- `ctrl+s` atomic save; `ctrl+z`/`ctrl+y` undo/redo; `ctrl+f` forward search;
  `ctrl+n`/`ctrl+p` (or `F3`/`Shift+F3`) next/previous match.
- `ctrl+←`/`ctrl+→` word movement; `ctrl+backspace` delete a word;
  `ctrl+u` delete to line start.
- `esc`/`ctrl+q` close; dirty buffers require `s` save or `d` discard.
- `ctrl+v`/terminal paste inserts at the cursor.
- Config saves are safe but require restart for engine/alias changes.
- Future: syntax highlighting and a file picker, not a Vim emulation layer.

## 9. Persistence

The TUI reads one per-user store: config under `~/.config/workbench`, saved
pipelines, job history, and logs under `~/.local/share/workbench`, and caches
under `~/.cache/workbench`. Model owns the live job state, while
`internal/history` appends one JSONL record per finished job. The TUI loads the
newest records at startup, marks them `Historical`, and merges them with live
jobs. Historical rows can be logged, previewed, rerun, or retried, but not
canceled. See [`persistence.md`](persistence.md) for paths, rotation, and the
`wb history` commands.

## 10. Testing

- Buffer: edit/undo/redo, joins, find, atomic save, dirty state.
- Themes: parser, list precedence, path traversal, dark detection.
- Screens: direct model tests at 80×24 and 100×32; assert titles, keymap,
  scrolling, and cursor visibility.
- Shell: one golden frame for Home, Compose, Jobs, and Editor at 80×24 and
  100×32 once the layout settles.
- No screen test starts a real engine or touches the user's config.

## 11. Phased work

1. **Shell and identity** — header/footer rules, logo, home, breadcrumb.
2. **Themes** — loader, picker, config persistence.
3. **Editor** — buffer, screen, cursor, save, search, config handoff.
4. **Component extraction** — list viewport and status bar once two screens need
   the same scroll behavior.
5. **Golden frames** — lock the 80×24 and 100×32 layouts.
6. **Polish** — focus rings, empty states, long-name truncation, toast timing.
