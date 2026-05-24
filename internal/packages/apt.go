package packages

import (
	"fmt"
	"os/exec"
	"strings"
)

// Apt is the package manager backend for Ubuntu and Debian.
type Apt struct {
	Runner Runner
	Sudo   bool // prepend `sudo` to mutating commands when not root
}

// Name returns "apt".
func (a *Apt) Name() string { return "apt" }

// IsAvailable reports whether dpkg and apt-get are on PATH.
func (a *Apt) IsAvailable() bool {
	if _, err := exec.LookPath("dpkg"); err != nil {
		return false
	}
	_, err := exec.LookPath("apt-get")
	return err == nil
}

// IsInstalled reports whether dpkg considers pkg installed. `dpkg -s`
// exits non-zero when the package is unknown; when it succeeds, the
// output contains a Status line we have to inspect — `dpkg -s` reports
// half-removed packages too, and those should count as not-installed.
func (a *Apt) IsInstalled(pkg string) bool {
	out, err := a.Runner("dpkg", "-s", pkg)
	if err != nil {
		return false
	}
	return strings.Contains(string(out), "Status: install ok installed")
}

// Install installs pkgs that aren't already installed. Empty input is a
// no-op; an empty filtered list (everything already installed) is also a
// no-op without invoking apt-get. Refreshes the package cache once before
// install so fresh systems (containers, first-boot workstations) don't
// fail with "Unable to locate package".
func (a *Apt) Install(pkgs []string) error {
	todo := filterUninstalled(a, pkgs)
	if len(todo) == 0 {
		return nil
	}
	updateCmd, updateRest := a.command([]string{"apt-get", "update", "-qq"})
	if out, err := a.Runner(updateCmd, updateRest...); err != nil {
		return fmt.Errorf("apt-get update: %w: %s", err, strings.TrimSpace(string(out)))
	}
	args := []string{"apt-get", "install", "-y"}
	args = append(args, todo...)
	cmd, rest := a.command(args)
	out, err := a.Runner(cmd, rest...)
	if err != nil {
		return fmt.Errorf("apt-get install: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func (a *Apt) command(args []string) (string, []string) {
	if a.Sudo {
		return "sudo", args
	}
	return args[0], args[1:]
}
