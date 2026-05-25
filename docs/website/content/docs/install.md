---
title: "Install"
description: "Ways to get the hm binary onto your machine."
weight: 15
---

The fastest path is the install script, but Homie is a single static
binary — there's nothing stopping you from grabbing it any way you like.

---

## Install script (recommended)

Detects your arch, downloads the matching release, verifies its SHA256,
and drops `hm` into `/usr/local/bin` (or `~/.local/bin` if you're not
root):

```sh
curl -fsSL https://homie.sh/install.sh | bash
```

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
directly. There are two assets per release: the binary
(`hm-linux-<arch>`) and a `SHA256SUMS` file that covers all arches.

```sh
ARCH=$(uname -m | sed 's/x86_64/amd64/;s/aarch64/arm64/')
BASE=https://github.com/kurowski/homie/releases/latest/download

curl -fsSL -o /tmp/hm           "$BASE/hm-linux-${ARCH}"
curl -fsSL -o /tmp/SHA256SUMS   "$BASE/SHA256SUMS"

( cd /tmp && sha256sum -c --ignore-missing SHA256SUMS )

chmod +x /tmp/hm
sudo mv /tmp/hm /usr/local/bin/hm
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
