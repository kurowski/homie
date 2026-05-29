package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/kurowski/homie/internal/config"
	"github.com/kurowski/homie/internal/detect"
	"github.com/kurowski/homie/internal/repo"
	"github.com/kurowski/homie/internal/runner"
	"github.com/spf13/cobra"
)

var (
	runHome  string
	runPhase string
)

var runCmd = &cobra.Command{
	Use:   "run",
	Short: "Run scripts/ in order",
	Long: `Run scripts/*.sh in lexical order. Each script gets a clean
bash subprocess with these environment variables set:

  HM_REPO   — absolute path of the user repo
  HM_HOME   — absolute path being treated as $HOME
  HM_TAGS   — comma-joined active tag set
  <each>    — every key from [vars] in homie.toml

Scripts are user code — Homie does not enforce idempotency. The
convention is for each script to no-op when its work is already done
(e.g. ` + "`command -v X >/dev/null && exit 0`" + ` at the top).

Run from a terminal, scripts inherit your stdin/stdout/stderr, so an
in-band prompt — a ` + "`sudo`" + ` password, ` + "`gh auth login`" + `, a package-manager
confirmation — reaches you directly. When stdin isn't a terminal (CI,
piped, redirected), output is captured per-script and stdin is
/dev/null, so a script that would block on a prompt fails fast instead
of hanging.

Tag-conditional trees: sibling directories named ` + "`scripts.tag-X[.tag-Y]/`" + `
run only when all of their tags are active (AND), mirroring the
` + "`home.tag-X/`" + ` convention. Plain ` + "`scripts/`" + ` always runs. Files are
ordered by filename across every active tree — the numeric prefix is the
single global order, so ` + "`scripts.tag-fedora/05-x.sh`" + ` runs at position
05 alongside ` + "`scripts/04-y.sh`" + `. The same filename in two active trees
is an error.

Phases:

  --phase=post   (default) every script whose name does NOT start with
                 "pre-". The scripts step of ` + "`hm apply`" + `.
  --phase=pre    only pre-*.sh scripts — the step that runs ahead of
                 the package install in ` + "`hm apply`" + `. Used for adding
                 third-party package sources (dnf COPRs, apt keyrings,
                 RPM Fusion, flatpak remote setup, ...).
  --phase=all    pre-scripts then post-scripts, matching the order
                 ` + "`hm apply`" + ` uses.

A failing script doesn't abort the rest of the phase — failures are
collected and surfaced in a non-zero exit code at the end.`,
	RunE: runRun,
}

func init() {
	runCmd.Flags().StringVar(&runHome, "home", "", "override target home directory (default $HOME)")
	runCmd.Flags().StringVar(&runPhase, "phase", "post", "which scripts to run: pre, post, or all")
	rootCmd.AddCommand(runCmd)
}

func runRun(cmd *cobra.Command, args []string) error {
	phases, err := parseRunPhase(runPhase)
	if err != nil {
		return err
	}
	repoDir, err := repo.Find()
	if err != nil {
		return err
	}
	home := runHome
	if home == "" {
		home, err = os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("resolve home: %w", err)
		}
	}
	env := detect.Detect()
	cfg, err := config.Load(repoDir, env.Hostname)
	if err != nil {
		return err
	}

	w := cmd.OutOrStdout()
	tags := cfg.AllTags(env)
	var ran, failed int
	for i, p := range phases {
		if len(phases) > 1 {
			if i > 0 {
				fmt.Fprintln(w)
			}
			fmt.Fprintf(w, "== %s-scripts ==\n", p)
		}
		res := runner.Run(repoDir, home, cfg, tags, p, w)
		ran += len(res.Ran)
		failed += len(res.Errors)
		for _, r := range res.Ran {
			status := "ok"
			if r.Err != nil {
				status = "fail"
			}
			fmt.Fprintf(w, "  %-5s %s\n", status, filepath.Base(r.Path))
		}
		// Errors with no corresponding script run (e.g. a filename
		// collision between active tag trees) are pre-flight failures —
		// print them so the user sees the cause, not just a count. When
		// scripts did run, their stderr already streamed live and each
		// shows a "fail" line, so the bare error here would be redundant.
		if len(res.Ran) == 0 {
			for _, e := range res.Errors {
				fmt.Fprintf(w, "  error %s\n", e)
			}
		}
	}
	// Surface failures before the "nothing to run" message: a pre-flight
	// error (collision) leaves ran == 0 but must not be swallowed.
	if failed > 0 {
		return fmt.Errorf("%d script error(s)", failed)
	}
	if ran == 0 {
		fmt.Fprintln(w, "No scripts to run.")
		// If the user only asked for one phase but the other has scripts,
		// nudge them toward the right flag rather than leave them wondering.
		if len(phases) == 1 {
			if other := otherPhase(phases[0]); hasScripts(repoDir, tags, other) {
				fmt.Fprintf(w, "Hint: %s-scripts exist — try `hm run --phase=%s`.\n", other, other)
			}
		}
	}
	return nil
}

// otherPhase returns the phase opposite to p — used to suggest the flag
// when `hm run` runs the requested phase and finds nothing.
func otherPhase(p runner.Phase) runner.Phase {
	if p == runner.PhasePre {
		return runner.PhasePost
	}
	return runner.PhasePre
}

// hasScripts reports whether any *.sh file in an active script tree
// (scripts/ or an active scripts.tag-X sibling) belongs to the given
// phase. Errors are swallowed — the caller only uses this to gate an
// optional hint message.
func hasScripts(repoDir string, tags []string, phase runner.Phase) bool {
	paths, _ := runner.Plan(repoDir, tags, phase)
	return len(paths) > 0
}

// parseRunPhase resolves the --phase flag to one or two runner.Phase
// values, preserving execution order when "all" is requested.
func parseRunPhase(v string) ([]runner.Phase, error) {
	switch v {
	case "pre":
		return []runner.Phase{runner.PhasePre}, nil
	case "post":
		return []runner.Phase{runner.PhasePost}, nil
	case "all":
		return []runner.Phase{runner.PhasePre, runner.PhasePost}, nil
	default:
		return nil, fmt.Errorf("--phase must be one of pre, post, all; got %q", v)
	}
}
