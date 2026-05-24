---
title: "Quick start"
description: "From zero to a working Linux environment in under five minutes."
weight: 10
---

This walks through scaffolding a fresh Homie repo, putting your first
dotfile under management, and bootstrapping the same environment on a
second machine. About five minutes if you already have git + GitHub set
up.

## 1. Install `hm`

Grab the latest static binary from
[Releases](https://github.com/kurowski/homie/releases). One file, no
runtime, no dependencies.

```sh
ARCH=$(uname -m | sed 's/x86_64/amd64/;s/aarch64/arm64/')
curl -fsSL -o /tmp/hm \
  "https://github.com/kurowski/homie/releases/latest/download/hm-linux-${ARCH}"
chmod +x /tmp/hm
sudo mv /tmp/hm /usr/local/bin/hm
hm --version
```

## 2. Scaffold a user environment repo

`hm init` writes a starter repo — `homie.toml`, a `bootstrap.sh`, an
example dotfile, an example template, and a sample script — that's
yours to grow into.

```sh
hm init ~/dotfiles
```

It'll ask for your name, email, and GitHub username so it can wire up
`bootstrap.sh` with the right clone URL.

## 3. Push it to GitHub

Create an empty repo at `github.com/<you>/dotfiles` (or whatever you
named it during `hm init`), then:

```sh
cd ~/dotfiles
git init -b main
git add -A
git commit -m "initial scaffold"
git remote add origin git@github.com:<you>/dotfiles.git
git push -u origin main
```

## 4. Apply on the machine you're on

Reconcile your `$HOME` with the scaffolded repo:

```sh
hm apply
```

You'll see Charm-styled progress as Homie installs the listed packages,
symlinks your dotfiles, renders your templates, and runs your scripts.

## 5. Bootstrap on a fresh machine

On _any other_ Linux box — bare metal, VM, container, Codespace — run:

```sh
curl https://raw.githubusercontent.com/<you>/dotfiles/main/bootstrap.sh | bash
```

That downloads `hm`, clones your repo, and runs `hm apply`. Done.

## Where to next?

- [Commands](/docs/commands/) — every `hm` subcommand explained.
- [`homie.toml`](/docs/config/) — full config reference.
- [Templates](/docs/templates/) — conditionals, profile tags, the works.
- [Recipes](/docs/recipes/) — concrete patterns for common setups.
