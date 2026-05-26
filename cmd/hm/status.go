package main

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strings"

	"github.com/kurowski/homie/internal/config"
	"github.com/kurowski/homie/internal/detect"
	"github.com/kurowski/homie/internal/doctor"
	"github.com/kurowski/homie/internal/packages"
	"github.com/kurowski/homie/internal/repo"
	"github.com/spf13/cobra"
)

var statusHome string

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show sync state, no changes",
	RunE:  runStatus,
}

func init() {
	statusCmd.Flags().StringVar(&statusHome, "home", "", "override target home directory (default $HOME)")
	rootCmd.AddCommand(statusCmd)
}

func runStatus(cmd *cobra.Command, args []string) error {
	env := detect.Detect()
	w := cmd.OutOrStdout()

	fmt.Fprintln(w, "Environment:")
	fmt.Fprintf(w, "  Distro:          %s\n", env.Distro)
	fmt.Fprintf(w, "  Package manager: %s\n", env.PackageManager)
	fmt.Fprintf(w, "  Arch:            %s\n", env.Arch)
	fmt.Fprintf(w, "  Hostname:        %s\n", env.Hostname)
	fmt.Fprintf(w, "  Container:       %v\n", env.IsContainer)
	fmt.Fprintf(w, "  Root:            %v\n", env.IsRoot)
	fmt.Fprintf(w, "  Interactive:     %v\n", env.IsInteractive)
	fmt.Fprintf(w, "  Auto tags:       %s\n", strings.Join(env.Tags, ", "))

	repoDir, err := repo.Find()
	if err != nil {
		fmt.Fprintln(w)
		fmt.Fprintln(w, "No user environment repo found (run `hm init` or set $HM_REPO).")
		return nil
	}

	cfg, err := config.Load(repoDir, env.Hostname)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("no homie.toml in %s", repoDir)
		}
		return err
	}

	fmt.Fprintln(w)
	fmt.Fprintf(w, "Repo: %s\n", repoDir)
	fmt.Fprintf(w, "  User:     %s <%s>\n", cfg.User.Name, cfg.User.Email)
	if cfg.Profile.Name != "" {
		fmt.Fprintf(w, "  Profile:  %s\n", cfg.Profile.Name)
	}
	if cfg.Profile.DefaultShell != "" {
		fmt.Fprintf(w, "  Shell:    %s\n", cfg.Profile.DefaultShell)
	}
	fmt.Fprintf(w, "  Tags:     %s\n", strings.Join(cfg.AllTags(env), ", "))
	fmt.Fprintf(w, "  Packages: %s\n", strings.Join(cfg.PackagesFor(env), ", "))

	for _, warning := range cfg.Warnings {
		fmt.Fprintf(w, "  warning: %s\n", warning)
	}

	// Run doctor's read-only walk for a one-line health summary.
	// The deep dive lives in `hm doctor`; status just nudges users
	// toward it when something looks off.
	home := statusHome
	if home == "" {
		home, _ = os.UserHomeDir()
	}
	if home != "" {
		report := doctor.Run(repoDir, home, cfg, env, packages.For(env))
		errs, warns := report.Counts()
		fmt.Fprintln(w)
		switch {
		case errs == 0 && warns == 0:
			fmt.Fprintln(w, "Health: all checks passed.")
		default:
			fmt.Fprintf(w, "Health: %s, %s — run `hm doctor` for detail.\n",
				pluralize(errs, "error"), pluralize(warns, "warning"))
		}
	}
	return nil
}
