// Package externals keeps declared external git repositories present
// and current on disk: cloned when missing, updated in place when
// already checked out. It is the engine behind the `externals` phase of
// `hm apply` and the [externals] table in homie.toml.
//
// Idempotency follows the same no-state-file rule as the rest of Homie:
// every run inspects the checkout directly (HEAD, remote URL, the
// declared ref) and does only the work needed to converge. A pinned ref
// is checked out detached and held — it never moves until the declared
// value changes. An unpinned entry tracks the remote default branch and
// fast-forwards on each run; a checkout that can't fast-forward (local
// commits, dirty tree) fails rather than clobbering local state.
package externals

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Spec is one external to sync. Dest is the destination path as
// declared in homie.toml (~/..., $HOME/..., or absolute); Repo is the
// clone URL; Ref optionally pins a branch, tag, or commit.
type Spec struct {
	Dest string
	Repo string
	Ref  string
}

// Kind classifies what Sync did for one spec.
type Kind string

const (
	KindClone  Kind = "clone"  // destination was missing; cloned fresh
	KindUpdate Kind = "update" // checkout existed; HEAD moved
	KindSkip   Kind = "skip"   // checkout existed; already converged
)

// Action records the outcome for one successfully synced spec.
type Action struct {
	Spec   Spec
	Dest   string // absolute destination after ~/$HOME expansion
	Kind   Kind
	Detail string // human extra: source repo, old -> new, "up to date"
}

// Result aggregates an Apply run. Specs that fail land in Errors (one
// entry each, with the destination in the message) and produce no
// Action; the rest proceed regardless, matching the non-fatal phase
// behavior of the other apply phases.
type Result struct {
	Actions []Action
	Errors  []error
}

// Runner executes an external command and returns its combined output.
// Tests inject a fake; production uses ExecRunner.
type Runner func(name string, args ...string) ([]byte, error)

// ExecRunner runs the command for real via os/exec.
func ExecRunner(name string, args ...string) ([]byte, error) {
	return exec.Command(name, args...).CombinedOutput()
}

// Apply syncs every spec, expanding destinations against home.
func Apply(home string, specs []Spec, run Runner) Result {
	var res Result
	for _, s := range specs {
		action, err := Sync(home, s, run)
		if err != nil {
			res.Errors = append(res.Errors, err)
			continue
		}
		res.Actions = append(res.Actions, action)
	}
	return res
}

// Sync converges one spec: clone if the destination is missing, update
// if it's a checkout of the declared repo, error if it's anything else.
func Sync(home string, s Spec, run Runner) (Action, error) {
	dest, err := expand(s.Dest, home)
	if err != nil {
		return Action{}, err
	}
	// Lstat, not Stat: a symlink at dest — dangling or not — must be
	// refused, never followed. A dangling one would otherwise look
	// absent and the clone would write through it into wherever it
	// points.
	info, err := os.Lstat(dest)
	if err != nil {
		if !errors.Is(err, fs.ErrNotExist) {
			return Action{}, fmt.Errorf("stat %s: %w", dest, err)
		}
		return clone(dest, s, run)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return Action{}, fmt.Errorf("%s is a symlink — refusing to operate through it (point the entry at the real path)", dest)
	}
	// Destination exists: it must be a git checkout of the declared
	// repo. Anything else is the user's data — never clobber it.
	// (.git may be a file for worktrees, so existence is the test.)
	if _, err := os.Stat(filepath.Join(dest, ".git")); err != nil {
		return Action{}, fmt.Errorf("%s exists but is not a git repository — refusing to touch it (remove it or change the destination)", dest)
	}
	origin, err := gitRun(run, "-C", dest, "remote", "get-url", "origin")
	if err != nil {
		return Action{}, err
	}
	if normalizeURL(origin) != normalizeURL(s.Repo) {
		return Action{}, fmt.Errorf("%s tracks %s but homie.toml declares %s — update the entry or remove the checkout", dest, origin, s.Repo)
	}
	if s.Ref == "" {
		return trackUpdate(dest, s, run)
	}
	return pinnedUpdate(dest, s, run)
}

// expand resolves a declared destination to an absolute path under
// home. Only unambiguous forms are accepted; a bare relative path has
// no obvious base, so it's an error rather than a guess.
func expand(dest, home string) (string, error) {
	var out string
	switch {
	case strings.HasPrefix(dest, "~/"):
		out = filepath.Join(home, dest[2:])
	case strings.HasPrefix(dest, "$HOME/"):
		out = filepath.Join(home, strings.TrimPrefix(dest, "$HOME/"))
	case filepath.IsAbs(dest):
		out = filepath.Clean(dest)
	default:
		return "", fmt.Errorf("externals destination %q must start with ~/ or $HOME/ or be an absolute path", dest)
	}
	// "~/" and "/" pass the prefix checks but mean cloning into $HOME
	// or the filesystem root — always a config typo; refuse it here
	// with a clear message instead of relaying git's confusing one.
	if out == filepath.Clean(home) || out == string(os.PathSeparator) {
		return "", fmt.Errorf("externals destination %q resolves to %s itself — declare a subdirectory", dest, out)
	}
	return out, nil
}

// clone materializes a missing destination. Unpinned clones are shallow
// — the checkout exists to be used, not developed in. Pinned clones are
// full: `--branch` would cover branches and tags but not commit SHAs,
// so one full-history path handles all three ref kinds uniformly, then
// detaches at the ref so nothing moves it until the pin changes.
func clone(dest string, s Spec, run Runner) (Action, error) {
	if s.Ref == "" {
		if _, err := gitRun(run, "clone", "--depth", "1", s.Repo, dest); err != nil {
			return Action{}, err
		}
		return Action{Spec: s, Dest: dest, Kind: KindClone, Detail: s.Repo}, nil
	}
	if _, err := gitRun(run, "clone", s.Repo, dest); err != nil {
		return Action{}, err
	}
	if _, err := gitRun(run, "-C", dest, "checkout", "--detach", s.Ref); err != nil {
		return Action{}, err
	}
	return Action{Spec: s, Dest: dest, Kind: KindClone, Detail: s.Repo + " @ " + s.Ref}, nil
}

// trackUpdate fast-forwards an unpinned checkout to its remote default
// branch. A detached HEAD here means the entry used to be pinned; it is
// re-attached to the default branch first (resolving it may ask the
// remote) so the pull has something to fast-forward.
func trackUpdate(dest string, s Spec, run Runner) (Action, error) {
	before, err := gitRun(run, "-C", dest, "rev-parse", "HEAD")
	if err != nil {
		return Action{}, err
	}
	branch, berr := gitRun(run, "-C", dest, "symbolic-ref", "--short", "-q", "HEAD")
	if berr != nil || branch == "" {
		branch, err = defaultBranch(dest, run)
		if err != nil {
			return Action{}, err
		}
		if _, err := gitRun(run, "-C", dest, "checkout", branch); err != nil {
			return Action{}, err
		}
	}
	// Name the remote and branch explicitly so a stray pull.default or
	// rewired upstream in the checkout can't redirect the update.
	if _, err := gitRun(run, "-C", dest, "pull", "--ff-only", "origin", branch); err != nil {
		return Action{}, err
	}
	after, err := gitRun(run, "-C", dest, "rev-parse", "HEAD")
	if err != nil {
		return Action{}, err
	}
	if before == after {
		return Action{Spec: s, Dest: dest, Kind: KindSkip, Detail: "up to date"}, nil
	}
	return Action{Spec: s, Dest: dest, Kind: KindUpdate, Detail: short(before) + " -> " + short(after)}, nil
}

// pinnedUpdate holds a checkout at the declared ref: nothing to do when
// HEAD already points at it, otherwise check the ref out detached. The
// network is only touched when the ref can't be resolved locally (a new
// or moved pin since the last fetch).
func pinnedUpdate(dest string, s Spec, run Runner) (Action, error) {
	head, err := gitRun(run, "-C", dest, "rev-parse", "HEAD")
	if err != nil {
		return Action{}, err
	}
	want, err := gitRun(run, "-C", dest, "rev-parse", s.Ref+"^{commit}")
	if err != nil {
		if ferr := fetch(dest, run); ferr != nil {
			return Action{}, ferr
		}
		want, err = gitRun(run, "-C", dest, "rev-parse", s.Ref+"^{commit}")
		if err != nil {
			return Action{}, fmt.Errorf("%s: ref %q not found in %s", dest, s.Ref, s.Repo)
		}
	}
	if head == want {
		return Action{Spec: s, Dest: dest, Kind: KindSkip, Detail: "pinned at " + s.Ref}, nil
	}
	if _, err := gitRun(run, "-C", dest, "checkout", "--detach", s.Ref); err != nil {
		return Action{}, err
	}
	// Deliberately asymmetric: where we were (a commit) -> what was
	// asked for (the ref name). The ref reads better than its hash.
	return Action{Spec: s, Dest: dest, Kind: KindUpdate, Detail: short(head) + " -> " + s.Ref}, nil
}

// fetch pulls down new refs for a pinned checkout. A shallow checkout
// (the entry was unpinned once) is unshallowed first, since the newly
// pinned ref may point anywhere in history.
func fetch(dest string, run Runner) error {
	args := []string{"-C", dest, "fetch", "--tags", "origin"}
	if _, err := os.Stat(filepath.Join(dest, ".git", "shallow")); err == nil {
		args = append(args, "--unshallow")
	}
	_, err := gitRun(run, args...)
	return err
}

// defaultBranch names the remote default branch (e.g. "main") recorded
// in origin/HEAD, asking the remote to (re)set it if the local clone
// doesn't have one.
func defaultBranch(dest string, run Runner) (string, error) {
	ref, err := gitRun(run, "-C", dest, "rev-parse", "--abbrev-ref", "origin/HEAD")
	if err != nil {
		if _, err := gitRun(run, "-C", dest, "remote", "set-head", "origin", "--auto"); err != nil {
			return "", err
		}
		ref, err = gitRun(run, "-C", dest, "rev-parse", "--abbrev-ref", "origin/HEAD")
		if err != nil {
			return "", err
		}
	}
	return strings.TrimPrefix(ref, "origin/"), nil
}

// gitRun executes git and returns trimmed output, folding the output
// into the error on failure — git's stderr is the diagnosis the user
// needs (auth failures, unknown refs, non-fast-forward).
func gitRun(run Runner, args ...string) (string, error) {
	out, err := run("git", args...)
	trimmed := strings.TrimSpace(string(out))
	if err != nil {
		if trimmed != "" {
			return "", fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, trimmed)
		}
		return "", fmt.Errorf("git %s: %w", strings.Join(args, " "), err)
	}
	return trimmed, nil
}

// normalizeURL canonicalizes a clone URL just enough that the usual
// cosmetic variants (trailing slash, trailing .git) compare equal. A
// scheme change (https vs ssh) still mismatches deliberately: the
// declared URL is the one the user wants the checkout to use.
func normalizeURL(u string) string {
	u = strings.TrimSpace(u)
	u = strings.TrimSuffix(u, "/")
	return strings.TrimSuffix(u, ".git")
}

func short(hash string) string {
	if len(hash) > 7 {
		return hash[:7]
	}
	return hash
}
