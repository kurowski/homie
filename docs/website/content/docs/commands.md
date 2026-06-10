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
`brew`, `flatpak`, `snap`, ...) → clone/update `[externals]` git repos →
materialize `home/` (symlinks + rendered templates) → run scripts →
summary. Backend phases skip with a warning when the tool isn't on PATH
or the backend name isn't recognized. Externals run before the home
phase so templates and symlinks can point into a checkout that's
guaranteed to exist; entries with a `ref` are held at it, unpinned
entries fast-forward to the remote default branch (see
[Config](/docs/config/#externals)).

```sh
hm apply
```

`apply` is idempotent. Running it twice in a row should produce a clean
summary with nothing changed.

Flags:

- `--skip-packages` — skip the native and backend package phases.
- `--skip-externals` — skip the externals (git clone/update) phase.
- `--skip-scripts` — skip pre-scripts and post-scripts.
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

Flags:

- `--dry-run` — write nothing. Prints the plan (every link and render
  target with its winning source), then the full rendered content of
  every active template. Use it to preview how templates resolve on
  this host before applying; exits non-zero if any template fails to
  render.

---

## `hm render`

Renders a single `.tmpl` file to stdout — no writes, no UI chrome, safe
to pipe. The data is exactly what a real `hm home` would use on this
host: active tags, `[vars]`, user identity, distro, and the `hasTag`
helper, so the preview is faithful.

```sh
hm render home/.gitconfig.tmpl
```

The path is tried as given first, then relative to the repo root, so
the command works from anywhere. A parse or execution error exits
non-zero — handy as a template check in CI, and it gives automated
agents a feedback loop for template authoring without touching your
real `$HOME`. To preview every active template at once, use
`hm home --dry-run`.

---

## `hm context`

Prints the exact data passed to every template render on this host, as
JSON. The keys match the template fields one-to-one — a key named
`"Email"` means a template may reference `{{ .Email }}` — so one read
tells you (or an agent) every field a template can use, with the values
it would resolve to right now.

```sh
$ hm context
{
  "Name": "Scout Homes",
  "Email": "scout@homie.sh",
  "Profile": "personal",
  "DefaultShell": "zsh",
  "Distro": "fedora",
  "Arch": "amd64",
  "IsContainer": false,
  "IsRoot": false,
  "Tags": ["amd64", "coach", "fedora", "personal"],
  "Vars": { "EDITOR": "nvim" }
}
```

Output is always JSON, nothing else on stdout — safe to pipe into `jq`.
The `hasTag` helper and the Sprig function library are callable from
templates but aren't data fields, so they don't appear here. Pairs
naturally with the preview commands: introspect the context, then check
a template with `hm render` or `hm home --dry-run`. See
[Dotfiles](/docs/dotfiles/) for the full data reference.

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

Run from a terminal, scripts inherit your stdin/stdout/stderr, so an
in-band prompt (a `sudo` password, `gh auth login`, a package-manager
confirmation) reaches you directly. When stdin isn't a terminal (CI,
piped, redirected), output is captured per-script and stdin is
`/dev/null`, so a script that would block on a prompt fails fast instead
of hanging.

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

Flags:

- `--json` — emit the same information as one JSON document: detected
  environment, repo path, identity, profile, active tags, the native and
  per-backend package lists, config warnings, and the error/warning
  counts from a doctor pass. When no environment repo is found, `"repo"`
  is `null` and the command still exits zero; an `HM_REPO` that points
  at a directory without a `homie.toml` is an error instead.

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
  on this host, and which multi-tag `[packages."tag:X.tag:Y"]` blocks are
  active (informational).
- Unknown distro detection.

Exit code is `0` if everything is clean, `1` otherwise — useful as a
post-deploy health check in CI.

Flags:

- `--json` — emit the findings as structured records instead of the
  styled report:

  ```json
  {
    "findings": [
      { "severity": "warn", "area": "packages", "message": "1 not installed: ripgrep" }
    ],
    "errors": 0,
    "warnings": 1
  }
  ```

  Severity is `error`, `warn`, or `info`; area is the check group
  (`env`, `config`, `home`, `link`, `render`, `packages`, `scripts`).
  The exit-code rule is unchanged — parse the document, then check the
  exit code. A non-zero exit caused by error findings still emits one
  valid JSON document on stdout; a failure before the checks run (no
  repo, unreadable config) prints to stderr with no JSON.

---

## `hm bootstrap`

Installs the minimum tools needed for `apply` to proceed: `git` and
`ca-certificates`. Called by `bootstrap.sh` on a fresh machine before
the user repo is cloned. Most users never invoke this directly.

---

## Help topics

Beyond per-command `--help`, the binary ships one reference topic:

- `hm help templating` — the template data fields, the `hasTag` helper,
  Sprig availability, and the missing-key rules. The offline version of
  the [Dotfiles](/docs/dotfiles/) reference; `hm help template` and
  `hm help templates` resolve to the same page.

---

## Global flags

These work on every subcommand:

- `--repo <path>` — point at a user repo other than the cwd (or use
  `HM_REPO=<path>`).
- `--no-tty` — force plain output.
- `--verbose` — Debug-level logging.
- `--version` — print version and exit.
