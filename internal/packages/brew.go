package packages

import (
	"bufio"
	"bytes"
	"fmt"
	"os/exec"
	"strings"
	"sync"
)

// Brew is the package manager backend for Homebrew-on-Linux. Useful on
// immutable distros (Universal Blue, Bluefin, Bazzite) where dnf is
// discouraged and brew is the default install path.
type Brew struct {
	Runner Runner

	// loadOnce + installed cache the parsed result of one `brew list`
	// invocation per Manager instance. See Flatpak for the rationale.
	loadOnce  sync.Once
	installed map[string]struct{}
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
// The installed set is loaded lazily on first call (one `brew list
// --formula` shellout) and reused thereafter for the lifetime of this
// Manager instance.
func (b *Brew) IsInstalled(formula string) bool {
	b.loadOnce.Do(b.loadInstalled)
	_, ok := b.installed[formula]
	return ok
}

func (b *Brew) loadInstalled() {
	b.installed = make(map[string]struct{})
	out, err := b.Runner("brew", "list", "--formula", "-1")
	if err != nil {
		return
	}
	sc := bufio.NewScanner(bytes.NewReader(out))
	for sc.Scan() {
		if name := strings.TrimSpace(sc.Text()); name != "" {
			b.installed[name] = struct{}{}
		}
	}
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
