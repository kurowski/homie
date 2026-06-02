# syntax=docker/dockerfile:1
#
# Recording image for homie screencasts.
#
# Base = the official VHS image (Debian 13), pinned by digest so a local
# `record.sh` renders byte-for-byte like CI. It already bundles the whole
# render stack: ttyd + ffmpeg + headless chromium (/usr/bin/chromium, run with
# VHS_NO_SANDBOX) + JetBrainsMono Nerd Font.
#
# On top we add just enough to be a believable personal Linux laptop that runs
# the real curl|bash bootstrap:
#   - curl / ca-certificates / git / sudo — fetch, clone, and (no-op) bootstrap
#   - the packages the demo repo declares, pre-installed so `hm bootstrap` and
#     `hm apply`'s package phase are offline no-ops at render time. A genuinely
#     fresh box installs these on first run; pre-baking just keeps the looping
#     hero short and deterministic.
#   - the committed e2e test CA, so the in-cast HTTPS to the nginx sidecar
#     (which impersonates github.com / raw.githubusercontent.com) validates.
#   - a non-root user `scout` with a personal prompt: the homepage one-liner is
#     `curl … | bash` (no sudo), so scout@laptop + ~/dotfiles reads as "your
#     laptop", not a server. chromium is system-wide, so scout uses it directly
#     — no per-user warm step needed.
#
# Build context is e2e/certs (that's all we COPY); see record.sh.
FROM ghcr.io/charmbracelet/vhs@sha256:9d5fc3dc0c160b0fb1d2212baff07e6bdf3fa9438c504a3237484567302fcf93

RUN apt-get update \
 && apt-get install -y --no-install-recommends \
      curl ca-certificates git sudo \
      zsh neovim tmux ripgrep fd-find fzf \
 && rm -rf /var/lib/apt/lists/*

COPY ca.crt /usr/local/share/ca-certificates/homie-cast.crt
RUN update-ca-certificates

RUN useradd -m -s /bin/bash scout \
 && echo 'scout ALL=(ALL) NOPASSWD: ALL' > /etc/sudoers.d/scout

# Personal prompt. VHS launches bash WITHOUT rc files (so .bashrc is ignored)
# and mangles non-ASCII typed input — so stash the ❯ prompt in a file and let
# the tape source it with a pure-ASCII line. The glyph never touches VHS's
# keystroke path.
RUN cat > /etc/homie-ps1.sh <<'EOF'
PS1='\[\e[36m\]scout@laptop\[\e[0m\] \[\e[35m\]❯\[\e[0m\] '
EOF

USER scout
ENV HOME=/home/scout USER=scout
WORKDIR /home/scout
