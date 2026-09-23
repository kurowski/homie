---
title: The home tree, and a pass that changes nothing
region: config
depth: working
mode: write
requires: [how-a-change-ships]
assumes: [go, go-templates, toml]
test: held
---

What a user's repo says, and how it becomes files in `$HOME`: the
config, the tags that select parts of it, the tree of dotfiles, and
the two things Homie does with each file, a symlink or a rendered
template. And the rule that holds all of it together: a second
`apply` on a machine where nothing changed does nothing, and says so.

The config. `config.Load` (`internal/config/config.go` line 353)
decodes `homie.toml`, then merges `hosts/<hostname>.toml` over it when
one exists (`merge`, line 406), then validates. Unknown fields are
warnings, not errors (`loadFile`, line 382), because a typo should not
stop a machine from converging; `testdata/unknown-field/` pins that.
Tags are the selector for everything else: the ones the user declares,
plus the auto-tags `detect` adds (distro, architecture, hostname), all
in `AllTags` (line 651). `[packages]` keys can be tags too, and an AND
of tags (`tag:work.tag:ubuntu`), parsed into a canonical form by
`parseTagKey` (line 273).

The tree. `internal/tree/tree.go` owns the convention: `home/` is
always active, a sibling `home.tag-X[.tag-Y…]/` is active when every
tag it names is (`Classify`, line 62; `ParseDir`, line 200). `Resolve`
(line 135) walks every active tree and decides, per `$HOME` target,
which source wins: the more specific tree (more tags) silently, and
two equally specific ones are an error the user has to resolve. Its
comment (lines 119 to 134) is the rule in full, with an example.
`CLAUDE.md`, "The home/ tree (internal model)", says why the rule is
enforced exactly once.

The two consumers. `link.Plan` (`internal/link/link.go` line 66) takes
the non-templates and classifies each target (`classify`, line 87):
nothing there is `create`, the right symlink is `skip`, a symlink
elsewhere is `replace`, a real file is `backup` and then replace, with
the backup timestamped. `render.Apply` (`internal/render/render.go`
line 130) takes the `.tmpl` files and writes the rendered output as a
real file, not a link, because its content comes from data. `Render`
(line 76) is Go's `text/template` with Sprig, a `hasTag` helper, and
`missingkey=error`, so a typo in a field fails loudly instead of
writing `<no value>` into someone's `.gitconfig`. `Data` (line 37) is
what a template sees, and `homie context` prints it as JSON.

"No state" means each of these decides from what is on disk, every
run. For `link` that is cheap: a symlink either points at the source
or it does not. `applyHomePhase` (`cmd/homie/apply.go` line 276)
reports each kind of action and totals the ones that did nothing as
`skip N already in sync`.

Task source for the tutor: build from `f352881` with
`internal/render/render_test.go` held. On the starting state `render`
rewrote every template on every `apply`, so a converged machine
reported `render ~/.gitconfig` on every pass. The fix compares the
rendered bytes and the source's permission bits with what is at the
target and, when both match and the target is a regular file, writes
nothing and reports it as skipped; a symlink at the target is still
replaced. `render.Result` gains `Skipped` beside `Rendered`, the way
`link.Result` already had one, and `hm apply` and `hm render` print
`skip N already in sync`. The held tests are
`TestApplySkipsAlreadyInSync`, `TestApplyRewritesWhenContentDiffers`,
and `TestApplyRewritesWhenModeDiffers`; they read `res.Skipped`, so
the brief names that field and says it mirrors `link`'s. The held line
is `test ./internal/render/`. Verify with `build` and `vet
./internal/render/ ./cmd/hm/` besides; scope `internal/render` and
`cmd/hm`. The starting state is two days older than the home tree as
it is now: templates lived in their own `templates/` directory,
walked by `render.Apply` directly, `link` and `render` were separate
phases (`applyLink`, `applyRender` in `cmd/hm/apply.go`), and
`applyLink` already reported `skip`. Say so in the brief, in a line,
so the learner is not hunting for `tree.Resolve`. The same commit
dropped a package from the scaffold's seed `homie.toml`; that is not
part of the task.

In the walkthrough, show `render.Apply` and `Render`, not
`renderFile`'s in-sync check and `inSync` (`render.go` lines 157 to
207 at the pin): on the learner's branch those are the answer, for
after `done`.

## Rubric

- A second `apply` on an unchanged machine writes nothing in the
  render phase and says `skip`; a changed template, a changed mode,
  or a symlink at the target is still written.
- "In sync" compares the rendered output, not the template source,
  and the permission bits, not the whole mode; a symlink or a
  non-regular file at the target is never in sync.
- `render.Result` reports skipped targets beside rendered ones, in
  the same shape as `link.Result`, and both commands that render
  report them.
- No record of the last run is kept anywhere; the decision is made
  from the target on disk.

## Talk through

- How a host overlay and a tag tree differ, and which a user reaches
  for when one laptop needs a different `.gitconfig`.
- What `Resolve` does when `home/.gitconfig` and
  `home.tag-work/.gitconfig.tmpl` both exist on a work machine, and
  when two single-tag trees both have one.
- Why a template is rendered to a real file and a dotfile is linked,
  and what that means for a user who edits `~/.gitconfig` by hand.
