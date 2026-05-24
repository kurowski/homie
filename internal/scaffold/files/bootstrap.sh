#!/usr/bin/env bash
# Bootstrap script for {{ .GitHubUser }}/{{ .GitHubRepo }}.
#
# Run this on a fresh Linux machine:
#   curl -fsSL https://raw.githubusercontent.com/{{ .GitHubUser }}/{{ .GitHubRepo }}/main/bootstrap.sh | bash
#
# Flow:
#   1. Download the hm binary for this arch.
#   2. `hm bootstrap` installs git + ca-certificates so HTTPS clones
#      work and the next step plus all future `hm apply` runs can
#      reach GitHub.
#   3. Clone this repo and exec `hm apply`.
set -euo pipefail

REPO_URL="https://github.com/{{ .GitHubUser }}/{{ .GitHubRepo }}.git"
REPO_DIR="${HM_REPO:-$HOME/{{ .GitHubRepo }}}"
HM_RELEASE="${HM_RELEASE:-latest}"

arch="$(uname -m)"
case "$arch" in
  x86_64)        arch=amd64 ;;
  aarch64|arm64) arch=arm64 ;;
  *) echo "Unsupported architecture: $arch" >&2; exit 1 ;;
esac

if [ "$(id -u)" = "0" ]; then
  bindir=/usr/local/bin
else
  bindir="$HOME/.local/bin"
  mkdir -p "$bindir"
fi

if ! command -v hm >/dev/null 2>&1; then
  url="https://github.com/kurowski/homie/releases/${HM_RELEASE}/download/hm-linux-${arch}"
  echo "Downloading hm from ${url} -> ${bindir}/hm"
  curl -fsSL "$url" -o "$bindir/hm"
  chmod +x "$bindir/hm"
  # TODO: verify SHA256 against the release checksum file
  export PATH="$bindir:$PATH"
fi

# Let hm install the rest of its own prereqs (git, ca-certificates) so
# the distro-detection lives in one place (Go) and this script stays
# tiny.
hm bootstrap

if [ ! -d "$REPO_DIR/.git" ]; then
  echo "Cloning ${REPO_URL} -> ${REPO_DIR}"
  git clone "$REPO_URL" "$REPO_DIR"
fi

cd "$REPO_DIR"
exec hm apply
