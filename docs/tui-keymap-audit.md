# TUI keymap audit

> **Status:** the changes below are implemented. The “Current” tables describe
> the pre-change baseline and are kept for comparison. The implemented map is
> the “Proposed direction” section plus these corrections: search-first screens
> (operation picker, theme picker) use `ctrl+j/k` because plain `j/k` must type;
> the editor uses `ctrl+n/p` (and `F3`/`Shift+F3`) for next/previous match
> because `n` must type in an insert-oriented editor.

This is the actual key handling in `internal/tui`, not the intended map in the
README or `docs/tui-architecture.md`. Source files are named so the table can be
checked.

## Global keys

Handled in `App.handleKey` before the active screen:

| Key | Behavior | Caveat |
|---|---|---|
| `ctrl+c` | Quit; in the editor, close (dirty buffer asks to save) | Never reaches text fields |
| `ctrl+v` | Read the system clipboard into the focused text target | Not the terminal's own paste; `tea.PasteMsg` is separate |
| `ctrl+p` | Toggle the command palette | |
| `?` | Toggle help | Only when `textEntryFocused()` is false |
| `q` | Quit | Only when `textEntryFocused()` is false |
| `esc` | Close overlay, clear the operation filter, or go back one screen | On Compose it discards the chain |

`textEntryFocused()` only knows about Compose. It does not know about the
operation search, the theme filter, or the aliases/engines lists. That is the
root of several conflicts below.

## Per-screen keys

### Home (`home.go`, `app.go`)

| Key | Action |
|---|---|
| `n` | New job → operation picker |
| `a` | AI detector |
| `e` | Open editor (file prompt) |
| `j` | Jobs |
| `s` | Settings |
| `t` | Theme picker |
| `?` | Help |

### Operation picker (`screens.go`, `app.go`)

| Key | Action |
|---|---|
| any text | Filter operations |
| `up` / `down` | Move |
| `enter` | Open operation in Compose |
| `backspace` | Delete filter character |
| `ctrl+u` | Clear filter |
| `esc` | Clear filter, then back Home |

No `j`/`k`; typing `q` quits instead of filtering; typing `?` opens help.

### Compose, chain view (`compose.go`, `app.go`)

| Key | Action |
|---|---|
| `ctrl+right`, `alt+right`, `ctrl+l` | Focus next pane |
| `ctrl+left`, `alt+left`, `ctrl+h` | Focus previous pane |
| `j` / `k` or `up` / `down` | Move step selection (steps pane only) |
| `n` | Add step (steps pane only) |
| `d` | Delete selected step (steps pane only) |
| `J` / `K` | Move step down / up |
| `enter` | Edit selected step |
| `ctrl+r` | Run chain |
| `ctrl+s` | Save chain |
| `ctrl+o` | Preview first input |
| `esc` | Cancel step edit, or discard the chain and return to the picker |

### Compose, step editor

| Key | Action |
|---|---|
| `ctrl+right`, `alt+right`, `ctrl+l` | Next field |
| `ctrl+left`, `alt+left`, `ctrl+h` | Previous field |
| `up` / `down` | Previous / next field |
| `left` / `right` | Decrement / increment numeric or choice field |
| `space` | Toggle boolean / cycle enum |
| `backspace` | Delete last character |
| `ctrl+u` | Clear field |
| `enter` | Commit step |
| `ctrl+s` | Commit and save pipeline |
| `ctrl+r` | Commit and run |
| `ctrl+o` | Preview |
| `esc` | Cancel step edit |

Text fields have no cursor movement: `left`/`right` do nothing for text fields,
and `home`/`end` are not handled.

### Jobs (`screens.go`, `app.go`)

| Key | Action |
|---|---|
| `j` / `k` or `up` / `down` | Move selection |
| `enter` | Open AI report, only if the job produced one |
| `ctrl+o` | Preview an output (or input) |
| `d` | Open log overlay |
| `R` | Retry failed/cancelled items only |
| `r` | Rerun all items |
| `o` | Reveal output folder |
| `c` | Copy job error |
| `x` | Cancel active job |
| `esc` | Back Home |

The footer only advertises `x cancel · esc back`, hiding the primary actions.

### Log overlay (`logs.go`)

| Key | Action |
|---|---|
| `j` / `k` or `up` / `down` | Scroll |
| `pgup` / `pgdown` | Scroll 10 lines |
| `g` / `G` | Top / bottom |
| `c` | Copy log |
| `esc` / `d` | Close |

### Detector (`detector.go`)

| Key | Action |
|---|---|
| `j` / `k` or `up` / `down` | Scroll |
| `space` / `pgdown`, `pgup` | Page |
| `g` / `G` | Top / bottom |
| `h` or `enter` | Humanize this text |
| `o` | Open report |
| `c` | Copy summary |
| `r` | Re-run detection |
| `esc` | Back |

### Detector report (`detector.go`)

Same scrolling keys; `h`/`enter` humanize, `c` copy, `esc` back. No `o`, no `r`.

### Preview (`screens.go`, `app.go`)

| Key | Action |
|---|---|
| `e` | Open in editor |
| `esc` | Back |

### Aliases (`screens.go`)

| Key | Action |
|---|---|
| `e` | Open `config.toml` in the editor |
| `esc` | Back |

### Engines (`screens.go`)

| Key | Action |
|---|---|
| `r` | Rescan engines |
| `esc` | Back |

### Settings (`settings.go`)

| Key | Action |
|---|---|
| `j` / `k` or `up` / `down` | Move |
| `enter` | Edit selected value |
| `t` | Theme picker |
| `e` | Edit `config.toml` |
| `esc` | Back |

### Theme picker (`themes.go`)

| Key | Action |
|---|---|
| `up` / `down` | Move |
| any text | Filter |
| `backspace` | Delete character |
| `ctrl+u` | Clear filter |
| `enter` | Apply and save |
| `esc` | Cancel |

No `j`/`k`.

### Editor (`editor.go`)

| Key | Action |
|---|---|
| Arrows | Move cursor |
| `home` / `ctrl+a`, `end` / `ctrl+e` | Line start / end |
| `pgup` / `pgdown` | Page |
| `ctrl+home`, `ctrl+end` | Document start / end |
| `enter` | New line |
| `tab` | Insert two spaces |
| `backspace` / `delete` | Delete |
| `ctrl+s` | Save |
| `ctrl+z` / `ctrl+y` | Undo / redo |
| `ctrl+f` | Find |
| `enter` in find | Next match |
| `esc` / `ctrl+q` | Close (dirty buffer asks) |
| `s` / `d` in dirty prompt | Save / discard |
| `ctrl+v` | Paste via global handler |

No `n`/`N` next/previous match, no word movement, no selection, no copy/cut.

### Palette (`palette.go`)

| Key | Action |
|---|---|
| `up` / `down` | Move |
| any text | Filter |
| `enter` | Run/select |
| `ctrl+d` | Permanently move the highlighted saved pipeline to `.trash` |
| `backspace`, `ctrl+u` | Delete / clear query |
| `esc` / `ctrl+p` | Close |

### Prompt (`prompt.go`)

| Key | Action |
|---|---|
| `up` / `down` | Suggestion |
| `tab` | Complete |
| `enter` | Submit |
| `backspace`, `ctrl+u` | Delete / clear |
| `esc` | Cancel |

### Onboarding (`onboarding.go`)

| Key | Action |
|---|---|
| `j` / `k` or `up` / `down` | Move |
| `space` / `x` | Toggle pack |
| `enter` / `f` / `ctrl+s` | Finish and write config |
| `esc` | Also finish and write config |
| `?` | Help |
| `q` | Quit |

## Problems found

1. **Typing is swallowed on the operation picker.** `q` quits and `?` opens
   help before the picker sees the text. `textEntryFocused()` only checks
   Compose, so the filter cannot contain `q` or `?`.
2. **`q` quits from nested screens.** The architecture doc says `q` is only an
   unambiguous quit on Home, but Jobs, Detector, Preview, Aliases, and Engines
   all quit instead of going back. `esc` is the real back key. Pick one story:
   the survey favors “`q` quits from any non-text context, `esc` always backs
   out”; if you keep that, update the architecture doc rather than gating `q`
   to Home.
3. **`enter` is not “open” on the detector.** It humanizes. Everywhere else
   `enter` opens/confirms. This is the most likely mis-press in the app.
4. **`d` means three different things:** open logs (Jobs), delete a step
   (Compose), close logs (Log overlay). `l` is the widespread “logs” key (k9s,
   lazygit-style detail views) and is unused.
5. **`ctrl+l` and `ctrl+h` are terminal control characters.** `ctrl+l` is a
   redraw/clear, `ctrl+h` is often Backspace. Using them for focus is fragile
   across terminals and surprising to anyone who uses a shell.
6. **`ctrl+d` deletes a saved pipeline with no confirmation.** It is also a
   page-down/eof key in many TUIs, so a scroll habit can destroy a pipeline.
7. **`d` deletes a compose step with no confirmation or undo.**
8. **`esc` discards an entire unsigned chain** and returns to the picker, with
   no prompt and no draft. A misplaced `esc` loses the work.
9. **The quit dialog treats `enter` as “yes, quit”.** Enter is the most common
   accidental key; destructive confirmation should default to staying.
10. **Help and the footer are hand-written and already stale.** The help card
    omits `ctrl+c`, `g/G`, `ctrl+d/u`, `space`, `J/K`, `h`, `c`, `tab`
    completion, the editor keys, and the palette delete. The Jobs footer hides
    logs, retry, rerun, reveal, and copy.
11. **Navigation is inconsistent.** `j`/`k` work in Jobs, Settings, Detector,
    Logs, Onboarding, and Compose steps, but not in the operation picker or the
    theme picker. `g`/`G` only exist in Logs/Detector. `home`/`end` only exist
    in the editor.
12. **`r` is overloaded and case-sensitive.** `r` reruns on Jobs, rescans on
    Engines, re-detects on Detector; `R` retries failed on Jobs. The difference
    is invisible unless the footer says so.
13. **No search/filter on Jobs, Logs, Aliases, or Engines.**
14. **Editor find has no next/previous.** `enter` finds the next match, but
    there is no `n`/`N` and no wrap indicator.
15. **No word movement or selection in the editor.** `ctrl+left/right`,
    `ctrl+backspace`, and `shift+arrows` do nothing. `ctrl+c` quits instead of
    copying, so there is no copy path at all.
16. **`esc` on first-run setup writes the config.** Cancel/back semantics would
    normally mean “skip”, not “commit”.
17. **`x` cancels a running job immediately.** It is a destructive action with
    no confirmation.
18. **`ctrl+s` is XOFF in terminals that have not disabled flow control.**
    Bubble Tea normally puts the terminal in raw mode, but users running under
    `screen`/`tmux`/odd terminals can still hit it. Worth a note in the README.
19. **`space` means page-down in the detector/report but select/toggle
    elsewhere.** Context-specific is fine, but the footer must say which.
20. **The editor has no `ctrl+w`/`ctrl+u` line editing.** `ctrl+u` is handled in
    prompts and compose but not in the editor.

## Proposed direction

The full convention comparison, per-app tables, and source URLs are in
[`tui-keybindings-research.md`](tui-keybindings-research.md). The guiding rule
from that survey: **copy vim's navigation, not vim's modal semantics.** Arrows
always work, `j/k` is additive, and every command key yields to text entry.

The strongest surveyed conventions worth adopting:

- `Esc` backs out one layer (14/15 apps).
- `Enter` activates the focused thing (11/15).
- `/` opens search/filter (10/15).
- `Tab`/`Shift+Tab` cycles focus (8/15).
- `Space` selects/tags in lists (5/15); it is not page-down.
- `Home`/`End` go to top/bottom (8/15); `g`/`G` is a vim alias.
- `y` copies (7/15); `l` opens logs (k9s precedent); `d` is destructive.
- `q` quits, but only outside text inputs. It is never “back”.
- `?` is help; `F1` is the function-key fallback.
- `Ctrl+P` is a GUI import (1/15); `:` is the TUI-native command path (9/15).
- `F5` for refresh is a trap: ranger and mc use it for **copy**.

### P0 — fix the real conflicts

| # | Change | Current | Proposed | Why |
|---|---|---|---|---|
| 1 | Make global keys text-aware | `q`/`?` are checked before the operation and theme filters, so they cannot be typed | Add a `capturesText()` predicate covering operation search, theme filter, palette, prompt, editor, and compose fields; `q`/`?` only act when it is false | Type-to-filter must never be able to quit or open help |
| 2 | Detector `Enter` | `enter` and `h` both humanize | `enter` opens the report; `h` humanizes | `Enter` = open/activate in 11/15 apps; this is the most likely mis-press |
| 3 | Logs key | `d` opens logs on Jobs, closes them in the overlay, and deletes a step in Compose | `l` opens logs everywhere; `esc` closes the overlay; `d` remains delete | `l` = logs is the k9s convention; `d` = logs is the outlier |
| 4 | Focus keys | `ctrl+l` / `ctrl+h` change focus | `tab` / `shift+tab`; keep `ctrl+arrows` / `alt+arrows` for text fields | `ctrl+l` is redraw and `ctrl+h` is often Backspace; both are terminal control characters |
| 5 | Destructive confirmations | `d` deletes a compose step, `ctrl+d` deletes a pipeline, `x` cancels a job, all immediately | Confirm step/pipeline delete; confirm cancel; or provide undo | The survey’s destructive keys always open a menu/dialog or move to trash |
| 6 | Compose `esc` | Discards the whole chain silently | Confirm discard when the chain has unsaved work, or auto-save a draft | `Esc` should back out one layer, not destroy work |
| 7 | Quit confirmation | `enter`, `q`, or `y` quits | `y` or `q` quits; `esc`/`n` stays; `enter` does not quit | Keep Enter as “activate” and make the safe choice the default |
| 8 | Help/footer drift | Hand-written lists missing half the keys | Generate both from one keymap registry | The audit already found stale help; a registry makes drift a test failure |

### P1 — align with the common conventions

| Screen | Change |
|---|---|
| Operation picker | Use `ctrl+j`/`ctrl+k` (plain `j/k` must type); `q`/`?` type into the filter instead of quitting/opening help; keep type-to-filter; `esc` clears, then backs out |
| Themes | Use `ctrl+j`/`ctrl+k` (plain `j/k` must type); `q`/`?` type into the filter; `esc` clears, then cancels |
| Jobs | Add `g`/`G`, `ctrl+d`/`ctrl+u`, `home`/`end`; add `/` name/status filter; `l` logs; `enter` opens the report; `r` rerun; `R` retry failed; `o` reveal; `c`/`y` copy; `x` cancel with confirm |
| Log overlay | Add `/` search with `n`/`N`, `ctrl+d`/`ctrl+u`, `home`/`end`; `c`/`y` copy; `esc` close; remove `d` as close |
| Detector | `enter` report, `h` humanize, `c`/`y` copy, `r` re-detect, `esc` back; replace `space` page-down with `pgdn`/`ctrl+d`; keep `g`/`G` |
| Detector report | `h` humanize, `c`/`y` copy, `esc` back; `pgdn`/`ctrl+d`; `g`/`G` |
| Compose | `tab`/`shift+tab` focus outside text fields; `alt+up`/`alt+down` reorder (keep `J`/`K` as aliases); `d` delete step with confirm; `ctrl+r` run; `ctrl+s` save; `ctrl+o`/`v` preview; `n` add; `enter` edit |
| Step editor | `tab`/`shift+tab` field; `left`/`right` moves the cursor in text fields and adjusts numeric/enum fields; `home`/`end`; `ctrl+left/right` word; `ctrl+backspace` delete word; `ctrl+u` clear; `enter` commit; `esc` cancel |
| Editor | Use `ctrl+n`/`ctrl+p` (or `F3`/`Shift+F3`) next/previous match; `ctrl+left/right` word movement; `ctrl+backspace` delete word; `ctrl+u` delete to line start; keep `ctrl+f` as find |
| Palette | `ctrl+j`/`ctrl+k` and `ctrl+n`/`ctrl+p` move; `ctrl+d` deletes with confirm; `:` opens the palette in command mode (alias for `>`); keep `ctrl+p` |
| Prompt | Add `ctrl+w` delete word; `ctrl+n`/`ctrl+p` move suggestions; `tab` completes |
| Home | Keep `a` for AI detector, but only on Home (the survey says `a` = create is a 1/15 outlier; never use `a` for add in lists — use `n`) |

### Keep deliberately

- **`ctrl+r` = run** in Compose. The survey found no consensus for “run”, and
  keeping it leaves `r` (rerun) and `R` (retry) free in Jobs.
- **`ctrl+s` = save.** It is the GUI-standard save, but document the XOFF
  caveat (`stty -ixon`) and prefer auto-save/draft for compose.
- **`ctrl+p` = palette.** It is a GUI/editor import, not a TUI standard; keep it
  for muscle memory, but add `:` as the discoverable path and show it in the
  footer.
- **`c` = copy** in Workbench. `y` is the stronger survey convention, so add
  `y` as an alias; keep `c` because it is clearer to non-vim users.
- **`x` = cancel job.** `x` has no consensus; it is fine as a scoped job action,
  but confirm it and show it in the footer.

### Final map by context

| Context | Keys |
|---|---|
| Global | `esc` back/close · `q` quit (non-text contexts) · `ctrl+c` interrupt/quit · `?`/`F1` contextual help · `ctrl+p`/`:` palette · `/` search |
| Lists | `j`/`k` or arrows · `g`/`G` · `ctrl+d`/`ctrl+u` half-page · `home`/`end` · `enter` open · `space` select · `esc` back |
| Search-first lists | type to filter · `ctrl+j`/`ctrl+k` move · `enter` open · `esc` clear/back |
| Home | `n` new · `a` detector · `e` editor · `j` jobs · `s` settings · `t` theme · `?` help |
| Compose | `tab`/`shift+tab` focus · `ctrl+r` run · `ctrl+s` save · `ctrl+o`/`v` preview · `n` add step · `d` delete step (confirm) · `alt+up/down` reorder · `enter` edit |
| Step editor | `tab`/`shift+tab` field · `left`/`right` adjust or move cursor · `space` toggle · `enter` commit · `esc` cancel |
| Jobs | `j`/`k` · `enter` open report · `l` logs · `r` rerun · `R` retry failed · `o` reveal · `c`/`y` copy · `x` cancel (confirm) · `esc` back |
| Logs | `j`/`k` · `g`/`G` · `ctrl+d`/`ctrl+u` · `/` search · `n`/`N` · `c`/`y` copy · `esc` close |
| Detector | `j`/`k` · `g`/`G` · `pgdn`/`ctrl+d` · `enter` report · `h` humanize · `c`/`y` copy · `r` re-detect · `esc` back |
| Editor | `ctrl+s` save · `ctrl+z`/`ctrl+y` undo/redo · `ctrl+f` find · `ctrl+n/p` next/prev · `ctrl+left/right` word · `ctrl+backspace` delete word · `esc` close |
| Palette | `up`/`down` or `ctrl+j/k` · `enter` run · `ctrl+d` delete (confirm) · `:` command mode · `esc` close |

### On `ctrl+c`

The survey’s strongest recommendation is “interrupt first, quit second”
(`Ctrl+C` cancels a running operation in mc/ranger; it quits in lazygit, k9s,
btop, gitui, tig, opencode). Workbench currently treats `ctrl+c` as quit with a
confirmation when jobs are active. A more terminal-native behavior is:

1. If a job is selected and active, `ctrl+c` cancels it.
2. If nothing is active, `ctrl+c` quits (with a draft/unsaved-work prompt).

Keep `x` as the explicit cancel alias. This is a behavior change, so put it in
the same pass as the other global-key fixes.

## Implementation notes

The advertised map lives in `internal/tui/keymap.go`:

- `hints` and `sectionTitles` drive `footerHints()`, `footerText()`, and
  `helpView()`.
- `activeContext()` chooses the context; `capturesText()` gates single-letter
  global commands so filters and text fields keep their keystrokes.
- Handlers still live with their screens; a test asserts no duplicate key
  within a context and that every context has hints.
- `[keys]` in `config.toml` can override these tables later; the duplicate-key
  test is the validation hook.

Tests added or updated:

- `internal/tui/keymap_test.go` checks the registry shape.
- `internal/tui/app_test.go` covers text-aware `q`/`?`, job filtering, step
  deletion confirmation, compose discard confirmation, and history loading.
- `internal/tui/detector_test.go` covers `enter` opening the report and `h`
  humanizing.
- `internal/editor/buffer_test.go` covers word movement/deletion and backward
  search.
- `internal/history` and `internal/engines/cache_test.go` cover persistence and
  cache round-trips.
