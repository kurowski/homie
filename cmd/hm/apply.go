package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/kurowski/homie/internal/config"
	"github.com/kurowski/homie/internal/detect"
	"github.com/kurowski/homie/internal/link"
	"github.com/kurowski/homie/internal/packages"
	"github.com/kurowski/homie/internal/render"
	"github.com/kurowski/homie/internal/repo"
	"github.com/kurowski/homie/internal/runner"
	"github.com/kurowski/homie/internal/ui"
	"github.com/spf13/cobra"
)

var (
	applyHome         string
	applySkipPackages bool
	applySkipScripts  bool
)

var applyCmd = &cobra.Command{
	Use:   "apply",
	Short: "Full reconciliation: detect → pre-scripts → packages → backends → home → scripts",
	Long: `Apply the user environment repo end-to-end. Each phase is
idempotent — running twice in a row produces no work on the second run.

Phases, in order:

  1. detect       — distro, arch, container, hostname, tags
  2. config       — load homie.toml (+ hosts/<hostname>.toml overlay)
  3. pre-scripts  — pre-*.sh from scripts/ and active scripts.tag-X/
                    siblings (third-party repo setup, GPG keys, ...)
  4. packages     — install [packages] via the native manager (apt/dnf
                    on Linux, brew on macOS); a missing brew warns and
                    skips rather than failing
  5. backends     — install each declared backend ([packages.brew],
                    [packages.flatpak], ...); a phase skips with a
                    warning when its tool isn't on PATH
  6. home         — symlink plain files and render *.tmpl files from
                    home/ (and active home.tag-X/ siblings) into $HOME
  7. scripts      — non-pre *.sh from scripts/ and active scripts.tag-X/
                    siblings, ordered by filename across all trees

Non-fatal errors are collected and surfaced in the summary rather than
aborting. ` + "`hm apply`" + ` exits non-zero if any error was collected.

Flags ` + "`--skip-packages`" + ` and ` + "`--skip-scripts`" + ` cover the umbrella phases —
` + "`--skip-packages`" + ` covers native and all backend phases, ` + "`--skip-scripts`" + `
covers pre-scripts and post-scripts.

See https://homie.sh/docs/commands/ for a fuller treatment.`,
	RunE: runApply,
}

func init() {
	applyCmd.Flags().StringVar(&applyHome, "home", "", "override target home directory (default $HOME)")
	applyCmd.Flags().BoolVar(&applySkipPackages, "skip-packages", false, "skip the native and non-native (brew, flatpak, snap) package phases")
	applyCmd.Flags().BoolVar(&applySkipScripts, "skip-scripts", false, "skip the run-scripts phase")
	rootCmd.AddCommand(applyCmd)
}

func runApply(cmd *cobra.Command, args []string) error {
	repoDir, err := repo.Find()
	if err != nil {
		return err
	}
	home := applyHome
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

	noTTY, _ := cmd.Root().PersistentFlags().GetBool("no-tty")
	u := ui.New(cmd.OutOrStdout(), noTTY)
	defer func() { _ = u.Close() }()

	var errs []error
	errs = append(errs, applyScriptPhase(u, repoDir, home, cfg, env, runner.PhasePre)...)
	errs = append(errs, applyPackages(u, cfg, env)...)
	for _, backend := range declaredBackends(cfg) {
		errs = append(errs, applyBackendPackages(u, cfg, env, backend)...)
	}
	errs = append(errs, applyHomePhase(u, repoDir, home, cfg, env)...)
	errs = append(errs, applyScriptPhase(u, repoDir, home, cfg, env, runner.PhasePost)...)

	u.Summary(errs)
	if len(errs) > 0 {
		return fmt.Errorf("%d error(s) during apply", len(errs))
	}
	return nil
}

func applyPackages(u ui.UI, cfg config.Config, env detect.Env) []error {
	u.Phase("packages")
	if applySkipPackages {
		u.Info("skipped (--skip-packages)")
		return nil
	}
	pkgs := cfg.PackagesFor(env)
	if len(pkgs) == 0 {
		u.Info("no packages declared")
		return nil
	}
	mgr := packages.For(env)
	if mgr.Name() == "noop" {
		// TODO(contrib): add support for additional package managers.
		u.Warn(fmt.Sprintf("distro %q not yet supported — see homie.sh/contributing", env.Distro))
		return nil
	}
	if !mgr.IsAvailable() {
		if mgr.Name() == "brew" {
			// brew is the default macOS manager but optional — degrade to a
			// warning+skip like the opt-in backends, rather than failing apply.
			u.Warn("brew not on PATH — install it (or add a scripts/pre-*.sh that installs it) to install these packages")
			return nil
		}
		return []error{fmt.Errorf("package manager %q not on PATH", mgr.Name())}
	}
	// Catch malformed specs (e.g. a bad brew /cask suffix) before the
	// "install ..." line, so the UI never announces an install it can't do.
	if v, ok := mgr.(packages.Validator); ok {
		if err := v.Validate(pkgs); err != nil {
			return []error{err}
		}
	}
	var todo, already []string
	for _, p := range pkgs {
		if mgr.IsInstalled(p) {
			already = append(already, p)
		} else {
			todo = append(todo, p)
		}
	}
	if len(already) > 0 {
		u.Action("skip", fmt.Sprintf("%d already installed", len(already)))
	}
	if len(todo) == 0 {
		return nil
	}
	u.Action("install", fmt.Sprintf("%s (via %s)", strings.Join(todo, ", "), mgr.Name()))
	if err := mgr.Install(todo); err != nil {
		return []error{err}
	}
	return nil
}

// declaredBackends returns the names of non-native package backends
// mentioned in cfg, in alphabetical order. The set is whatever the
// user actually declared — known (flatpak, brew) and unknown
// (cargo/npm/etc., kept around for forward compatibility) all flow
// through the same install loop. applyBackendPackages handles the
// "no Manager" case for unknowns by warning and skipping.
func declaredBackends(cfg config.Config) []string {
	names := make([]string, 0, len(cfg.Packages.Backends))
	for n := range cfg.Packages.Backends {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

func applyBackendPackages(u ui.UI, cfg config.Config, env detect.Env, backend string) []error {
	pkgs := cfg.PackagesForBackend(env, backend)
	if len(pkgs) == 0 {
		return nil // nothing to do; don't even announce the phase
	}
	u.Phase(backend)
	if applySkipPackages {
		u.Info("skipped (--skip-packages)")
		return nil
	}
	mgr := packages.ForBackend(backend)
	if mgr == nil {
		u.Warn(fmt.Sprintf("backend %q is not recognized — homie has no Manager for it", backend))
		return nil
	}
	if !mgr.IsAvailable() {
		u.Warn(fmt.Sprintf("%s not on PATH — install it (or add a scripts/pre-*.sh that installs it) to apply these packages", backend))
		return nil
	}
	// Catch malformed specs (e.g. a bad snap confinement suffix) before the
	// "install ..." line, so the UI never announces an install it can't do.
	if v, ok := mgr.(packages.Validator); ok {
		if err := v.Validate(pkgs); err != nil {
			return []error{err}
		}
	}
	var todo, already []string
	for _, p := range pkgs {
		if mgr.IsInstalled(p) {
			already = append(already, p)
		} else {
			todo = append(todo, p)
		}
	}
	if len(already) > 0 {
		u.Action("skip", fmt.Sprintf("%d already installed", len(already)))
	}
	if len(todo) == 0 {
		return nil
	}
	u.Action("install", fmt.Sprintf("%s (via %s)", strings.Join(todo, ", "), backend))
	if err := mgr.Install(todo); err != nil {
		return []error{err}
	}
	return nil
}

// applyHomePhase runs the link + render phases against the unified
// home tree under one "home" UI section. Action verbs (create / replace
// / backup / render) disambiguate which step each line came from.
func applyHomePhase(u ui.UI, repoDir, home string, cfg config.Config, env detect.Env) []error {
	u.Phase("home")
	var errs []error

	actions, err := link.Plan(repoDir, home, cfg.AllTags(env))
	if err != nil {
		return []error{err}
	}
	linkRes := link.Apply(actions, time.Now())
	for _, a := range linkRes.Created {
		u.Action("create", relTarget(home, a.Target))
	}
	for _, a := range linkRes.Replaced {
		u.Action("replace", relTarget(home, a.Target))
	}
	for _, b := range linkRes.Backed {
		u.Action("backup", fmt.Sprintf("%s -> %s", relTarget(home, b.Action.Target), relTarget(home, b.Backup)))
	}
	errs = append(errs, linkRes.Errors...)

	renderRes := render.Apply(repoDir, home, cfg, env)
	for _, a := range renderRes.Rendered {
		u.Action("render", relTarget(home, a.Target))
	}
	errs = append(errs, renderRes.Errors...)

	if skipped := len(linkRes.Skipped) + len(renderRes.Skipped); skipped > 0 {
		u.Action("skip", fmt.Sprintf("%d already in sync", skipped))
	}
	if len(linkRes.Created)+len(linkRes.Replaced)+len(linkRes.Backed)+len(linkRes.Skipped)+
		len(renderRes.Rendered)+len(renderRes.Skipped) == 0 && len(errs) == 0 {
		u.Info("nothing in home/")
	}
	return errs
}

func applyScriptPhase(u ui.UI, repoDir, home string, cfg config.Config, env detect.Env, phase runner.Phase) []error {
	label := "scripts"
	if phase == runner.PhasePre {
		label = "pre-scripts"
	}
	u.Phase(label)
	if applySkipScripts {
		u.Info("skipped (--skip-scripts)")
		return nil
	}
	tags := cfg.AllTags(env)
	// When scripts run interactively (stdin is a TTY), they inherit the
	// terminal so prompts like sudo work — release the live TUI's hold on it
	// for the duration, then restore. Gate on the phase actually having
	// scripts so an empty phase doesn't flicker the TUI with a no-op
	// release/restore. Plain UIs no-op both calls. (A Plan error is surfaced
	// below by runner.Run; here we just skip the release.)
	if scripts, err := runner.Plan(repoDir, tags, phase); err == nil && len(scripts) > 0 && runner.Interactive() {
		if err := u.Suspend(); err != nil {
			u.Warn(fmt.Sprintf("couldn't release the terminal for interactive scripts: %v", err))
		}
		defer func() {
			if err := u.Resume(); err != nil {
				u.Warn(fmt.Sprintf("couldn't restore the terminal after scripts: %v", err))
			}
		}()
	}
	res := runner.Run(repoDir, home, cfg, tags, phase, u.Writer())
	if len(res.Ran) == 0 {
		// A pre-flight error (e.g. a filename collision between active tag
		// trees) leaves nothing run but must not be swallowed.
		if len(res.Errors) > 0 {
			for _, e := range res.Errors {
				u.Action("error", e.Error())
			}
			return res.Errors
		}
		u.Info("no " + label)
		return nil
	}
	for _, r := range res.Ran {
		status := "ok"
		if r.Err != nil {
			status = "fail"
		}
		u.Action(status, filepath.Base(r.Path))
	}
	return res.Errors
}
