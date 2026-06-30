---
title: "Contributing"
description: "Adding a distro, a package manager, or anything else."
weight: 80
---

Homie is small on purpose, but the surface that needs to grow first is
**distro and package-manager coverage**. The code is structured so adding
a new one is a focused change in two or three files.

Source lives at [`github.com/kurowski/homie`](https://github.com/kurowski/homie).
PRs welcome; the rest of this page walks through the common paths.

---

## Adding a new distro

Distro support has three touch points. Grep for `TODO(contrib)` to see
the current set:

```sh
git grep -n 'TODO(contrib)'
```

1. **`internal/detect/detect.go`** — recognise the distro's
   `/etc/os-release` `ID=` value and return it from `Detect()`. Map it
   to the right package manager (`apt`, `dnf`, or a new one you're
   adding alongside). Two platforms are special-cased *before* the
   `/etc/os-release` parse: macOS (`GOOS == "darwin"` → platform key
   `macos`, manager `brew`) and Termux (`$TERMUX_VERSION` set → platform
   key `termux`, manager `pkg`), neither of which has an os-release at the
   real root.

2. **`internal/packages/`** — if the distro uses an existing manager
   (`apt` or `dnf`), you're done after step 1. If it needs a new manager,
   see [adding a package manager](#adding-a-package-manager) below.

3. **`e2e/dockerfiles/`** — add a minimal base image so the e2e harness
   exercises the new distro. Copy the existing `fedora.Dockerfile` or
   `ubuntu.Dockerfile` and adapt the base image plus any bootstrap
   packages (`bash`, `git`, `sudo`, `ca-certificates`).

Add a test entry in `e2e/e2e_test.go` for the new distro and run
`make e2e` locally to confirm.

---

## Adding a package manager

The interface is small:

```go
type Manager interface {
    IsAvailable() bool
    IsInstalled(pkg string) bool
    Install(packages []string) error
}
```

To add one (e.g. `pacman`, `zypper`, `apk`):

1. Create `internal/packages/<name>.go` implementing `Manager`. Mirror
   the structure of `apt.go` or `dnf.go` — both use an injectable
   command runner so they're unit-testable without shelling out for
   real.
2. Wire it into the selector in `internal/packages/manager.go` so
   `detect.Env.PackageManager == "<name>"` returns your implementation.
3. Update `internal/detect/detect.go` to map the relevant distros to
   the new manager name.
4. Add a unit test alongside (`<name>_test.go`) with table-driven
   IsInstalled/Install cases using the fake runner pattern.

Key invariants every manager must hold:

- **Idempotent.** `Install` filters out already-installed packages
  before calling out to the real tool.
- **Sudo only when needed.** Check `os.Geteuid()`; prepend `sudo` only
  when not root. Never assume passwordless sudo — return the underlying
  exit error and let the user see it. The one deliberate exception is
  `pkg` (Termux): Termux runs unprivileged with no root and no `sudo`
  binary at all, so it never escalates regardless of the effective uid.
- **No prompts.** Pass whatever flag suppresses interactive prompts
  (`-y` for apt/dnf, `--noconfirm` for pacman, etc.). A `hm apply` mid-run
  should never block on a TTY question.

---

## Running tests

```sh
make build     # static binary at ./hm
make test      # go test ./... — unit tests only
make lint      # go vet + golangci-lint if installed
make e2e       # container-based e2e suite (needs Docker / Podman)
```

The e2e suite builds the binary, builds one image per distro, runs
`hm apply` against a fixture user-repo, and asserts the resulting state.
It's slow (~60s) but it's the only way to catch regressions in the
package phase.

---

## Code conventions

- **No `panic`** outside `main.go` initialization.
- **Wrap errors** with context: `fmt.Errorf("link %s: %w", path, err)`.
- **No public API.** Everything except `cmd/hm` lives under `internal/`.
- **Tests next to source.** Fixtures in `<pkg>/testdata/`.
- **No external state writes** other than `$HOME` and (when root) what
  the package manager touches on its behalf.

---

## Filing issues

Bug reports and feature requests go to
[github.com/kurowski/homie/issues](https://github.com/kurowski/homie/issues).
Useful things to include:

- Distro and version (`cat /etc/os-release`).
- `hm --version`.
- Output of `hm doctor` if `hm apply` is the failing command.
- Whether you're running in a container / Codespace.

For security issues, please don't open a public issue — email the
maintainer instead.
