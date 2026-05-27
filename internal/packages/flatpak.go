package packages

import (
	"bufio"
	"bytes"
	"fmt"
	"os/exec"
	"strings"
	"sync"
)

// Flatpak is the package manager backend for Flatpak refs (e.g.
// "md.obsidian.Obsidian"). Installs come from the flathub remote;
// adding a remote is out of scope here and belongs in scripts/pre-*.sh.
type Flatpak struct {
	Runner Runner

	// loadOnce + installed cache the parsed result of one `flatpak list`
	// invocation per Manager instance, so the apply path's "bucket into
	// already / todo then call Install" doesn't shell out N times per
	// package and then re-shell out N more times inside filterUninstalled.
	loadOnce  sync.Once
	installed map[string]struct{}
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
// installed. The installed set is loaded lazily on first call and
// reused thereafter for the lifetime of this Manager instance — apply
// constructs a fresh Manager per phase, so staleness across phases is a
// non-issue.
func (f *Flatpak) IsInstalled(ref string) bool {
	f.loadOnce.Do(f.loadInstalled)
	_, ok := f.installed[ref]
	return ok
}

func (f *Flatpak) loadInstalled() {
	f.installed = make(map[string]struct{})
	out, err := f.Runner("flatpak", "list", "--app", "--columns=application")
	if err != nil {
		return
	}
	sc := bufio.NewScanner(bytes.NewReader(out))
	for sc.Scan() {
		if ref := strings.TrimSpace(sc.Text()); ref != "" {
			f.installed[ref] = struct{}{}
		}
	}
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
