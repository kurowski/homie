package main

import (
	"fmt"
	"io"
	"strings"

	"github.com/kurowski/homie/internal/config"
	"github.com/kurowski/homie/internal/detect"
	"github.com/kurowski/homie/internal/packages"
	"github.com/kurowski/homie/internal/repo"
	"github.com/spf13/cobra"
)

var installCmd = &cobra.Command{
	Use:   "install",
	Short: "Install packages declared in homie.toml",
	RunE:  runInstall,
}

func init() {
	rootCmd.AddCommand(installCmd)
}

func runInstall(cmd *cobra.Command, args []string) error {
	repoDir, err := repo.Find()
	if err != nil {
		return err
	}
	env := detect.Detect()
	cfg, err := config.Load(repoDir, env.Hostname)
	if err != nil {
		return err
	}
	w := cmd.OutOrStdout()

	native := cfg.PackagesFor(env)
	backends := declaredBackends(cfg)
	declared := len(native) > 0
	for _, backend := range backends {
		if len(cfg.PackagesForBackend(env, backend)) > 0 {
			declared = true
			break
		}
	}
	if !declared {
		fmt.Fprintln(w, "No packages declared.")
		return nil
	}

	if err := installNative(w, env, native); err != nil {
		return err
	}
	for _, backend := range backends {
		if err := installBackend(w, cfg, env, backend); err != nil {
			return err
		}
	}
	return nil
}

func installNative(w io.Writer, env detect.Env, pkgs []string) error {
	if len(pkgs) == 0 {
		return nil
	}
	mgr := packages.For(env)
	if mgr.Name() == "noop" {
		// TODO(contrib): add support for additional package managers.
		fmt.Fprintf(w, "  warning  distro %q has no package manager backend yet — see homie.sh/contributing\n", env.Distro)
		fmt.Fprintf(w, "  skipped  %s\n", strings.Join(pkgs, ", "))
		return nil
	}
	if !mgr.IsAvailable() {
		return fmt.Errorf("package manager %q is not available on PATH", mgr.Name())
	}
	return doInstall(w, mgr, pkgs)
}

func installBackend(w io.Writer, cfg config.Config, env detect.Env, backend string) error {
	pkgs := cfg.PackagesForBackend(env, backend)
	if len(pkgs) == 0 {
		return nil
	}
	mgr := packages.ForBackend(backend)
	if mgr == nil {
		fmt.Fprintf(w, "  warning  backend %q is not recognized — homie has no Manager for it\n", backend)
		return nil
	}
	if !mgr.IsAvailable() {
		fmt.Fprintf(w, "  warning  %s not on PATH — skipping (install it or add scripts/pre-*.sh)\n", backend)
		return nil
	}
	return doInstall(w, mgr, pkgs)
}

func doInstall(w io.Writer, mgr packages.Manager, pkgs []string) error {
	var todo, already []string
	for _, p := range pkgs {
		if mgr.IsInstalled(p) {
			already = append(already, p)
		} else {
			todo = append(todo, p)
		}
	}
	if len(already) > 0 {
		fmt.Fprintf(w, "  skip     %d already installed (via %s)\n", len(already), mgr.Name())
	}
	if len(todo) == 0 {
		return nil
	}
	fmt.Fprintf(w, "  install  %s (via %s)\n", strings.Join(todo, ", "), mgr.Name())
	return mgr.Install(todo)
}
