package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

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
	}
	if ran == 0 {
		fmt.Fprintln(w, "No scripts to run.")
		// If the user only asked for one phase but the other has scripts,
		// nudge them toward the right flag rather than leave them wondering.
		if len(phases) == 1 {
			if other := otherPhase(phases[0]); hasScripts(repoDir, other) {
				fmt.Fprintf(w, "Hint: %s-scripts exist — try `hm run --phase=%s`.\n", other, other)
			}
		}
		return nil
	}
	if failed > 0 {
		return fmt.Errorf("%d script(s) failed", failed)
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

// hasScripts reports whether any *.sh file in <repoDir>/scripts belongs
// to the given phase. Errors are swallowed — the caller only uses this
// to gate an optional hint message.
func hasScripts(repoDir string, phase runner.Phase) bool {
	matches, _ := filepath.Glob(filepath.Join(repoDir, runner.ScriptsDir, "*"+runner.Extension))
	for _, m := range matches {
		name := filepath.Base(m)
		isPre := strings.HasPrefix(name, runner.PrePrefix)
		if (phase == runner.PhasePre) == isPre {
			return true
		}
	}
	return false
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
