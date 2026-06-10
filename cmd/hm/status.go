package main

import (
	"errors"
	"fmt"
	"io"
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

var (
	statusHome string
	statusJSON bool
)

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show sync state, no changes",
	Long: `Print a read-only summary of how the current host looks to
Homie. No writes, no installs — useful before an apply to verify what
Homie thinks it should be doing, or in CI to confirm a fresh checkout
is healthy.

The output covers:

  - Environment: distro, package manager, arch, hostname, container/
    root/interactive flags, the auto-detected tag set.
  - Repo:        active homie.toml path, user identity, profile,
                 the merged active tag set, and the package list that
                 would install on this host.
  - Warnings:    anything captured by config load (unknown keys,
                 typos, etc.).
  - Health:      a one-line counts summary from ` + "`hm doctor`" + ` — run that
                 command for the detail.

With --json the same information is emitted as a single JSON document
on stdout, with per-backend package lists broken out, so scripts and
agents can consume host state without scraping the text. When no
environment repo is found, "repo" is null instead of an error; a $HM_REPO
that points at a directory without a homie.toml is still an error.

Exits zero even if doctor reports problems; use ` + "`hm doctor`" + ` for a
non-zero gate.`,
	RunE: runStatus,
}

func init() {
	statusCmd.Flags().StringVar(&statusHome, "home", "", "override target home directory (default $HOME)")
	statusCmd.Flags().BoolVar(&statusJSON, "json", false, "emit machine-readable JSON instead of text")
	rootCmd.AddCommand(statusCmd)
}

// statusOutput is the document emitted by `hm status --json`.
type statusOutput struct {
	Environment statusEnv     `json:"environment"`
	Repo        *statusRepo   `json:"repo"` // null when no environment repo is found
	Health      *statusHealth `json:"health,omitempty"`
}

type statusEnv struct {
	Distro         string   `json:"distro"`
	PackageManager string   `json:"package_manager"`
	Arch           string   `json:"arch"`
	Hostname       string   `json:"hostname"`
	Container      bool     `json:"container"`
	Root           bool     `json:"root"`
	Interactive    bool     `json:"interactive"`
	AutoTags       []string `json:"auto_tags"`
}

// List- and map-valued fields are always present (as [] / {}) so
// consumers can iterate without an absent-vs-empty check; only the
// optional scalar strings use omitempty.
type statusRepo struct {
	Path            string              `json:"path"`
	User            statusUser          `json:"user"`
	Profile         string              `json:"profile,omitempty"`
	DefaultShell    string              `json:"default_shell,omitempty"`
	Tags            []string            `json:"tags"`
	Packages        []string            `json:"packages"`
	BackendPackages map[string][]string `json:"backend_packages"`
	Warnings        []string            `json:"warnings"`
}

type statusUser struct {
	Name  string `json:"name"`
	Email string `json:"email"`
}

type statusHealth struct {
	Errors   int `json:"errors"`
	Warnings int `json:"warnings"`
}

func runStatus(cmd *cobra.Command, args []string) error {
	env := detect.Detect()
	if statusJSON {
		return runStatusJSON(cmd.OutOrStdout(), env)
	}
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
		// Only the walk-up finding nothing is benign; a set-but-wrong
		// $HM_REPO is a misconfiguration the user needs to hear about.
		if !errors.Is(err, repo.ErrNotFound) {
			return err
		}
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
		report := doctor.Run(repoDir, home, cfg, env, packages.For(env), packages.ForBackend)
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

// runStatusJSON emits the same information as the text path as one JSON
// document. Only a genuinely absent environment repo is data
// ("repo": null); a set-but-wrong $HM_REPO or a repo with a broken
// homie.toml is still an error so a misconfiguration can't be mistaken
// for "no repo".
func runStatusJSON(w io.Writer, env detect.Env) error {
	out := statusOutput{
		Environment: statusEnv{
			Distro:         env.Distro,
			PackageManager: env.PackageManager,
			Arch:           env.Arch,
			Hostname:       env.Hostname,
			Container:      env.IsContainer,
			Root:           env.IsRoot,
			Interactive:    env.IsInteractive,
			AutoTags:       orEmpty(env.Tags),
		},
	}
	repoDir, err := repo.Find()
	if err != nil {
		if !errors.Is(err, repo.ErrNotFound) {
			return err
		}
		return writeJSON(w, out)
	}
	cfg, err := config.Load(repoDir, env.Hostname)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("no homie.toml in %s", repoDir)
		}
		return err
	}
	backends := make(map[string][]string, len(cfg.Packages.Backends))
	for name := range cfg.Packages.Backends {
		backends[name] = orEmpty(cfg.PackagesForBackend(env, name))
	}
	out.Repo = &statusRepo{
		Path:            repoDir,
		User:            statusUser{Name: cfg.User.Name, Email: cfg.User.Email},
		Profile:         cfg.Profile.Name,
		DefaultShell:    cfg.Profile.DefaultShell,
		Tags:            cfg.AllTags(env),
		Packages:        orEmpty(cfg.PackagesFor(env)),
		BackendPackages: backends,
		Warnings:        orEmpty(cfg.Warnings),
	}
	home := statusHome
	if home == "" {
		home, _ = os.UserHomeDir()
	}
	if home != "" {
		report := doctor.Run(repoDir, home, cfg, env, packages.For(env), packages.ForBackend)
		errs, warns := report.Counts()
		out.Health = &statusHealth{Errors: errs, Warnings: warns}
	}
	return writeJSON(w, out)
}

// orEmpty keeps list-valued JSON fields as [] rather than null when a
// resolver returns a nil slice, so consumers get a stable shape.
func orEmpty(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}
