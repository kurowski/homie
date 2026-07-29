package packages

import (
	"bufio"
	"bytes"
	"fmt"
	"os/exec"
	"strings"
	"sync"
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

	// loadOnce + installed cache one `pacman -Qq` per Manager instance,
	// so the apply path's "bucket into already / todo, then call Install"
	// doesn't fork pacman N times and then N more inside filterUninstalled.
	// Same trick the non-native backends use; pacman is the one native
	// backend where a whole-database dump is a single cheap command that
	// needs no sync database.
	loadOnce  sync.Once
	installed map[string]struct{}

	// resolved memoizes the -T answers for specs that aren't literal
	// installed package names, so the second filtering pass over a list of
	// genuinely-missing packages doesn't re-fork for every one of them.
	mu       sync.Mutex
	resolved map[string]bool
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
// Two queries, because neither answers the whole question. `pacman -Qq`
// dumps every installed package name in one fork and is cached, so the
// common case — a literal package name — is a set lookup, and a hit is
// conclusive. A name absent from that set may still be satisfied by a
// *provides* (`sh` comes from bash, `java-runtime` from a JDK), which
// only `pacman -T` (deptest) resolves — -Q matches literal names only —
// so a miss falls through to one memoized -T fork. -T exits non-zero,
// listing the misses, when a requirement isn't satisfied.
//
// The cached view is frozen for this Manager instance's lifetime. apply
// builds a fresh Manager per phase and nothing re-checks after Install,
// so staleness never surfaces — the same contract the non-native
// backends' caches carry.
//
// Two spec shapes satisfy neither query, so both are re-offered to
// Install on every run however converged the machine is: a group (`xorg`
// — a label on a set of packages, absent from -Qq and not a dependency
// -T can resolve) and a repo-qualified name (`extra/tmux` — -Qq prints
// bare names and -T doesn't parse the prefix). `base-devel` is *not* an
// example: it's an ordinary meta-package now and resolves normally.
//
// What that costs depends on the sync database. With one present,
// `pacman -S --needed` re-offers and does nothing, so only the *report*
// is wrong — doctor warns "not installed" forever, apply announces an
// install it won't perform. With no synced database, which is the
// environment Install's own error path is written for, that same call
// exits 1 with "target not found": the package phase then *fails* every
// run on a host where every member is already installed, and the
// `pacman -Syu` remediation makes the failure look transient when the
// underlying property isn't.
//
// Resolving a group in code means `pacman -Sg`, which reads the sync
// database Install deliberately never refreshes. /docs/config/ tells
// users to name packages instead.
func (p *Pacman) IsInstalled(name string) bool {
	p.loadOnce.Do(p.loadInstalled)
	if _, ok := p.installed[name]; ok {
		return true
	}

	p.mu.Lock()
	defer p.mu.Unlock()
	if satisfied, ok := p.resolved[name]; ok {
		return satisfied
	}
	_, err := p.Runner("pacman", "-T", name)
	satisfied := err == nil
	if p.resolved == nil {
		p.resolved = make(map[string]bool)
	}
	p.resolved[name] = satisfied
	return satisfied
}

// loadInstalled parses `pacman -Qq` — one package name per line, read
// from the local database, so it works on a host that has never synced.
// A failure leaves the set empty, which degrades to a -T fork per spec:
// slower, same answers.
func (p *Pacman) loadInstalled() {
	p.installed = make(map[string]struct{})
	out, err := p.Runner("pacman", "-Qq")
	if err != nil {
		return
	}
	sc := bufio.NewScanner(bytes.NewReader(out))
	for sc.Scan() {
		name := strings.TrimSpace(sc.Text())
		// Package names never contain spaces, so this drops pacman's
		// stderr chatter ("warning: database file for 'core' does not
		// exist"), which execRunner merges into the same stream.
		if name == "" || strings.ContainsAny(name, " \t") {
			continue
		}
		p.installed[name] = struct{}{}
	}
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
