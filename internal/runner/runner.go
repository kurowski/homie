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
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/kurowski/homie/internal/config"
)

// ScriptsDir is the directory under the user repo that holds scripts.
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
type Phase int

const (
	PhasePre  Phase = iota // pre-*.sh
	PhasePost              // every other *.sh
)

// String returns the lowercase name used by `hm run --phase=<name>`.
func (p Phase) String() string {
	switch p {
	case PhasePre:
		return "pre"
	default:
		return "post"
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

// Run executes <repoDir>/scripts/*.sh matching the given phase in
// lexical order. Each script gets HM_REPO, HM_HOME, HM_TAGS
// (comma-joined) plus every cfg.Vars entry exported in its environment.
// Stdout and stderr are streamed to out so the user sees progress live.
//
// A missing scripts directory is a no-op (the user repo may legitimately
// have none). Individual script failures don't abort the run; they're
// collected in Result.Errors.
func Run(repoDir, home string, cfg config.Config, tags []string, phase Phase, out io.Writer) Result {
	var res Result
	dir := filepath.Join(repoDir, ScriptsDir)
	entries, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return res
	}
	if err != nil {
		res.Errors = append(res.Errors, fmt.Errorf("read %s: %w", dir, err))
		return res
	}

	var scripts []string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), Extension) {
			continue
		}
		if scriptPhase(e.Name()) != phase {
			continue
		}
		scripts = append(scripts, filepath.Join(dir, e.Name()))
	}
	sort.Strings(scripts)

	env := buildEnv(repoDir, home, cfg.Vars, tags)
	for _, path := range scripts {
		runErr := exec_(path, env, out)
		res.Ran = append(res.Ran, ScriptRun{Path: path, Err: runErr})
		if runErr != nil {
			res.Errors = append(res.Errors, fmt.Errorf("%s: %w", path, runErr))
		}
	}
	return res
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

// exec_ runs one script and streams its output. The trailing underscore
// avoids shadowing the os/exec import in code search.
func exec_(path string, env []string, out io.Writer) error {
	cmd := exec.Command("bash", path)
	cmd.Env = env
	cmd.Stdout = out
	cmd.Stderr = out
	return cmd.Run()
}
