# Homie

A single-binary Linux environment manager. Dotfiles by symlink, packages by
your distro, ordered setup scripts — all driven by one `homie.toml` in a git
repo you own.

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

v0.0.2. All MVP milestones complete; install → bootstrap → apply →
idempotent reapply verified end-to-end on Ubuntu, Debian, and Fedora.
User-facing docs at <https://homie.sh>; design brief in
[`CLAUDE.md`](./CLAUDE.md).

## License

MIT
