---
title: "Homie"
description: "One command to make a fresh Linux or macOS box your own."
---

# One command to make a fresh Linux or macOS box your own.

Homie gives you your own curl bash command to paste into a fresh Linux or macOS install 
that will install all your preferred packages, install your dotfiles, and run setup
scripts to make a blank machine _your_ machine.

Step 1:

```sh
curl -fsSL https://raw.githubusercontent.com/USERNAME/dotfiles/main/bootstrap.sh | bash
```

_There is no step 2_.

I wrote this because I setup new computers frequently, and nothing I could find did everything I wanted. Ansible was too much ceremony for my personal machines. Chezmoi was awkward for dotfiles and awkward for provisioning. Stow was too limited, Nix was too much.

If you try Homie, I think you'll find it's _just right_: you tell it what packages you want installed, and you give it your dotfiles. It makes simple things easy, and complex things possible. Want to templatize your dotfiles? We got you. Need a script to customize your install? Done.

But to get back to the original story: it's a single copy-pasted curl-bash line that's all you need to setup (or update) your system. And it comes from _your own repo_, so there's nobody to trust: the shebang you're running is the shebang you wrote. So give it a whirl, I think you'll like it.

{{< cast name="bootstrap" alt="A fresh machine becomes a working environment with one curl | bash command" >}}

---

## Get started

1. Install `hm`:
   ```sh
   curl -fsSL https://homie.sh/install.sh | bash
   ```
2. `hm init ~/dotfiles` to scaffold a starter repo.
3. Edit `homie.toml`, commit, and push to your preferred git hosting service.
4. On any other Linux or macOS box: `curl …/bootstrap.sh | bash`.

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
all    = ["git", "zsh", "neovim", "tmux", "ripgrep", "fzf"]
fedora = ["util-linux-user", "fd-find"]
ubuntu = ["fd-find"]
arch   = ["fd"]

[vars]
EDITOR = "nvim"
```

```text
dotfiles/             ← your repo (call it whatever you like)
  homie.toml
  bootstrap.sh
  home/               ← files into $HOME — .tmpl renders, the rest symlinks
    .zshrc
    .gitconfig.tmpl
  scripts/            ← ordered setup steps
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

