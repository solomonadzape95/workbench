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
with `WORKBENCH_VERSION=v0.1.0`.

For a private checkout, pass a token:

```sh
GITHUB_TOKEN="$(gh auth token)" ./install.sh
```

## Go

```sh
go install github.com/solomonadzape95/workbench/cmd/workbench@latest
```

This requires read access to the repository. For a private module:

```sh
export GOPRIVATE=github.com/solomonadzape95/workbench
go install github.com/solomonadzape95/workbench/cmd/workbench@latest
```

## Homebrew

The GoReleaser config generates a `workbench` cask. The tap publication is
currently disabled with `skip_upload: true` because the source repository is
private. Once the repository or release artifacts are public:

1. Create `solomonadzape95/homebrew-tap`.
2. Add a `HOMEBREW_TAP_GITHUB_TOKEN` repository secret with write access to the
   tap.
3. Remove `skip_upload: true` from `.goreleaser.yaml`.
4. Tag a release (`git tag v0.1.0 && git push --tags`).

Then:

```sh
brew install --cask solomonadzape95/tap/workbench
```

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
