---
title: "Recipes"
description: "Concrete patterns for common Homie setups."
weight: 60
---

Working examples for the setups that come up most. Each recipe shows
just the relevant slice of `homie.toml` and the files you'd add — drop
into a `hm init`-scaffolded repo and adapt.

---

## Work laptop, personal laptop, same repo

The classic "one repo, different identity per machine" pattern. Use
the `[profile]` to mark the machine, and `[vars]` to carry the
work-only email so it never lands in your personal commits.

`homie.toml` (work machine):

```toml
[user]
name  = "Scout Homes"
email = "scout@homie.sh"

[profile]
name = "work"

[vars]
WORK_EMAIL = "scout@uceap.example.com"
```

`home/.gitconfig.tmpl`:

```gotmpl
[user]
    name  = {{ .Name }}
{{- if hasTag "work" }}
    email = {{ .Vars.WORK_EMAIL }}
{{- else }}
    email = {{ .Email }}
{{- end }}
```

Same `homie.toml` on the personal laptop, just `profile.name = "personal"`
and `WORK_EMAIL` omitted. The template renders the right address based
on which tag is active.

---

## Fedora workstation with extras

Fedora ships a few packages under different names than Debian/Ubuntu,
and a few that only Fedora has. Use a per-distro override.

```toml
[packages]
all = [
  "git", "zsh", "neovim", "tmux",
  "ripgrep", "fd", "fzf", "jq",
]
fedora = [
  "util-linux-user",    # provides `chsh`
  "dejavu-sans-fonts",
  "dejavu-sans-mono-fonts",
]
ubuntu = [
  "fd-find",            # `fd` on Ubuntu/Debian
  "fonts-dejavu",
]
debian = [
  "fd-find",
  "fonts-dejavu",
]
```

`hm apply` resolves the active set as `all ∪ <distro>`, deduped, and
only installs the missing ones.

---

## Devcontainer / GitHub Codespaces

Containers detect automatically (`/.dockerenv`, cgroup, or
`REMOTE_CONTAINERS` / `CODESPACES` env). Branch off `.IsContainer` or
the `container` tag.

```toml
[profile]
name = "devcontainer"

[packages]
all = ["git", "zsh", "ripgrep", "fd-find", "fzf"]
```

`home/.zshrc.tmpl` — keep it lean in containers:

```gotmpl
{{- if .IsContainer }}
# Container-tuned: no heavy prompt, no fzf history fancy bindings.
PS1='%n@%m %~ %# '
{{- else }}
# Full personal shell.
source ~/.config/zsh/prompt.zsh
source ~/.config/zsh/fzf-bindings.zsh
{{- end }}
```

In your devcontainer's `postCreateCommand`, just run
`bash <(curl -fsSL https://raw.githubusercontent.com/you/dotfiles/main/bootstrap.sh)`.

---

## Shared server, no root

You don't have `sudo`, but you can still apply dotfiles, render
templates, and run user-space setup scripts. The package phase will
fail without root — skip it by leaving `[packages]` empty (or by adding
guards in scripts that use `command -v` to check before install).

```toml
[packages]
# Empty — no system packages on this host.

[vars]
PREFIX = "/home/scout/.local"
```

`scripts/01-tools.sh`:

```sh
#!/usr/bin/env bash
set -euo pipefail

# Skip if mise is already installed.
command -v mise >/dev/null && exit 0

mkdir -p "$PREFIX/bin"
curl -fsSL https://mise.run | MISE_INSTALL_PATH="$PREFIX/bin/mise" sh
```

Run with `hm apply` as usual — `hm install` will print a friendly
notice when there's nothing to install.

---

## Shell and editor plugins without a plugin manager

Anything that "installs" by being a git checkout at a known path — zsh
plugins, `tpm`, an AstroNvim template — is an `[externals]` entry, not
a script. Homie clones it on the first apply and keeps it updated after
that.

```toml
[externals."~/.zsh/plugins/zsh-autosuggestions"]
repo = "https://github.com/zsh-users/zsh-autosuggestions"
ref  = "v0.7.1"          # pinned — your shell won't change under you

[externals."~/.tmux/plugins/tpm"]
repo = "https://github.com/tmux-plugins/tpm"
ref  = "v3.1.0"

[externals."~/.config/nvim"]
repo = "https://github.com/AstroNvim/template"
ref  = "v4.7.7"
```

Then source the checkouts from your dotfiles as usual, e.g. in
`home/.zshrc`:

```sh
source ~/.zsh/plugins/zsh-autosuggestions/zsh-autosuggestions.zsh
```

Updating a plugin is a one-line diff: bump the `ref`, run `hm apply`,
commit. Leaving `ref` off tracks the upstream default branch instead —
fine for a theme, risky for the shell you'd need to debug a bad update.
See [Config](/docs/config/#externals) for the full semantics.

---

## Tagged secrets via your password manager

Homie has no built-in secret support. Use your favourite secret store
to write files at known paths, then reference those paths from
scripts/templates.

`homie.toml`:

```toml
[vars]
SECRETS_DIR = "/run/user/1000/secrets"

[tags]
extra = ["has-secrets"]
```

`scripts/00-secrets.sh` (runs before any script that needs them):

```sh
#!/usr/bin/env bash
set -euo pipefail

# Idempotent: only fetch if we haven't this session.
test -f "$SECRETS_DIR/github-token" && exit 0

mkdir -p "$SECRETS_DIR"
pass show work/github-token > "$SECRETS_DIR/github-token"
chmod 600 "$SECRETS_DIR"/*
```

`home/.netrc.tmpl`:

```gotmpl
{{- if hasTag "has-secrets" }}
machine github.com
  login {{ .Vars.WORK_EMAIL }}
  password file://{{ .Vars.SECRETS_DIR }}/github-token
{{- end }}
```

The `has-secrets` tag is just for your own readability — strip it on
hosts where secrets aren't available, and the template skips itself.

---

## Third-party package repos via pre-scripts

`[packages]` runs against the native package manager — `dnf` or `apt`.
To install something that lives in a *third-party* repo (VS Code,
1Password, HashiCorp, Docker, RPM Fusion, etc.) you need that repo
registered with the package manager **before** `hm apply`'s install
step. That's what `scripts/pre-*.sh` is for: every script whose name
begins with `pre-` runs ahead of the package phase.

Lifecycle, end to end:

```
detect → pre-scripts → packages → link → render → scripts
```

Same env (`HM_REPO`, `HM_HOME`, `HM_TAGS`, `[vars]`) as the post-scripts
you already write. Both groups are ordered lexically inside their phase,
and each script is responsible for its own idempotency.

`scripts/pre-01-vscode-repo.sh` (Fedora):

```sh
#!/usr/bin/env bash
set -euo pipefail

# Idempotent: skip if already configured.
test -f /etc/yum.repos.d/vscode.repo && exit 0

sudo rpm --import https://packages.microsoft.com/keys/microsoft.asc
sudo tee /etc/yum.repos.d/vscode.repo > /dev/null <<EOF
[code]
name=Visual Studio Code
baseurl=https://packages.microsoft.com/yumrepos/vscode
enabled=1
gpgcheck=1
gpgkey=https://packages.microsoft.com/keys/microsoft.asc
EOF
```

`homie.toml`:

```toml
[packages]
fedora = ["code"]
```

To run only the pre-scripts without touching packages or dotfiles:

```sh
hm run --phase=pre
```

---

## CI verification step

A useful idiom: run `hm status` and `hm doctor` in CI on the user
environment repo itself, so you catch a broken template or a missing
package reference before it bites on a fresh box.

`.github/workflows/check.yml`:

```yaml
name: check
on: [push, pull_request]
jobs:
  doctor:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v6
      - name: Install hm
        run: |
          curl -fsSL -o hm https://github.com/kurowski/homie/releases/latest/download/hm-linux-amd64
          chmod +x hm
          sudo mv hm /usr/local/bin/
      - run: hm status --no-tty
      - run: hm doctor --no-tty
```

`hm doctor` exits non-zero on any problem, so the job fails loudly when
the repo drifts.

The `hm-linux-amd64` binary is correct for the `ubuntu-latest` runner
above; on a `macos-latest` runner, download `hm-darwin-arm64` (or
`hm-darwin-amd64` on Intel) instead.

---

## A complete real-world repo

The recipes above are sliced down to one idea each. To see them combined
in a repo that's actually in daily use, Homie's author keeps their own
environment repo public:

**[github.com/kurowski/dotfiles](https://github.com/kurowski/dotfiles)**

It exercises just about every Homie feature at once:

- A `home/` tree of real configs (zsh, Neovim, tmux, Ghostty, eza) as
  symlinks, plus `.tmpl` templates for the files that vary per machine
  (`.gitconfig`, a work-only `.zshrc.local`).
- Tag-gated `home.tag-work/` and multi-tag `home.tag-work.tag-kde/` trees
  for files that only belong on certain machines.
- Native `[packages]` for Fedora, Debian, and macOS — with Homebrew
  `/cask` GUI apps on the Mac side.
- `flatpak` and `snap` backends alongside the native lists.
- Tag-keyed and multi-tag **AND** package blocks, e.g.
  `[packages."tag:desktop"]`, `[packages."tag:personal.tag:ubuntu".snap]`,
  and even a three-tag `[packages."tag:personal.tag:desktop.tag:ubuntu".snap]`.
- Per-host overlays in `hosts/` for half a dozen real machines (work
  laptops, a desktop, a server).
- Ordered `scripts/` plus tag-conditional `scripts.tag-fedora/`,
  `scripts.tag-ubuntu/`, `scripts.tag-gnome.tag-personal/`, and friends —
  including `pre-*` scripts that add third-party package repos before the
  package phase runs.
- `[vars]` for per-machine identity and a generated `bootstrap.sh`.

Clone it for a concrete reference, or borrow whichever pieces map onto
your own setup.
