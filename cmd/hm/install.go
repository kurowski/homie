package main

import (
	"fmt"
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
	pkgs := cfg.PackagesFor(env)
	w := cmd.OutOrStdout()

	if len(pkgs) == 0 {
		fmt.Fprintln(w, "No packages declared.")
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
		return err
	}
	return nil
}
