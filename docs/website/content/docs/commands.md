---
title: "Commands"
description: "Reference for every `hm` subcommand."
weight: 20
---

Every Homie command except `hm init` expects to be run from the root of
a user environment repo, or with `HM_REPO` set to its path.

---

## `hm init`

Interactively scaffolds a brand new user environment repo. Run this once
on the first machine. Subsequent machines clone the result via
`bootstrap.sh`.

```sh
hm init ~/dotfiles
```

You can also pass flags non-interactively (useful in CI):

```sh
hm init \
  --name "Scout Homes" \
  --email scout@homie.sh \
  --github-user scouthomes \
  --github-repo dotfiles \
  --profile personal \
  --shell zsh \
  ~/dotfiles
```

---

## `hm apply`

Full reconciliation pass — detect → load config → run pre-scripts →
install packages → symlink dotfiles → render templates → run scripts →
summary.

```sh
hm apply
```

`apply` is idempotent. Running it twice in a row should produce a clean
summary with nothing changed.

Flags:

- `--no-tty` — force plain output (no Bubble Tea spinner). Auto-detected
  when stdout isn't a terminal; this flag overrides the detection.
- `--verbose` — raise log level to Debug.

---

## `hm link`

Just the dotfile phase of `apply`. Walks `dotfiles/` and ensures every
file under it is symlinked into `$HOME` at the matching path.

Conflicts (a real file exists at the destination) are backed up to
`<path>.homie-backup-<timestamp>` before linking — Homie never silently
overwrites your data.

---

## `hm render`

Just the template phase. Walks `templates/*.tmpl`, renders each through
Go `text/template` + Sprig with the active data set, and writes the
output (without the `.tmpl` suffix) into `$HOME`. Source mode is
preserved (so `templates/bin/foo.sh.tmpl` renders executable).

---

## `hm install`

Just the package phase. Resolves `[packages].all + [packages].<distro>`
against the detected package manager and installs any missing entries.
On unsupported distros, prints a friendly "not yet supported" notice and
skips.

---

## `hm run`

Just the script phase. Runs `scripts/*.sh` in lexical order, each as a
separate bash subprocess with `HM_TAGS`, `HM_REPO`, `HM_HOME`, and every
`[vars]` entry exported to its environment.

Scripts are user code — Homie doesn't enforce idempotency. Convention is
that each script is individually idempotent (e.g. `command -v X && exit
0` at the top).

Flags:

- `--phase=post` (default) — every script whose name does NOT begin with
  `pre-`. The "scripts" step of `hm apply`.
- `--phase=pre` — only `pre-*.sh` scripts. The "pre-scripts" step of
  `hm apply`; useful for setting up third-party package sources before
  `hm install`.
- `--phase=all` — pre-scripts then post-scripts, matching the order
  `hm apply` uses.

---

## `hm status`

Read-only view of what `apply` _would_ do. No changes, no installs, no
file writes. Useful for previewing, for CI checks, or for confidence
before a real run.

---

## `hm doctor`

Walks the repo and reports:

- Broken symlinks (target doesn't exist).
- Symlinks in `$HOME` that point outside the repo.
- Missing packages.
- Unrendered templates.
- Scripts that aren't executable.
- Unknown distro detection.

Exit code is `0` if everything is clean, `1` otherwise — useful as a
post-deploy health check in CI.

---

## `hm bootstrap`

Installs the minimum tools needed for `apply` to proceed: `git` and
`ca-certificates`. Called by `bootstrap.sh` on a fresh machine before
the user repo is cloned. Most users never invoke this directly.

---

## Global flags

These work on every subcommand:

- `--repo <path>` — point at a user repo other than the cwd (or use
  `HM_REPO=<path>`).
- `--no-tty` — force plain output.
- `--verbose` — Debug-level logging.
- `--version` — print version and exit.
