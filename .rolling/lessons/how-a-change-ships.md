---
title: How a change ships here
region: cli
depth: orientation
mode: write
requires: [local-dev-setup]
assumes: [go, git]
test: shown
---

The rules a maintainer will hold your first pull request to, and where
each one lives. Most of them are in `CLAUDE.md`; this lesson has you
find the mechanism behind each, by making one small change in a
package backend and running every check the repository has on it.

Start with how a run is put together. `cmd/homie/apply.go` is the
spine: `runApply` (line 78) finds the user's repo (`repo.Find`),
detects the host (`detect.Detect`), loads `homie.toml`
(`config.Load`), and then runs the phases in the order its `Long`
lists: pre-scripts, native packages, each declared backend, externals,
the home tree, scripts. Each phase is a function in the same file that
calls one `internal/` package and reports through `ui.UI`
(`internal/ui/ui.go` line 13), which is a live TUI on a terminal and
plain lines otherwise. Errors are collected, not returned early: one
failing phase does not stop the rest, and `apply` exits non-zero at
the end if anything failed. Every other region of this map is one of
those phases.

Packages are the phase to trace first, because the seam is the
clearest. `applyPackages` (line 117) asks `packages.For(env)`
(`internal/packages/manager.go` line 50) for the host's native
`Manager`, splits the declared list into installed and not by asking
`IsInstalled`, and hands the rest to `Install`. Every backend has an
injected `Runner` (line 42), so its tests replace the real command
with `fakeRunner` (`manager_test.go` line 13), which records each call
and answers from a table: a backend's test says exactly which
commands it would run, `sudo` included, on a machine without the tool.
`docs/website/content/docs/contributing.md` is the maintainer's
account of adding a distro or a manager, and it is written for exactly
this reader.

How a change is shaped here, from `CLAUDE.md` and the history:

- One concern per branch and pull request (`feat/…`, `fix/…`), the
  issue closed by the commit (`Closes #28`). Subjects are `fix:`,
  `feat:`, `docs:`, or the package's name; the body says why, and a
  review's findings land as their own commits, named as such.
- A test with every behaviour change, beside the code, in the style
  of its neighbours; a table when there are cases.
- User-visible behaviour is documented twice, in the command's `Long`
  and in the page under `docs/website/content/docs/`, and never in
  `CLAUDE.md` alone, which is for engineers. End-user pages carry no
  Go identifiers.
- Before a pull request: `go vet ./...`, `go build ./...`, `go test
  ./...` (CI runs them on Linux and macOS), `golangci-lint run` with
  the exceptions in `.golangci.yml`, `gofmt`. Before a tag: `make
  e2e`, and a release commit bringing `CLAUDE.md`'s "Current state"
  and the README up to date.

This is a `write` lesson with the test shown: the tutor hands you a
failing unit test with the task, and the work is to make it pass in
the backend, then run the repository's own checks before you say done.
The point is the mechanics, not the analysis; the review is about the
shape of the change and which check would have caught each
alternative.

Task source for the tutor: `1349c31` (#30, closing #28). A batched
`brew install --cask A B C` aborts on the first conflict (most often an
app already in `/Applications` from the App Store) and silently skips
the rest, so an unrelated cask later in a user's `[packages].macos`
never installs. The fix installs casks one at a time in `Brew.Install`
(`internal/packages/brew.go`), collects the failures with
`errors.Join` instead of returning on the first, points each at `brew
install --cask --adopt <name>`, and leaves formulae batched. Bring
`internal/packages/manager_test.go` forward with `--shown`; the new
test is `TestBrewInstallCasksContinueOnConflict`, and the verifier's
test line is `test ./internal/packages/ -run TestBrew`. Scope the task
to `internal/packages`, and verify with `build` and `vet
./internal/packages/` besides. The starting state predates the rename,
so the command layer is `cmd/hm/`; nothing in this task touches it.
The brief names the symptom and what the test wants (each cask its own
invocation, every failure reported, formulae still one call); it does
not say how to collect the errors.

## Rubric

- The test passes, and `build` and `vet` are green: with `go test
  ./...` and `golangci-lint`, what a maintainer runs before opening a
  pull request here.
- The change is in `Brew.Install` and nowhere else: formulae still
  install in one invocation, casks one each through the same `run`
  helper, and no call bypasses the `Runner`.
- Every cask is attempted and every failure is reported: errors are
  collected and joined, not returned on the first, and each names the
  cask and what to do about it.
- The method's comment says why casks are installed one at a time,
  since a reader will otherwise batch them again.

## Talk through

- Where each phase of `apply` lives, and why an error in one does not
  stop the next.
- Why every backend takes a `Runner`, and what a test of one could not
  check without it.
- Where this change would be documented if it changed what a user
  sees, and what `make e2e` covers that this test does not.
