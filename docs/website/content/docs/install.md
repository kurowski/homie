---
title: "Install"
description: "Ways to get the hm binary onto your machine."
weight: 15
---

The fastest path is the install script, but Homie is a single static
binary — there's nothing stopping you from grabbing it any way you like.

---

## Install script (recommended)

Detects your OS (Linux or macOS) and arch, downloads the matching release,
verifies its SHA256, and drops `hm` into `/usr/local/bin` (or `~/.local/bin`
if you're not root):

```sh
curl -fsSL https://homie.sh/install.sh | bash
```

The same one-liner works on Linux and macOS (Apple Silicon and Intel) — the
script picks `hm-linux-*` or `hm-darwin-*` for you.

The script honours two environment overrides:

- `HM_RELEASE` — release tag to install (default: `latest`).
- `HM_BINDIR` — install location (default: `/usr/local/bin` when root,
  `$HOME/.local/bin` otherwise).

### Pin to a specific release

```sh
curl -fsSL https://homie.sh/install.sh | HM_RELEASE=v0.1.0 bash
```

### Install to a custom location

```sh
curl -fsSL https://homie.sh/install.sh | HM_BINDIR=$HOME/bin bash
```

---

## Manual download

If you'd rather not pipe a script into your shell, grab the binary
directly. Each release publishes a binary per OS/arch
(`hm-linux-<arch>`, `hm-darwin-<arch>`) plus a `SHA256SUMS` file that
covers them all.

```sh
OS=$(uname -s | tr '[:upper:]' '[:lower:]')   # linux or darwin
ARCH=$(uname -m | sed 's/x86_64/amd64/;s/aarch64/arm64/')
BASE=https://github.com/kurowski/homie/releases/latest/download
BIN="hm-${OS}-${ARCH}"

cd "$(mktemp -d)"
curl -fsSL -o "$BIN"      "$BASE/$BIN"
curl -fsSL -o SHA256SUMS  "$BASE/SHA256SUMS"

# Download under the real name so the checklist matches. macOS has no
# sha256sum, so filter to our line and fall back to shasum.
grep " ${BIN}\$" SHA256SUMS > hm.sum
if command -v sha256sum >/dev/null; then sha256sum -c hm.sum; else shasum -a 256 -c hm.sum; fi

chmod +x "$BIN"
sudo mv "$BIN" /usr/local/bin/hm
hm --version
```

Swap `latest` for a tag (e.g.
`https://github.com/kurowski/homie/releases/download/v0.1.0`) to pin a
specific version.

---

## Build from source

Homie has no `cgo` requirement and no runtime dependencies — a recent
Go toolchain is enough.

```sh
git clone https://github.com/kurowski/homie.git
cd homie
make build              # CGO_ENABLED=0 go build -ldflags=... -o hm ./cmd/hm
sudo install -m 0755 hm /usr/local/bin/hm
hm --version
```

Or skip the clone with `go install`:

```sh
go install github.com/kurowski/homie/cmd/hm@latest
```

That drops `hm` into `$(go env GOBIN)` (or `$(go env GOPATH)/bin` if
`GOBIN` is unset). Make sure that directory is on your `PATH`.

The version string baked into the binary will say `(devel)` with
`go install`; the `make build` path uses `git describe` so `hm --version`
prints the tag.

---

## Verifying the install

```sh
hm --version
hm doctor
```

`hm doctor` runs a no-op health check — useful even right after install
to confirm the binary is wired up correctly.

---

## Updating

`hm` updates itself:

```sh
hm selfupdate          # replace the binary with the latest release
hm selfupdate --check  # just see whether a newer release exists
```

It downloads the right binary for your OS and architecture, verifies it
against the release's `SHA256SUMS`, and replaces the binary in place —
the same checks the install script performs. A binary in
`/usr/local/bin` usually needs `sudo hm selfupdate`; the default user
install in `~/.local/bin` doesn't.

Re-running the install script does the same job, and is the way to pin
or downgrade:

```sh
curl -fsSL https://homie.sh/install.sh | bash                      # latest
curl -fsSL https://homie.sh/install.sh | HM_RELEASE=v0.2.0 bash    # pin / downgrade
hm --version                                                       # confirm
```

Builds from source update the way they were installed — re-run
`go install …@latest`, or `git pull` and `make build` from the checkout.
`hm selfupdate` refuses to overwrite a from-source build, so a dev
binary can't be clobbered by accident.

If `hm --version` doesn't show the version you just installed, a copy
left on `PATH` by a different install method is probably shadowing it —
`which hm` to find which one wins.

Re-running your environment repo's `bootstrap.sh` also pulls the latest
`hm` before applying, but that's a full reapply of your environment, not
just a binary bump — `hm selfupdate` when you only want the binary.
