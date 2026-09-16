#!/usr/bin/env bash
# Install the homie binary from a GitHub release.
#
# Usage:
#   curl -fsSL https://homie.sh/install.sh | bash
#
# Environment overrides:
#   HOMIE_RELEASE  release tag to install (default: latest)
#   HOMIE_BINDIR   install location (default: /usr/local/bin if root, $HOME/.local/bin otherwise)

set -euo pipefail

HOMIE_RELEASE="${HOMIE_RELEASE:-latest}"

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

if [ -n "${HOMIE_BINDIR:-}" ]; then
  bindir="$HOMIE_BINDIR"
elif [ "$(id -u)" = "0" ]; then
  bindir=/usr/local/bin
else
  bindir="$HOME/.local/bin"
fi
mkdir -p "$bindir"

if [ "$HOMIE_RELEASE" = "latest" ]; then
  base="https://github.com/kurowski/homie/releases/latest/download"
else
  base="https://github.com/kurowski/homie/releases/download/${HOMIE_RELEASE}"
fi
binary="homie-${os}-${arch}"

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

echo "Downloading ${base}/${binary}"
curl -fsSL "$base/$binary"    -o "$tmp/$binary"
curl -fsSL "$base/SHA256SUMS" -o "$tmp/SHA256SUMS"

# SHA256SUMS carries an entry for every published os/arch. macOS shasum
# has no --ignore-missing, so filter to just our binary's line and verify
# that (the other binaries aren't downloaded here). -F keeps the match a
# fixed string in case a future arch name ever carries a regex metachar.
( cd "$tmp" && grep -F " ${binary}" SHA256SUMS > "$binary.sum" && verify "$binary.sum" )

install -m 0755 "$tmp/$binary" "$bindir/homie"

echo
echo "homie installed to $bindir/homie"
if ! printf '%s' ":$PATH:" | grep -q ":$bindir:"; then
  echo "Note: $bindir is not on your PATH. Add it with:"
  echo "  export PATH=\"$bindir:\$PATH\""
fi
echo "Run 'homie --help' to get started, or 'homie init ~/dotfiles' to scaffold a new environment repo."
