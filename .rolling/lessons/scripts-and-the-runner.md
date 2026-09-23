---
title: Scripts and the runner
region: scripts
depth: working
mode: write
requires: [how-a-change-ships]
assumes: [go, bash]
test: held
---

What `apply` runs that is not Homie's own code: the user's scripts,
and the git checkouts `[externals]` declares. How each is found, in
what order, with what environment, and how a script that wants a
password reaches a human.

Scripts are user code, and the runner says so in its first comment
(`internal/runner/runner.go` lines 1 to 8): Homie finds them, orders
them, runs each with `bash`, and reports which failed; idempotency is
the script author's job. A script is any `*.sh` directly in `scripts/`
or in an active `scripts.tag-X/` sibling (the same tag-tree discovery
the home tree uses, `tree.Active`). A `pre-` prefix puts it in the
phase before packages (`PrePrefix`, line 51, and `Phase`, lines 53 to
65), for things `[packages]` needs first: a COPR, a signed-by keyring.
`Plan` (line 158) is the one place the order is decided, and `Run`,
`doctor`, and `homie run` all go through it: the filename orders the
whole set across every active tree, and two active trees offering the
same filename is an error, not an override, because a script has no
"more specific" version the way a dotfile does.

A script's environment is the contract the user writes against. `Run`
(line 125) builds it once per phase and hands it to every script: the
parent's environment, then `HOMIE_REPO`, `HOMIE_HOME`, `HOMIE_TAGS`,
and every `[vars]` entry from `homie.toml`, later entries winning.
`internal/scaffold/files/scripts/01-shell.sh`, the example every new
repo gets, shows what users write against it: `set -euo pipefail`,
then `"$USER"`.

A script that prompts is the hard case. `exec_` (line 234) runs one
script, and its comment explains both branches: on a terminal the
script gets the parent's stdin, stdout, and stderr, so `sudo` can ask
for a password, and `apply` releases the TUI around the phase for it
(`cmd/homie/apply.go`, `applyScriptPhase`, lines 312 to 360);
otherwise output is captured, stdin is `/dev/null`, and the script
runs under `setsid` with no controlling terminal, so a `sudo` that
would read `/dev/tty` fails at once instead of hanging where nobody
can see it. `b21d3ba` (#31) is the change that made it so, and its
body is worth reading for why redirecting stdin alone was not enough.

Externals are the declarative version of a clone script.
`internal/config/externals.go` parses `[externals]` into specs
(`ExternalsFor`, line 146, with the tag gating), and
`internal/externals/externals.go` keeps each one converged: `Sync`
(line 85) clones what is missing, fast-forwards what tracks a branch
(`trackUpdate`, 172), and holds what is pinned (`pinnedUpdate`, 206),
through its own injected `Runner` (line 62) so the tests fake `git`.
The phase runs before the home tree, so a template or a symlink can
point into a checkout that is guaranteed to exist (`apply`'s `Long`
says so).

Task source for the tutor: build from `d9a9c24` (#29, closing #26)
with `internal/runner/runner_test.go` held. In a devcontainer, a bare
`docker exec`, some CI runners, and systemd services, `$USER` and
`$LOGNAME` are unset even though the OS knows the user, so the
scaffolded `01-shell.sh` dies on `USER: unbound variable` under `set
-u`. The fix, in `buildEnv`, fills both from `os/user.Current()` when
the inherited environment lacks them, and never overwrites one that is
set. The held tests are `TestBuildEnvNormalizesUser` and
`TestBuildEnvKeepsExistingUser`; the held line is `test
./internal/runner/ -run TestBuildEnv`. Verify with `build` and `vet
./internal/runner/` besides; scope `internal/runner`. The starting
state is before the rename and before #31: the variables are
`HM_REPO`, `HM_HOME`, `HM_TAGS`, `exec_` has no interactive branch,
and the command layer is `cmd/hm/`. The brief gives the symptom and
the rule (filled when missing, kept when set, both names); it does not
name `os/user`.

In the walkthrough, show `Run`, `Plan`, and `exec_`, and name
`buildEnv` as where the environment is built, but not its
normalisation (`runner.go` lines 201 to 217 at the pin): on the
learner's branch that is the answer, for after `done`.

## Rubric

- `USER` and `LOGNAME` are each filled from the OS's own idea of the
  current user when, and only when, the inherited environment lacks
  them; a value that is set, even to something else, is kept.
- The change is in `buildEnv`, the one place a script's environment
  is composed, so every phase and `homie run` get it.
- A failure to look the user up leaves the environment as it was
  rather than failing the run.
- The comment says which environments leave these unset and why Homie
  fills them, since a reader will otherwise think it redundant.

## Talk through

- Why a script is run with `bash` and not executed directly, and what
  that means for its shebang and its mode.
- Why two active trees with the same script name is an error for
  scripts and an override for dotfiles.
- What `setsid` changes for a script run without a terminal, and why
  `</dev/null` alone would not have been enough.
