# Homie

A single-binary Linux & macOS environment manager. Dotfiles by symlink,
packages via your distro's manager or Homebrew, ordered setup scripts — all
driven by one `homie.toml` in a git repo you own.

```sh
# Bootstrap any machine
curl https://raw.githubusercontent.com/<you>/<your-dotfiles>/main/bootstrap.sh | bash
```

Documentation: <https://homie.sh>

## Why?

Existing tools each force a tradeoff:

- **chezmoi** copies dotfiles through an indirection layer — you edit files in
  `~/.local/share/chezmoi`, not where they live.
- **Ansible** handles provisioning well but is heavy and fleet-oriented.
- **Stow** does symlinks well and nothing else.
- **Nix / Home Manager** does everything but is too heavy for ephemeral
  environments like Codespaces and devcontainers.

Homie does it all in one, from one repo, with no daily friction: editing
`~/.zshrc` edits the repo file directly.

## Status

v0.4.0. Supported platforms: Ubuntu, Debian, Fedora, and macOS (Apple
Silicon & Intel). Dotfiles (symlinks + Go-template files), per-host overlays
and tag-conditional `home/` and `scripts/` trees, native packages (apt/dnf, or
Homebrew formulae + casks on macOS) plus flatpak and snap backends, declarative
pinned git clones (`[externals]` — plugins, themes, editor distros), and
ordered pre/post setup scripts — install → bootstrap → apply → idempotent
reapply verified end-to-end. Template previews (`hm render`, `hm home
--dry-run`) and machine-readable host state (`hm status --json`, `hm doctor
--json`, `hm context`) give scripts and AI agents a clean interface.
User-facing docs at <https://homie.sh>; design brief in
[`CLAUDE.md`](./CLAUDE.md).

## License

MIT
