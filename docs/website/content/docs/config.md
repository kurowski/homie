---
title: "homie.toml"
description: "The single file that drives everything Homie does."
weight: 30
---

`homie.toml` lives at the root of your user environment repo. It is the
only configuration Homie reads. Everything else — your `home/` tree and
`scripts/` — is on disk, where you can see and version it.

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
Ubuntu/Debian, `dnf` on Fedora, `brew` on macOS). Idempotent — each
package is checked with `dpkg -s` / `rpm -q` / `brew list` before install.

```toml
[packages]
all    = ["git", "zsh", "neovim", "tmux", "ripgrep", "fd", "fzf"]
fedora = ["util-linux-user"]
ubuntu = ["fd-find"]
debian = ["fd-find"]
macos  = ["coreutils", "firefox/cask"]
```

`all` runs on every platform. Per-platform keys (`fedora`, `ubuntu`,
`debian`, `macos`) merge on top — useful for the rename-on-this-platform
case (`fd` vs `fd-find`) or for platform-specific tools.

On macOS, native packages install through Homebrew. A GUI app (a Homebrew
**cask**) is named with a `/cask` suffix — `firefox/cask` installs with
`brew install --cask firefox`; a bare name is a formula. A typo'd suffix
is reported by `hm doctor` before any install runs.

**brew is optional.** macOS ships no system package manager, so if you only
manage dotfiles (no `[packages]`), you never need it — `hm apply` and
`hm doctor` won't complain. If brew isn't on `PATH` when packages *are*
declared, the native phase warns and skips instead of failing; install brew
(or add a `scripts/pre-*.sh` that does) to have those packages applied.

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
block in alphabetical key order (its `all`, then its `<distro>`).
Duplicates across these sources are removed on insertion, so a package
named in both base and a tag sub-table installs exactly once. Tags with
no matching sub-table contribute nothing — they aren't an error.

#### Requiring several tags (AND)

Chain `tag:` segments with `.` to require **all** of them — the same
`.`-delimited convention the `home.tag-X.tag-Y/` and `scripts.tag-X.tag-Y/`
trees use:

```toml
# snap is Ubuntu-only and AWS is a personal-machine thing:
[packages."tag:personal.tag:ubuntu".snap]
all = ["aws-cli/classic"]

# desktop apps only on personal desktops:
[packages."tag:personal.tag:desktop".snap]
all = ["gimp", "spotify"]

[packages."tag:work.tag:ubuntu".flatpak]
all = ["us.zoom.Zoom"]
```

A chained block applies only when every listed tag is active. Tag order
doesn't matter (`tag:personal.tag:ubuntu` and `tag:ubuntu.tag:personal`
are the same block). Single-tag `[packages."tag:X"]` is just the one-tag
form of the same rule. Nested backends (`.snap`, `.flatpak`, `.brew`)
work under a chained key exactly as under a single-tag one.

A malformed key — a segment that isn't `tag:<name>`, an empty `tag:`, a
trailing `.` — is a hard error at load, not a silent no-op. `hm doctor`
lists which AND-blocks were active for the current host.

---

## `[externals]`

External git repos to keep on disk — zsh/tmux/nvim plugins, themes,
editor distributions. Each entry is keyed by its destination path;
`hm apply` clones it when missing and updates it in place when present,
replacing the hand-rolled clone-or-pull script this usually takes.

```toml
[externals."~/.zsh/plugins/zsh-autosuggestions"]
repo = "https://github.com/zsh-users/zsh-autosuggestions"
ref  = "v0.7.1"   # pinned: checked out and held; never auto-moves

[externals."~/.tmux/plugins/tpm"]
repo = "https://github.com/tmux-plugins/tpm"
# no ref: track the remote default branch, fast-forward on each apply

# tag-gated, exactly like [packages."tag:X"] (AND across tags)
[externals."tag:desktop"."~/.config/some-theme"]
repo = "https://github.com/example/some-theme"
```

- **`repo`** (required) — the clone URL.
- **`ref`** (optional) — a branch, tag, or commit to pin. A pinned
  checkout is detached at the ref and held there until you change the
  value. **Prefer pinning**: an unpinned plugin that follows upstream
  `HEAD` on every apply can break the shell you'd use to debug it.
- **No `ref`** — track the remote default branch. Each apply
  fast-forwards; a checkout with local commits or edits fails the
  fast-forward and surfaces as a phase error instead of being clobbered.

Destinations must start with `~/` or `$HOME/` or be absolute. The same
no-surprises rules as the rest of Homie apply: a destination that exists
but isn't a git checkout is an error (your data is never overwritten),
and so is a checkout whose `origin` doesn't match `repo`.

When two entries claim the same destination, the one requiring more
tags wins (a plain entry counts as zero) — the same more-specific-wins
rule as the `home/` trees. Two active entries at equal specificity with
different settings are an error. In a [per-host overlay](#per-host-overlay),
an entry replaces the base entry for the same destination outright —
handy for pinning a different `ref` on one machine.

Keep externals destinations out of the directories `home/` manages:
the home phase owns those paths and the two will fight over them.

Skip the phase with `hm apply --skip-externals`.

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

- **Detected:** the platform (`ubuntu`, `debian`, `fedora`, `macos`), the
  arch (`amd64`, `arm64`), the short hostname (so `hasTag "coach"` works
  with no config), plus `container` and `root` when those apply.
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

### Non-native backends

Beyond the native package manager, `[packages]` accepts sub-tables for
non-native managers. v1 ships `flatpak`, `brew`, and `snap`; the namespace
is reserved for `cargo`, `npm`, `pip`, etc. to follow.

`[packages.brew]` is Homebrew **as a Linux backend** — handy on immutable
distros (Universal Blue, Bazzite) where dnf is discouraged. On macOS, brew
is the *native* manager instead, so list those packages under
`[packages].macos`, not here.

```toml
[packages.flatpak]
all = ["md.obsidian.Obsidian"]
fedora = ["org.localsend.localsend_app"]

[packages.brew]
all = ["fd", "ripgrep", "bat"]

[packages.snap]
all = ["gimp", "spotify"]
```

Each backend mirrors the base shape — `all`, distro keys, and tag-keyed
sub-tables — and follows the same resolution and dedup rules. Combined
with tag-keyed packages:

```toml
[packages."tag:work".flatpak]
all = ["us.zoom.Zoom"]
```

Backends are **opt-in by tool presence**. If the backend's CLI tool
isn't on PATH, `hm apply` logs a warning and skips that phase — it
doesn't fail. Setting up a flatpak remote or installing brew belongs in
`scripts/pre-*.sh` so it runs before the backend's install step.

The Flatpak backend installs from the `flathub` remote. References from
`flathub-beta`, GNOME nightly, or a custom remote aren't supported by
`[packages.flatpak]`; install those via `scripts/*.sh`.

The Snap backend installs with `snap install`. Snaps that need
unconfined (classic) confinement — common for developer tools like the
AWS CLI or editors — carry a `/classic` suffix on the package name;
`/devmode` and `/jailmode` work the same way. A bare name installs under
default (strict) confinement.

```toml
[packages.snap]
all = ["gimp"]                 # strict confinement

[packages."tag:work".snap]
all = ["aws-cli/classic", "code/classic"]
```

An unrecognized suffix (e.g. `foo/bogus`) is a hard error. The suffix
only expresses confinement — non-default channels or tracks (`--channel`,
`--channel=22/stable`) aren't expressible here; install those from a
`scripts/*.sh`. Installing `snapd` itself, or removing a conflicting
distro package first, also belongs in `scripts/pre-*.sh`.

Unknown backend names (a typo, or one that doesn't exist yet) decode
with a warning rather than hard-failing the load — `hm doctor` and
`hm apply` surface them so the file stays forward-compatible with
newer `hm` binaries.

The `hm apply` lifecycle becomes:
`detect → pre-scripts → packages → backends → link → render → scripts`,
where "backends" iterates whatever non-native backends you declared,
in alphabetical order. Backends run after native packages so a brew or
flatpak installed by `[packages]` is available before its own phase
fires.

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
