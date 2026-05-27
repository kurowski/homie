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
	Short: "Full reconciliation: detect → pre-scripts → packages → backends → link → render → scripts",
	RunE:  runApply,
}

func init() {
	applyCmd.Flags().StringVar(&applyHome, "home", "", "override target home directory (default $HOME)")
	applyCmd.Flags().BoolVar(&applySkipPackages, "skip-packages", false, "skip the native and non-native (brew, flatpak) package phases")
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
	errs = append(errs, applyLink(u, repoDir, home, cfg, env)...)
	errs = append(errs, applyRender(u, repoDir, home, cfg, env)...)
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
		return []error{fmt.Errorf("package manager %q not on PATH", mgr.Name())}
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

func applyLink(u ui.UI, repoDir, home string, cfg config.Config, env detect.Env) []error {
	u.Phase("link")
	actions, err := link.Plan(repoDir, home, cfg.AllTags(env))
	if err != nil {
		return []error{err}
	}
	if len(actions) == 0 {
		u.Info("no dotfiles")
		return nil
	}
	res := link.Apply(actions, time.Now())
	for _, a := range res.Created {
		u.Action("create", relTarget(home, a.Target))
	}
	for _, a := range res.Replaced {
		u.Action("replace", relTarget(home, a.Target))
	}
	for _, b := range res.Backed {
		u.Action("backup", fmt.Sprintf("%s -> %s", relTarget(home, b.Action.Target), relTarget(home, b.Backup)))
	}
	if len(res.Skipped) > 0 {
		u.Action("skip", fmt.Sprintf("%d already in sync", len(res.Skipped)))
	}
	return res.Errors
}

func applyRender(u ui.UI, repoDir, home string, cfg config.Config, env detect.Env) []error {
	u.Phase("render")
	res := render.Apply(repoDir, home, cfg, env)
	if len(res.Rendered) == 0 && len(res.Skipped) == 0 && len(res.Errors) == 0 {
		u.Info("no templates")
		return nil
	}
	for _, a := range res.Rendered {
		u.Action("render", relTarget(home, a.Target))
	}
	if len(res.Skipped) > 0 {
		u.Action("skip", fmt.Sprintf("%d already in sync", len(res.Skipped)))
	}
	return res.Errors
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
	res := runner.Run(repoDir, home, cfg, cfg.AllTags(env), phase, u.Writer())
	if len(res.Ran) == 0 {
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
