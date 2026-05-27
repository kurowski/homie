# {{ .Name }}'s environment

Managed by [Homie](https://homie.sh).

## Bootstrap on a fresh machine

```sh
curl -fsSL https://raw.githubusercontent.com/{{ .GitHubUser }}/{{ .GitHubRepo }}/main/bootstrap.sh | bash
```

Downloads `hm`, clones this repo into `~/{{ .GitHubRepo }}`, and runs `hm apply`.

## Daily use

Edit files in `home/` directly — plain files are symlinked into `$HOME`
with no indirection layer; files ending in `.tmpl` are rendered through
Go `text/template` + Sprig. Commit and push when you're happy.

```sh
hm apply     # full reconciliation
hm status    # show what would change, no writes
hm doctor    # check for broken symlinks and other drift
```

## Layout

| Path              | Purpose                                                       |
|-------------------|---------------------------------------------------------------|
| `homie.toml`      | Identity, profile, packages, tags, vars                       |
| `home/`           | Files into `$HOME` — `.tmpl` is rendered, everything else is symlinked |
| `home.tag-X/`     | Same, but only on hosts where tag X is active                 |
| `hosts/<name>.toml` | Per-host overlay merged onto `homie.toml`                   |
| `scripts/`        | Ordered setup scripts (`scripts/*.sh`, `scripts/pre-*.sh`)    |
| `bootstrap.sh`    | Curl-bash entrypoint that fetches `hm` and runs `apply`       |
