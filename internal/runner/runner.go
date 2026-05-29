// Package runner executes the ordered scripts under <repo>/scripts/.
//
// Scripts are user code — Homie doesn't enforce idempotency, that's the
// convention each script's author should follow (typically with a guard
// like `command -v X >/dev/null && exit 0` at the top). The runner's job
// is to find the scripts, sort them lexically, invoke each one as a
// bash subprocess with the right environment, and report which ones
// failed so the rest of `hm apply` can keep going.
package runner

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/kurowski/homie/internal/config"
	"github.com/kurowski/homie/internal/tree"
	"golang.org/x/term"
)

// stdinIsTTY reports whether the process's stdin is an interactive terminal.
// Indirected through a var so tests can drive both the interactive and
// captured branches without a pseudo-terminal.
var stdinIsTTY = func() bool { return term.IsTerminal(int(os.Stdin.Fd())) }

// Interactive reports whether Run will hand the terminal through to scripts
// (i.e. stdin is a TTY). The apply command uses it to release a live TUI's
// hold on the terminal around the script phase so an interactive prompt
// (sudo, gh auth login, ...) inside a script can reach the user.
func Interactive() bool { return stdinIsTTY() }

// ScriptsDir is the bare directory under the user repo that holds scripts.
// Sibling directories named `scripts.tag-X[.tag-Y...]` are additional
// tag-conditional script trees — they run only when all their tags are
// active. This mirrors the home/ tree convention (see internal/tree).
const ScriptsDir = "scripts"

// Extension is the suffix scripts must have to be picked up by Run.
// Other files (READMEs, libs, partials) sit in scripts/ untouched.
const Extension = ".sh"

// PrePrefix marks a script as belonging to the pre-package phase. Scripts
// whose basename starts with this prefix run before `[packages]` install;
// every other *.sh script runs in the post-package phase.
const PrePrefix = "pre-"

// Phase selects which set of scripts Run executes. The pre-package phase
// is for installing third-party package sources (dnf COPRs, apt
// signed-by keyrings, RPM Fusion, flatpak remotes) that must exist before
// `[packages]` resolves — see issue #2 for the rationale.
//
// PhasePost is iota (the zero value) so a default-constructed Phase
// matches `hm run`'s default.
type Phase int

const (
	PhasePost Phase = iota // every *.sh whose basename does not start with pre-
	PhasePre               // pre-*.sh
)

// String returns the lowercase name used by `hm run --phase=<name>`.
func (p Phase) String() string {
	switch p {
	case PhasePre:
		return "pre"
	case PhasePost:
		return "post"
	default:
		return fmt.Sprintf("phase(%d)", int(p))
	}
}

// scriptPhase classifies a script by filename. Lowercase comparison
// matches the convention but doesn't try to canonicalize — users
// naming their files `pre-foo.sh` get the pre phase; `Pre-foo.sh`
// (capital P) does not. Keeping it case-sensitive matches the unix
// filesystem convention.
func scriptPhase(name string) Phase {
	if strings.HasPrefix(name, PrePrefix) {
		return PhasePre
	}
	return PhasePost
}

// ScriptRun records the outcome of one script invocation.
type ScriptRun struct {
	Path string // absolute path of the script
	Err  error  // nil on success; non-nil if bash exited non-zero
}

// Result aggregates the outcome of Run.
type Result struct {
	Ran    []ScriptRun
	Errors []error
}

// Run executes the *.sh scripts matching the given phase from the active
// script trees (scripts/ plus any active scripts.tag-X siblings), ordered
// by filename across all trees. Each script gets HM_REPO, HM_HOME, HM_TAGS
// (comma-joined) plus every cfg.Vars entry exported in its environment.
//
// Script stdio depends on whether stdin is a terminal:
//   - Interactive (stdin is a TTY): the script inherits the parent's
//     stdin/stdout/stderr, so an in-band prompt (sudo password, gh auth
//     login, package-manager confirmation) reaches the user. Homie doesn't
//     wrap the output in this mode — scripts are user code and own their
//     terminal. Callers running a live TUI must release it first (see
//     [Interactive]).
//   - Non-interactive (CI, piped, stdin redirected): stdout/stderr are
//     captured to out and stdin is /dev/null, so a script that would block
//     on a prompt fails fast instead of hanging silently.
//
// No script trees is a no-op (the user repo may legitimately have none).
// A filename collision between two active trees is a fatal error returned
// in Result.Errors before any script runs. Individual script failures
// don't abort the run; they're collected in Result.Errors too.
func Run(repoDir, home string, cfg config.Config, tags []string, phase Phase, out io.Writer) Result {
	var res Result
	scripts, err := Plan(repoDir, tags, phase)
	if err != nil {
		res.Errors = append(res.Errors, err)
		return res
	}

	env := buildEnv(repoDir, home, cfg.Vars, tags)
	interactive := stdinIsTTY()
	for _, path := range scripts {
		runErr := exec_(path, env, out, interactive)
		res.Ran = append(res.Ran, ScriptRun{Path: path, Err: runErr})
		if runErr != nil {
			res.Errors = append(res.Errors, fmt.Errorf("%s: %w", path, runErr))
		}
	}
	return res
}

// Plan returns the absolute paths of the scripts Run would execute for
// the given phase, in execution order — the bare scripts/ tree plus any
// active scripts.tag-X[.tag-Y...] siblings, filtered to phase. It returns
// an error if two active trees provide the same filename. Run, doctor,
// and the `hm run` hint all go through Plan so they stay in sync.
//
// Tree discovery is shared with the home/ tree via tree.Active. The merge
// rule, though, differs from tree.Resolve: scripts have no override/
// more-specific-wins semantic, so any two active trees offering the same
// filename is a hard error rather than a silent win. The numeric filename
// prefix is the single global ordering across all trees — the tag trees
// only decide which files participate, they don't fork into separate
// ordered streams.
func Plan(repoDir string, tags []string, phase Phase) ([]string, error) {
	roots, err := tree.Active(repoDir, ScriptsDir, tags)
	if err != nil {
		return nil, err
	}
	byName := make(map[string]string) // filename -> winning absolute path
	for _, root := range roots {
		entries, err := os.ReadDir(root)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", root, err)
		}
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), Extension) {
				continue
			}
			if scriptPhase(e.Name()) != phase {
				continue
			}
			path := filepath.Join(root, e.Name())
			if prev, clash := byName[e.Name()]; clash {
				return nil, fmt.Errorf("script %s is provided by both %s and %s — rename one, or narrow a tag tree so only one applies",
					e.Name(), tree.RelTo(repoDir, prev), tree.RelTo(repoDir, path))
			}
			byName[e.Name()] = path
		}
	}

	scripts := make([]string, 0, len(byName))
	for _, path := range byName {
		scripts = append(scripts, path)
	}
	// Order by filename, not full path: a script's numeric prefix is its
	// position in the unified order regardless of which tree it lives in.
	sort.Slice(scripts, func(i, j int) bool {
		return filepath.Base(scripts[i]) < filepath.Base(scripts[j])
	})
	return scripts, nil
}

// buildEnv composes the environment passed to each script. The parent
// process's environment is inherited so PATH, HOME, USER etc. still
// work; Homie's variables are appended (later entries win in os/exec).
func buildEnv(repoDir, home string, vars map[string]string, tags []string) []string {
	env := os.Environ()
	env = append(env,
		"HM_REPO="+repoDir,
		"HM_HOME="+home,
		"HM_TAGS="+strings.Join(tags, ","),
	)
	for k, v := range vars {
		env = append(env, k+"="+v)
	}
	return env
}

// exec_ runs one script. When interactive, it hands the parent's terminal
// to the script (stdin/stdout/stderr) so prompts like sudo work; otherwise
// it captures output to out and leaves stdin at /dev/null. The trailing
// underscore avoids shadowing the os/exec import in code search.
func exec_(path string, env []string, out io.Writer, interactive bool) error {
	cmd := exec.Command("bash", path)
	cmd.Env = env
	if interactive {
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
	} else {
		// cmd.Stdin stays nil → /dev/null, so a script blocking on a prompt
		// fails fast rather than hanging on an invisible read.
		cmd.Stdout = out
		cmd.Stderr = out
	}
	return cmd.Run()
}
