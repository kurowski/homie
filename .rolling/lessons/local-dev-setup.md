---
title: Local dev setup
region: cli
depth: orientation
mode: write
requires: []
assumes: [go, git]
---

Get Homie building and its tests green on your machine, and make one
small change go through every check the repository has. This is the
one lesson where the task is the environment: there is no fix to
revert and no test to hold or show, so the lesson declares neither,
and the tutor builds the task from this page.

There is nothing to start. Homie's environment is a Go toolchain and
`git`, and that is all: no database, no services, no `.env`. What you
will have at the end is the module's dependencies downloaded, the tool
built, the whole unit suite passing, and a first change of your own
made and checked.

Pointers:

- `go.mod`: the module path (`github.com/kurowski/homie`) and the
  language version, `go 1.25.0`, which is the oldest toolchain that
  will build it. The `require` blocks are the Charm TUI stack
  (`bubbletea`, `lipgloss`, `bubbles`, `fang`), `cobra`, `BurntSushi/toml`,
  and `sprig`; the first build downloads them into the module cache.
- `Makefile`: `build` stamps the version from `git describe` into
  `main.version` (`cmd/homie/main.go` line 19) and writes `./homie`;
  `test` is `go test ./...`; `lint` is `go vet ./...` plus
  `golangci-lint` when it is installed; `e2e` is the container suite,
  behind the `e2e` build tag (`e2e/e2e_test.go` line 1).
- `.github/workflows/ci.yml`: the three lines every pull request must
  pass, `go vet ./...`, `go build ./...`, `go test ./...`, on
  `ubuntu-latest` and `macos-latest`, then the lint job and the e2e
  job. What CI runs is what "green" means here.
- Where the tests are: beside the code, one `_test.go` per file or per
  concern, package-internal (`package runner`, not `runner_test`) so
  they can reach unexported helpers. `internal/config/testdata/` holds
  fixture repos. Nothing in the unit suite touches the real `$HOME`,
  the real package manager, or the network.
- `./homie --help` after `make build`, then `./homie status` from
  anywhere: it prints what `detect` found about your machine (distro,
  architecture, container, root) and, unless `$HOMIE_REPO` points at
  one, says no user repo was found, which is correct: this is the
  tool's repository, not an environment repo. `homie apply` is never
  run from here.

## Confirming the state, step by step

An environment that is already set up is confirmed, not performed and
not quizzed about. Each step has a command that shows it is done; the
learner runs the command, and the tutor reads the result. Nothing here
is asked as a question.

- **A toolchain new enough.** `go version` reports 1.25 or later.
  With an older one and `GOTOOLCHAIN` left at its default, the `go`
  command downloads the right toolchain itself on the first build,
  which is fine and takes a minute.
- **Dependencies downloaded, and the tool builds.** `go build ./...`
  exits quietly. On a fresh machine the first run prints the modules
  it downloads.
- **The suite is green.** `go test ./...` shows `ok` for every package
  and `[no test files]` for `internal/repo`.
- **The binary runs.** `make build`, then `./homie --version` prints a
  `git describe` of this checkout, and `./homie status` reports the
  machine.

## The task, and its verifier

A task is provable only with a check that fails before the work is
done, and on a machine that is already set up there is nothing left to
fail. So the exercise is a smoke test along a small seam: a pure
function of a few lines beside an existing one, with its test shown,
so that the learner's first change here is small and the whole
pipeline runs on something real. The seam is the tutor's to choose,
and it should be one no later lesson is built on:
`internal/externals/externals.go` (`normalizeURL` and `short`, lines
280 to 291) and `internal/detect/detect.go` (`shortHostname`, line 81)
are the size. Not `runner.buildEnv`, `render.inSync`, `Pacman`'s
installed check, or `scaffold.CloneTarget`: those are other lessons'
exercises.

The verifier is these three lines and no others: `build`; `vet` on the
seam's package; and `test` on the seam's package with `-run` and the
new test's name. On the starting state `vet` fails as well as `test`,
since it compiles the new test, which calls a function that does not
exist yet: both are `expect-fail-on-base` lines. The tutor does not
add a check at `done` that the verifier did not run, and does not
leave one of these out.

Notes for the tutor: the task is built from this page, with the test
shown and committed into the starting state (`--here`); it is never a
reverted fix from history. The tutor never installs a toolchain,
changes `GOTOOLCHAIN`, or fixes the module cache; it may say what a
failure means. `make e2e` is not part of this lesson.

## Rubric

- `build`, `vet`, and the seam's test pass in the verifier, and `go
  test ./...` passes in the learner's environment.
- The toolchain, the dependencies, and the built binary are confirmed
  the way the list above says.
- The seam's function is small and beside its sibling, its test sits
  in the package's existing `_test.go` in the same table-driven style
  as its neighbours, and the test asserts behaviour, not
  implementation.
- The code is `gofmt`-clean.

## Talk through

- Why the unit suite needs nothing but Go and `git`, and what the
  `e2e` suite needs that it does not.
- What `go test ./...` does not run, and when the maintainer runs
  that instead.
