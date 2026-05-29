---
title: "Compare"
description: "Homie vs chezmoi, Ansible, Stow, and Nix."
weight: 50
---

The honest version of the matrix on the homepage. Each of these tools
is good at what it's designed for. Pick Homie when the tradeoffs below
match how you actually work.

Homie covers Linux *and* macOS from one tool and one repo, so the same
dotfiles, templates, and provisioning work across both. That's worth
keeping in mind below: on macOS you might otherwise reach for
`brew bundle` (packages only — no dotfiles, templating, or multi-host
overlays) or Nix / Home Manager (more powerful, but heavier).

---

## vs chezmoi

The closest tool. Both are single Go binaries, both use Go
`text/template` + Sprig, both target the same "dotfiles for one
developer across many machines" problem.

**Where they differ:**

- **Source model.** chezmoi copies your dotfiles into
  `~/.local/share/chezmoi`, then renders into `$HOME`. Editing
  `~/.zshrc` doesn't edit your repo — `chezmoi edit` does. Homie uses
  symlinks: `~/.zshrc` *is* the repo file. `git diff` shows what
  changed, no extra step.
- **Provisioning.** chezmoi handles dotfiles. System packages and
  one-shot setup are left to scripts you wire up yourself. Homie does
  packages and ordered scripts in the same `apply` pass.
- **Secrets.** chezmoi has rich integrations (age, 1Password, Bitwarden,
  keepassxc, AWS Parameter Store). Homie has none — secrets are out of
  scope for v1.

**Pick chezmoi if** you want best-in-class secret-manager integration,
or if you specifically prefer the copy-and-render model (it survives
deleting the repo clone in a way symlinks don't).

**Pick Homie if** you want your dotfiles editable in place and your
package install + setup scripts in the same tool.

---

## vs Ansible

Ansible is a configuration management tool for fleets of machines, run
from a control node over SSH. It's powerful, mature, and totally
overkill for one developer's laptop.

**Where they differ:**

- **Scope.** Ansible plays target groups of remote hosts. Homie targets
  the machine it's running on.
- **Runtime.** Ansible needs Python on the control node and on every
  target. Homie is a single static binary.
- **Dotfiles.** Ansible has no first-class dotfile model — you cobble
  one together with `template`, `copy`, and `file` modules.
- **Learning curve.** YAML + Jinja + inventory + roles + playbooks vs.
  one TOML file and three directories.

**Pick Ansible if** you're configuring more than one machine you don't
own (servers, fleets, CI builders).

**Pick Homie if** the answer is "my workstation, my laptop, and
whatever Codespace I'm in this week."

---

## vs GNU Stow

Stow is the original dotfile symlinker. It does one thing, does it
well, and is in every distro's package repo.

**Where they differ:**

- **Provisioning.** Stow only symlinks. Packages and setup scripts are
  on you.
- **Templating.** Stow has none. If you need a `.gitconfig` that uses
  `WORK_EMAIL` on the work laptop and `HOME_EMAIL` everywhere else, Stow
  asks you to maintain two files.
- **Conflict handling.** Stow refuses to overwrite. Homie backs up the
  existing file to `<path>.homie-backup-<timestamp>` and links anyway.

**Pick Stow if** symlinks are *all* you want and you're allergic to new
dependencies.

**Pick Homie if** you also want templating and provisioning in the same
pass.

---

## vs Nix / Home Manager

Nix is the maximalist answer: a purely functional package manager with
a declarative configuration model. Home Manager applies the same
philosophy to user environments.

**Where they differ:**

- **Model.** Nix is declarative — you describe the desired state and
  Nix realises it. Homie is imperative with idempotency guards — each
  step checks before acting, but the steps run in order.
- **Reproducibility.** Nix is bit-for-bit reproducible. Homie is
  "best-effort idempotent" — packages can drift with upstream repos.
- **Weight.** Nix needs `/nix/store`, the Nix daemon, and a substantial
  initial install. Homie is a 3 MB static binary.
- **Learning curve.** Nix language + flakes + Home Manager modules vs.
  TOML + Go templates.
- **Ephemeral environments.** Codespaces and devcontainers start fast
  with Homie. Bootstrapping Nix from scratch is slow enough to be
  painful in short-lived environments.

**Pick Nix if** you want true reproducibility, atomic rollback, or
declarative purity is non-negotiable.

**Pick Homie if** "imperative with idempotency" is good enough and you
value boot time, binary size, and a small mental model.

---

## vs `brew bundle` / Brewfile

A macOS-only option. A `Brewfile` lists formulae, casks, and Mac App
Store apps, and `brew bundle` installs them.

**Where they differ:**

- **Scope.** `brew bundle` installs packages. It has no dotfile model,
  no templating, and no per-host overrides. Homie does packages,
  dotfiles, templates, and ordered scripts in one pass.
- **Platform.** `brew bundle` is macOS (and Linuxbrew) only. Homie
  manages the same repo across Linux and macOS, with native packages on
  each (apt/dnf on Linux, `brew` on macOS — formulae by bare name, casks
  with a `/cask` suffix).

**Pick `brew bundle` if** all you want is a reproducible package list on
a single Mac.

**Pick Homie if** you also want your dotfiles, templates, and setup
scripts managed alongside those packages, on Linux as well as macOS.

---

## When Homie is the wrong answer

- **Multiple users on one machine.** Homie is single-user, configured
  per user. There's no fleet mode.
- **Windows.** v1 supports Linux and macOS, not Windows.
- **Secrets management.** Out of scope. Pair Homie with `sops`, `age`,
  `pass`, or a cloud secret store and reference paths from your scripts.
- **You need rollback.** Homie has no state, so it can't roll back.
  Recovery is "edit your repo, `hm apply` again."
