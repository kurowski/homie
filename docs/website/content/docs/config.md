---
title: "homie.toml"
description: "The single file that drives everything Homie does."
weight: 30
---

`homie.toml` lives at the root of your user environment repo. It is the
only configuration Homie reads. Everything else — dotfiles, templates,
scripts — is on disk, where you can see and version it.

Minimal valid file:

```toml
[user]
name  = "Scout Homes"
email = "scout@homie.sh"
```

That's it. Every other table is optional. The sections below describe
each one in order.

---

## `[user]`

Identity. **Both fields are required.**

```toml
[user]
name  = "Scout Homes"
email = "scout@homie.sh"
```

Templates see these as `{{ .Name }}` and `{{ .Email }}`. Scripts see
neither — pass identity in via `[vars]` if scripts need it.

---

## `[profile]`

Selects which kind of machine you're configuring. Affects rendering and
tag membership; nothing else.

```toml
[profile]
name          = "personal"   # personal | work | devcontainer | ...
default_shell = "zsh"
```

`profile.name` becomes an active tag automatically — so a template can
branch on `{{ if hasTag "work" }}` and a script can read `$HM_TAGS`.

Both fields default to empty if omitted. Convention is `personal`,
`work`, `devcontainer`, or whatever short label distinguishes the
machines you actually use.

---

## `[packages]`

Native packages to install via the detected package manager (`apt` on
Ubuntu/Debian, `dnf` on Fedora). Idempotent — each package is checked
with `dpkg -s` / `rpm -q` before install.

```toml
[packages]
all    = ["git", "zsh", "neovim", "tmux", "ripgrep", "fd", "fzf"]
fedora = ["util-linux-user"]
ubuntu = ["fd-find"]
debian = ["fd-find"]
```

`all` runs on every distro. Per-distro keys (`fedora`, `ubuntu`,
`debian`) merge on top — useful for the rename-on-this-distro case
(`fd` vs `fd-find`) or for distro-specific tools.

On unsupported distros, the package phase prints a friendly notice and
skips. The rest of `hm apply` continues normally.

### Tag-keyed package lists

Sub-tables of the form `[packages."tag:<name>"]` contribute only when
the matching tag is active for the current host. Useful when a work
laptop and a personal laptop share a base set but each needs its own
extras.

```toml
[packages]
fedora = ["git", "zsh", "neovim"]            # base, always

[packages."tag:work"]
fedora = ["kubectl", "helm", "terraform"]

[packages."tag:personal"]
fedora = ["steam", "tailscale"]
```

Resolution: the final install set is `[packages]` plus every
`[packages."tag:X"]` sub-table where `X` is in the active tag set
(auto-detected, profile-derived, or `[tags].extra`). Each sub-table
honors the same per-distro split as the base — `[packages."tag:work"].fedora`
and `[packages."tag:work"].ubuntu` both work.

Order is deterministic: base `all`, base `<distro>`, then each matching
tag in alphabetical tag-name order (its `all`, then its `<distro>`).
Duplicates across these sources are removed on insertion, so a package
named in both base and a tag sub-table installs exactly once. Tags with
no matching sub-table contribute nothing — they aren't an error.

---

## `[tags]`

User-defined tags layered on top of the auto-detected ones. Tags are how
templates and scripts branch on machine type without hard-coding distro
checks.

```toml
[tags]
extra = ["laptop"]
```

Active tags on every run are the union of:

- **Detected:** the distro (`ubuntu`, `debian`, `fedora`), the arch
  (`amd64`, `arm64`), the short hostname (so `hasTag "coach"` works with
  no config), plus `container` and `root` when those apply.
- **Profile:** `profile.name`, if set.
- **Extra:** everything in `tags.extra`.

Duplicates are deduped; the resulting list is sorted, exposed to
templates as `{{ .Tags }}`, and to scripts as `$HM_TAGS` (space-joined).

---

## `[vars]`

Free-form string key/value pairs. Use these for anything Homie's core
schema doesn't cover.

```toml
[vars]
WORK_EMAIL = "scout@work.example.com"
EDITOR     = "nvim"
DOTFILES   = "https://github.com/scouthomes/dotfiles"
```

Vars are exposed two ways:

- **In templates** as `{{ .Vars.WORK_EMAIL }}`. To make a var optional,
  use `{{ if hasKey .Vars "X" }}{{ .Vars.X }}{{ end }}` — `missingkey=error`
  applies, so referencing an undefined var fails the render.
- **In scripts** as environment variables: `$WORK_EMAIL`, `$EDITOR`,
  etc., exported into every `scripts/*.sh` subprocess.

Keys are case-sensitive. Convention is `UPPER_SNAKE` since they
double as shell env vars.

---

## What `hm init` writes

A fresh `hm init` produces something like:

```toml
[user]
name  = "Scout Homes"
email = "scout@homie.sh"

[profile]
name          = "personal"
default_shell = "zsh"

[packages]
all = ["git", "zsh", "neovim", "tmux", "ripgrep", "fd", "fzf"]

[vars]
EDITOR = "nvim"
```

From there, add per-distro overrides, tags, and vars as your environment
demands. The schema is intentionally small — anything more dynamic
belongs in `scripts/`.

---

## Per-host overlay

When the same repo serves multiple machines, ship a `hosts/<short-hostname>.toml`
alongside `homie.toml`. If the file matching the current host exists, it's
deep-merged onto the base at load time:

```
dotfiles/
  homie.toml              # base, applies everywhere
  hosts/
    coach.toml            # profile=personal + laptop packages
    uceap-dev01.toml      # profile=work + work vars and packages
```

Merge rules:

- **`[user]` and `[profile]` scalars** in the overlay replace the base
  when set non-empty.
- **`[packages].*` arrays** append to the base (overlap is deduped, order
  preserved).
- **`[tags].extra`** appends.
- **`[vars]`** override per-key (base keys not mentioned by the overlay
  survive).

The hostname used for the lookup is the short form — everything before
the first dot — so `coach.lan` matches `hosts/coach.toml`. If
`os.Hostname()` fails or returns something that looks unsafe (a path
separator), no overlay is loaded and `hm doctor` surfaces a warning.

Validation runs *after* the merge, so an overlay can legitimately fill
in required `[user]` fields if you'd rather not commit them to the base.

---

## Unknown fields

Unknown TOML keys are recorded as warnings, not errors. This lets you
add new fields for a newer `hm` binary without breaking older clients on
the same repo. Run `hm status` to see warnings without applying.

Required-field violations (missing `user.name` or `user.email`) are
hard errors — `hm apply` refuses to proceed.
