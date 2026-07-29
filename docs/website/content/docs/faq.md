---
title: "FAQ"
description: "Quick answers to questions skeptics ask first."
weight: 70
---

Short answers to the questions that come up most. For the long version
of the tool comparisons, see [Compare](/docs/compare/).

---

## Why TOML?

Because it's structured enough to validate, simple enough to hand-edit,
and free of YAML's foot-guns (significant whitespace, `no` parsing as
`false`, sexagesimal numbers, the Norway problem). It's also what
chezmoi uses, so anyone arriving from there has zero ramp-up.

## Why symlinks instead of copies?

So that `~/.zshrc` *is* the repo file. Edit it, `git diff` shows the
change, commit, push — no `chezmoi edit` round-trip, no "did I edit the
source or the rendered copy?" confusion.

The tradeoff: deleting the repo clone breaks everything in `$HOME` that
pointed into it. We think that's a fair price for not having to remember
an extra command every time you tweak your shell config.

## Is this just Nix with a worse model?

No. Nix is purely declarative and bit-for-bit reproducible — it builds
the world from a flake. Homie is imperative with idempotency guards —
each step checks before acting, so re-runs converge to a known state,
but Homie never claims reproducibility. If you need reproducibility,
use Nix. If you need a 3 MB binary that boots a Codespace in seconds,
use Homie.

## How is this different from chezmoi?

Three things:
- **Source model.** chezmoi copies dotfiles into
  `~/.local/share/chezmoi`. Homie symlinks them in place.
- **Provisioning.** chezmoi handles dotfiles. Homie handles dotfiles,
  packages, and ordered setup scripts in one pass.
- **Secrets.** chezmoi has rich secret-manager integrations. Homie has
  none — pair it with `sops`, `age`, or `pass` and reference paths from
  scripts.

Full breakdown on [Compare](/docs/compare/).

## How does Homie handle secrets?

It doesn't. Use your favourite secret store (`pass`, `sops`, `age`,
1Password CLI, Bitwarden CLI) to fetch secrets into known paths, then
reference those paths from `scripts/*.sh` or `home/*.tmpl`. See the
["tagged secrets via your password manager"](/docs/recipes/#tagged-secrets-via-your-password-manager)
recipe.

## Does Homie work on macOS?

Yes — macOS is a first-class platform alongside Ubuntu, Debian, Fedora,
and Arch, on both Apple Silicon (arm64) and Intel (amd64). The same
install one-liner works:

```sh
curl -fsSL https://homie.sh/install.sh | bash
```

Native packages install through Homebrew (`brew`), declared under
`[packages].macos` just like any other per-platform key. Dotfiles,
templates, and scripts all behave exactly as they do on Linux.

## Do I need Homebrew?

Only if you declare `[packages]`. macOS ships no system package manager,
so a dotfiles-only setup (no `[packages]`) needs nothing extra — `hm
apply` and `hm doctor` won't complain. If you *do* declare packages and
`brew` isn't installed, the native package phase just warns and skips
(it doesn't fail). To actually install those packages, install Homebrew
yourself, or add a `scripts/pre-*.sh` that installs it before the
package phase runs.

## How do I install GUI / cask apps on macOS?

Add the `/cask` suffix to the app name. A bare name is a Homebrew
formula; a `/cask` suffix is a Homebrew cask (a GUI app):

```toml
[packages]
macos = ["ripgrep", "firefox/cask", "rectangle/cask"]
```

A bad suffix is caught by `hm doctor` before any install runs.

## Does Homie work on Termux (Android)?

Yes. [Termux](https://termux.dev) is detected as the `termux` platform,
and native packages install through `pkg` (Termux's wrapper over its own
apt repos), declared under `[packages].termux`. The bootstrap one-liner
downloads the `linux/arm64` binary — Termux runs native Linux binaries on
the Android kernel — and installs it under `$HOME/.local/bin`, no root
required. Termux is unprivileged with no `sudo`, so package installs never
escalate. Dotfiles, templates, scripts, and externals all resolve against
Termux's `$HOME` exactly as they do elsewhere. Gate Android-only entries
with `hasTag "termux"`.

One caveat if you run a Linux distro under Termux with `proot-distro`:
Homie detects Termux from the `TERMUX_VERSION` variable, and Termux
exports it into the proot guest's environment. So a `proot-distro login
ubuntu` session is still detected as `termux` (installing through `pkg`)
rather than as `ubuntu` (installing through `apt`). Run Homie in native
Termux, or `unset TERMUX_VERSION` before `hm apply` inside the guest to
have it detected as the guest distro.

## Why no Windows support?

Scope. v1 covers Linux and macOS — workstations, servers, CI,
Codespaces, devcontainers, and the box your USB stick boots into.
Windows isn't ruled out forever; it's ruled out for v1 so we ship the
Linux and macOS story cleanly first.

## My distro isn't Ubuntu, Debian, Fedora, or Arch. Now what?

`hm apply` will detect your distro as `unknown` and print a friendly
notice with a link to the [contributing guide](/docs/contributing/).
You can still use Homie — dotfiles, templates, and scripts all work —
but the package install phase becomes a no-op. Adding distro support is
a small, well-isolated change; PRs welcome.

This includes derivatives of a supported distro — Mint, Pop!\_OS,
Manjaro, EndeavourOS. Only the exact `ID=` in `/etc/os-release` is
matched, never `ID_LIKE`, because a derivative is free to rename or drop
packages its parent ships and a silent guess would fail at install time
instead of detection time.

## Does Homie install from the AUR?

No. `[packages]` on Arch covers the official repos via `pacman`. The AUR
needs a helper (`yay`, `paru`) that isn't part of a base install, and
building from source is a different proposition from fetching a signed
package — so it stays an explicit choice you make in a `scripts/` step:

```sh
# scripts/20-aur.sh
command -v paru >/dev/null || {
  sudo pacman -S --needed --noconfirm base-devel git
  rm -rf /tmp/paru   # a half-finished earlier run would fail the clone
  git clone https://aur.archlinux.org/paru-bin.git /tmp/paru && \
    (cd /tmp/paru && makepkg -si --noconfirm)
}
paru -S --needed --noconfirm my-aur-package
```

**This one can't run as root.** `makepkg` refuses to, by design — so unlike
the rest of `hm apply`, an AUR step only works on the run-as-your-user path,
not the fresh-bare-metal-as-root one. It sudoes for the `pacman` calls it
needs and no further.

Homie also never runs `pacman -Sy` or `-Syu` for you: refreshing the
database and then installing is the documented partial-upgrade footgun,
and upgrading your whole system isn't a decision `hm apply` should make.
If a package can't be found, Homie says so and suggests `pacman -Syu`.

## `sudo` says "a terminal is required" when I pipe `bootstrap.sh` into bash

Your `bootstrap.sh` predates Homie v0.5.1. Under `curl … | bash`, stdin
is the pipe carrying the script rather than your terminal, so the first
`sudo` in a setup script has nowhere to prompt — and because scripts run
under `set -euo pipefail`, everything downstream of it (including the
package phase) can go with it.

Bootstrap scripts generated by v0.5.1 and later hand the terminal to
their children. Refresh yours in place:

```sh
cd ~/dotfiles
hm init --update
git diff              # review
git commit -am "chore: refresh bootstrap.sh"
```

See [How do I refresh a generated file?](#how-do-i-refresh-a-generated-file)
for what `--update` will and won't touch.

## How do I refresh a generated file?

```sh
cd ~/dotfiles && hm init --update
```

Most of what `hm init` writes is a *seed*: `homie.toml`, `home/`,
`scripts/` are yours the moment they exist, and Homie never touches them
again. `bootstrap.sh` is the exception — it encodes how the current `hm`
wants to be launched (which release URL, which os/arch names, how stdin
reaches `sudo`), so it goes stale when you upgrade. `--update`
re-renders it in place. It takes no answers: identity comes from your
`homie.toml`, the GitHub user and repo from your `origin` remote. Pass
`--github-user` / `--github-repo` if there's no remote to read.

Generated files carry a stamp recording the `hm` version that wrote them
and a digest of the file as written:

```sh
# hm:generated version=v0.5.2 sha256=7b7bdbc4…
```

`--update` rewrites a file only when that digest still matches — i.e.
nobody has edited it since. If you *have* edited it, or it predates
stamps (every repo scaffolded before v0.5.2), update prints the diff and
stops:

```
  bootstrap.sh   skipped   no hm:generated stamp — treating it as yours
```

From there: `--force` takes Homie's version, or delete the
`hm:generated` line to opt the file out permanently. Either way nothing
leaves your working tree, so `git diff` is the final say.

Running `--update` when everything is current is a no-op, so it's safe
to run after every `hm selfupdate`.

## Can I share one Homie repo across multiple users on the same box?

No. Homie is single-user — every path is rooted at `$HOME`, every config
is per-user. If you need fleet management for a shared host, use
Ansible.

## Why is there no state file?

Because every check Homie performs is cheap:

- Symlink: `readlink` the destination, compare strings.
- Package: `dpkg -s` / `rpm -q`.
- Template: render to memory, compare bytes to existing file.

State files create their own class of bugs — drift between the file and
reality, lock contention, corruption after a crash. Going stateless
trades a tiny amount of work-per-run for a *lot* less to go wrong.

## Why is the binary called `hm`?

Short, easy to type, doesn't collide with anything common. The longer
form would be `homie`; we shipped the short one because you'll type it
many times a week.
