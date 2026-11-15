# Install Workbench

Workbench ships as a single `wb` binary. It has no runtime dependencies; the
optional engines (ffmpeg, whisper.cpp, LibreOffice, Real-ESRGAN, and friends)
are detected only when an operation needs them.

## Release binary

Download the archive for your platform from
[GitHub Releases](https://github.com/solomonadzape95/workbench/releases) and put
`wb` somewhere on your `PATH`.

## Install script

```sh
curl -fsSL https://raw.githubusercontent.com/solomonadzape95/workbench/main/install.sh | sh
```

The script detects macOS/Linux and arm64/amd64, verifies `checksums.txt`, and
installs to `~/.local/bin` by default. Override with `PREFIX` or pin a release
with `WORKBENCH_VERSION=v0.1.2`.

## Install missing engines

Workbench works without the optional engines, but each one unlocks more
operations. `wb engines install` opens a picker with every missing engine:

```sh
wb engines install          # choose engines
wb engines install --all    # preselect everything with an automatic installer
```

On macOS the picker runs Homebrew and Python installs for you. Linux package
manager commands that need `sudo` are shown as manual commands instead of being
run unattended. Real-ESRGAN and OpenCode are manual downloads; whisper.cpp needs
a ggml model after the binary is installed. Restart Workbench after installing
so the engine registry sees the new tools.

## Go

```sh
go install github.com/solomonadzape95/workbench/cmd/workbench@latest
```

## Homebrew

```sh
brew install solomonadzape95/tap/workbench
```

The tap lives at
[`solomonadzape95/homebrew-tap`](https://github.com/solomonadzape95/homebrew-tap)
and currently carries `v0.1.2`. Until the GoReleaser publisher is enabled with a
tap token, update `Formula/workbench.rb` in that repository when a new tag is
released.

## Build from source

```sh
git clone https://github.com/solomonadzape95/workbench
cd workbench
make build
./bin/wb
```

## Release process

Pushing a `v*` tag runs `.github/workflows/release.yml`, which builds macOS and
Linux archives for arm64 and amd64, writes `checksums.txt`, and publishes the
GitHub release.
