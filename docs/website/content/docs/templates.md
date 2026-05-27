---
title: "Templates"
description: "Go text/template + Sprig + hasTag, with a cookbook of patterns."
weight: 40
---

Templates live in `home/` alongside plain symlink sources — the `.tmpl`
suffix is what distinguishes "render this" from "symlink this." A file
named `home/.gitconfig.tmpl` renders to `~/.gitconfig`; the suffix is
stripped on output, everything else maps one-to-one.

Rendered files are **real files**, not symlinks — the template is the
source of truth, the output is the artifact. Source file mode is
preserved, so `home/bin/foo.sh.tmpl` renders executable.

### Tag-gated home trees

Sibling directories named `home.tag-<X>[.tag-<Y>...]` are processed
only when every named tag is active on the host. Plain `home/` always
applies. The same convention covers both kinds of file — plain and
`.tmpl` — because they share one tree:

```
home/                            # always
  .zshrc                           # plain → symlink
  .gitconfig.tmpl                  # → renders to ~/.gitconfig
home.tag-work/                   # only when hasTag "work"
  .ssh/config.tmpl
home.tag-work.tag-kde/           # AND: both tags must be active
  .config/plasma/some-template.tmpl
```

Use a tag-gated directory when an entire file is conditional on a tag.
For per-line conditionals inside a single rendered file, use
`{{ if hasTag ... }}` within one template — that's what `hasTag` is for.

### Overrides

Two trees can legitimately claim the same `$HOME` target. The
**more-specific tree wins**, where specificity is the number of
required tags on the directory name (bare `home/` is 0, `home.tag-X/`
is 1, `home.tag-X.tag-Y/` is 2):

```
home/.gitconfig               (spec 0) — applies on every host
home.tag-work/.gitconfig.tmpl (spec 1) — overrides on a work host
```

On a work host, the templated `.gitconfig` wins. On every other host,
the plain `home/.gitconfig` symlinks. Works across classes too — a
template in a more-specific tree overrides a plain file in a less-
specific one, and vice versa.

If two trees claim the same target at the **same specificity** (e.g.
`home/.gitconfig` and `home/.gitconfig.tmpl`, or
`home.tag-work/.foo` and `home.tag-personal/.foo` with both tags active),
`hm apply` errors — Homie won't guess which one you meant. Disambiguate
by narrowing one tree (add a tag) or merging the files into a single
template with `{{ if hasTag ... }}`.

`hm doctor` lists tag-gated trees that aren't active on the current
host as informational findings.

Tag names in directory suffixes can't contain `.` — that character is
how Homie splits multiple tags inside one directory name. So
`home.tag-fedora.42/` parses as two segments (`fedora` and `42`), and
since the second one is malformed (`42` doesn't start with `tag-`),
the whole directory is rejected. Use only `[A-Za-z0-9_-]`-style tag
names when naming a tag-gated directory.

---

## Syntax

Templates use Go's [`text/template`](https://pkg.go.dev/text/template)
syntax, augmented with [Sprig](https://masterminds.github.io/sprig/)
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

---

## Data context

Every template has these fields available:

| Field           | Type                | Source |
|-----------------|---------------------|--------|
| `.Name`         | string              | `[user].name` |
| `.Email`        | string              | `[user].email` |
| `.Profile`      | string              | `[profile].name` |
| `.DefaultShell` | string              | `[profile].default_shell` |
| `.Distro`       | string              | detected (`ubuntu`, `debian`, `fedora`, `unknown`) |
| `.Arch`         | string              | detected (`amd64`, `arm64`) |
| `.IsContainer`  | bool                | detected (`/.dockerenv`, cgroup, env) |
| `.IsRoot`       | bool                | `os.Geteuid() == 0` |
| `.Tags`         | `[]string`          | merged auto + profile + extra |
| `.Vars`         | `map[string]any`    | the `[vars]` table |

---

## The `hasTag` helper

Tag membership is the most common branch, so it gets a dedicated
function:

```gotmpl
{{ if hasTag "fedora" }} ... {{ end }}
{{ if hasTag "container" }} ... {{ end }}
```

`hasTag` returns `true` if the name is in `.Tags`. Equivalent to
`has "fedora" .Tags` but easier to read at a glance.

---

## Sprig functions

You get all of Sprig's ~100 helpers. The ones that come up most:

- **`default`** for fallbacks: `{{ default "nvim" .Vars.EDITOR }}` — but
  see the note below about `missingkey=error`.
- **`hasKey`** for optional vars: `{{ if hasKey .Vars "WORK_EMAIL" }}...{{ end }}`.
- **`dig`** for nested fallback: `{{ dig "WORK_EMAIL" "fallback" .Vars }}`.
- **String ops:** `lower`, `upper`, `trim`, `replace`, `contains`.
- **List ops:** `has`, `join`, `sortAlpha`.

Full reference: <https://masterminds.github.io/sprig/>.

### `default` vs `missingkey=error`

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

## Cookbook

### Per-profile block

```gotmpl
{{- if eq .Profile "work" -}}
[includeIf "gitdir:~/work/"]
    path = ~/.config/git/work
{{- end }}
```

### OS-conditional config

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
