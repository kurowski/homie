# {{ .Name }}'s environment

Managed by [Homie](https://homie.sh).

## Bootstrap on a fresh machine

```sh
curl -fsSL https://raw.githubusercontent.com/{{ .GitHubUser }}/{{ .GitHubRepo }}/main/bootstrap.sh | bash
```

Downloads `hm`, clones this repo into `~/{{ .GitHubRepo }}`, and runs `hm apply`.

## Daily use

Edit files in `dotfiles/` directly — they're symlinked into `$HOME`, no
indirection layer. Commit and push when you're happy.

```sh
hm apply     # full reconciliation
hm status    # show what would change, no writes
hm doctor    # check for broken symlinks and other drift
```

## Layout

| Path          | Purpose                                                   |
|---------------|-----------------------------------------------------------|
| `homie.toml`  | Identity, profile, packages, tags, vars                   |
| `dotfiles/`   | Files symlinked into `$HOME`                              |
| `templates/`  | Files rendered into `$HOME` via Go text/template + Sprig  |
| `scripts/`    | Ordered setup scripts (`scripts/*.sh`)                    |
| `bootstrap.sh`| Curl-bash entrypoint that fetches `hm` and runs `apply`   |
