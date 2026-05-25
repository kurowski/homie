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

arch="$(uname -m)"
case "$arch" in
  x86_64)        arch=amd64 ;;
  aarch64|arm64) arch=arm64 ;;
  *) echo "Unsupported architecture: $arch" >&2; exit 1 ;;
esac

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
binary="hm-linux-${arch}"

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

echo "Downloading ${base}/${binary}"
curl -fsSL "$base/$binary"    -o "$tmp/$binary"
curl -fsSL "$base/SHA256SUMS" -o "$tmp/SHA256SUMS"

# --ignore-missing verifies only the line matching $binary; the file
# carries entries for every published arch.
( cd "$tmp" && sha256sum -c --ignore-missing SHA256SUMS )

install -m 0755 "$tmp/$binary" "$bindir/hm"

echo
echo "hm installed to $bindir/hm"
if ! printf '%s' ":$PATH:" | grep -q ":$bindir:"; then
  echo "Note: $bindir is not on your PATH. Add it with:"
  echo "  export PATH=\"$bindir:\$PATH\""
fi
echo "Run 'hm --help' to get started, or 'hm init ~/dotfiles' to scaffold a new environment repo."
