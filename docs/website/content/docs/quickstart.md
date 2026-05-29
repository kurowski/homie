---
title: "Quick start"
description: "From zero to a working Linux or macOS environment in under five minutes."
weight: 10
---

This walks through scaffolding a fresh Homie repo, putting your first
dotfile under management, and bootstrapping the same environment on a
second machine. About five minutes if you already have git + GitHub set
up.

## 1. Install `hm`

One static binary, no runtime, no dependencies:

```sh
curl -fsSL https://homie.sh/install.sh | bash
```

Prefer not to pipe a script? See [Install](/docs/install/) for manual
download, version pinning, and building from source.

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

On _any other_ Linux or macOS box — bare metal, VM, container,
Codespace — run:

```sh
curl https://raw.githubusercontent.com/<you>/dotfiles/main/bootstrap.sh | bash
```

That downloads the right `hm` binary for the OS and CPU, clones your
repo, and runs `hm apply`. Done.

`bootstrap.sh` makes sure `git` is present first. On Linux it also
installs `ca-certificates` if needed. On macOS `git` comes from the
Xcode Command Line Tools (`xcode-select --install`), and Homebrew is
*not* required — install it only if you declare `[packages]`.

## Where to next?

- [Commands](/docs/commands/) — every `hm` subcommand explained.
- [`homie.toml`](/docs/config/) — full config reference.
- [Dotfiles](/docs/dotfiles/) — symlinks, templates, tag-gated trees, overrides.
- [Recipes](/docs/recipes/) — concrete patterns for common setups.
