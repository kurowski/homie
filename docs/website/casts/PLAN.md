# Screencasts for homie.sh — plan & runbook

**Status:** P1 done — the `bootstrap` hero is live, rendered manually with
`record.sh`. **Scope:** docs site only (`docs/website/`).

## Why this doc exists

Most of this is simple enough to just execute. It's written down because the
pipeline starts as a manual `record.sh` and should later **render in CI**;
that direction — and the approach we deliberately rejected — is worth recording
(see [Future: render in CI](#future-render-in-ci)). A previous version of this
plan was lost because it only lived in a Claude transcript on another host —
hence a tracked file.

## Goal

homie's pitch — friendly, colorful, idempotent — is a *visual* claim the docs
only assert in prose. Short, on-brand, reproducible terminal casts let the real
thing do the selling.

## Tooling

**VHS only** (`charmbracelet/vhs`), and **rendering is containerized**: the
recording image is pinned by digest (`FROM ghcr.io/charmbracelet/vhs@sha256:…`,
Debian 13 — already bundles ttyd + ffmpeg + headless chromium + JetBrainsMono
Nerd Font). That makes a local `record.sh` produce the same artifacts as CI,
which matters because the output is committed binary media.

- Inline command demos → optimized **GIF**.
- Homepage hero → **WebM + GIF fallback** (smaller, sharper, autoplay-loop).

## Casts

| Cast | Page | Shows | ~len |
|------|------|-------|------|
| **`bootstrap`** (hero) | `_index.md` | a **fresh machine → working environment** via the exact `curl … bootstrap.sh \| bash` one-liner we tell people to run. The outcome, not the internals. | 15–25s |
| `apply` | `commands.md` | `hm apply` reconciling: detect → packages → home (link/render) → scripts → pass/warn/fail summary | 12–18s |
| `status` / `doctor` | `commands.md` | `hm status`, then `hm doctor`'s read-only audit + summary | 8–12s ea |
| `home` | `commands.md` | `hm home` materializing the tree | ~8s |
| `symlink` | `dotfiles.md` | `ls -l ~/.zshrc` → repo; edit; `git diff` — proves the no-indirection model | ~12s |

The homepage shows only the **outcome** (bootstrap). The `hm`-subcommand demos
live on their respective pages. No long `init → push → bootstrap` *authoring*
cast — too long for a loop.

## Recording harness

The hero runs the *real* `curl | bash` bootstrap, so it must be both isolated
and offline/deterministic. We get all of that by reusing the **e2e harness**:

- **`hm init`** scaffolds the demo repo (Scout Homes / scout@homie.sh /
  scouthomes/dotfiles), exactly as e2e does.
- An **nginx sidecar** impersonates `github.com` + `raw.githubusercontent.com`
  over the **committed e2e test CA** (`e2e/certs`), serving the scaffolded repo
  (dumb-HTTP git) + the hm release artifacts. Docker network aliases point both
  hostnames at nginx, so the in-cast HTTPS resolves locally — no internet, fully
  reproducible, but the displayed command is the genuine one.
- The **recording container** (the pinned VHS image + a thin "fresh Linux box"
  layer) joins that network and runs `vhs bootstrap.tape`. Because VHS runs
  *inside* the fresh box, the tape is literally what a user types — no hidden
  `docker exec`.
- **Pre-baked packages.** The image pre-installs the demo repo's declared
  packages (and git/ca-certificates), so `hm bootstrap` and `hm apply`'s package
  phase are no-ops at render time — fast, offline, deterministic. A genuinely
  fresh box installs these on first run; pre-baking just keeps the loop short.
  (Showing live installs would need a local apt cache — a later refinement.)
- **Runs as a non-root user `scout`** (NOPASSWD sudo) — the homepage one-liner
  is `curl … | bash`, so `scout@laptop ❯` + `~/dotfiles` reads as a personal
  laptop, not a server. chromium is system-wide in the base image, so scout
  uses it directly (no per-user warm step). The `scout@laptop ❯` prompt is
  *sourced from a file* the tape loads with a pure-ASCII line — VHS runs bash
  without rc files and mangles non-ASCII typed input, so the ❯ glyph never goes
  through its keystroke path.

Note: `hm` selects its TUI from **stdout** being a terminal, not stdin — so even
under `curl | bash` (stdin is the pipe) the cast shows the full colorful TUI,
because VHS provides a PTY on stdout.

Example (the hero):

```tape
Output bootstrap.gif
Output bootstrap.webm
Set Shell "bash"
Set Theme "Catppuccin Mocha"
Set FontSize 18
Set Width 1320
Set Height 600
Set TypingSpeed 50ms
Hide
Type "source /etc/homie-ps1.sh" Enter
Type "clear" Enter
Show
Sleep 1s
Type "curl -fsSL https://raw.githubusercontent.com/scouthomes/dotfiles/main/bootstrap.sh | bash"
Sleep 700ms
Enter
Sleep 5s
```

## Repo layout

```
docs/website/
  casts/
    Dockerfile         # recorder image: pinned VHS base + fresh-box layer
    nginx.conf         # TLS sidecar (impersonates github / raw.github)
    record.sh          # orchestration (build hm, scaffold, nginx, vhs, collect)
    *.tape
    PLAN.md
  static/casts/        # rendered .gif/.webm (committed, served by Hugo)
  layouts/shortcodes/cast.html
```

## Build (now): manual `record.sh`

```
./record.sh                 # render every *.tape
./record.sh bootstrap.tape  # one cast
```

`record.sh` builds hm, builds the pinned recorder image, stands up nginx,
renders, copies `.gif`/`.webm` into `static/casts/`, and tears everything down.
Run it by hand when CLI output changes; commit the rendered media.

## Future: render in CI (deferred)

Today the media is rendered manually with `record.sh` and committed. The
intended next step is to **render during the website build itself**: add a
`record.sh` step to `pages.yml` before `hugo` and publish the fresh media
*without committing it* (drop `static/casts/*.{gif,webm}` from git and
`.gitignore` them). Rendering is already pinned in a container, so a CI render
matches a local one.

Deferred, not abandoned. Tradeoffs to accept when we do it:

- Every site deploy gains the render cost (~3–5 min: VHS image pull + recorder
  build + render), and a flaky render would block the publish. Mitigate by
  prebuilding the recorder image (e.g. to GHCR) so the step is just a pull.
- The GIF/WebM stop living in the repo, so PRs no longer preview them inline.

**Explicitly rejected:** a separate workflow that regenerates and *commits the
media back* (a bot pushing generated binaries into history). Not going there.

## Phasing

- **P1 (now):** the `bootstrap` hero end-to-end + the `cast` shortcode +
  homepage embed. Proves the whole pipeline.
- **P2:** the `hm`-subcommand casts (`apply`, `status`/`doctor`, `home`,
  `symlink`) on their pages — same harness, simpler (no nginx; just run `hm`
  against a scaffolded box).
- **P3:** WebM hero + reduced-motion poster, size budgets, alt/captions, and
  the deferred render-in-CI step above.
