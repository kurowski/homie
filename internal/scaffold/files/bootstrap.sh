#!/usr/bin/env bash
# Bootstrap script for {{ .GitHubUser }}/{{ .GitHubRepo }}.
#
# Run this on a fresh Linux or macOS machine:
#   curl -fsSL https://raw.githubusercontent.com/{{ .GitHubUser }}/{{ .GitHubRepo }}/main/bootstrap.sh | bash
#
# Flow:
#   1. Download the hm binary for this os/arch.
#   2. `hm bootstrap` ensures prerequisites: git + ca-certificates on
#      Linux, just git (via the Xcode CLT) on macOS, so HTTPS clones
#      work and the next step plus all future `hm apply` runs can
#      reach GitHub.
#   3. Clone this repo and exec `hm apply`.
#
# This file is generated and stays Homie's — it tracks how the current hm
# wants to be launched, so it goes stale when you upgrade. Refresh it with
# `hm init --update` from this repo, then commit the diff. Local edits are
# safe: update shows what it would change and stops rather than clobbering
# (`--force` to override). Delete the hm:generated line below to opt out
# and own this file yourself.
set -euo pipefail

REPO_URL="https://github.com/{{ .GitHubUser }}/{{ .GitHubRepo }}.git"
# Where this repo lands on a fresh machine. Generated from wherever the
# repo lived when you last ran `hm init` / `hm init --update`, so keeping
# it somewhere other than $HOME/<repo> needs no hand-editing — move the
# repo, re-run --update, commit.
REPO_DIR="${HM_REPO:-{{ .RepoDir }}}"
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

# Under `curl ... | bash` stdin is the pipe this script is being read from,
# not the terminal. Any child that needs to prompt — `sudo` inside `hm
# bootstrap`, `sudo` inside a setup script during apply, a credential helper
# during the clone — then dies with "a terminal is required to read the
# password" and takes the run down with it. So hand those children the
# controlling terminal when there is one; in CI and containers there's no
# /dev/tty (and sudo is passwordless anyway), so they keep the stdin they had.
#
# This can't be a single `exec </dev/tty` up front: bash is still reading the
# rest of this script from that same stdin. The probe runs in a subshell
# deliberately — a failed redirection on `exec`, a special builtin, can
# terminate a non-interactive shell outright.
tty_in=""
if (: </dev/tty) 2>/dev/null; then
  tty_in=/dev/tty
fi

# withtty runs a command with the controlling terminal on stdin, or with
# stdin untouched when there isn't one.
withtty() {
  if [ -n "$tty_in" ]; then
    "$@" <"$tty_in"
  else
    "$@"
  fi
}

if [ "$(id -u)" = "0" ]; then
  bindir=/usr/local/bin
else
  bindir="$HOME/.local/bin"
  mkdir -p "$bindir"
fi

if ! command -v hm >/dev/null 2>&1; then
  # The `latest` keyword has its own URL shape; specific tags use a
  # different one. Both end with /download.
  if [ "$HM_RELEASE" = "latest" ]; then
    base="https://github.com/kurowski/homie/releases/latest/download"
  else
    base="https://github.com/kurowski/homie/releases/download/${HM_RELEASE}"
  fi
  binary="hm-${os}-${arch}"
  tmp="$(mktemp -d)"
  trap 'rm -rf "$tmp"' EXIT

  echo "Downloading ${base}/${binary}"
  curl -fsSL "$base/$binary"     -o "$tmp/$binary"
  curl -fsSL "$base/SHA256SUMS"  -o "$tmp/SHA256SUMS"

  # SHA256SUMS lists every published os/arch. macOS shasum has no
  # --ignore-missing, so filter to our binary's line and verify that. -F
  # keeps it a fixed-string match in case a future arch name ever carries
  # a regex metachar.
  ( cd "$tmp" && grep -F " ${binary}" SHA256SUMS > "$binary.sum" && verify "$binary.sum" )

  install -m 0755 "$tmp/$binary" "$bindir/hm"
  export PATH="$bindir:$PATH"
fi

# Let hm install the rest of its own prereqs (git, ca-certificates) so
# the distro-detection lives in one place (Go) and this script stays
# tiny.
withtty hm bootstrap

if [ ! -d "$REPO_DIR/.git" ]; then
  echo "Cloning ${REPO_URL} -> ${REPO_DIR}"
  # git clone won't create nested parents; a no-op when REPO_DIR sits
  # directly under $HOME.
  mkdir -p "$(dirname "$REPO_DIR")"
  withtty git clone "$REPO_URL" "$REPO_DIR"
fi

cd "$REPO_DIR"
if [ -n "$tty_in" ]; then
  exec hm apply <"$tty_in"
fi
exec hm apply
