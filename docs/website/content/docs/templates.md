---
title: "Templates"
description: "Go text/template + Sprig + hasTag, with a cookbook of patterns."
weight: 40
---

Files under `templates/` get rendered into `$HOME` on every `hm apply`.
A file named `templates/.gitconfig.tmpl` writes to `~/.gitconfig`. The
`.tmpl` suffix is stripped; everything else maps one-to-one.

Rendered files are **real files**, not symlinks — the template is the
source of truth, the output is the artifact. Source file mode is
preserved, so `templates/bin/foo.sh.tmpl` renders executable.

### Tag-gated template trees

Sibling directories named `templates.tag-<X>[.tag-<Y>...]` are processed
only when every named tag is active on the host. Plain `templates/`
always applies. The dotfiles tree uses the same convention
(`dotfiles.tag-<X>...`), so the layout is symmetric:

```
templates/                       # always
  .gitconfig.tmpl
templates.tag-work/              # only when hasTag "work"
  .ssh/config.tmpl
templates.tag-work.tag-kde/      # AND: both tags must be active
  .config/plasma/some-template.tmpl
```

Use this when an entire file is conditional on a tag. For per-line
conditionals inside a single rendered file, use `{{ if hasTag ... }}`
within one template — that's what `hasTag` is for. If two trees produce
the same output path, `hm apply` errors out so the conflict surfaces.
`hm doctor` lists tag-gated trees that aren't active on the current
host as informational findings.

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

`templates/bin/sync-secrets.sh.tmpl`:

```gotmpl
#!/usr/bin/env bash
set -euo pipefail

REMOTE={{ .Vars.SECRETS_REMOTE | quote }}
mkdir -p "$HOME/.secrets"
rsync -a --delete "$REMOTE/" "$HOME/.secrets/"
```

The source file's executable bit carries through, so the rendered
`~/bin/sync-secrets.sh` is runnable.
