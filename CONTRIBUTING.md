# Contributing

Thanks for helping. Workbench is a Go project; the TUI, CLI, HTTP API, and MCP
server all use the same operation registry and runner, so a new operation shows
up everywhere at once.

## Setup

- Go 1.26 or newer.
- `make build` builds `./bin/wb`.
- `make check` runs gofmt, `go vet`, and `go test -race`.
- `make lint` runs `golangci-lint` when it is installed.
- `make doctor` prints the engine report for your machine.

The optional engines (ffmpeg, yt-dlp, whisper.cpp, LibreOffice, pandoc,
Tesseract, Ghostscript, pdf2docx, poppler, rembg, Real-ESRGAN, and OpenCode) are
detected at runtime. Tests that need an engine should skip cleanly when it is
missing; unit tests should not require one.

## Before opening a pull request

- Run `make check` (and `make lint` if available).
- Add or update tests for the behavior you changed.
- Update the README or a file under `docs/` for user-visible changes.
- Keep commits focused. Conventional prefixes are used in the history:
  `feat:`, `fix:`, `docs:`, `test:`, `refactor:`.
- Do not commit generated binaries, model files, or personal sample media.

## Adding an operation

1. Add a `core.Spec` and implementation under `internal/ops/<domain>`.
2. Give every parameter a type, default, help text, and sensible min/max.
3. Put cross-parameter rules in `Validate` with an actionable hint.
4. In `Run`, read only `req.Items`, write only inside `req.Target.Dir`, report
   stage/progress, and honor `ctx`.
5. Test the unit behavior; add an engine-argument test if the operation shells
   out. Test the missing-engine hint path too.
6. Register the operation in `internal/app/app.go`. Add a preset in
   `internal/tui/presets.go` if it composes well with other steps.
7. Verify it appears in the palette, that a pipeline containing it saves and
   loads, and that `wb ops` lists it.
8. Add it to the README's "What works" list and to any relevant docs.

## Reporting bugs

Use the bug report template. Include:

- `wb version`
- your OS and architecture
- the operation or pipeline you ran
- the job log from `~/.local/share/workbench/logs/`
- the relevant `wb doctor` lines

Remove personal paths, tokens, and file contents before posting.

## Security

Do not paste secrets, private file contents, or full home-directory paths into a
public issue. For a vulnerability, use GitHub's private security advisory flow
instead of a public issue.

## License

By contributing, you agree that your changes are licensed under the MIT License.
