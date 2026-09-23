---
name: Homie
mode: write
commands:
  build: go build ./...
  test: go test
  vet: go vet
  e2e: go test -tags=e2e -timeout=20m ./e2e/...
---

# Homie

A single-binary environment manager for Linux and macOS: dotfiles by
symlink, templates rendered from Go `text/template` plus Sprig,
packages through the host's own manager, declared git clones, and
ordered setup scripts, all driven by one `homie.toml` in a git repo
the user owns. This repository is the tool, in Go: the cobra commands
in `cmd/homie/`, one package per concern under `internal/`, the Hugo
site under `docs/website/`, and a container harness under `e2e/`. The
map was last read at `v0.7.0` (`d09af90`, 2026-09-16), and every path,
line, and sha on it is from that commit.

Read `CLAUDE.md` at the repository root before anything else. It is
the maintainer's own account of the design and the reasons behind it,
and the lessons below assume you have. Two repositories are involved
in any Homie setup, and they must not be conflated: this one, the
tool; and a user's environment repo (`homie.toml`, `home/`,
`scripts/`, `bootstrap.sh`), which `homie init` generates and which
never appears here except as test fixtures and the embedded seed under
`internal/scaffold/files/`.

The commands above are what a task's verifier may select. `test` and
`vet` take a package path, always: `test ./internal/runner/`, `test
./internal/packages/ -run TestPacman`, `vet ./cmd/homie/`, or `test
./...` for the whole suite, which is what `make test` runs and takes
about two seconds. Without a path they look for Go files at the
repository root and find none. A `-run` pattern is a single word here
(no `|`, no quotes), so a task that needs two tests names their common
prefix. `vet` and `test` compile a package's test files with it, so
when a fix changes a function a held test calls, `vet` on the held
test's package fails with the reference in place (the learner's own
copy of that test still calls the old form): vet the callers' packages
instead, and let the held line and `build` cover the rest. `build` is
CI's `go build ./...`: it compiles everything and
writes nothing (`make build` is the same with the version stamped in,
and leaves a `homie` binary at the root, which `.gitignore` covers).
`e2e` is `make e2e` spelled out, and it is never in a task's verifier;
see the environment below.

Tasks built from history start from the commit before a fix, and
before `v0.7.0` (`62e58d1`, #50) the binary was `hm`: a starting state
from then has `cmd/hm/`, not `cmd/homie/`, and the scripts' variables
are `HM_REPO`, `HM_HOME`, `HM_TAGS`. A verifier line names whichever
the starting state has.

## Environment

Nothing to start. The unit suite needs a Go toolchain at least as new
as `go.mod` says (`go 1.25.0`; a newer one is fine) and `git` on the
path, since the externals, doctor, and `init --update` tests make real
repositories in temporary directories. Every test works in
`t.TempDir()` and fakes what it would otherwise touch: package
managers through an injected `Runner`, the host through `detect`'s
`Detector` fields, `$HOME` through a flag or `t.Setenv`. So a failing
unit test is the code, not the machine, with one class of exception
worth knowing (below, and in `53e1f3a`): a test that quietly assumed
something about the environment it ran in.

`e2e` is different. It drives the real `curl | bash` flow against
Ubuntu, Debian, Fedora, and Arch containers: it needs `docker` on the
path (it skips without one), builds four images and an nginx sidecar,
and takes minutes, not seconds. `go test ./...` does not build it,
because it is behind the `e2e` build tag. The maintainer runs it
before tagging a release, and CI runs it on every push; a learner runs
it when a lesson asks, never as a check on a task, and whether Docker
is on their machine is theirs to know. The tutor never starts it for
them.

## Regions

- **cli** — the command layer and how a run is put together:
  `cmd/homie/`, one cobra file per subcommand with its `_test.go`
  beside it and a `Long` that is the in-terminal documentation;
  `apply.go`, which orders the phases every other region supplies;
  `internal/ui/` (the Bubble Tea TUI and the plain no-TTY output);
  `internal/repo/` (finding the user's repo); and the read-only
  commands, `doctor`, `status`, and `context`, with
  `internal/doctor/`.
- **config** — what a user's repo says and how it becomes files in
  `$HOME`: `internal/config/` (`homie.toml`, host overlays, tags and
  tag-keyed blocks), `internal/tree/` (the `home/` and
  `home.tag-X/` convention and the more-specific-wins rule),
  `internal/link/` (symlinks), and `internal/render/` (templates).
- **packages** — the host and its package managers:
  `internal/detect/` (distro, architecture, container, root, the
  auto-tags) and `internal/packages/` (the `Manager` interface, the
  native backends apt, dnf, pacman, brew, and pkg, and the opt-in
  flatpak, snap, and brew-on-Linux).
- **scripts** — the user's own code that `apply` runs, and the git
  checkouts it keeps: `internal/runner/` (pre- and post-package
  scripts, `scripts.tag-X/` trees, the environment a script gets, how
  it reaches a terminal) and `internal/externals/` with
  `internal/config/externals.go` (the `[externals]` table).
- **scaffold** — how Homie reaches a new machine and keeps up with
  it: `internal/scaffold/` (`homie init`, seeds and tool-owned files,
  the provenance stamp, `init --update`), the embedded
  `files/bootstrap.sh`, `cmd/homie/init.go`, `init_update.go`,
  `bootstrap.go`, and `internal/selfupdate/`.

## Suggested courses

Everyone starts with the two `cli` lessons, in this order: without
`local-dev-setup` nothing builds, and without `how-a-change-ships`
nothing ships. Both are `write` lessons, and the second hands you its
test. Every other lesson requires them, and none requires another, so
the order after them is the learner's.

### Generalist

1. cli, `orientation`: `local-dev-setup`, `how-a-change-ships`
2. packages, `working`: `package-backends`
3. config, `working`: `home-tree-and-render`
4. scripts, `working`: `scripts-and-the-runner`
5. scaffold, `working`: `tool-owned-files`

For someone who will work anywhere in the tool. Each `working` lesson
is one reverted fix with its test held, in one package, and each
teaches one of the design principles in `CLAUDE.md` through the code
that keeps it: a pass that forks once instead of eighty times, a pass
that changes nothing when nothing changed, a contract a script can
rely on, a generated file that has to run on a machine that shares
nothing with the one that wrote it.

### Platform porter

1. cli, `orientation`: `local-dev-setup`, `how-a-change-ships`
2. packages, `working`: `package-backends`
3. scaffold, `working`: `tool-owned-files`

For someone adding a distribution or a package backend, the shape of
macOS (#27), Termux (#45), and Arch (#48): detection, a backend,
`bootstrap.sh` taught the new OS, and an e2e image. The packages
lesson is the backend; the scaffold lesson is why `bootstrap.sh`
cannot assume anything about the machine it lands on. Take scripts at
`working` if the platform needs a pre-package script to set up its
repositories.

### Dotfiles features

1. cli, `orientation`: `local-dev-setup`, `how-a-change-ships`
2. config, `working`: `home-tree-and-render`
3. scripts, `working`: `scripts-and-the-runner`

For someone working on what a user's repo can express: tags, overlays,
templates, scripts, externals. Skips the backends, which the
`cli` opener already opens up far enough to follow a package list
through `apply`.

## Corpus

Exemplary, copy the shape of:

- `internal/packages/flatpak.go` — a backend in full: an injected
  `Runner` (the type is `manager.go` line 42), `IsAvailable` by
  `exec.LookPath`, the installed set loaded once per `Manager` with
  `sync.Once` (lines 22 and 42 to 61), and `Install` filtered through
  `filterUninstalled` (`manager.go` line 102). `snap.go` and `brew.go`
  are the same shape with a spec suffix and a `Validator`.
- `internal/packages/manager_test.go` — `fakeRunner` (line 13)
  records every call and answers from a table, so a test asserts the
  exact commands a backend would run, `sudo` included, without the
  backend's tool on the machine.
- `internal/detect/detect.go` — `Detector` (line 31): every source of
  truth about the host (`FS`, `Getenv`, `GOOS`, `Geteuid`) is a field
  with a real default, so `detect_test.go` builds any host in a table.
- `internal/config/testdata/` with `config_test.go` — one directory
  per case, a real `homie.toml` in each, `unknown-field/` and
  `packages-typos/` for what the loader warns about.
- `internal/runner/runner.go` `exec_` (line 234) and
  `internal/scaffold/files/bootstrap.sh` — comments that say why,
  with the issue that forced it (`#46`), which is the house style for
  anything surprising.
- `cmd/homie/apply.go` — the `Long` (lines 33 to 66) is the
  documentation a user reads in the terminal, and it is kept as
  accurate as the code under it.

Legacy, read but do not copy: the repository is young and has little.
What reads old is history. Before `v0.1.0` a user repo had
`dotfiles/` and `templates/` (`CLAUDE.md`, "Layout migration"); before
`v0.7.0` the binary was `hm`. `ForBackend`'s snap case (`manager.go`
line 87) derives root from the effective uid rather than
`detect.Env`, and says so in a `TODO`.

Pull requests and commits that show how work is done here:

- #30 `c7fc579` (fix `1349c31`, closing #28) — one backend change and
  the fake-runner test that pins it, nothing else.
- #29 `f0f71f4` (fix `d9a9c24`, closing #26) — a contract a user's
  scripts rely on, restored in the one function that builds their
  environment, with the test that simulates the machine where it broke.
- #48 `6b6415e` — adding a platform end to end: detection, a
  backend, an e2e image, the docs page, and the review's follow-ups as
  their own commits (`92f8e14`, `a051c28`, `9a04075`), each with a
  body that says what the reviewer found and why the fix is the one it
  is.
- `f3ba646` — e2e caught a regression the unit tests could not; the
  rule was wrong, not the test, and the fix adds the unit-level
  version of the e2e case so it never needs a container again.
- `039d6db` — the release commit: `CLAUDE.md` "Current state" and the
  README brought current, one entry per release, written for the next
  engineer.

Definition of ready, before a pull request: CI's three lines, `go vet
./...`, `go build ./...`, `go test ./...`, which it runs on Linux and
macOS; `golangci-lint run` (CI's lint job, with the exceptions in
`.golangci.yml`; `make lint` runs it when it is installed and says so
when it is not); `gofmt`-clean (`112ccfd` fixed drift). A
user-visible change updates the command's `Long` and the page under
`docs/website/content/docs/`, and never `CLAUDE.md` alone. Before a
tag, `make e2e`. Commit subjects are `fix:`, `feat:`, `docs:`, or the
package's name (`pacman:`), and the body says why.

## Mistakes agents make here

The ways an agentic change to this repository goes wrong. What the
learner is taught to catch in `direct` lessons, and what the tutor
reads a change against.

- Shells out with `exec.Command` inside a backend instead of through
  its `Runner`, so no test can fake it, and the test that gets written
  anyway needs the tool on the machine.
- Asks the package database once per package: `IsInstalled` that forks
  per call, run twice per list (once to report, once inside
  `Install`'s `filterUninstalled`). The non-native backends and
  `Pacman` cache the installed set; `apt`, `dnf`, and `pkg` cannot.
- Hardcodes `sudo` in a command or a message instead of going through
  the backend's `command` and its `Sudo` field, which is false as root
  and absent on Termux (`pkg.go`), where there is no `sudo` at all.
- Adds a backend in fewer than the three places `CLAUDE.md` names
  (`config.KnownBackends`, a `Manager` in `internal/packages`, a case
  in `ForBackend`), or a distro without its key in
  `knownDistroKeyOrder` (`config.go` line 100).
- Matches `ID_LIKE` to catch a derivative distro. Detection matches
  the exact `ID` on purpose (`CLAUDE.md`, "Environment detection").
- Adds state: a lockfile, a cache on disk, a record of what the last
  run did. "No state" is the second design principle; every pass is a
  full reconciliation, and every step checks before it acts.
- Writes a step that does work on a second, unchanged `apply`, or
  reports work it did not do. A converged machine says `skip`.
- Documents a user-visible feature in `CLAUDE.md`, or puts Go
  identifiers in an end-user page under `docs/website/content/`
  (`CLAUDE.md`, "Documentation website", has the audience note).
- Adds a field to `render.Data` without the two hand-written tables
  that describe it (`render.go` lines 31 to 36 say which).
- Changes a tool-owned file's template (`bootstrap.sh`) as if it were
  a seed, or leaves something in it a user will want to edit, which
  under the stamp rule costs them every later refresh (`CLAUDE.md`,
  "Seeds vs. tool-owned files").
- Bakes a path from the machine that ran `homie init` into a file
  whose job is to run on another machine (`f3ba646`).
- Writes a test that assumes its environment: `$USER` set, `TMPDIR`
  outside `$HOME` (`53e1f3a`), or an order out of a map (`d4c345f`).
- Treats `go test ./...` as the release gate. It does not build the
  `e2e` tag; `make e2e` is the gate, and it runs before tagging.
