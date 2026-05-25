---
title: "Homie"
description: "Make every Linux box feel like home."
---

# Make every Linux box feel like home.

Homie is a single binary that turns a fresh Linux install into _your_
Linux install — dotfiles, packages, setup scripts, all from one repo
you own. One command from a bare box to a working environment:

```sh
curl https://raw.githubusercontent.com/YOU/dotfiles/main/bootstrap.sh | bash
```

That's it! That's the whole setup story.

---

## Get started

1. Install `hm`:
   ```sh
   curl -fsSL https://homie.sh/install.sh | bash
   ```
2. `hm init ~/dotfiles` to scaffold a starter repo.
3. Edit `homie.toml`, commit, and push to your preferred git hosting service.
4. On any other Linux box: `curl …/bootstrap.sh | bash`.

[Read the quickstart →](/docs/quickstart/)

## What you get

- **Symlinks, not copies.** Edit `~/.zshrc` and you're editing the file in your repo. `git diff` shows what changed. No `chezmoi edit` indirection.
- **One repo, three jobs.** Dotfiles + system packages + setup scripts, declared in one `homie.toml`. No glue between separate tools.
- **No state file.** Every `hm apply` is a full reconciliation. Idempotent by construction — re-running is always safe.
- **Static binary.** No Python, no Ruby, no daemon. ~3 MB, single file.
- **Charm-powered TUI.** Spinners, progress, a friendly summary at the end. Plain output in CI.

## How it looks

```toml
# homie.toml
[user]
name  = "Scout Homes"
email = "scout@homie.sh"

[profile]
name          = "personal"
default_shell = "zsh"

[packages]
all    = ["git", "zsh", "neovim", "tmux", "ripgrep", "fd", "fzf"]
fedora = ["util-linux-user"]
ubuntu = ["fd-find"]

[vars]
EDITOR = "nvim"
```

```text
dotfiles/
  homie.toml
  bootstrap.sh
  dotfiles/        ← symlinked into $HOME
    .zshrc
    .gitconfig
  templates/       ← rendered into $HOME with var-sub + conditionals
    .gitconfig.tmpl
  scripts/         ← ordered setup steps
    01-shell.sh
    02-tools.sh
```

## Why not just...?

| | Homie | chezmoi | Ansible | Stow | Nix |
|---|---|---|---|---|---|
| Dotfile model | 🔗 symlink | 📋 copy + indirection | 📋 copy / template | 🔗 symlink | ❄️ declarative store |
| Provisioning | ✅ | ❌ | ✅ | ❌ | ✅ |
| State file | ❌ | ✅ | ❌ | ❌ | ✅ |
| Runtime | ⚡ Native | ⚡ Native | 🐍 Python | 🐪 Perl | ❄️ Nix |
| Weight | 🐁 tiny | 🐕 medium | 🐘 heavy | 🐁 tiny | 🐘 heavy |

[See the full comparison →](/docs/compare/)

