package packages

import (
	"fmt"
	"os/exec"
	"strings"
)

// Brew is the package manager backend for Homebrew-on-Linux. Useful on
// immutable distros (Universal Blue, Bluefin, Bazzite) where dnf is
// discouraged and brew is the default install path.
type Brew struct {
	Runner Runner
}

// Name returns "brew".
func (b *Brew) Name() string { return "brew" }

// IsAvailable reports whether the brew command-line tool is on PATH.
// If it isn't, the apply phase logs a warning and skips silently —
// brew is opt-in and not every host runs it.
func (b *Brew) IsAvailable() bool {
	_, err := exec.LookPath("brew")
	return err == nil
}

// IsInstalled reports whether the named formula is currently installed.
// `brew list --formula <name>` exits non-zero when the formula isn't
// present.
func (b *Brew) IsInstalled(formula string) bool {
	_, err := b.Runner("brew", "list", "--formula", formula)
	return err == nil
}

// Install installs formulae that aren't already installed. Empty input
// is a no-op.
func (b *Brew) Install(formulae []string) error {
	todo := filterUninstalled(b, formulae)
	if len(todo) == 0 {
		return nil
	}
	args := []string{"install"}
	args = append(args, todo...)
	out, err := b.Runner("brew", args...)
	if err != nil {
		return fmt.Errorf("brew install: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}
