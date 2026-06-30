---
title: "Dotfiles"
description: "How files in home/ become files in $HOME — symlinks, templates, tag-gated trees, and the override rule."
weight: 40
---

Everything Homie writes into `$HOME` comes from one directory in your
repo: `home/`. The shape of the file decides what happens to it:

- A **plain file** (`home/.zshrc`) becomes a **symlink** in `$HOME` at
  the matching path (`~/.zshrc`).
- A file whose name ends in **`.tmpl`** (`home/.gitconfig.tmpl`) is
  **rendered** through Go `text/template` + [Sprig](https://masterminds.github.io/sprig/),
  with the `.tmpl` suffix stripped on the way out (`~/.gitconfig`).

That's it. The suffix is the only disambiguator. Two trees writing the
same target are resolved by [tag specificity](#overrides), not by
configuration.

"Dotfiles" is the conventional name for what people manage this way,
but Homie doesn't actually care about the leading dot — any file under
your `$HOME` can live in `home/`: `home/bin/foo`, `home/Pictures/wallpaper.jpg`,
whatever.

---

## Symlinks

A plain file at `home/<path>` symlinks to `~/<path>` on `hm apply`. The
symlink points at the real file inside your repo, so:

```sh
$ ls -l ~/.zshrc
~/.zshrc -> /home/scout/dotfiles/home/.zshrc
```

- **Edit the source directly.** `vim ~/.zshrc` opens the file in the
  repo through the symlink. Save, `git diff`, commit — no separate
  "stage this back to the repo" step.
- **Mode is preserved** by the filesystem, not by Homie — symlinks
  resolve through to the source's permissions.
- **Conflicts are backed up.** If a real file already exists at the
  destination, `hm apply` moves it aside to
  `<path>.homie-backup-<timestamp>` before creating the symlink. Your
  data is never silently overwritten.
- **Stale symlinks are replaced.** If `~/.zshrc` already points
  somewhere unexpected (a previous tool, an old config), Homie removes
  the old link and creates a fresh one to the repo. The previous
  target file isn't touched.

`hm home` runs the symlink + render phases against this tree without
touching packages or scripts. `hm doctor` reports broken symlinks (a
symlink in `$HOME` pointing into your repo, but the source file no
longer exists).

---

## Templates

A file at `home/<path>.tmpl` is parsed as a Go [`text/template`](https://pkg.go.dev/text/template)
and rendered into `~/<path>` (with the `.tmpl` suffix stripped). The
output is a **real file**, not a symlink — the template is the source
of truth, the output is the artifact. Re-run `hm apply` (or `hm home`)
to refresh.

Source file mode carries through: `home/bin/foo.sh.tmpl` renders
executable.

### Previewing

To see what a template resolves to on the current host without writing
into `$HOME`, render it to stdout:

```sh
hm render home/.gitconfig.tmpl   # one template, raw output
hm home --dry-run                # every active template, plus the link/render plan
```

Both use the same data as a real run (active tags, `[vars]`, `hasTag`),
so the preview is faithful — a tight edit-render-inspect loop while
authoring, and safe for CI or automated agents to call.

### Syntax

Templates use Go's [`text/template`](https://pkg.go.dev/text/template)
syntax augmented with [Sprig](https://masterminds.github.io/sprig/)
helpers and one Homie extension (`hasTag`). Same delimiters as
chezmoi/Helm: `{{ }}`.

```gotmpl
[user]
    name  = {{ .Name }}
{{- if hasTag "work" }}
    email = {{ .Vars.WORK_EMAIL }}
{{- else }}
    email = {{ .Email }}
{{- end }}
```

`{{- ... }}` trims preceding whitespace and the newline; `{{ ... -}}`
trims following. Standard `text/template` mechanics — use them to keep
the rendered output clean.

**Missing fields fail loudly.** Homie sets `missingkey=error`, so a
typo like `{{ .Eamil }}` errors out instead of rendering `<no value>`.

### Data context

Every template has these fields available:

| Field           | Type                | Source |
|-----------------|---------------------|--------|
| `.Name`         | string              | `[user].name` |
| `.Email`        | string              | `[user].email` |
| `.Profile`      | string              | `[profile].name` |
| `.DefaultShell` | string              | `[profile].default_shell` |
| `.Distro`       | string              | detected (`ubuntu`, `debian`, `fedora`, `macos`, `termux`, `unknown`) |
| `.Arch`         | string              | detected (`amd64`, `arm64`) |
| `.IsContainer`  | bool                | detected (`/.dockerenv`, cgroup, env) |
| `.IsRoot`       | bool                | detected (running as root) |
| `.Tags`         | `[]string`          | merged auto + profile + extra |
| `.Vars`         | `map[string]any`    | the `[vars]` table |

To see this table populated with the live values on the current host,
run `hm context` — it prints the data as JSON with keys matching the
field names above, so it doubles as a machine-readable reference for
scripts and agents. The reference itself also ships inside the binary:
`hm help templating` lists these fields, the `hasTag` helper, and the
missing-key rules, so it works offline on any machine with `hm`
installed.

### The `hasTag` helper

Tag membership is the most common branch, so it gets a dedicated
function:

```gotmpl
{{ if hasTag "fedora" }} ... {{ end }}
{{ if hasTag "container" }} ... {{ end }}
```

`hasTag` returns `true` if the name is in `.Tags`. Equivalent to
`has "fedora" .Tags` but easier to read at a glance.

### Sprig functions

You get all of Sprig's ~100 helpers. The ones that come up most:

- **`default`** for fallbacks: `{{ default "nvim" .Vars.EDITOR }}` — but
  see the note below about `missingkey=error`.
- **`hasKey`** for optional vars: `{{ if hasKey .Vars "WORK_EMAIL" }}...{{ end }}`.
- **`dig`** for nested fallback: `{{ dig "WORK_EMAIL" "fallback" .Vars }}`.
- **String ops:** `lower`, `upper`, `trim`, `replace`, `contains`.
- **List ops:** `has`, `join`, `sortAlpha`.

Full reference: <https://masterminds.github.io/sprig/>.

#### `default` vs `missingkey=error`

`default` cannot rescue a missing map key. Because `missingkey=error`
is set, evaluating `.Vars.MISSING` errors before `default` ever sees
the value. For optional vars use `hasKey` or `dig`:

```gotmpl
{{ /* Wrong — errors if WORK_EMAIL is unset */ }}
{{ default .Email .Vars.WORK_EMAIL }}

{{ /* Right */ }}
{{ if hasKey .Vars "WORK_EMAIL" }}{{ .Vars.WORK_EMAIL }}{{ else }}{{ .Email }}{{ end }}

{{ /* Also right, more concise */ }}
{{ dig "WORK_EMAIL" .Email .Vars }}
```

---

## Tag-gated trees

Sibling directories named `home.tag-<X>[.tag-<Y>...]` are processed
only when every named tag is active on the host. Plain `home/` always
applies. The convention covers both kinds of file — plain and `.tmpl`
— because they share one tree:

```
home/                            # always
  .zshrc                           # plain → symlink
  .gitconfig.tmpl                  # → renders to ~/.gitconfig
home.tag-work/                   # only when hasTag "work"
  .ssh/config.tmpl
home.tag-work.tag-kde/           # AND: both tags must be active
  .config/plasma/some-template.tmpl
```

Use a tag-gated directory when an **entire file** is conditional on a
tag — including binary blobs, opaque desktop files, anything that
isn't worth templating just to gate it. For per-line conditionals
inside a single rendered file, use `{{ if hasTag ... }}` within one
template — that's what `hasTag` is for.

Tag names in directory suffixes can't contain `.` — that character is
how Homie splits multiple tags inside one directory name. So
`home.tag-fedora.42/` parses as two segments (`fedora` and `42`), and
since the second one is malformed (`42` doesn't start with `tag-`),
the whole directory is rejected. Use only `[A-Za-z0-9_-]`-style tag
names when naming a tag-gated directory.

`hm doctor` lists tag-gated trees that aren't active on the current
host as informational findings — useful for sanity-checking a
multi-tag layout from a host where only some of the trees apply.

The same naming convention extends to scripts: `scripts.tag-<X>/`
directories run only when their tags are active. Unlike home trees,
which resolve same-target collisions by specificity, scripts have no
override rule — the same filename in two active script trees is an
error. See [`hm run`](/docs/commands/#hm-run) for the ordering rules.

---

## Overrides

Two trees can legitimately claim the same `$HOME` target. The
**more-specific tree wins**, where specificity is the number of
required tags on the source tree's directory name:

| Directory                  | Specificity |
|----------------------------|-------------|
| `home/`                    | 0           |
| `home.tag-work/`           | 1           |
| `home.tag-work.tag-kde/`   | 2           |

Different specificity → the deeper tree wins silently, regardless of
class. So a templated `.gitconfig` in the work tree overrides a plain
`.gitconfig` in the base, and vice versa:

```
home/.gitconfig               (spec 0) — applies on every host
home.tag-work/.gitconfig.tmpl (spec 1) — overrides on a work host
```

On a work host, the templated `.gitconfig` wins; the base symlink is
not created. Everywhere else, the plain `home/.gitconfig` symlinks
normally. Same shape works in reverse: a plain file in a more-specific
tree can override a template in the base.

**Same specificity → error.** When two trees claim the same target at
the same depth, Homie won't guess. Two common shapes:

- **Same tree, both classes:** `home/.gitconfig` *and*
  `home/.gitconfig.tmpl` (both spec 0). Collapse into one — usually
  the template.
- **Sibling tag dirs:** `home.tag-work/.foo` and
  `home.tag-personal/.foo` (both spec 1, with both tags somehow
  active). Narrow one tree (`home.tag-work.tag-laptop/`), or merge
  into a single template with `{{ if hasTag ... }}`.

The override is resolved once, before either phase writes anything,
so the symlink phase and the template phase agree on which source
wins for each target.

---

## Cookbook

### One file, two hosts

A `.gitconfig` whose email changes between work and personal:

```toml
# homie.toml
[user]
name  = "Scout Homes"
email = "scout@homie.sh"

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

One template, one file in `$HOME`, branch on tag. Pair with a per-host
overlay (`hosts/coach.toml`, `hosts/uceap-dev01.toml`) that sets the
profile or extra tag.

### A whole file only on KDE hosts

Some files don't make sense to template — a KDE plasma config is an
opaque blob, a binary desktop file, an executable script that's
either-or. Use a tag-gated tree:

```
home/
  .zshrc
home.tag-kde/
  .config/plasma-workspace/env/01-keychain.sh
  .config/autostart/walgrun.desktop
```

On KDE hosts, both files materialize under `~/.config/`. Off KDE,
they're absent — no empty templates with `{{ if hasTag "kde" }}{{ end }}`
wrappers around the whole body.

### Per-profile block in a template

```gotmpl
{{- if eq .Profile "work" -}}
[includeIf "gitdir:~/work/"]
    path = ~/.config/git/work
{{- end }}
```

### OS-conditional template lines

```gotmpl
{{- if hasTag "fedora" }}
[diff]
    tool = meld
{{- else if hasTag "ubuntu" }}
[diff]
    tool = vimdiff
{{- end }}
```

### Container-aware defaults

```gotmpl
[core]
    editor = {{ if .IsContainer }}vi{{ else }}nvim{{ end }}
    pager  = {{ if .IsContainer }}cat{{ else }}delta{{ end }}
```

### Looping over tags

```gotmpl
# tags active on this machine:
{{- range .Tags }}
# - {{ . }}
{{- end }}
```

### A template that is a shell script

`home/bin/sync-secrets.sh.tmpl`:

```gotmpl
#!/usr/bin/env bash
set -euo pipefail

REMOTE={{ .Vars.SECRETS_REMOTE | quote }}
mkdir -p "$HOME/.secrets"
rsync -a --delete "$REMOTE/" "$HOME/.secrets/"
```

The source file's executable bit carries through, so the rendered
`~/bin/sync-secrets.sh` is runnable.

### A symlink that points anywhere except `$HOME`

`hm home` only writes into `$HOME` — it can't put a file at, say,
`/etc/sudoers.d/scout`. Use a `scripts/*.sh` for that:

```sh
#!/usr/bin/env bash
set -euo pipefail

dest=/etc/sudoers.d/scout
sudo install -m 0440 -o root -g root "$HM_REPO/home/sudoers" "$dest"
```

The source still lives in `home/`, so the install path stays in sync
with `git diff`.
