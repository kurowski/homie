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
reference those paths from `scripts/*.sh` or `templates/*.tmpl`. See the
["tagged secrets via your password manager"](/docs/recipes/#tagged-secrets-via-your-password-manager)
recipe.

## Why no Mac or Windows support?

Scope. v1 is Linux-only — that covers workstations, servers, CI,
Codespaces, devcontainers, and the box your USB stick boots into. macOS
and Windows aren't ruled out forever; they're ruled out for v1 so we
ship the Linux story cleanly first.

## My distro isn't Ubuntu, Debian, or Fedora. Now what?

`hm apply` will detect your distro as `unknown` and print a friendly
notice with a link to the [contributing guide](/docs/contributing/).
You can still use Homie — dotfiles, templates, and scripts all work —
but the package install phase becomes a no-op. Adding distro support is
a small, well-isolated change; PRs welcome.

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
