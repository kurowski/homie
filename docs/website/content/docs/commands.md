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
install native packages → install declared backends (alphabetical order:
`brew`, `flatpak`, `snap`, ...) → materialize `home/` (symlinks + rendered
templates) → run scripts → summary. Backend phases skip with a warning
when the tool isn't on PATH or the backend name isn't recognized.

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

## `hm home`

Just the home phase of `apply`. Walks `home/` and any active sibling
`home.tag-<X>[.tag-<Y>...]/` tree, then for each file:

- **Plain files** are symlinked into `$HOME` at the matching path. If
  a real file already lives at the destination, it's backed up to
  `<path>.homie-backup-<timestamp>` before linking — Homie never
  silently overwrites your data.
- **Files ending in `.tmpl`** are rendered through Go `text/template` +
  Sprig with the active data set and written into `$HOME` with the
  suffix stripped. Source mode is preserved (so `home/bin/foo.sh.tmpl`
  renders executable).

When two trees claim the same target, the more-specific tree (more
required tags in its directory name) wins; same-specificity collisions
error out so you can disambiguate. See [Dotfiles](/docs/dotfiles/) for
the full model.

---

## `hm install`

Just the package phases. Resolves `[packages].all + [packages].<distro>`
plus matching `[packages."tag:X"]` against the detected package manager
and installs missing entries, then runs the non-native backend phases
(`[packages.brew]`, `[packages.flatpak]`, `[packages.snap]`) in the same
order. On
unsupported distros, prints a friendly "not yet supported" notice and
skips the native phase. When a backend's tool isn't on PATH, prints a
warning and skips that backend.

---

## `hm run`

Just the script phase. Runs `scripts/*.sh` in lexical order, each as a
separate bash subprocess with `HM_TAGS`, `HM_REPO`, `HM_HOME`, and every
`[vars]` entry exported to its environment.

Scripts are user code — Homie doesn't enforce idempotency. Convention is
that each script is individually idempotent (e.g. `command -v X && exit
0` at the top).

Sibling directories named `scripts.tag-<X>[.tag-<Y>...]/` run only when
all of their tags are active (AND), mirroring the `home.tag-X/` trees.
Plain `scripts/` always runs. Scripts are ordered by filename across
every active tree — the numeric prefix is the single global order, so a
`scripts.tag-fedora/05-repos.sh` runs at position 05 next to a
`scripts/04-base.sh`. The tag trees decide which scripts participate, not
a separate ordered stream. The same filename appearing in two active
trees is a hard error — unlike the `home/` tree, scripts have no
"more-specific source wins" override rule, because an imperative script
has no single source of truth to override. See
[Dotfiles](/docs/dotfiles/) for the parallel file-tree model.

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
- Scripts that aren't executable, and filename collisions between active
  script trees.
- Tag-gated `home.tag-X/` and `scripts.tag-X/` trees that aren't active
  on this host (informational).
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
