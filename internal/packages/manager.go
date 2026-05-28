// Package packages installs OS packages via the host's native package
// manager. Each backend implements the same Manager interface so the rest
// of Homie doesn't care whether we're on apt or dnf.
//
// Idempotency: Install filters out already-installed packages before
// invoking the manager, so repeated `hm apply` runs are cheap. There is
// no state file — IsInstalled queries the package database directly.
package packages

import (
	"os"
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
		// (pacman, zypper, apk). Brew/Flatpak live in ForBackend.
		return &Noop{Distro: env.Distro}
	}
}

// ForBackend returns the Manager for a named non-native backend, or
// nil if the name isn't recognized. Backends are opt-in: callers
// invoke this when they see packages declared for the backend in
// homie.toml, and check IsAvailable before installing so a missing
// tool degrades to a warning rather than an error.
func ForBackend(name string) Manager {
	switch name {
	case "flatpak":
		return &Flatpak{Runner: execRunner}
	case "brew":
		return &Brew{Runner: execRunner}
	case "snap":
		// snap install needs root; derive sudo from the effective uid
		// here since ForBackend has no detect.Env to consult.
		return &Snap{Runner: execRunner, Sudo: os.Geteuid() != 0}
	default:
		return nil
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
