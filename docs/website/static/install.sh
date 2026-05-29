#!/usr/bin/env bash
# Install the hm binary from a GitHub release.
#
# Usage:
#   curl -fsSL https://homie.sh/install.sh | bash
#
# Environment overrides:
#   HM_RELEASE  release tag to install (default: latest)
#   HM_BINDIR   install location (default: /usr/local/bin if root, $HOME/.local/bin otherwise)

set -euo pipefail

HM_RELEASE="${HM_RELEASE:-latest}"

os="$(uname -s)"
case "$os" in
  Linux)  os=linux ;;
  Darwin) os=darwin ;;
  *) echo "Unsupported OS: $os" >&2; exit 1 ;;
esac

arch="$(uname -m)"
case "$arch" in
  x86_64)        arch=amd64 ;;
  aarch64|arm64) arch=arm64 ;;
  *) echo "Unsupported architecture: $arch" >&2; exit 1 ;;
esac

# verify checks a checklist file with whichever tool is present: GNU
# sha256sum on Linux, BSD shasum on macOS (which has no sha256sum).
verify() {
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum -c "$1"
  else
    shasum -a 256 -c "$1"
  fi
}

if [ -n "${HM_BINDIR:-}" ]; then
  bindir="$HM_BINDIR"
elif [ "$(id -u)" = "0" ]; then
  bindir=/usr/local/bin
else
  bindir="$HOME/.local/bin"
fi
mkdir -p "$bindir"

if [ "$HM_RELEASE" = "latest" ]; then
  base="https://github.com/kurowski/homie/releases/latest/download"
else
  base="https://github.com/kurowski/homie/releases/download/${HM_RELEASE}"
fi
binary="hm-${os}-${arch}"

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

echo "Downloading ${base}/${binary}"
curl -fsSL "$base/$binary"    -o "$tmp/$binary"
curl -fsSL "$base/SHA256SUMS" -o "$tmp/SHA256SUMS"

# SHA256SUMS carries an entry for every published os/arch. macOS shasum
# has no --ignore-missing, so filter to just our binary's line and verify
# that (the other binaries aren't downloaded here).
( cd "$tmp" && grep " ${binary}\$" SHA256SUMS > "$binary.sum" && verify "$binary.sum" )

install -m 0755 "$tmp/$binary" "$bindir/hm"

echo
echo "hm installed to $bindir/hm"
if ! printf '%s' ":$PATH:" | grep -q ":$bindir:"; then
  echo "Note: $bindir is not on your PATH. Add it with:"
  echo "  export PATH=\"$bindir:\$PATH\""
fi
echo "Run 'hm --help' to get started, or 'hm init ~/dotfiles' to scaffold a new environment repo."
