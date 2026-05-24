package main

import (
	"fmt"
	"io"
	"strings"

	"github.com/kurowski/homie/internal/detect"
	"github.com/kurowski/homie/internal/packages"
	"github.com/spf13/cobra"
)

// bootstrapPkgs is the list of CLI tools hm itself needs to clone a
// user repo and run apply on a minimal host (think Docker base image).
// ca-certificates is in here because HTTPS git clones fail without it
// in minimal containers — a class of issue that's confusing to debug
// at clone time.
var bootstrapPkgs = []string{"git", "ca-certificates"}

var bootstrapCmd = &cobra.Command{
	Use:   "bootstrap",
	Short: "Install prerequisites needed before hm apply",
	Long: `Bootstrap installs the small set of tools hm itself relies on:
git (to clone your environment repo) and ca-certificates (so HTTPS
clones work on minimal hosts).

Run this once on a fresh machine after downloading the hm binary,
before cloning your dotfiles repo. It's idempotent — packages already
present are skipped.`,
	RunE: runBootstrap,
}

func init() {
	rootCmd.AddCommand(bootstrapCmd)
}

func runBootstrap(cmd *cobra.Command, args []string) error {
	env := detect.Detect()
	return doBootstrap(packages.For(env), env.Distro, cmd.OutOrStdout())
}

// doBootstrap is the testable core of `hm bootstrap` — takes a Manager
// directly so unit tests can inject a fake instead of shelling out.
func doBootstrap(mgr packages.Manager, distro string, w io.Writer) error {
	if mgr.Name() == "noop" {
		// TODO(contrib): add a package-manager backend for this distro.
		return fmt.Errorf("distro %q not yet supported — see homie.sh/contributing", distro)
	}
	if !mgr.IsAvailable() {
		return fmt.Errorf("package manager %q is not on PATH", mgr.Name())
	}
	var todo []string
	for _, p := range bootstrapPkgs {
		if !mgr.IsInstalled(p) {
			todo = append(todo, p)
		}
	}
	if len(todo) == 0 {
		fmt.Fprintln(w, "All bootstrap prereqs already installed.")
		return nil
	}
	fmt.Fprintf(w, "Installing bootstrap prereqs via %s: %s\n", mgr.Name(), strings.Join(todo, ", "))
	return mgr.Install(todo)
}
