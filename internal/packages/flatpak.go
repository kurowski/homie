package packages

import (
	"bufio"
	"bytes"
	"fmt"
	"os/exec"
	"strings"
)

// Flatpak is the package manager backend for Flatpak refs (e.g.
// "md.obsidian.Obsidian"). Installs come from the flathub remote;
// adding a remote is out of scope here and belongs in scripts/pre-*.sh.
type Flatpak struct {
	Runner Runner
}

// Name returns "flatpak".
func (f *Flatpak) Name() string { return "flatpak" }

// IsAvailable reports whether the flatpak command-line tool is on PATH.
// If it isn't, the apply phase logs a warning and skips silently —
// flatpak is opt-in and not every host runs it.
func (f *Flatpak) IsAvailable() bool {
	_, err := exec.LookPath("flatpak")
	return err == nil
}

// IsInstalled reports whether the given application ref is currently
// installed. We list once and check membership rather than running
// `flatpak info` per package because list is the cheaper round-trip.
func (f *Flatpak) IsInstalled(ref string) bool {
	out, err := f.Runner("flatpak", "list", "--app", "--columns=application")
	if err != nil {
		return false
	}
	sc := bufio.NewScanner(bytes.NewReader(out))
	for sc.Scan() {
		if strings.TrimSpace(sc.Text()) == ref {
			return true
		}
	}
	return false
}

// Install installs refs that aren't already installed via the flathub
// remote. Empty input is a no-op.
func (f *Flatpak) Install(refs []string) error {
	todo := filterUninstalled(f, refs)
	if len(todo) == 0 {
		return nil
	}
	args := []string{"install", "-y", "--noninteractive", "flathub"}
	args = append(args, todo...)
	out, err := f.Runner("flatpak", args...)
	if err != nil {
		return fmt.Errorf("flatpak install: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}
