---
title: Package backends
region: packages
depth: working
mode: write
requires: [how-a-change-ships]
assumes: [go, linux-package-managers]
test: held
---

How Homie knows what machine it is on, how it asks that machine's
package manager what is installed, and how it installs the rest
without keeping any record of its own.

Detection first. `internal/detect/detect.go` builds an `Env` (line
18): distro, package manager, architecture, hostname, container, root,
interactive. `Detector` (line 31) makes every source of truth a field
(`FS` for `/etc/os-release`, `Getenv`, `GOOS`, `Geteuid`,
`LookupHostname`), filled with the real ones by `withDefaults` (line 96),
which is why `detect_test.go` can describe an Arch container or a
Termux phone in a table row. macOS and Termux are decided before the
os-release parse (lines 50 and 55); everything else is the exact `ID=`
(`parseDistro`, line 121), never `ID_LIKE`, and `packageManagerFor`
(line 150) maps it. `autoTags` (line 194) turns the result into the
tags every other region reads.

Then the backends. `internal/packages/manager.go` is the whole
contract: the `Manager` interface (line 18), the optional `Validator`
(line 36) for specs with a suffix, the `Runner` every backend calls
instead of `os/exec` (line 42), `For` (line 50) for the native manager
and `ForBackend` (line 81) for the opt-in ones, and `filterUninstalled`
(line 102), which every `Install` runs first so a converged machine
installs nothing. There is no state file: the package database is the
state, asked every run.

Asking it costs a fork per question, and `apply` asks twice per
package: once in `applyPackages` (`cmd/homie/apply.go` line 117) to
report what is already there, and again inside `Install`'s
`filterUninstalled`. `apt`, `dnf`, and `pkg` pay that, because `dpkg
-s` and `rpm -q` answer one name at a time. The backends whose tools
can list everything at once load the installed set once per `Manager`
and answer from it: `flatpak.go` is the plainest (the `sync.Once` and
the map at line 22, `IsInstalled` at 42, `loadInstalled` at 48);
`brew.go` keeps two, formulae and casks, each loaded only if asked
(lines 26 to 36 and 106 to 131); `snap.go` the same with confinement
suffixes. `CLAUDE.md`, "Package manager abstraction", says why each
backend is the way it is, and the `Pacman` paragraphs there are the
reading for this lesson's exercise.

Pacman is the interesting one. It is both the query tool and the
installer; it never refreshes the sync database (`pacman -Sy` then an
install is Arch's partial-upgrade footgun, so `Install` says to run
`pacman -Syu` instead, spelled with or without `sudo` by the same
`command` helper every call goes through, `pacman.go` line 164); and
its question "is this installed?" has two answers depending on whether
the name is a package or something a package provides (`sh` is
provided by `bash`).

Task source for the tutor: build from `a051c28` (in #48) with
`internal/packages/manager_test.go` held. On the starting state
`Pacman.IsInstalled` forks `pacman -T <name>` for every name, so a
40-package list forks 80 times per `apply`. The fix loads `pacman -Qq`
(every installed package name, one fork, from the local database, so
it works on a host that has never synced) once per `Manager` with
`sync.Once`, answers a literal name from that set, and only on a miss
falls through to `pacman -T`, which resolves *provides*, memoized per
name under a mutex. The held tests pin that contract:
`TestPacmanQueryIsCachedAcrossCalls` (the first call is `pacman -Qq`,
and a second `IsInstalled` forks nothing) and
`TestPacmanFallsBackToDeptestForProvides` (a name absent from the dump
is asked of `-T` once, and asked again it is not). So the brief says
what those tests will check, as behaviour: one `pacman -Qq` on first
use and never again for that `Manager`, a name found there is
installed, a name not found is resolved with `pacman -T` at most once,
and the `Manager` interface does not change. Point at `flatpak.go` as
the pattern; do not describe the mutex or the memo's type. The
verifier is `build` and `vet ./internal/packages/`, with the held line
`test ./internal/packages/ -run TestPacman`; scope
`internal/packages`. The fix's commit also rewrote `CLAUDE.md`'s
paragraph and the Arch e2e image; neither is part of the task.

In the walkthrough, show the pattern in `flatpak.go` and `brew.go`,
not `Pacman`'s own `IsInstalled` and `loadInstalled` (`pacman.go`
lines 86 to 127 at the pin): on the learner's branch those are the
answer. They are the reference, for after `done`.

## Rubric

- A converged run forks `pacman` once for the whole list, and the
  answers are the same as before: a literal installed name is
  installed, a provided name (`sh`) is resolved by `-T`, a missing
  one is not installed.
- The installed set is loaded lazily, once per `Manager`, the way the
  non-native backends load theirs, and a failed `-Qq` degrades to
  asking `-T` rather than reporting everything missing.
- The fallback is memoized, and safe if two goroutines ask at once.
- `pacman -Qq`'s output is parsed defensively: blank lines and the
  warnings pacman prints on the same stream are not package names.
- The `Manager` interface, `Install`, and every other backend are
  untouched; the comment on the struct or on `IsInstalled` says why
  there are two queries.

## Talk through

- Why there is no state file, and what answers "is it installed?"
  instead, every run.
- Why `apt` and `dnf` still fork per package, and what `dpkg -s` and
  `rpm -q` would have to offer for them to change.
- Why `Install` never refreshes pacman's sync database, and what the
  error says instead; what a group (`xorg`) or a repo-qualified name
  (`extra/tmux`) does to the installed check (`CLAUDE.md`).
