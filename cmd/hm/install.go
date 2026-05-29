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
	Long: `Install the packages declared in homie.toml — native first
(apt or dnf on Linux, brew on macOS), then each non-native backend
(flatpak, snap, ...) in alphabetical order. Already-installed packages
are filtered out, so re-running is cheap.

The native phase resolves [packages].all + [packages].<distro> +
matching [packages."tag:X"] sub-tables. On macOS, native packages go
through brew; a GUI app (cask) is named with a "/cask" suffix, e.g.
"firefox/cask". brew is optional — if it isn't on PATH this phase warns
and skips rather than failing, so a dotfiles-only setup needs nothing
extra. Each backend phase resolves its corresponding [packages.<backend>]
tables; a backend whose CLI tool isn't on PATH warns and skips — install
the tool (or add a scripts/pre-*.sh that does) and re-run.

This is the same phase ` + "`hm apply`" + ` runs; use ` + "`hm install`" + ` when you only
want to update packages without touching dotfiles or scripts.

See https://homie.sh/docs/config/#packages for the table reference.`,
	RunE: runInstall,
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
		if mgr.Name() == "brew" {
			// brew is the default macOS manager but optional — warn and skip
			// like a backend rather than failing the command.
			fmt.Fprintf(w, "  warning  brew not on PATH — skipping (install it or add scripts/pre-*.sh)\n")
			return nil
		}
		return fmt.Errorf("package manager %q is not available on PATH", mgr.Name())
	}
	if v, ok := mgr.(packages.Validator); ok {
		if err := v.Validate(pkgs); err != nil {
			return err
		}
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
	// Mirror installNative (and applyBackendPackages): flag a malformed spec
	// with a clean pre-shellout error rather than letting it bubble up from
	// mgr.Install.
	if v, ok := mgr.(packages.Validator); ok {
		if err := v.Validate(pkgs); err != nil {
			return err
		}
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
