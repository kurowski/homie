# zshrc — symlinked into $HOME by `hm link`.
# Edit this file directly; the symlink at ~/.zshrc tracks it.

export EDITOR=nvim
export PATH="$HOME/.local/bin:$PATH"

# A few sensible defaults — replace with your own.
setopt AUTO_CD
setopt HIST_IGNORE_DUPS
HISTSIZE=10000
SAVEHIST=10000
HISTFILE=~/.zsh_history
