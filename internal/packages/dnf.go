package packages

import (
	"fmt"
	"os/exec"
	"strings"
)

// Dnf is the package manager backend for Fedora.
type Dnf struct {
	Runner Runner
	Sudo   bool
}

// Name returns "dnf".
func (d *Dnf) Name() string { return "dnf" }

// IsAvailable reports whether rpm and dnf are on PATH.
func (d *Dnf) IsAvailable() bool {
	if _, err := exec.LookPath("rpm"); err != nil {
		return false
	}
	_, err := exec.LookPath("dnf")
	return err == nil
}

// IsInstalled reports whether rpm has pkg in its database. `rpm -q` exits
// non-zero when the package isn't installed.
func (d *Dnf) IsInstalled(pkg string) bool {
	_, err := d.Runner("rpm", "-q", pkg)
	return err == nil
}

// Install installs pkgs that aren't already installed.
func (d *Dnf) Install(pkgs []string) error {
	todo := filterUninstalled(d, pkgs)
	if len(todo) == 0 {
		return nil
	}
	args := []string{"dnf", "install", "-y"}
	args = append(args, todo...)
	cmd, rest := d.command(args)
	out, err := d.Runner(cmd, rest...)
	if err != nil {
		return fmt.Errorf("dnf install: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func (d *Dnf) command(args []string) (string, []string) {
	if d.Sudo {
		return "sudo", args
	}
	return args[0], args[1:]
}
