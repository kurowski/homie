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

`templates/.gitconfig.tmpl`:

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

`templates/.zshrc.tmpl` — keep it lean in containers:

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

`templates/.netrc.tmpl`:

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
