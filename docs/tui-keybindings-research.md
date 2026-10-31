# TUI keybinding conventions — a primary-source survey

**Scope:** stock/default keybindings of 15 widely used terminal UIs, verified against each project's own docs, default config files, man pages, or source code on **2026-10-28**.

**Method:** every binding below was read from an official source, not a blog post. Where a project documents a key in more than one place (site docs + repo config), both are linked. Where a key is *not* documented, the table says **not documented** rather than guessing.

**Notation:** `Ctrl+X`, `Alt+X`, `Shift+X`, `F1`, `Esc`, `Enter`, `Space`, `Tab`, `PgUp`, `PgDn`, `Home`, `End`, `Delete`. `Leader` means an app-specific prefix key (`Ctrl+X` in opencode, `Space` in helix, `Ctrl+B` in tmux).

> Counts in the frequency table are "how many of these 15 apps document this as a default", not how often users press it. Several apps also accept undocumented keys.

**Apps surveyed (15):** lazygit, k9s, btop, htop, yazi, ranger, helix, neovim, gitui, fzf, superfile, opencode, tig, mc, tmux.

---

## 0. TL;DR

1. **`Esc` = back/cancel** in 14/15 documented apps. It is the closest thing to a universal TUI key; mc additionally requires `Esc Esc` by default because single Esc is a Meta prefix.
2. **`q` = quit** in 7/15 (counting "close current view"), but the editor-like apps either don't use it or use it for something else. Never bind `q` to "back".
3. **`/` = search/filter** in 10/15. Use it. `Ctrl+F` is *not* a TUI search convention (it is page-forward in vim/less and forward-char in readline).
4. **`?` = help** in 7/15; **`F1` = help** in 6/15. Bind both; modal editors use `?` for backward search.
5. **`:` = command mode** in 9/15. A **`Ctrl+P` palette** appears in only 1/15 (opencode) — it is an editor/GUI import, not a terminal convention.
6. **`Tab` = focus** in 8/15; **`Space` = select/tag/stage** in 5/15. Do not make `Tab` select or `Space` page down.
7. **`Enter` = confirm/activate** in 11/15; the exceptions are modal editors, where Enter is a text-editing key.
8. **`F5` is a terrible refresh key** in this survey: 1/15 uses it for refresh (tig), while ranger and mc use it for **copy** and htop for **tree**.
9. **`r`, `d`, `x`, `o`, `e` have no consensus.** Each has multiple incompatible meanings. Pick one meaning per app and scope it by focus.
10. The safest baseline is **non-modal + focus-scoped**: arrows *and* `j/k`, `Esc` backs out one layer, `q` quits only from a list, and every destructive action confirms.

---

## 1. App-by-app defaults

### 1.1 lazygit

Source: [Keybindings_en.md](https://github.com/jesseduffield/lazygit/blob/master/docs/keybindings/Keybindings_en.md) (auto-generated from `pkg/i18n`, so it tracks the shipped defaults).

| Operation | Default | Notes |
|---|---|---|
| Quit | `q`, `Ctrl+C` | Global "Quit". |
| Back / cancel | `Esc` | Global "Cancel". |
| Move | `Up`/`Down` | `j`/`k` are **not listed** in the keybinding docs. |
| Page | `,` previous, `.` next | `PgUp`/`PgDn` (and `K`/`J`, `Ctrl+U`/`Ctrl+D`) scroll the main window. |
| Top / bottom | `<` or `Home` / `>` or `End` | List panel navigation. |
| Search / filter | `/` | "Search the current view by text"; files panel says "Filter the current view". |
| Command mode | `:` | Runs a **shell command**, not a command palette. `Ctrl+P` opens custom-patch options. |
| Help | `?` | "Open keybindings menu". |
| Refresh | `R` (global git state), `r` (files panel) | `R` explicitly does **not** run `git fetch`; `f` is fetch. |
| New | `n` | New branch (branches panel). `a` is "stage all" (files) or "amend attribute" (commits). |
| Delete | `d` | Discard changes (files), drop commit (commits), delete branch (branches). Always opens options. |
| Rename | `R` | Rename branch (branches panel). In the files panel `r` is refresh, not rename. |
| Select | `Space` | Stage file / pick hunk / checkout branch, depending on panel. `v` toggles range select. |
| Confirm | `Enter` | Focus diff / open / checkout / view commits. |
| Focus / panes | `[` / `]` previous/next tab; `0` main view; `Enter` focus diff | `Tab` is not the panel key. |
| Run / open | `Enter`; `e` editor; `o` default app | `c` commit, `p` pull, `P` push. |
| Copy | `y`; `Ctrl+O` copies path | `Ctrl+O` is "copy path" here, not "open". |
| Paste | — | `p` is **pull**; no paste binding in the files panel. |

**Notable idioms:** two refresh levels (`R` global, `r` panel); `p`/`P` are pull/push; `:` is a shell, not a palette; `Space` means stage; destructive `d` always opens a menu.

---

### 1.2 k9s

Sources: [official Commands page](https://k9scli.io/topics/commands/) · [README Key Bindings](https://github.com/derailed/k9s#key-bindings).

| Operation | Default | Notes |
|---|---|---|
| Quit | `:q`, `:quit`, `Ctrl+C` | Bare `q` is **not documented** as quit. |
| Back | `Esc` | "Bails out of view/command/filter mode"; with crumbs, goes to previous crumb. |
| Move | Arrow keys | `j`/`k` not documented on these pages. |
| Page | — | Not documented. |
| Search / filter | `/` | Regex2, inverse `!`, labels `-l`, fuzzy `-f`. |
| Command mode | `:` | Resource aliases (`:pod`, `:ctx`, `:ns`, `:pulses`). |
| Help | `?` | "Show active keyboard mnemonics and help". |
| Refresh | — | Not documented as a key. |
| New | — | Not documented (resources are created via manifests/edit). |
| Delete | `Ctrl+D` | Confirmation is **`Tab` then `Enter`**, not `y`. |
| Kill | `Ctrl+K` | No confirmation; equivalent to `kubectl delete --now`. |
| Inspect / view | `d` describe, `v` view, `e` edit, `l` logs | The README lists `d`, `v`, `e`, `l` as the describe/view/edit/logs keys. |
| Select | — | Not documented. |
| Confirm | `Enter` | In the `Ctrl+D` flow, `Tab` selects the confirm button. |
| Focus / panes | — | Not a pane-based UI. |
| Copy / paste | — | Not documented. |
| Palette | — | `:` is a resource command prompt, not a fuzzy palette. |

**Notable idioms:** `d` is *describe*, not delete; deletion is `Ctrl+D` with a `Tab`+`Enter` confirm; `q` alone does nothing documented, so vim users must learn `:q`.

---

### 1.3 btop

Source: the help text rendered by btop itself, defined in [`src/btop_menu.cpp` L174 `help_text`](https://github.com/aristocratos/btop/blob/main/src/btop_menu.cpp#L174) (there is no keybinding section in the man page or README; the README only links a screenshot of this menu).

| Operation | Default | Notes |
|---|---|---|
| Quit | `q`, `Ctrl+C` | "Quits program." |
| Back / cancel | `Esc`, `m` | Toggles/returns to the main menu. |
| Move | `Up`/`Down` | Process list. Vim keys are a config option (`vim_keys`), not the default help text. |
| Page | `PgUp`/`PgDn` | One page in the process list. |
| Top / bottom | `Home`/`End` | First/last page. |
| Search / filter | `f` or `/` | `!` prefix = regex. `Delete` **clears the filter**. |
| Command mode | — | No command palette. |
| Help | `F1`, `?`, `h` | `h` becomes `H` when `vim_keys` is enabled. |
| Options / settings | `F2`, `o` | Not `Ctrl+,`. |
| Refresh | `Ctrl+R` | Reloads the config file from disk; the UI itself auto-refreshes. |
| New | — | Not applicable. |
| Delete | `Delete` | Clears the process filter; it does **not** delete a process. |
| Rename | — | Not applicable. |
| Kill / terminate | `k` kill (SIGKILL), `t` terminate (SIGTERM), `s` signal | `k` is not "up" here. |
| Select | `Space` | Expand/collapse the selected process in tree view. |
| Confirm | `Enter` | Show detailed process info. |
| Focus / panes | Number keys `1`–`5` toggle boxes; `p`/`Shift+P` cycle presets | Boxes, not focus panes. |
| Run / open | — | Not applicable. |
| Copy / paste | — | Not documented. |
| Sort | `r` reverse sort; `Left`/`Right` choose column | `r` is **reverse sort**, not refresh. |

**Notable idioms:** `k` kills a process; `r` reverses sort; `Delete` clears a filter; `Esc` opens the menu instead of closing the app.

---

### 1.4 htop

Source: [official man page `htop.1.in` L145 "INTERACTIVE COMMANDS"](https://github.com/htop-dev/htop/blob/main/htop.1.in#L145) · rendered: [man7.org htop(1)](https://man7.org/linux/man-pages/man1/htop.1.html).

| Operation | Default | Notes |
|---|---|---|
| Quit | `F10`, `q` | |
| Back / cancel | `Esc` | Cancels filter/search and closes dialogs. |
| Move | `Up`/`Down`; `Alt+K`/`Alt+J` | Plain `j`/`k` are **not** the default; vim letters are Alt-modified. |
| Page | `PgUp`/`PgDn` | |
| Top / bottom | `Home`/`End` | Also `Alt+<` / `Alt+>` first/last entry. |
| Search | `F3`, `/` | Incremental command-line search; `Shift+F3` cycles backward. |
| Filter | `F4`, `\` | Case-insensitive fixed strings; a pipe character separates OR terms. |
| Command mode | — | No command palette. |
| Help | `F1`, `h`, `?` | |
| Setup / settings | `F2`, `S` | Meters, columns, colors. |
| Refresh | — | Auto-refresh; no explicit refresh key documented. |
| New / delete / rename | — | Not applicable. |
| Kill | `F9`, `k` | Opens a signal menu. |
| Select / tag | `Space`; `c` tags children; `U` untags all | Multi-select is called "tagging". |
| Confirm | — | Enter is not listed among interactive commands. |
| Focus / panes | `Tab` / `Shift+Tab` | Next/previous screen tab. |
| Sort | `F6`, `<`, `>` choose field; `I` invert | `P` CPU, `M` memory, `T` time, `N` PID. |
| Inspect | `s` strace, `l` lsof, `w` command line, `x` file locks | `x` is **not** delete/kill here. |
| Copy / paste | — | Not applicable. |

**Notable idioms:** function keys carry the primary actions; `Space` tags; `x` inspects file locks; `k` kills; plain `j/k` are not bound.

---

### 1.5 yazi

Sources: [default keymap TOML](https://github.com/sxyazi/yazi/blob/main/yazi-config/preset/keymap-default.toml) · [official keymap docs](https://yazi-rs.github.io/docs/configuration/keymap/).

| Operation | Default | Notes |
|---|---|---|
| Quit | `q`; `Q` quit without writing cwd-file | |
| Close tab | `Ctrl+C` | Closes current tab, quits only if it is the last tab. |
| Back / cancel | `Esc`; `Ctrl+[` | Exit visual mode, clear selection, cancel filter/search. |
| Move | `k`/`j` and `Up`/`Down` | |
| Page | `PgUp`/`PgDn` full page; `Ctrl+B`/`Ctrl+F` full page; `Ctrl+U`/`Ctrl+D` half page | |
| Top / bottom | `gg` / `G`; `Home` / `End` | |
| Search / find | `/` find next, `?` find previous, `n`/`N` | `?` is **not help** here. |
| Filter | `f` | "Filter files". |
| Command mode | `:` shell command that blocks; `;` shell that does not | Neither is a palette. |
| Help | `~`, `F1` | `?` is find-previous. |
| Refresh | — | No refresh binding in the default keymap. |
| New / create | `a` create file (trailing `/` = directory); `A` bulk create | The only surveyed app where `a` means create. |
| Delete | `d` trash; `D` permanently delete | Confirmation layer uses `y`/`n`/`Enter`/`Esc`. |
| Rename | `r` | Cursor placed before the extension. |
| Select | `Space` toggle + move down; `Ctrl+A` select all; `Ctrl+R` invert; `v` visual mode | `Space` both selects and advances. |
| Confirm | `Enter` (dialogs), `y`/`n` | `[confirm]` layer: `y` submit, `n` cancel, `Enter` submit, `Esc` cancel. |
| Focus / panes | `Tab` = file-info spot; `w` task manager; `1`–`9` tabs; `[`/`]` tabs | `Tab` is **not** focus here. |
| Open / run | `Enter` or `o` open; `O` interactive open; `;` shell | |
| Copy / cut / paste | `y` yank, `x` cut, `p` paste, `P` force paste, `Y` unyank | |
| Copy path | `c c` path, `c f` filename | `c` is a prefix. |

**Notable idioms:** `d` trash vs `D` permanent; `x` is cut, not close; `q` quits but `Ctrl+C` only closes a tab; `?` searches backward; help is `~`.

---

### 1.6 ranger

Sources: [default `rc.conf`](https://github.com/ranger/ranger/blob/master/ranger/config/rc.conf) · [ranger(1) man page source](https://github.com/ranger/ranger/blob/master/doc/ranger.1).

| Operation | Default | Notes |
|---|---|---|
| Quit | `q` quit, `Q` quitall, `F10` exit | `q` closes one tab/view; `Q` closes everything. |
| Back / cancel | `Esc` → normal mode; `Ctrl+C` abort | `H` history back, `L` history forward. |
| Move | `Up`/`Down`; `J`/`K` half-page | Plain `j`/`k` are **not** in the default `rc.conf`. |
| Page | `PgUp`/`PgDn`; `J`/`K` | |
| Top / bottom | `Home`/`End` | `gg` is not defined in the default rc.conf. |
| Search | `/`; `n`/`N` | `f` opens `find` (jump-by-name), not search. |
| Command mode | `:` and `;` | Both open the console; `!` opens a shell console. |
| Help | `?`, `F1` | |
| Refresh | `R` reload cwd; `Ctrl+R` reset | |
| New | `F7` mkdir; `Insert` touch | `n` is search-next, not new. |
| Delete | `dD` delete, `F8` delete, `dT` trash | A single `d` is only a **prefix** (cut family). |
| Rename | `a` rename_append, `A` rename, `cw` console rename, `F2` | |
| Select | `Space` mark toggle; `v` mark all/invert; `V` visual mode; `t` tag | Two selection systems: marks and tags. |
| Confirm | prompt when deleting multiple files | `confirm_on_delete` defaults to `multiple`; the man page does not enumerate the prompt's yes/no keys. |
| Focus / panes | `Tab` next tab, `Shift+Tab` previous; `Ctrl+N` new tab, `Ctrl+W` close tab | `Tab` is tabs, not panes. |
| Open / run | `Enter`; `r` open_with; `E`/`F4` edit; `S` shell | `r` is **open-with**, not rename/refresh. |
| Copy / cut / paste | `yy` copy, `dd` cut, `pp` paste, `pO` overwrite | Double-key chords. |
| Copy path | `yp` path, `yd` dir, `yn` name | `y` is a prefix. |

**Notable idioms:** `d` and `y` are prefixes (`dd`, `yy`, `dD`, `yp`); single `d` does nothing visible, which surprises non-vim users; `q` vs `Q` vs `F10`; `a` renames.

---

### 1.7 helix

Sources: [official keymap docs](https://docs.helix-editor.com/keymap.html) · [book source `keymap.md`](https://github.com/helix-editor/helix/blob/master/book/src/keymap.md).

| Operation | Default | Notes |
|---|---|---|
| Quit | — | No quit binding in normal mode; use `:q`/`:quit`. `q` is **replay macro**, `Q` records one. |
| Back / cancel | `Esc` | Returns to normal mode (not "close app"). |
| Move | `h`/`j`/`k`/`l` plus arrows | |
| Page | `Ctrl+B`/`PgUp`, `Ctrl+F`/`PgDn`; `Ctrl+U`/`Ctrl+D` half page | |
| Top / bottom | `gg` / `G`; `Home`/`End` line start/end | |
| Search | `/` forward, `?` **backward**, `n`/`N` | `?` is not help. |
| Command mode | `:` | Ex-command line. |
| Palette | `Space ?` | Command palette. `Space` is the leader. |
| Help | — | No help key documented; help lives in the external docs/tutor, not the keymap. |
| Refresh | — | Not documented. |
| New | `o`/`O` open line below/above; `i`/`a` insert | Editor semantics, not file creation. |
| Delete | `d` delete selection; `Alt+D` delete without yank | |
| Rename | `Space r` rename symbol (LSP) | Not a file rename. |
| Select | `v` select/extend mode; multiple selections are core | `Space` is the leader, not select. |
| Confirm | `Enter` accepts completion; `Esc` closes menus | Not a dialog-driven UI. |
| Focus / panes | `Ctrl+W` window mode; `Space w` window mode | |
| Open / file picker | `Space f` file picker, `Space b` buffer picker, `Space e` explorer | |
| Copy / paste | `y` yank, `p`/`P` paste; `Space y`/`Space p` system clipboard | |
| Save | `:w` | **`Ctrl+S` is "save selection to jumplist", not save file.** |

**Notable idioms:** `q` does not quit; `?` searches backward; `Ctrl+S` does not save; `Space` is a leader; `Ctrl+O`/`Ctrl+I` jump backward/forward.

---

### 1.8 neovim / vim

Sources: [Nvim quickref](https://neovim.io/doc/user/quickref/) ([raw `quickref.txt`](https://github.com/neovim/neovim/blob/master/runtime/doc/quickref.txt)) · [`repeat.txt` for `q`](https://github.com/neovim/neovim/blob/master/runtime/doc/repeat.txt) · [`helphelp.txt` for `F1`](https://github.com/neovim/neovim/blob/master/runtime/doc/helphelp.txt).

| Operation | Default | Notes |
|---|---|---|
| Quit | `:q`, `:qa`, `ZQ` (no write) | `q` is **record macro**; it does not quit. |
| Save + quit | `:wq`, `:x`, `ZZ` | |
| Back / cancel | `Esc` → normal mode; `Ctrl+C` interrupts | `Esc` never closes the app. |
| Move | `h`/`j`/`k`/`l`, arrows, `w`/`b`/`e` words | |
| Page | `Ctrl+F`/`Ctrl+B` full page; `Ctrl+D`/`Ctrl+U` half page | |
| Top / bottom | `gg` / `G` | `Home`/`End` are line start/end. |
| Search | `/` forward, `?` backward, `n`/`N` | `?` is not help. |
| Command mode | `:` | |
| Help | `:help` / `:h`, `F1` | |
| Refresh / redraw | `Ctrl+L` | Redraws the screen; not a data refresh. |
| New | `i`/`a`/`o`/`O` insert modes | |
| Delete | `d` operator (`dd`, `dw`), `x` delete char | |
| Rename | — | No file rename in stock vim; `:saveas` is a different operation. |
| Select | `v` visual, `V` linewise, `Ctrl+V` blockwise | No `Space` select. |
| Confirm | `Enter` newline; `y`/`n` at prompts | Enter is not "confirm" in normal mode. |
| Focus / panes | `Ctrl+W` + direction | Window prefix, not Tab. |
| Open | `:e file`, `gf` | |
| Copy / paste | `yy` yank line, `p`/`P` paste, `x` delete char | |
| Undo / redo | `u` / `Ctrl+R` | |

**Notable idioms:** `:q` is the only documented way to close a help window (`:help help` says "Use `:q` to close the help window"); `q` itself still records a macro in normal buffers.

---

### 1.9 gitui

Sources: [default key list in `src/keys/key_list.rs`](https://github.com/gitui-org/gitui/blob/master/src/keys/key_list.rs) · [KEY_CONFIG.md](https://github.com/gitui-org/gitui/blob/master/KEY_CONFIG.md) · [vim-style config](https://github.com/gitui-org/gitui/blob/master/vim_style_key_config.ron).

| Operation | Default | Notes |
|---|---|---|
| Quit | `q`; `Ctrl+C` exit | |
| Back / cancel | `Esc` | "Exit popup". |
| Move | Arrow keys | `j`/`k` only via the optional vim config; default is explicitly "based on arrow keys". |
| Page | `PgUp`/`PgDn` | |
| Top / bottom | `Home`/`End` | |
| Search | `f` (log find); `Ctrl+J` find commit SHA | `/` not bound. |
| Command bar | `.` | Toggle command bar. No `:` or `Ctrl+P`. |
| Help | `h` | **`h` is help, not left.** `?` is not bound. |
| Options | `o` | Settings/options menu. |
| Refresh | — | Not documented. |
| New | `c` create branch | Context-dependent. `n` not bound. |
| Delete | `D` delete branch / drop stash / reset item | |
| Rename | `r` rename branch; `r` reword commit | Context-dependent. |
| Select / mark | `Space` mark commit | |
| Confirm | `Enter` | |
| Focus / tabs | `Tab` next tab, `Shift+Tab` previous; `1`–`5` direct tabs | |
| Open / edit | `e` edit file; `Enter` open commit | |
| Copy | `y` | |
| Paste | — | Not bound. |

**Notable idioms:** arrow-first by default; `h` is help; `.` opens the command bar; `D` is destructive while `d` resets a hunk; vim keys are an opt-in config file, not the default.

---

### 1.10 fzf

Sources: [fzf(1) man page — "KEY/EVENT BINDINGS" and the action/default table](https://github.com/junegunn/fzf/blob/master/man/man1/fzf.1) · [shell integration `key-bindings.bash`](https://github.com/junegunn/fzf/blob/master/shell/key-bindings.bash).

| Operation | Default | Notes |
|---|---|---|
| Quit / abort | `Esc`, `Ctrl+C`, `Ctrl+G`, `Ctrl+Q` | "Abort". |
| Accept | `Enter`, double-click | |
| Move | `Ctrl+J`/`Down`, `Ctrl+K`/`Up` | Plain `j`/`k` type into the query. |
| Page | `PgUp`/`PgDn` | |
| Top / bottom | `Ctrl+A`/`Home` and `Ctrl+E`/`End` move in the **query line** | Not list top/bottom. |
| Search | type to filter | No `/` binding; the whole UI is a query. |
| Command / palette | — | No command mode. |
| Help | — | No help binding documented. |
| Refresh | — | `Ctrl+L` clears the screen. |
| New / delete / rename | — | Not applicable. |
| Delete char | `Ctrl+D` (or `Delete`); aborts if query is empty | Not item deletion. |
| Select | `Tab` toggle + move down; `Shift+Tab` toggle + move up | `Space` is **not** a selection key; unbound characters go into the query. |
| Confirm | `Enter` | |
| Focus / panes | `Tab` selects, it does **not** move focus | Preview scrolling: `Shift+Up`/`Shift+Down`. |
| Copy / paste | — | Not applicable. |
| Other | `Ctrl+/` toggle wrap-word; `Ctrl+N` next match; `Ctrl+P` previous history | `Ctrl+P` only with `--history`. |
| Shell integration | `Ctrl+T` paste file paths, `Ctrl+R` history, `Alt+C` cd | From the official shell scripts, not the fzf UI. |

**Notable idioms:** `Tab` is multi-select, not focus; `Ctrl+J/K` move; `Ctrl+D` deletes a character; the shell widget keys (`Ctrl+T/R`, `Alt+C`) are the most-copied fzf bindings.

---

### 1.11 superfile

Sources: [default `hotkeys.toml`](https://github.com/yorukot/superfile/blob/main/src/superfile_config/hotkeys.toml) · [official hotkey list](https://superfile.dev/list/hotkey-list/) · [vim hotkey variant](https://github.com/yorukot/superfile/blob/main/src/superfile_config/vimHotkeys.toml).

| Operation | Default | Notes |
|---|---|---|
| Quit | `q`, `Esc`; `Q` quit + cd to cwd | **`Esc` quits the app at top level.** `Q` requires the `cd_on_quit` setting. |
| Back | `h`, `Left`, `Backspace` | Parent directory. |
| Move | `j`/`k`, `Up`/`Down` | |
| Page | `PgUp`/`PgDn` | |
| Top / bottom | — | Not documented. |
| Sort | `o` sort options; `R` reverse sort | `o` is options, not open. |
| Search | `/` | Toggle search bar. |
| Command mode | `:` shell prompt; `>` spf prompt | Two different prompts. |
| Help | `?` | Hotkey list. |
| Refresh | — | No refresh binding in the default keymap. |
| New | `n` new file panel; `Ctrl+N` create file/folder | `n` is not "new file". |
| Delete | `Ctrl+D`, `Delete`; `D` permanent | |
| Rename | `Ctrl+R` | |
| Select | `v` selection mode; `Shift+Up/Down` or `J`/`K`; `A` select all | No `Space` toggle. |
| Confirm | `Enter`, `Right`, `l` | Cancel typing: `Esc`, `Ctrl+C`. |
| Focus / panes | `Tab`/`L` next panel, `Shift+Left`/`H` previous | |
| Open | `Enter` confirm; `e` editor; `E` directory in editor | |
| Copy / cut / paste | `Ctrl+C` copy, `Ctrl+X` cut, `Ctrl+V`/`Ctrl+W` paste | **`Ctrl+C` copies, it does not quit.** |
| Copy path | `Ctrl+P`; `c` copies cwd | `Ctrl+P` is not a palette. |
| Preview | `f` toggle preview panel | `f` is not find here. |

**Notable idioms:** `Esc` quits; `Ctrl+C` copies; `Ctrl+D` deletes; `f` toggles preview; `n` opens a panel rather than creating a file. The project itself warns vim users to switch to `vimHotkeys.toml`.

---

### 1.12 opencode

Sources: [official keybinds doc / default `tui.json`](https://github.com/sst/opencode/blob/dev/packages/web/src/content/docs/keybinds.mdx) · [opencode.ai/docs/keybinds](https://opencode.ai/docs/keybinds/).

| Operation | Default | Notes |
|---|---|---|
| Quit | `Ctrl+C`, `Ctrl+D`, `Leader q` | Leader is `Ctrl+X`. `q` alone is not quit. |
| Back / interrupt | `Esc` | `session_interrupt`; also closes popovers. |
| Move | Arrow keys in dialogs/lists | `j`/`k` not bound globally. |
| Page | `PgUp`/`PgDn`; `Ctrl+Alt+U`/`Ctrl+Alt+D` half page | Messages pane. |
| Top / bottom | `Ctrl+G`/`Home`, `Ctrl+Alt+G`/`End` | Messages. |
| Search | — | No global search binding documented. |
| Palette | `Ctrl+P` | `command_list`. |
| Command mode | — | No `:`; palette only. |
| Help | — | `help_show` defaults to `none`. |
| Refresh | — | Not documented. |
| New | `Leader n` new session; `Leader l` list | |
| Delete | `Ctrl+D` delete session | **Same key as app exit**, context-dependent. |
| Rename | `Ctrl+R` rename session | |
| Select | `Tab`/`Shift+Tab` cycle agents; `Space` toggles the focused checkbox in MCP/plugin dialogs | Not a general multi-select list. |
| Confirm | `Enter` | `input_submit` = `return`. |
| Focus | `Tab`/`Shift+Tab`; left/right cycle child sessions; up goes to parent | |
| Open | `Leader e` editor; `Leader m` models; `Leader a` agents; `Leader t` themes | |
| Copy | `Leader y` copy messages | `Ctrl+C` is exit, **not** copy. |
| Paste | `Ctrl+V` into the prompt | `input_paste`; `preventDefault: false`. |
| Newline | `Shift+Enter`, `Ctrl+Enter`, `Alt+Enter`, `Ctrl+J` | Enter submits. |
| Which-key | `Ctrl+Alt+K` | Leader discoverability. |

**Notable idioms:** leader key avoids terminal conflicts; `Ctrl+P` palette; `Ctrl+C` exits; `Ctrl+D` both exits and deletes a session depending on context; no default help key.

---

### 1.13 tig

Sources: [default `tigrc`](https://github.com/jonas/tig/blob/master/tigrc) · [tigrc(5) man page](https://jonas.github.io/tig/doc/tigrc.5.html).

| Operation | Default | Notes |
|---|---|---|
| Quit | `Q`, `Ctrl+C`; `q` closes current view | `q` = back one view, not quit. |
| Back | `q`, `<` (back) | `<Lt>` = previous view state. |
| Move | `j`/`k`, `Up`/`Down`; `Ctrl+N`/`Ctrl+P` | |
| Page | `PgUp`/`PgDn`; `Space` page down; `-` page up | `Space` is **page down**, not select. |
| Top / bottom | `Home`/`End` | `G` toggles the revision graph, not bottom. |
| Search | `/`, `?` backward, `n`/`N` | `?` is not help. |
| Command mode | `:` | Prompt. |
| Help | `h` | `?` is backward search. |
| Refresh | `R`, `F5` | |
| New | — | Not applicable. |
| Delete | — | Not applicable. |
| Rename | — | Not applicable. |
| Select | — | No multi-select; views are per-entry. |
| Confirm | `Enter` | Open selected entry. |
| Focus / views | `Tab` next view | |
| Open / edit | `Enter`; `e` editor; `o` options | `o` is **options**, not open. |
| Copy | — | Not documented. |
| Paste | — | `p` opens the pager view. |
| Other | `d` diff, `l` log, `t` tree, `b` blame, `s` status, `c` stage, `y` stash view | Single letters switch views. |

**Notable idioms:** `q` = close view, `Q` = quit; `Space` = page down; `G` = graph toggle; `h` = help; `?` = backward search.

---

### 1.14 mc (Midnight Commander)

Source: [mc(1) man page source](https://github.com/MidnightCommander/mc/blob/master/doc/man/mc.1.in) — "Keys", "Directory Panels", "File Menu" sections.

| Operation | Default | Notes |
|---|---|---|
| Quit | `F10`, `Shift+F10` | No `q`. |
| Back / cancel | `Esc Esc` (default), `Esc` ×1 only with the "single press" option | Single Esc is a Meta prefix by default, so dialogs need a double press; `Ctrl+C` aborts a running copy. |
| Move | Arrow keys | No `j`/`k`. |
| Page | `PgDn`/`Ctrl+V`; `PgUp`/`Alt+V` | |
| Top / bottom | `Home`/`End`; `Alt+<` / `Alt+>` | `Alt+G`/`Alt+R`/`Alt+J` go top/middle/bottom. |
| Search | `Ctrl+S`/`Alt+S` quick search | No `/`; type-ahead search. Double-pressing `Ctrl+S` reuses the previous quick-search pattern. |
| Command mode | — | Type a shell command on the bottom command line instead. |
| Help | `F1` | Full hypertext help. |
| Refresh | `Ctrl+L` repaint; `F2` / `Ctrl+R` rescan (tree view) | Man page: `Ctrl+L` "repaint all information"; `Ctrl+R`/`F2` documented as "Rescan" in the directory tree. |
| New | `F7` mkdir | |
| Delete | `F8` | |
| Rename / move | `F6` | |
| Copy | `F5` | **F5 is copy, not refresh.** |
| Select / tag | `Insert`, `Ctrl+T`; `+` select group; `\` unselect group | `Space` is not select. |
| Confirm | `Enter`; dialogs show Yes/No buttons | The man page says file operations "always ask you for confirmation first"; it does not enumerate y/n keys. |
| Focus / panes | `Tab`, `Ctrl+I`, `Left`/`Right` | Switches active panel. |
| Open / view / edit | `Enter`; `F3` view; `F4` edit | |
| Menu | `F9` | Pull-down menu bar. |
| Subshell | `Ctrl+O` | Show/hide the suspended shell. |
| Extended ops | `Ctrl+X` prefix | `Ctrl+X c` chmod, `Ctrl+X o` chown, `Ctrl+X l` link, etc. |

**Notable idioms:** function keys are the primary interface; `F5` copy, `F6` rename/move, `F8` delete; `Insert` tags; `Ctrl+O` toggles the subshell; `Esc` is a Meta prefix, so exiting a dialog takes `Esc Esc` unless the "single press" option is enabled.

---

### 1.15 tmux

Sources: [tmux(1) man page — "DEFAULT KEY BINDINGS"](https://github.com/tmux/tmux/blob/master/tmux.1) · [man.openbsd.org tmux](https://man.openbsd.org/tmux).

All keys below are after the prefix **`Ctrl+B`** unless noted.

| Operation | Default | Notes |
|---|---|---|
| Detach (not quit) | `d` | The server keeps running. `D` chooses a client to detach. |
| Kill pane / window | `x` / `&` | `x` kills the **pane**, `&` the window. Both confirm (`y`/`n`). |
| Command prompt | `:` | |
| Help / list keys | `?` | Lists all bindings. |
| New window | `c` | |
| Split | `%` left/right, `"` top/bottom | |
| Move between panes | Arrow keys; `o` next pane; `;` last pane | |
| Move between windows | `n`/`p`, `0`–`9`, `l` last | |
| Choose window/session | `w`, `s`, `Tab` (fuzzy) | |
| Copy mode / paste | `[` / `]` | `Page Up` also enters copy mode. |
| Rename window / session | `,` / `$` | |
| Pane numbers | `q` | Briefly displays indexes — **not quit**. |
| Redraw | `r` | |
| Zoom pane | `z` | |
| Layouts | `Space` next layout; `Alt+1`–`Alt+7` presets | `Space` is not select. |
| Search windows | `f` | Prompts for text across windows. |

**Notable idioms:** every binding is behind a prefix, so there are almost no bare-key conflicts; `q` shows pane numbers; `x` kills a pane; `d` detaches; quitting is "exit the last shell", not a key.

---

## A. Frequency table

Counts are documented defaults across the 15 apps. "Examples" lists representative apps, not all of them.

| Convention | Count | Verdict | Examples / notes |
|---|---:|---|---|
| `Esc` = cancel / back / leave mode | **14/15** | Near-universal | lazygit, k9s, btop, htop, yazi, ranger, helix, neovim, gitui, fzf, superfile, opencode, mc (`Esc Esc` by default), tmux. Superfile also quits at top level; tig has no documented general Esc binding. |
| Arrow keys navigate | **15/15** | Universal | Every app. Always support arrows even if `hjkl` also works. |
| `Enter` = confirm / activate / submit | **11/15** | Strong (non-modal) | lazygit, k9s, btop, yazi, ranger, gitui, fzf, superfile, opencode, tig, mc. Not htop or tmux (not listed as default bindings), helix (completion), neovim (newline). |
| `/` = search / filter | **10/15** | Strong | lazygit, k9s, btop, htop, yazi, ranger, helix, neovim, superfile, tig. |
| `:` = command mode / console | **9/15** | Strong | lazygit, k9s, yazi, ranger, helix, neovim, superfile, tig, tmux. Meaning varies: shell in lazygit/yazi/superfile, resource prompt in k9s, Ex in editors. |
| `Tab` = focus next | **8/15** | Strong where panes exist | htop, btop, ranger, gitui, superfile, tig, mc, opencode. `Tab` never means select except fzf. |
| `Home`/`End` = list top/bottom | **8/15** | Strong | lazygit, btop, htop, yazi, ranger, gitui, tig, mc. |
| `?` = help | **7/15** | Strong | lazygit, k9s, btop, htop, ranger, superfile, tmux. |
| `q` = quit (app or current view) | **7/15** | Common, not universal | lazygit, btop, htop, yazi, gitui, superfile quit the app; ranger `q` closes the current tab (`Q` quits all). tig uses `q` to close a view, `Q` to quit. k9s needs `:q`. |
| `Ctrl+C` = quit / abort | **7/15** | Common, risky | lazygit, k9s, btop, gitui, tig, fzf (abort), opencode. Also aborts dialogs/operations in mc, ranger. Superfile uses it for **copy**. |
| `y` = copy / yank | **7/15** | Common in list UIs | lazygit, yazi, ranger (`yy`), helix, neovim (`yy`), gitui, opencode (`Leader y`). |
| `F1` = help | **6/15** | Strong fallback | btop, htop, ranger, yazi, neovim, mc. |
| `Ctrl+D` / `Ctrl+U` = half page | **6/15** | Vim export | lazygit, yazi, helix, neovim, tig, opencode. In a text input it must mean delete-char/kill-line instead. |
| `Space` = select / tag / stage | **5/15** | Strong in list UIs | lazygit, htop, yazi, ranger, gitui. mc uses `Insert`; fzf uses `Tab`. |
| `p` = paste | **4/15** | Weak | yazi, ranger (`pp`), helix, neovim. lazygit `p` = pull; tig `p` = pager. |
| `y`/`n` = confirm dialog | **2/15** | Weakly documented | yazi (`[confirm]` layer), tmux (`confirm-before` default confirm key `y`). Most apps use `Enter`/`Esc` or UI buttons instead. |
| `o` = options / settings | **4/15** | Split | btop, gitui, tig, superfile. But `o` = open in lazygit and yazi. |
| `R` = refresh / reload | **3/15** | Weak | lazygit (global), ranger, tig. Note lazygit uses `r` for the files panel and `R` for rename on branches. |
| `gg` / `G` = top / bottom | **3/15** | Vim-only | yazi, helix, neovim. tig's `G` toggles the graph. |
| `n` = new | **2/15** | Weak | lazygit (new branch), superfile (new panel). `Ctrl+N` in superfile; `Leader n` in opencode. |
| `a` = create / add | **1/15** | **Do not use** | yazi only. Everywhere else `a` is stage-all, append, affinity, auto-scale, rename_append, or AI detector. |
| `Ctrl+P` = command palette | **1/15** | **Not a TUI convention** | opencode only. `Ctrl+P` is patch options in lazygit, previous history in fzf, previous window in tmux. |
| `F5` = refresh | **1/15** | **Avoid** | tig only. `F5` is **copy** in ranger and mc, tree view in htop. |
| `Tab` = select / toggle | **1/15** | Do not | fzf only (`Tab` toggles + moves down). Everywhere with panes, `Tab` is focus. |
| `d` = destructive action | **6/15** | Split | yazi trash, ranger `dD`, helix delete, neovim `dd`, superfile `Ctrl+D`, lazygit discard. Non-destructive: k9s describe, tig diff, btop disks, gitui reset hunk. |

---

## B. Strong conventions a new TUI should follow

These are the high-confidence rules. Each is backed by the counts above.

1. **`Esc` backs out exactly one layer** — close dialog → cancel prompt → leave mode → back a screen. At the root, do not quit silently; show "press q to quit" or require confirmation. (14/15 document Esc as a cancel key; only superfile makes it quit. mc needs `Esc Esc` because Esc is also its Meta prefix.)
2. **Keep arrow keys working everywhere.** `hjkl` is an additive vim layer, never a replacement. htop and gitui are arrow-first; even vim itself binds arrows.
3. **`Enter` activates the focused thing** — confirm, submit, open (11/15). In text fields, Enter inserts a newline only if there is an explicit submit alternative; otherwise submit. A pipeline "run" action may deserve its own chord (see D).
4. **`/` opens search/filter.** It is the single most portable text-entry key in the survey (10/15). Do not use `Ctrl+F` as the primary search key: vim/less use it for page-forward and readline uses it for forward-char.
5. **`?` and `F1` both open help.** `?` is the TUI habit (7/15), `F1` is the terminal/function-key fallback (6/15). If you support modal editing, keep `?` as help in the *app* and put backward search in the editor mode only.
6. **`Tab`/`Shift+Tab` cycle focus.** This is the only key that consistently means "next pane/field" (8/15). Never make Tab toggle selection; fzf is the lone exception because it has no panes.
7. **`Space` toggles selection/tagging in list views** (5/15). Pair it with `Shift+Up/Down` for ranges and `Ctrl+A` for select-all (yazi). Do not use Space for page-down; tig is the outlier.
8. **`q` quits, but only from a list/normal context.** Scope it away from text inputs. Accept that vim/helix users expect `q` to record a macro; the tradeoff is worth it for a general-audience TUI, but document it.
9. **`Ctrl+C` interrupts first, quits second.** Terminal users press Ctrl+C to stop the current thing. If nothing is running, quit (with confirmation if there is unsaved state). Never bind Ctrl+C to copy.
10. **Use `:` for command mode and `Ctrl+P` for the palette, but expect `Ctrl+P` to be unknown.** `:` has 9/15 precedent; the palette key does not. Make the palette discoverable in the footer and accept `:` as an alias.
11. **`y` copies, `p` pastes** in list/normal context (7/15 and 4/15). In text inputs, `Ctrl+V` pastes (2/15 but GUI-universal) and `Ctrl+W` deletes a word.
12. **Prefer `Home`/`End` over `gg`/`G` for top/bottom** (8/15 vs 3/15). Add `g`/`G` as a vim alias only if you already support `j/k`.
13. **Refresh is not worth a function key.** No consensus exists. Auto-refresh; if you need manual refresh, put it in the palette (or use `Ctrl+R` only if it is not already run/reload). Never use `F5` if your audience includes ranger/mc users.
14. **Confirm destructively, with `Enter`/`Esc` or explicit `y`/`n`.** `Enter`/`Esc` is the broad TUI pattern (11/15); explicit `y`/`n` is documented in only 2/15 (yazi, tmux). Pick one per dialog and show it in the footer.

---

## C. Common conflicts and where apps diverge

| Key | Meaning A | Meaning B | Meaning C | Lesson for a new TUI |
|---|---|---|---|---|
| `q` | Quit/close: lazygit, btop, htop, yazi, gitui, superfile; close tab: ranger | Record/replay macro: neovim (record), helix (replay) | Close current view: tig; pane indexes: tmux; nothing: k9s (`:q`) | `q` is a quit key for general TUIs and a **trap** for editor users. Never use it for "back"; scope it to non-text contexts. |
| `Esc` | Cancel/back: 14/15 | Quit app: superfile | Normal mode: vim/helix; `Esc Esc`: mc | Back out one layer. At root, confirm instead of quitting. |
| `?` | Help: lazygit, k9s, btop, htop, ranger, superfile, tmux | Backward search: neovim, helix, tig | Find previous: yazi | `?`=help is the TUI convention; modal editors broke it. If you add a vim mode, keep `?`=help outside the editor. |
| `/` | Search/filter: 10 apps | No conflicting meaning in the survey | — | Safe. Pair with `n`/`N`. |
| `:` | Command mode: lazygit, k9s, yazi, ranger, helix, neovim, superfile, tig, tmux | Shell command: lazygit, yazi, superfile | Resource prompt: k9s | Prefer `:` = app command mode. Put shell behind `!` (ranger) or `;` (yazi), not `:`. |
| `r` | Rename: yazi, gitui (branch) | Refresh: lazygit (files panel) | Reverse sort: btop; reword: gitui (commit); rebase: lazygit (branch); replace char: vim/helix; replay macro: helix; open-with: ranger; rerun/rescan: workbench | The most overloaded letter in the survey. Never rely on `r` alone for a destructive/irreversible action; prefer `R` or a named palette command. |
| `R` | Refresh: lazygit (global), ranger, tig | Rename branch: lazygit | Reset: gitui, lazygit (sub-commits); reverse sort: superfile; retry: workbench | Uppercase is not a reliable "stronger" variant. Context-scope it. |
| `d` | Delete: yazi, ranger (`dD`), helix, neovim (`dd`), superfile (`Ctrl+D`) | Discard: lazygit | Describe: k9s; diff: tig; disks: btop; reset hunk: gitui | `d` is destructive *or* informational. In a file/job TUI, `d`=delete with confirm is defensible; never make it "logs". |
| `x` | Cut: yazi, superfile (`Ctrl+X`) | Kill pane: tmux | File locks: htop; decrement: helix (`Ctrl+X`) | No consensus. Pick one meaning: cancel/kill in a job runner, cut in a file manager. Do not overload. |
| `Space` | Select/tag/stage: lazygit, htop, yazi, ranger, gitui | Page down: tig | Expand tree: btop; leader: helix; insert space: fzf; toggle boolean: workbench | Space=select is the strongest list convention. Never bind it to page-down in a list app. |
| `Tab` | Focus next: 8 apps | Select/toggle: fzf | Complete: helix; insert spaces: text editors; new window: tmux (`c`) | Tab=focus. Scope completion to text inputs only. |
| `Ctrl+C` | Quit: lazygit, k9s, btop, gitui, tig, opencode; abort: fzf | Abort operation: mc, ranger; interrupt insert: vim | Copy: superfile | Never copy on Ctrl+C. Interrupt-first, quit-second is the safest state machine. |
| `Ctrl+W` | Delete word: opencode/readline | Window prefix: vim/helix | Close tab: ranger; paste: superfile; whitespace: lazygit | In any text input, Ctrl+W must delete a word. Do not make it a global pane prefix. |
| `Ctrl+R` | Recent repo: lazygit | Reload config: btop | Rename session: opencode; history: fzf; redo: neovim | Five documented meanings. Do not add a sixth unless it is focus-scoped. |
| `Ctrl+P` | Palette: opencode | Patch options: lazygit | Previous history: fzf; previous window: tmux | Palette on Ctrl+P is an editor import, not a TUI standard. Provide `:` as the discoverable path. |
| `o` | Open: lazygit, yazi | Options/settings: btop, gitui, tig, superfile | Open line below: vim/helix | Pick one. If you need both, put settings on `F2` and keep `o` for open/reveal. |
| `e` | Edit/external: lazygit, gitui, tig, superfile, ranger (`E`), mc (`F4`), opencode (`Leader e`) | Word-end motion: vim/helix | — | `e`=edit in list apps, `e`=motion in modal editors. Scope by mode. |
| `h` | Left: vim/helix/yazi (parent) | Help: gitui | History back: ranger | `h`=help is a trap for vim users; avoid it unless you are arrow-first like gitui. |
| `F5` | Refresh: tig | Copy: ranger, mc | Tree view: htop | Do not use F5 for refresh. |
| `Delete` | Delete item: superfile | Clear filter: btop | Delete char: fzf | `Delete` is context-dependent even within one app; label it in the footer. |

**Additional anti-patterns observed**

- **Superfile** makes `Esc` quit the app and `Ctrl+C` copy. Both break terminal muscle memory; the project's own docs recommend a separate vim config.
- **k9s** requires `:q` to quit and uses `d` for describe. Powerful, but hostile to `q`-trained users.
- **tig** uses `q` to close a view and `Q` to quit; `Space` pages down; `G` toggles the graph.
- **Helix** uses `q` to replay a macro, `?` to search backward, and `Ctrl+S` to save the selection to the jumplist — not the file.
- **ranger** makes `d` and `y` prefixes; a single `d` does nothing, which reads as a broken key to non-vim users.
- **btop** uses `k` to kill a process and `Delete` to clear a filter.
- **gitui** uses `h` for help, so vim users press "left" and get a help screen.
- **opencode** binds `Ctrl+D` to both app exit and session delete (context-dependent), and has no default help key.
- **tmux** makes `q` show pane numbers, `x` kill a pane, and `d` detach — all sensible, all different from bare-app conventions.

---

## D. Recommended baseline keymap for a file-processing TUI

This is a synthesis, not a documented default. It assumes a **non-modal** app with: a job/step list, a composer/editor, a log/preview pane, and text fields. It covers the operations in the brief and deliberately resolves the conflicts above.

### D.1 Global keys

| Operation | Primary | Alias | Rationale |
|---|---|---|---|
| Quit | `q` | `Ctrl+C` when idle | `q` is the 7/15 habit; scope it to non-text contexts. If a job is running, `q` asks to cancel/quit. |
| Back / close layer | `Esc` | `Backspace` in tree | 14/15 document Esc as cancel. At root, Esc does not quit; show "q to quit". |
| Help | `?` | `F1` | 7/15 + 6/15; both are cheap. |
| Search / filter | `/` | `n` / `N` next/prev | 10/15. Do not steal `Ctrl+F`. |
| Command palette | `Ctrl+P` | `:` | Palette is the GUI import; `:` is the 9/15 TUI path. Both open the same list; `:` pre-fills command mode. |
| Settings | `F2` | `:settings` | F2 is the documented setup key in htop and btop. Avoid `o`, which means open in lazygit/yazi. |
| Theme | `Ctrl+T` | `:theme` | No consensus. Use only when a list (not a text field) is focused; `Ctrl+T` is transpose-char in readline. |
| Focus next / previous field | `Tab` / `Shift+Tab` | `Ctrl+Right` / `Ctrl+Left` for spatial panes | 8/15; arrows+modifier for left/right panes. |
| Cancel job | `Ctrl+C` | `x` | Interrupt-first. If no job is running, Ctrl+C becomes quit-with-confirm. `x` is a convenience alias, not the primary. |

### D.2 List / navigation and item actions

| Operation | Primary | Alias | Rationale |
|---|---|---|---|
| Move | `Up` / `Down` | `j` / `k` | Arrows universal; `j/k` is an additive vim layer (5/15 document it). |
| Page | `PgUp` / `PgDn` | `Ctrl+U` / `Ctrl+D` half page | 8/15 + 6/15. Do **not** use Space or `Ctrl+F/B`. |
| Top / bottom | `Home` / `End` | `g` / `G` | 8/15 for Home/End; single `g/G` is easy to discover. (Vim users may press `gg`; accept it as an alias.) |
| Select / multi-select | `Space` | `Shift+Up/Down` range; `Ctrl+A` all (list context) | 5/15. Keep `Tab` for focus. |
| Open / drill in | `Enter` | — | 11/15. |
| Preview | `v` | — | k9s uses `v` = view; yazi/superfile use `f`/`Tab` for preview but those collide with filter/focus. `v` is free once you drop visual mode. |
| Reveal output | `o` | — | lazygit/yazi use `o` = open; "open in file manager" is the same mental model. Settings moved to F2 so `o` is unambiguous. |
| Copy error / path | `y` | — | 7/15. Context: if the error pane is focused, copy the error; otherwise copy the path. |
| Logs | `l` | `L` for full-screen logs (optional) | k9s precedent (`l` = logs); avoids the `d`=delete vs `d`=logs collision. |
| Delete step | `d` | `Delete` | Destructive with `Enter`/`Esc` confirm; `d` is the 6/15 destructive habit. Never bind `d` to logs. |
| Reorder step | `Alt+Up` / `Alt+Down` | — | lazygit moves commits with `Alt+Up/Down` (and `Ctrl+J/K`). Alt-arrows do not collide with range-select (`Shift+arrows`) or focus (`Tab`). |
| New | `n` | `Ctrl+N` | lazygit/superfile use `n` for "new <thing>"; `a` is overloaded (1/15 create vs 7/15 other). |
| Retry failed | `R` | — | Uppercase = narrower/stronger action. Only affects failed/cancelled items. |
| Rerun | `r` | — | "Run again"; matches shell `r` intuition and the current workbench map. |
| Paste | `Ctrl+V` in fields; `p` in lists | — | 2/15 Ctrl+V, 4/15 `p`; split by focus. |
| Run / execute | `Ctrl+R` | `Enter` on a "Run" button | Run is the composer's primary action; keeping it on `Ctrl+R` leaves `r` (rerun) and `R` (retry) for the list. |
| Save | `Ctrl+S` | `:w`, auto-save | GUI standard. **Warn:** terminals may intercept Ctrl+S as XOFF; document `stty -ixon` or use auto-save. |

### D.3 Composer / step editor

| Operation | Primary | Alias | Rationale |
|---|---|---|---|
| Focus next / previous field | `Tab` / `Shift+Tab` | `Up` / `Down` (single-column forms) | 8/15; Tab is the only near-universal focus key. |
| Commit field / step | `Enter` | `Ctrl+Enter` for newline in multi-line fields | 11/15; make the newline override explicit. |
| Cancel edit | `Esc` | — | 14/15. If dirty, confirm discard. |
| Save | `Ctrl+S` | `:w` | Same as global. |
| Run from composer | `Ctrl+R` | — | Keeps run out of the `r` (rerun) namespace. |
| Preview | `v` | — | Same pane as the list preview. |
| Delete step | `d` | `Delete` | Confirm; undo via `u` if you have an undo stack. |
| Reorder step | `Alt+Up` / `Alt+Down` | — | Same as list. |

### D.4 Why these choices, in one paragraph

The baseline is **focus-scoped**: list keys, field keys, and global keys are separate namespaces. That is the only way to keep `q` (quit), `d` (delete), `x` (cancel), `y` (copy), and `Space` (select) from colliding with text entry. It keeps the strongest surveyed conventions (`Esc` back, `/` search, `Tab` focus, `Space` select, `Enter` activate), avoids the weakest/most conflicted ones (`F5` refresh, `Ctrl+P` as the only palette path, `a` create, `x` cut, `o` options), and gives every destructive action a confirmation.

### D.5 Optional vim layer (opt-in, not default)

| Vim key | Behavior in the app | Safe? |
|---|---|---|
| `j` / `k` | Move down/up | Yes, additive. |
| `g` / `G` | Top/bottom | Yes. |
| `Ctrl+D` / `Ctrl+U` | Half page | Yes in lists; must stay delete-char/kill-line in fields. |
| `/`, `n`, `N` | Search, next, previous | Yes. |
| `:` | Command mode | Yes; already the alias. |
| `y` / `p` | Copy/paste in list context | Yes; do not add `yy`/`dd` chords. |
| `Space` | Select | Yes; do not also make it page-down. |
| `q` | **Do not** make it record a macro | No — users expect quit. |
| `?` | **Do not** make it backward search | No — users expect help. |
| `Ctrl+W` | **Do not** make it a window prefix | No — it deletes a word in fields. |
| `dd` / `yy` | **Do not** add chords | No — latency and non-vim confusion. |
| `Esc` | **Do not** make it "normal mode" only | No — it must always back out a layer. |

---

This baseline is applied to Workbench's actual screens, with current-vs-proposed
diffs and code references, in
[`tui-keymap-audit.md`](tui-keymap-audit.md).

## E. Terminal-editor (vim) vs general-TUI conventions

### E.1 Where each convention comes from

| Convention | Origin | Apps that follow it | Safe for non-vim users? |
|---|---|---|---|
| `h/j/k/l` movement | vim | neovim, helix, yazi; gitui/superfile via optional vim configs | Yes **if arrows still work**. |
| `gg` / `G` top/bottom | vim | neovim, helix, yazi | Yes as an alias; Home/End is the primary. |
| `Ctrl+D` / `Ctrl+U` half page | vim/less | neovim, helix, yazi, lazygit, tig, opencode | Yes in lists, **no** in text fields. |
| `/` forward, `?` backward, `n`/`N` | vim/less | neovim, helix, tig, yazi | `/` yes; `?`=backward is a vim-ism, not general TUI. |
| `:` command mode | vim → general TUI | 9/15 | Yes, but label it. Many users know `:` from vim and from games/CLIs. |
| `dd` / `yy` double-key chords | vim | neovim, ranger; helix uses single `d`/`y` | **No** — chords are opaque and add latency. |
| `q` record macro | vim/helix | neovim, helix | **Never** copy into a general TUI. |
| `Esc` = normal mode | vim/helix | neovim, helix | **No** — general TUI users expect Esc to close/back. |
| `Ctrl+W` window prefix | vim/helix | neovim, helix | **No** — conflicts with delete-word in every text field. |
| `x` delete char | vim | neovim | No — general TUIs use `x` for cut/kill/cancel. |
| `o` open line below | vim | neovim, helix | No — general TUIs use `o` for open/options. |
| `?` help | general TUI | lazygit, k9s, btop, htop, ranger, superfile, tmux | Yes; the dominant non-modal convention. |
| `q` quit | general TUI | lazygit, btop, htop, yazi, ranger, gitui, superfile | Yes, scoped to non-text contexts. |
| `Space` select | general TUI/list UIs | lazygit, htop, yazi, ranger, gitui | Yes; the strongest list convention. |
| `Tab` focus | general TUI | 8/15 | Yes; universal in pane-based apps. |
| `F1` help | terminal/function-key era | btop, htop, ranger, yazi, neovim, mc | Yes; the safest fallback when `?` is taken. |
| Leader key (`Space`, `Ctrl+X`) | modern TUI/editor | helix (`Space`), opencode (`Ctrl+X`) | Yes, but only if the leader is shown in the footer and `Ctrl+Alt+K`-style discovery exists. |
| `Ctrl+P` palette | GUI/editor import | opencode | Only if you also provide `:`. |
| `F5` refresh | GUI/IDE import | tig (only) | **No** — file managers use F5 for copy. |

### E.2 Where copying vim will surprise non-vim users

1. **They press `q` to quit and start recording a macro** (helix). In a job runner this is harmless but confusing; in a file manager it can be destructive later.
2. **They press `?` for help and get backward search** (neovim, helix, tig, yazi). Always keep `?`=help in the app shell.
3. **They press `Esc` to close a dialog and end up in normal mode** (vim/helix), or press `Esc` again and nothing happens. Make Esc close the top layer first.
4. **They press `d` once and nothing happens** (ranger's `dd`/`dD` prefix). Single-key destructive actions with a confirm are friendlier.
5. **They press `Ctrl+W` to delete a word and the window layout changes** (vim/helix). Never take Ctrl+W away from text fields.
6. **They press `Ctrl+C` to interrupt and the app copies instead** (superfile). Never.
7. **They press `Space` to select and the page jumps** (tig). Pick select for list apps.
8. **They press `o` to open a file and get a settings menu** (btop/gitui/tig/superfile), or vice versa. Settings belong on `F2`.
9. **They press `x` to close/kill and get "cut"** (yazi) or "file locks" (htop). `x` is the most context-dependent letter in the survey.
10. **They press `e` to edit and the cursor jumps to the next word** (vim/helix). Scope editor motions to editor mode only.
11. **They press `l` to move right and get logs**, or `n` to find the next match and create a new item. In a non-modal list, `l`/`n`/`r`/`R` are app commands; keep vim motions inside the editor.
12. **Modal single-letter commands break type-to-filter.** In ranger/fzf/yazi, typing `q` in a filter must type `q`, not quit. The fix is an explicit focus model with a visible mode indicator, not "just add vim keys".
13. **`Ctrl+S` may freeze the terminal** (XOFF) before it ever reaches the app. If you bind Ctrl+S for save, also document `stty -ixon`, or auto-save.

### E.3 The one-sentence rule

> Copy vim's **navigation** (`hjkl`, `gg/G`, `Ctrl+D/U`, `/`), not vim's **modal semantics** (`q` record, `?` backward, `Esc` normal mode, `Ctrl+W` prefix, `dd/yy` chords). A general TUI is not a modal editor; users will type into it, and every command key must yield to text entry when a field is focused.

---

## Verification notes and gaps

- **k9s:** the official pages document `:q`/`Ctrl+C`, `?`, `/`, `:`, `Ctrl+D`, `Ctrl+K`, `d`/`v`/`e`/`l`; they do **not** document `j`/`k`, a refresh key, or a create key. Those are marked not documented rather than inferred.
- **btop:** there is no keybinding list in the man page or README. The authoritative source is the `help_text` array in `src/btop_menu.cpp` that renders the in-app help menu. It is source code, but it is the exact primary source for the displayed bindings.
- **htop:** Enter is not listed in "INTERACTIVE COMMANDS"; it is therefore excluded from the Enter count. Function keys may be intercepted by terminal emulators (the man page itself warns about Shift+Up/Down scrollback capture).
- **yazi:** no refresh key is documented; the default keymap has no `refresh` action. `?` is find-previous and help is `~`/`F1`.
- **ranger:** `j`/`k` are not in the default `rc.conf`; the man page describes arrow keys. `gg` is not defined in the default rc.conf.
- **helix:** there is no help key in the keymap; help lives in the external docs/tutor. `Ctrl+S` is `save_selection`, not file save.
- **neovim:** the quickref documents `q` only under recording macros; `:q`/`ZZ`/`ZQ` are the quit paths. `?` is backward search; help is `:help`/`F1`.
- **gitui:** defaults are arrow-based by design (KEY_CONFIG.md says so); the vim layout is a separate opt-in file. `?` is unbound; `h` is help.
- **fzf:** there is no command mode, help key, or item delete. Shell-integration keys (`Ctrl+T`, `Ctrl+R`, `Alt+C`) come from the official shell scripts, not the fzf UI.
- **superfile:** the docs and `hotkeys.toml` agree; `Esc` quits and `Ctrl+C` copies in the default map. The project ships a separate `vimHotkeys.toml`.
- **opencode:** the docs publish the full default `tui.json`. `help_show` is `none`, so there is no default help key. `Ctrl+D` appears in both `app_exit` and `session_delete`; behavior is context-dependent.
- **tig:** `tigrc` is the default config; `q` is `view-close`, `Q` is `quit`. `Space` is page-down.
- **mc:** the man page covers the main keys but not every keymap entry; the runtime `mc.keymap` is the full source and is not reproduced here.
- **tmux:** all bindings are prefix-relative; the default prefix is `Ctrl+B`, but users routinely change it. `q` shows pane indexes, not quit.
- **Versions drift.** These are default-branch snapshots as of 2026-10-28. Pin a release if you need long-term stability.

---

## Sources

Full URLs for every app and every secondary source used above.

### lazygit
- https://github.com/jesseduffield/lazygit/blob/master/docs/keybindings/Keybindings_en.md

### k9s
- https://k9scli.io/topics/commands/
- https://github.com/derailed/k9s#key-bindings

### btop
- https://github.com/aristocratos/btop/blob/main/src/btop_menu.cpp#L174
- https://github.com/aristocratos/btop#help-menu
- https://github.com/aristocratos/btop/blob/main/manpage.md

### htop
- https://github.com/htop-dev/htop/blob/main/htop.1.in#L145
- https://man7.org/linux/man-pages/man1/htop.1.html

### yazi
- https://github.com/sxyazi/yazi/blob/main/yazi-config/preset/keymap-default.toml
- https://yazi-rs.github.io/docs/configuration/keymap/

### ranger
- https://github.com/ranger/ranger/blob/master/ranger/config/rc.conf
- https://github.com/ranger/ranger/blob/master/doc/ranger.1

### helix
- https://docs.helix-editor.com/keymap.html
- https://github.com/helix-editor/helix/blob/master/book/src/keymap.md

### neovim / vim
- https://neovim.io/doc/user/quickref/
- https://github.com/neovim/neovim/blob/master/runtime/doc/quickref.txt
- https://github.com/neovim/neovim/blob/master/runtime/doc/repeat.txt
- https://github.com/neovim/neovim/blob/master/runtime/doc/helphelp.txt

### gitui
- https://github.com/gitui-org/gitui/blob/master/src/keys/key_list.rs
- https://github.com/gitui-org/gitui/blob/master/KEY_CONFIG.md
- https://github.com/gitui-org/gitui/blob/master/vim_style_key_config.ron

### fzf
- https://github.com/junegunn/fzf/blob/master/man/man1/fzf.1
- https://github.com/junegunn/fzf/blob/master/shell/key-bindings.bash

### superfile
- https://github.com/yorukot/superfile/blob/main/src/superfile_config/hotkeys.toml
- https://github.com/yorukot/superfile/blob/main/src/superfile_config/vimHotkeys.toml
- https://superfile.dev/list/hotkey-list/
- https://superfile.dev/configure/custom-hotkeys/

### opencode
- https://github.com/sst/opencode/blob/dev/packages/web/src/content/docs/keybinds.mdx
- https://opencode.ai/docs/keybinds/

### tig
- https://github.com/jonas/tig/blob/master/tigrc
- https://jonas.github.io/tig/doc/tigrc.5.html

### mc (Midnight Commander)
- https://github.com/MidnightCommander/mc/blob/master/doc/man/mc.1.in

### tmux
- https://github.com/tmux/tmux/blob/master/tmux.1
- https://man.openbsd.org/tmux

### Cross-cutting references
- https://github.com/jesseduffield/lazygit#keybindings (lazygit docs index)
- https://docs.helix-editor.com/ (helix docs index)
- https://neovim.io/doc/user/ (Nvim user manual index)
