#!/bin/sh
# Install the latest Workbench release binary.
#
#   curl -fsSL https://raw.githubusercontent.com/solomonadzape95/workbench/main/install.sh | sh
#
# Set GITHUB_TOKEN for private repositories, WORKBENCH_VERSION to pin a tag, and
# PREFIX to choose the install directory (default: ~/.local/bin).
set -eu

REPO="${WORKBENCH_REPO:-solomonadzape95/workbench}"
VERSION="${WORKBENCH_VERSION:-latest}"
PREFIX="${PREFIX:-$HOME/.local/bin}"

need() {
  command -v "$1" >/dev/null 2>&1 || {
    echo "install.sh: $1 is required" >&2
    exit 1
  }
}

need curl
need tar

os="$(uname -s | tr '[:upper:]' '[:lower:]')"
case "$os" in
  darwin|linux) ;;
  *) echo "install.sh: unsupported OS: $os" >&2; exit 1 ;;
esac

arch="$(uname -m)"
case "$arch" in
  x86_64|amd64) arch="amd64" ;;
  arm64|aarch64) arch="arm64" ;;
  *) echo "install.sh: unsupported architecture: $arch" >&2; exit 1 ;;
esac

api="https://api.github.com/repos/$REPO/releases"
if [ "$VERSION" = "latest" ]; then
  release_url="$api/latest"
else
  case "$VERSION" in
    v*) ;;
    *) VERSION="v$VERSION" ;;
  esac
  release_url="$api/tags/$VERSION"
fi

auth=""
if [ -n "${GITHUB_TOKEN:-}" ]; then
  auth="Authorization: Bearer $GITHUB_TOKEN"
fi

fetch() {
  if [ -n "$auth" ]; then
    curl -fsSL -H "$auth" "$1"
  else
    curl -fsSL "$1"
  fi
}

gh_bin=""
if command -v gh >/dev/null 2>&1; then
  gh_bin="gh"
fi

if [ -n "$gh_bin" ]; then
  tag_arg=""
  if [ "$VERSION" != "latest" ]; then
    tag_arg="$VERSION"
  fi
  if [ -n "${GITHUB_TOKEN:-}" ]; then
    tag="$(GH_TOKEN="$GITHUB_TOKEN" gh release view --repo "$REPO" $tag_arg --json tagName --jq .tagName 2>/dev/null || true)"
  else
    tag="$(gh release view --repo "$REPO" $tag_arg --json tagName --jq .tagName 2>/dev/null || true)"
  fi
fi
if [ -z "${tag:-}" ]; then
  tag="$(fetch "$release_url" | sed -n 's/.*"tag_name":[[:space:]]*"\([^"]*\)".*/\1/p' | head -n 1)"
fi
[ -n "${tag:-}" ] || { echo "install.sh: could not resolve a release tag" >&2; exit 1; }
[ -n "$tag" ] || { echo "install.sh: could not resolve a release tag" >&2; exit 1; }
version="${tag#v}"
asset="workbench_${version}_${os}_${arch}.tar.gz"
base="https://github.com/$REPO/releases/download/$tag"

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT
echo "downloading $asset"
downloaded=0
if [ -n "$gh_bin" ]; then
  if [ -n "${GITHUB_TOKEN:-}" ]; then
    GH_TOKEN="$GITHUB_TOKEN" gh release download "$tag" --repo "$REPO" --pattern "$asset" --pattern checksums.txt --dir "$tmp" >/dev/null 2>&1 && downloaded=1
  else
    gh release download "$tag" --repo "$REPO" --pattern "$asset" --pattern checksums.txt --dir "$tmp" >/dev/null 2>&1 && downloaded=1
  fi
fi
if [ "$downloaded" -eq 0 ]; then
  if [ -n "$auth" ]; then
    curl -fsSL -H "$auth" -o "$tmp/$asset" "$base/$asset"
    curl -fsSL -H "$auth" -o "$tmp/checksums.txt" "$base/checksums.txt" || true
  else
    curl -fsSL -o "$tmp/$asset" "$base/$asset"
    curl -fsSL -o "$tmp/checksums.txt" "$base/checksums.txt" || true
  fi
fi

if [ -s "$tmp/checksums.txt" ]; then
  if command -v shasum >/dev/null 2>&1; then
    (cd "$tmp" && grep " $asset\$" checksums.txt | shasum -a 256 -c -)
  elif command -v sha256sum >/dev/null 2>&1; then
    (cd "$tmp" && grep " $asset\$" checksums.txt | sha256sum -c -)
  fi
fi

mkdir -p "$tmp/unpack"
tar -xzf "$tmp/$asset" -C "$tmp/unpack"
binary="$(find "$tmp/unpack" -type f -name wb | head -n 1)"
[ -n "$binary" ] || { echo "install.sh: wb not found in archive" >&2; exit 1; }
mkdir -p "$PREFIX"
install -m 0755 "$binary" "$PREFIX/wb"
echo "installed $PREFIX/wb"
case ":$PATH:" in
  *":$PREFIX:"*) ;;
  *) echo "add $PREFIX to PATH to run wb" ;;
esac
"$PREFIX/wb" version
