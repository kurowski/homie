package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/kurowski/homie/internal/config"
	"github.com/kurowski/homie/internal/detect"
	"github.com/kurowski/homie/internal/link"
	"github.com/kurowski/homie/internal/packages"
	"github.com/kurowski/homie/internal/render"
	"github.com/kurowski/homie/internal/repo"
	"github.com/kurowski/homie/internal/runner"
	"github.com/spf13/cobra"
)

var (
	applyHome         string
	applySkipPackages bool
	applySkipScripts  bool
)

var applyCmd = &cobra.Command{
	Use:   "apply",
	Short: "Full reconciliation: detect → packages → link → render → scripts",
	RunE:  runApply,
}

func init() {
	applyCmd.Flags().StringVar(&applyHome, "home", "", "override target home directory (default $HOME)")
	applyCmd.Flags().BoolVar(&applySkipPackages, "skip-packages", false, "skip the package-install phase")
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
	cfg, err := config.Load(repoDir)
	if err != nil {
		return err
	}
	env := detect.Detect()
	w := cmd.OutOrStdout()

	var phaseErrors []error
	phase := func(name string, fn func() []error) {
		fmt.Fprintf(w, "\n== %s ==\n", name)
		errs := fn()
		phaseErrors = append(phaseErrors, errs...)
	}

	phase("packages", func() []error { return applyPackages(w, cfg, env) })
	phase("link", func() []error { return applyLink(w, repoDir, home) })
	phase("render", func() []error { return applyRender(w, repoDir, home, cfg, env) })
	phase("scripts", func() []error { return applyScripts(w, repoDir, home, cfg, env) })

	fmt.Fprintf(w, "\n== summary ==\n")
	if len(phaseErrors) == 0 {
		fmt.Fprintln(w, "  All phases completed cleanly.")
		return nil
	}
	for _, e := range phaseErrors {
		fmt.Fprintf(w, "  error    %s\n", e)
	}
	return fmt.Errorf("%d error(s) during apply", len(phaseErrors))
}

func applyPackages(w io.Writer, cfg config.Config, env detect.Env) []error {
	if applySkipPackages {
		fmt.Fprintln(w, "  skipped (--skip-packages)")
		return nil
	}
	pkgs := cfg.PackagesFor(env)
	if len(pkgs) == 0 {
		fmt.Fprintln(w, "  no packages declared")
		return nil
	}
	mgr := packages.For(env)
	if mgr.Name() == "noop" {
		// TODO(contrib): add support for additional package managers.
		fmt.Fprintf(w, "  warning  distro %q not yet supported — see homie.sh/contributing\n", env.Distro)
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
		fmt.Fprintf(w, "  skip     %d already installed\n", len(already))
	}
	if len(todo) == 0 {
		return nil
	}
	fmt.Fprintf(w, "  install  %s (via %s)\n", strings.Join(todo, ", "), mgr.Name())
	if err := mgr.Install(todo); err != nil {
		return []error{err}
	}
	return nil
}

func applyLink(w io.Writer, repoDir, home string) []error {
	actions, err := link.Plan(repoDir, home)
	if err != nil {
		return []error{err}
	}
	if len(actions) == 0 {
		fmt.Fprintln(w, "  no dotfiles")
		return nil
	}
	res := link.Apply(actions, time.Now())
	printResult(w, home, res)
	return res.Errors
}

func applyRender(w io.Writer, repoDir, home string, cfg config.Config, env detect.Env) []error {
	res := render.Apply(repoDir, home, cfg, env)
	if len(res.Rendered) == 0 && len(res.Errors) == 0 {
		fmt.Fprintln(w, "  no templates")
		return nil
	}
	for _, a := range res.Rendered {
		fmt.Fprintf(w, "  render   %s\n", relTarget(home, a.Target))
	}
	return res.Errors
}

func applyScripts(w io.Writer, repoDir, home string, cfg config.Config, env detect.Env) []error {
	if applySkipScripts {
		fmt.Fprintln(w, "  skipped (--skip-scripts)")
		return nil
	}
	res := runner.Run(repoDir, home, cfg, cfg.AllTags(env), w)
	if len(res.Ran) == 0 {
		fmt.Fprintln(w, "  no scripts")
		return nil
	}
	for _, r := range res.Ran {
		status := "ok"
		if r.Err != nil {
			status = "fail"
		}
		fmt.Fprintf(w, "  %-5s %s\n", status, filepath.Base(r.Path))
	}
	return res.Errors
}
