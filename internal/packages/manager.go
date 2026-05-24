// Package packages installs OS packages via the host's native package
// manager. Each backend implements the same Manager interface so the rest
// of Homie doesn't care whether we're on apt or dnf.
//
// Idempotency: Install filters out already-installed packages before
// invoking the manager, so repeated `hm apply` runs are cheap. There is
// no state file — IsInstalled queries the package database directly.
package packages

import (
	"os/exec"

	"github.com/kurowski/homie/internal/detect"
)

// Manager is the interface every backend implements.
type Manager interface {
	// Name returns a short identifier ("apt", "dnf", "noop").
	Name() string
	// IsAvailable reports whether the backend's tools are present on PATH.
	IsAvailable() bool
	// IsInstalled reports whether the named package is installed.
	IsInstalled(pkg string) bool
	// Install installs the named packages. Implementations filter to
	// only-not-yet-installed before shelling out.
	Install(pkgs []string) error
}

// Runner is the function used to execute external commands. Tests inject
// a fake; production uses execRunner which wraps os/exec.
type Runner func(name string, args ...string) ([]byte, error)

func execRunner(name string, args ...string) ([]byte, error) {
	return exec.Command(name, args...).CombinedOutput()
}

// For returns the Manager appropriate for env. Unknown distros get a
// NoopManager that warns and skips, keeping `hm apply` non-fatal.
func For(env detect.Env) Manager {
	switch env.PackageManager {
	case "apt":
		return &Apt{Runner: execRunner, Sudo: !env.IsRoot}
	case "dnf":
		return &Dnf{Runner: execRunner, Sudo: !env.IsRoot}
	default:
		// TODO(contrib): add support for additional package managers
		// (pacman, zypper, apk, brew). See homie.sh/contributing.
		return &Noop{Distro: env.Distro}
	}
}

// filterUninstalled returns the subset of pkgs that the manager reports
// as not installed. Used by every real backend before Install.
func filterUninstalled(m Manager, pkgs []string) []string {
	out := make([]string, 0, len(pkgs))
	for _, p := range pkgs {
		if !m.IsInstalled(p) {
			out = append(out, p)
		}
	}
	return out
}
