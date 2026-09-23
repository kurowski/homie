---
title: Tool-owned files, and the machine that is not this one
region: scaffold
depth: working
mode: write
requires: [how-a-change-ships]
assumes: [go, bash, go-templates]
test: held
---

How a user gets from nothing to a working environment repo, how that
repo gets Homie onto a fresh machine, and how a file Homie generated
keeps up with Homie afterwards without overwriting anything the user
changed.

`homie init` (`cmd/homie/init.go`, `runInit` at line 91) asks a few
questions it cannot derive and calls `scaffold.Run`
(`internal/scaffold/scaffold.go` line 100), which writes every entry
of `manifest` (line 75) from the files embedded under
`internal/scaffold/files/`. Most entries are **seeds**: `homie.toml`,
`home/`, `scripts/`, the README, written once and the user's from then
on. One is **tool-owned**: `bootstrap.sh` (line 81), because it
encodes how this `homie` expects to be downloaded and launched, which
changes between releases in a way a sample `.zshrc` never does.
`CLAUDE.md`, "Seeds vs. tool-owned files", is the reasoning, and it is
required reading here.

`bootstrap.sh` is the file with the hardest job in the repository: a
user runs it on a machine that shares nothing with the one that
generated it, as `curl … | bash`, often as root in a container. It
detects the OS and architecture, downloads the matching release,
clones the user's repo, and runs `homie apply`. Two of its comments
are lessons of their own: why every child that can prompt gets
`/dev/tty` one command at a time rather than with a script-wide
`exec` (#46, and `CLAUDE.md`, "Bootstrap and trust model"), and where
the clone lands, `REPO_DIR` (`files/bootstrap.sh` line 28), which is
filled from `Answers.RepoDir` (`scaffold.go` lines 33 to 39).

Keeping up. A tool-owned file is written with a provenance stamp under
its shebang (`provenance.go`: `stampProvenance`, line 56;
`readProvenance`, 75): the version that wrote it and a digest of the
file without the stamp. `homie init --update` (`cmd/homie/init_update.go`;
`scaffold.Update`, `update.go` line 48) re-renders tool-owned entries
only, and rewrites a file only when its digest proves nobody has
touched it; an edited or unstamped file is reported with the would-be
content and a diff, and `--force` is the override. `--update` derives
its answers rather than asking (`deriveAnswers`, `init_update.go` line
126): identity from `homie.toml`, the GitHub coordinates from the
`origin` remote, and where the repo lives from where it is. The
consequence `CLAUDE.md` draws is the one this lesson is about:
anything a user would otherwise hand-edit in a tool-owned file has to
become a derived answer, or the stamp turns a one-line tweak into
opting out of every refresh.

`internal/selfupdate/` is the same problem for the binary itself:
the latest tag from the `releases/latest` redirect, the checksum
before the download, an atomic rename, and two refusals (a local
build, a Homebrew cellar) documented in `CLAUDE.md`'s v0.4.1 entry.

Task source for the tutor: build from `f3ba646` with
`internal/scaffold/clonetarget_test.go` held. On the starting state
`CloneTarget` returns a `$HOME`-relative snippet for a repo inside
`$HOME` and the absolute path otherwise, so a repo scaffolded in a
`/tmp` build directory or a CI checkout bakes that path into
`bootstrap.sh`; e2e caught it, when the container went looking for
the host's temp directory. The rule was wrong, not the test: only a
location under `$HOME` means anything on another machine. The fix
makes `CloneTarget` return `(string, bool)`, `""` and false for
anything it cannot express relative to `$HOME` (outside it, `$HOME`
itself, an unknown `$HOME`), and both callers (`cmd/hm/init.go` and
`init_update.go`) keep the `$HOME/<repo>` default on false. The held
test does not compile on the starting state, since it takes two
results from a function that returns one; that is the expected
failure. The held line is `test ./internal/scaffold/ -run
TestCloneTarget`; verify with `build` (the callers must follow the
signature) and `vet ./cmd/hm/` besides, and not `vet` on
`internal/scaffold`: vet compiles the package's tests, the learner's
own copy of the held file among them, and with the fix in place that
copy still calls the one-result form, so the line fails on the
reference. Scope `internal/scaffold` and `cmd/hm`. The brief gives
the symptom, the rule, and the signature the test expects, and says
the package's existing test has to follow the new signature too; it
does not say what the callers do on false. The same commit added a `--repo-dir` flag, a
paragraph in `CLAUDE.md`, and the commands page; say in a sentence
that the flag was the original's follow-up and is not the task. The
starting state is before the rename, so the command layer is
`cmd/hm/`.

In the walkthrough, show `Answers.RepoDir`, the template line, and
`deriveAnswers`, and name `CloneTarget` as where the location is
derived, but not its body (`scaffold.go` lines 42 to 60 at the pin):
on the learner's branch that is the answer, for after `done`.

## Rubric

- Only a location strictly under `$HOME` is derived, as a
  `$HOME/…` snippet; anything else, `$HOME` itself and an unknown
  `$HOME` included, reports that nothing could be derived, and never
  an absolute path from this machine.
- Both callers keep the portable default when nothing is derived, so
  `homie init` and `homie init --update` agree on the same repo.
- The function's comment says why only `$HOME` is portable, since the
  next reader will otherwise "fix" the fallback back to an absolute
  path.
- A unit test pins the case e2e caught (a build directory outside
  `$HOME`), so it never needs a container to catch it again.

## Talk through

- Which files `homie init` writes are seeds and which is tool-owned,
  and what `--update` does to each.
- What the provenance stamp records, why a digest and not only a
  version, and what deleting the stamp line means.
- Why `bootstrap.sh` hands `/dev/tty` to each child one command at a
  time, and what `curl … | bash` does to a script's standard input.
