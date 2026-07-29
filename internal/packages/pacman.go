package packages

import (
	"fmt"
	"os/exec"
	"strings"
)

// Pacman is the package manager backend for Arch Linux.
//
// It covers the official repos only. The AUR is deliberately out of
// scope: it needs a helper (yay, paru) that isn't part of a base install,
// and building from source is a different risk profile than fetching a
// signed binary package. An AUR helper is a natural non-native backend
// (see ForBackend) if someone wants one.
type Pacman struct {
	Runner Runner
	Sudo   bool // prepend `sudo` to mutating commands when not root
}

// Name returns "pacman".
func (p *Pacman) Name() string { return "pacman" }

// IsAvailable reports whether pacman is on PATH. Unlike apt (dpkg) and
// dnf (rpm), pacman is both the query tool and the installer, so there's
// only one binary to look for.
func (p *Pacman) IsAvailable() bool {
	_, err := exec.LookPath("pacman")
	return err == nil
}

// IsInstalled reports whether name is already satisfied locally.
//
// `pacman -T` (deptest) rather than `pacman -Q`: -T asks "is this
// requirement met by the local database", which resolves virtual
// packages via provides — so a spec like `sh` or `java-runtime` counts
// as installed when something providing it is. -Q matches the literal
// package name only. -T exits non-zero (and lists the misses) when a
// requirement isn't satisfied.
//
// Groups (`base-devel`) are the gap in both: a group is a label on a set
// of packages, not a requirement, so neither -T nor -Q reports a
// fully-installed one as present. The state stays correct — Install
// re-offers the group every run and `pacman -S --needed` makes that a
// no-op — but every *caller that reports* on this method is then wrong
// about a converged machine: `hm doctor` warns "not installed" forever,
// and `hm apply` announces an install it doesn't perform. Expanding a
// group into its members needs `pacman -Sg`, which reads the sync
// database Install deliberately never refreshes, so there's no fix here
// that doesn't fight that rule. Documented instead — /docs/config/ tells
// users to declare group members rather than groups.
func (p *Pacman) IsInstalled(name string) bool {
	_, err := p.Runner("pacman", "-T", name)
	return err == nil
}

// Install installs pkgs that aren't already present.
//
// No sync-database refresh happens here, which is the one place Pacman
// deviates from Apt (`apt-get update` before install). On Arch, refreshing
// and then installing — `pacman -Sy foo` — is the documented partial-
// upgrade footgun: it can pull a package built against libraries newer
// than the ones installed. The only safe refresh is a full `pacman -Syu`,
// and upgrading the whole system is not a decision `hm apply` should make
// on the user's behalf. So we install against the database as it stands
// and, when that's why an install failed, say so in the error.
func (p *Pacman) Install(pkgs []string) error {
	todo := filterUninstalled(p, pkgs)
	if len(todo) == 0 {
		return nil
	}
	args := []string{"pacman", "-S", "--needed", "--noconfirm"}
	args = append(args, todo...)
	cmd, rest := p.command(args)
	out, err := p.Runner(cmd, rest...)
	if err != nil {
		msg := strings.TrimSpace(string(out))
		if strings.Contains(msg, "target not found") {
			// Spell the suggestion the way this host would run it. The
			// failure fires most often on a fresh root environment
			// (container, arch-chroot, rescue boot) — precisely where
			// there's no sudo binary to prepend.
			refresh, rest := p.command([]string{"pacman", "-Syu"})
			suggest := strings.Join(append([]string{refresh}, rest...), " ")
			return fmt.Errorf("pacman -S: %w: %s (the package database may be stale — run `%s` and re-run)", err, msg, suggest)
		}
		return fmt.Errorf("pacman -S: %w: %s", err, msg)
	}
	return nil
}

func (p *Pacman) command(args []string) (string, []string) {
	if p.Sudo {
		return "sudo", args
	}
	return args[0], args[1:]
}
