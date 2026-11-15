# The compose flow

`New job` is a three-phase wizard instead of a two-pane form:

```
1 Input  ──  2 Output  ──  3 Steps
```

Each phase answers one question. The stepper stays visible, completed phases
get a check, and `Esc` moves back one phase without losing anything typed.

## Why a wizard

The pattern is backed by the usual multi-step-form guidance (Nielsen Norman
Group, USWDS, GitLab Pajamas, Visa's wizard pattern) and by terminal tools that
ship a guided flow (Charm's `huh`, `SuperDirectory`):

- Name every step up front and show `step n of m` so the user knows the shape
  of the task.
- Put one logical group on each screen. Compact file jobs were mixing a file
  picker, an output destination, a chain editor, and step parameters in one
  view.
- Let `Esc` go back a phase, not destroy the draft. The draft is already
  persisted through the existing `state.json` path.
- Make the primary action explicit (`Enter continues`) and validate on the
  phase that owns the problem.
- Make the last phase a review: inputs, step count, destination, and the run
  key are visible together.

The old compose screen failed the first test: `Tab` was reserved for path
completion, so after a completion the cursor could not leave the input field
unless the user went to the step pane and came back. The wizard gives `Tab`
and `Enter` a phase-level meaning once the input is valid.

## Phase 1 — Input

- Type a path, glob, or `@alias`.
- `Tab` accepts the highlighted match. If the input already equals that match,
  the next `Tab` advances to Output.
- `Enter` continues as soon as the input resolves; before that it accepts the
  highlighted match.
- Up/down moves through matches. The current match is a selected row, and the
  resolved file count and total size sit directly under the field.
- `ctrl+f` opens a multi-select file picker. `space` picks a file, `enter`
  opens a folder or picks a file, `ctrl+s` applies the selection, and `esc`
  cancels. The picker filters to formats the operation accepts, keeps selection
  order (important for merge), and writes a comma-separated list back into the
  input field.
- An empty field shows three concrete examples (`~/Pictures/*.png`,
  `@downloads`, `@clipboard`) instead of dead space.

## Phase 2 — Output

- The folder field is the default destination.
- A checkbox, `Save beside input files`, writes each finished file next to its
  own source. `core.Target.BesideInput` resolves this per item, so a selection
  that spans folders still writes each output in the right place.
- The collision policy is spelled out (`keep both`, `overwrite`, `skip`, or
  `fail`) instead of leaving the user to discover it from a failed run.
- `Space` toggles the checkbox; `Tab` moves between the folder and the
  checkbox; `Enter` continues once the phase is valid.

## Phase 3 — Steps

- The chain, the selected step's options, and a run summary share one panel on
  wide terminals and stack on narrow ones.
- `Enter` opens the focused step editor, `n` adds a step, `d` removes one, and
  `J`/`K` reorders. The editor spaces fields out and shows the current step
  number.
- `ctrl+r` runs from any phase. The summary repeats the file count, step count,
  and destination so the run is a deliberate action.

## Batch vs aggregate

The input step names the mode so the result is predictable:

- `per file` — each selected file becomes its own output (5 WAVs → 5 MP3s).
- `combine` — every selected file becomes one output (3 PDFs → 1 merged PDF).
- `in place` — each file is updated directly.

The operation picker shows the same label next to the domain and formats.

## Discard confirmation

`Esc` from the input phase asks before discarding an unfinished pipeline. The
dialog includes a `Don't ask again` checkbox: `space` toggles it, and confirming
with `y` writes `general.confirm_discard = false`. The same setting is available
in Settings (`Confirm discard`); the draft is still saved in `state.json`.

## Footer

The bottom bar shows at most three actions for the current context, generated
from `keymap.go`. The full keymap stays on `?`/`F1`. Compose has separate
contexts for its input and output phases, so the bar always describes the keys
that the focused phase actually accepts.
