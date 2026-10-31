# Persistence: what survives a quit

Workbench keeps one per-user store on every platform. Settings, saved
pipelines, job history, and per-job logs survive a restart. Only per-job temp
files are intentionally ephemeral.

## What is stored

| What | Where | Format |
|---|---|---|
| Settings, theme, aliases, output dir, max jobs | `~/.config/workbench/config.toml` | TOML |
| Saved pipelines | `~/.local/share/workbench/pipelines/*.toml` | TOML |
| Finished job history | `~/.local/share/workbench/history.jsonl` | JSONL, rotated at 5 MB |
| Per-job log | `~/.local/share/workbench/logs/<job-id>.log` | plain text |
| Engine probe cache | `~/.cache/workbench/engines.json` | JSON, 24 h TTL |
| Per-job temp workspace | `~/.cache/workbench/jobs/<job-id>/` | files, deleted at job end |
| Config/data/cache roots | `WORKBENCH_CONFIG_DIR`, `WORKBENCH_DATA_DIR`, `WORKBENCH_CACHE_DIR` | env overrides |

The paths come from `internal/config/paths.go`. The resolver deliberately uses
XDG on macOS as well as Linux, because developer tools are expected under
`~/.config` there. The `WORKBENCH_*_DIR` overrides are what tests use, so test
runs never touch the real store.

## Job history

`internal/history` is an append-only JSONL store. One line is written when a job
finishes, from the runner's `defer` block, after the log is persisted:

- `runner.Record` carries the full `JobView` plus the original pipeline.
- `history.Store.Append` writes one JSON object per line and fsyncs.
- `history.Store.Recent(n)` reads the rotated file plus the current file and
  returns newest-first. Malformed lines are skipped.
- `history.Store.Clear` removes both files.

The TUI loads the newest 200 records at startup and merges them with live jobs.
Historical rows are marked `Historical` in `JobView`, so:

- `l` reads the log from `LogPath` instead of memory.
- `r`/`R` re-run or retry from the stored pipeline and item inputs.
- `x` is disabled (there is nothing to cancel).
- `enter` can still open an AI report, and `o` can still reveal the output dir.

`wb serve` uses the same store. `GET /v1/jobs` merges live and historical jobs and
deduplicates by ID; `GET /v1/jobs/{id}` falls back to history; retry/rerun work
for historical IDs.

CLI:

```sh
wb history list    # newest 50, status, name, id
wb history clear   # remove history.jsonl and its rotated file
```

History contains input and output paths. It is local only, with no cloud sync;
`wb history clear` is the privacy escape hatch.

## Privacy and retention

- History is capped at two 5 MB files. Older records are dropped on rotation.
- Per-job logs are not pruned yet. A retention policy (for example 30 days or
  500 files) is still worth adding; the current machine already has dozens of
  logs.
- Run files and output files are never deleted by history cleanup. Only the
  index is removed.

## Crash safety

`runner.CleanStale` removes `~/.cache/workbench/jobs/*` directories older than
24 hours at startup. Temp workspaces for normally finished jobs are deleted in
the job's `defer`; stale cleanup only matters after a crash or a hard kill.

## Engine cache

`engines.LoadCache`/`SaveCache` keep the last probe under
`~/.cache/workbench/engines.json` for 24 hours. The TUI paints the cached engine
list immediately and still probes in the background, so startup is not blocked
by an unavailable engine.

## Privacy note

History records contain the input path, output paths, job name, and errors.
They do not contain file contents. Treat `history.jsonl` and the job logs as
private: they can reveal filenames and folder structure.

## Session state

`internal/state` writes `state.json` beside the history file at
`~/.local/share/workbench/state.json`. It holds the last input and output, the
selected job, and one unsaved compose draft. It is a replaceable snapshot, not
an event log: saves are atomic (temp file + rename), and a corrupt file is
reported as a boot warning rather than blocking the TUI. The TUI saves on quit,
when a job starts, when a job finishes, and when a compose draft is discarded.
On boot it restores the selected job and reopens a non-empty draft.

## Still worth adding

- Log retention.
- Native Windows paths (`%APPDATA%` / `%LOCALAPPDATA%`) if Windows becomes a
  first-class target. XDG paths work there but look foreign.
- A lock file if two processes (`wb` and `wb serve`) append history at the same
  time. Single-line `O_APPEND` writes are atomic on the platforms Workbench
  targets, but rotation is not coordinated across processes.
