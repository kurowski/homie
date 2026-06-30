package packages

import (
	"fmt"
	"os/exec"
	"strings"
)

// Pkg is the package manager backend for Termux (Android). Termux ships
// dpkg and apt-get; `pkg` is Termux's thin wrapper over apt against its
// own repos. So Pkg is Apt-shaped — the same `dpkg -s` installed check —
// but it installs through `pkg install` and never sudoes.
//
// Unlike apt/dnf, there is no Sudo field. Termux runs under the Android
// app's own (non-zero) uid with no root and no `sudo` binary at all, so
// the not-root-therefore-prepend-sudo rule every other Linux backend
// follows would prepend a command that doesn't exist. This is the one
// place a backend deliberately ignores the effective uid.
type Pkg struct {
	Runner Runner
}

// Name returns "pkg".
func (p *Pkg) Name() string { return "pkg" }

// IsAvailable reports whether dpkg (for the installed check) and pkg (for
// install) are on PATH. Both ship with every Termux install.
func (p *Pkg) IsAvailable() bool {
	if _, err := exec.LookPath("dpkg"); err != nil {
		return false
	}
	_, err := exec.LookPath("pkg")
	return err == nil
}

// IsInstalled reports whether dpkg considers name installed, using the
// same `dpkg -s` status check as Apt — `pkg install` records into the same
// dpkg database. See Apt.IsInstalled for why the Status line is inspected
// rather than trusting the exit code alone.
func (p *Pkg) IsInstalled(name string) bool {
	out, err := p.Runner("dpkg", "-s", name)
	if err != nil {
		return false
	}
	return strings.Contains(string(out), "Status: install ok installed")
}

// Install installs the packages that aren't already present via
// `pkg install -y`. Empty input — or an empty filtered list (everything
// already installed) — is a no-op without shelling out. Unlike Apt there's
// no separate cache-refresh step: `pkg install` updates Termux's metadata
// itself before installing.
func (p *Pkg) Install(pkgs []string) error {
	todo := filterUninstalled(p, pkgs)
	if len(todo) == 0 {
		return nil
	}
	args := append([]string{"install", "-y"}, todo...)
	out, err := p.Runner("pkg", args...)
	if err != nil {
		return fmt.Errorf("pkg install: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}
