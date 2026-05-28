package packages

import (
	"bufio"
	"bytes"
	"fmt"
	"os/exec"
	"sort"
	"strings"
	"sync"
)

// Snap is the package manager backend for snapd. Useful on Ubuntu where
// some tools ship only as snaps (the AWS CLI's official distribution,
// Spotify, GIMP, ...). Like flatpak/brew it's opt-in by tool presence —
// a host without snapd skips the phase with a warning.
//
// Confinement is encoded as a suffix on the package name: "aws-cli/classic"
// installs with --classic, "foo/devmode" with --devmode, "foo/jailmode"
// with --jailmode. A bare name installs under default (strict) confinement.
// snap package names are [a-z0-9-] only and never contain a slash, so "/"
// is an unambiguous delimiter.
type Snap struct {
	Runner Runner
	// Sudo prepends sudo to install commands. snap install needs root;
	// ForBackend sets this from the effective uid, tests set it directly.
	Sudo bool

	// loadOnce + installed cache the parsed result of one `snap list`
	// invocation per Manager instance. See Flatpak for the rationale.
	loadOnce  sync.Once
	installed map[string]struct{}
}

// snapModes maps a confinement suffix to the snap install flag it implies.
// The bare (no-suffix) case means default strict confinement and takes no
// flag, so it's deliberately absent. This map is the single source of
// truth for which suffixes are valid.
var snapModes = map[string]string{
	"classic":  "--classic",
	"devmode":  "--devmode",
	"jailmode": "--jailmode",
}

// parseSnapSpec splits a package spec into its bare snap name and
// confinement mode. A spec with no "/" is strict confinement (mode ""). A
// "/<mode>" suffix outside snapModes is a hard error so a typo like
// "foo/bogus" fails loudly rather than installing under the wrong (or no)
// confinement.
func parseSnapSpec(spec string) (name, mode string, err error) {
	name, mode, found := strings.Cut(spec, "/")
	if !found {
		return spec, "", nil
	}
	if _, ok := snapModes[mode]; !ok {
		return "", "", fmt.Errorf("snap package %q has unknown confinement mode %q — valid: %s", spec, mode, validSnapModes())
	}
	return name, mode, nil
}

// validSnapModes returns the recognized confinement suffixes in sorted
// order, for error messages.
func validSnapModes() string {
	modes := make([]string, 0, len(snapModes))
	for m := range snapModes {
		modes = append(modes, m)
	}
	sort.Strings(modes)
	return strings.Join(modes, ", ")
}

// Name returns "snap".
func (s *Snap) Name() string { return "snap" }

// Validate reports the first spec with an unknown confinement suffix, or
// nil if every spec parses. Implements [Validator] so apply and doctor can
// flag a typo like "foo/bogus" before attempting (or pretending to skip)
// an install.
func (s *Snap) Validate(specs []string) error {
	for _, spec := range specs {
		if _, _, err := parseSnapSpec(spec); err != nil {
			return err
		}
	}
	return nil
}

// IsAvailable reports whether the snap command-line tool is on PATH.
// If it isn't, the apply phase logs a warning and skips silently — snap
// is opt-in and Fedora hosts won't have snapd by default.
func (s *Snap) IsAvailable() bool {
	_, err := exec.LookPath("snap")
	return err == nil
}

// IsInstalled reports whether the snap named by spec is installed. The
// confinement suffix is ignored for the lookup (snap list keys on the
// bare name). A spec with an invalid suffix reports not-installed so it
// flows into Install, where the suffix is validated and the error
// surfaced.
func (s *Snap) IsInstalled(spec string) bool {
	name, _, err := parseSnapSpec(spec)
	if err != nil {
		return false
	}
	s.loadOnce.Do(s.loadInstalled)
	_, ok := s.installed[name]
	return ok
}

func (s *Snap) loadInstalled() {
	s.installed = make(map[string]struct{})
	out, err := s.Runner("snap", "list")
	if err != nil {
		return
	}
	sc := bufio.NewScanner(bytes.NewReader(out))
	first := true
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		// `snap list` leads with a "Name  Version ..." column header.
		// Anchor on that literal so an unexpected header-less output (or a
		// future format change) doesn't silently drop the first real snap.
		if first {
			first = false
			if strings.HasPrefix(line, "Name") {
				continue
			}
		}
		s.installed[strings.Fields(line)[0]] = struct{}{}
	}
}

// Install installs the snaps not already present. Every spec is validated
// up front, so a bad confinement suffix fails the batch with a clear
// message before any shellout. Because --classic (etc.) applies to the
// whole `snap install` invocation, installs are grouped by confinement
// mode and run one command per mode.
//
// Unlike flatpak/brew, this can't reuse the shared filterUninstalled
// helper: every spec must be parsed (to split name from confinement) and
// bucketed by mode in the same pass, so the not-installed filter is folded
// into that loop rather than run separately.
func (s *Snap) Install(specs []string) error {
	byMode := make(map[string][]string)
	for _, spec := range specs {
		name, mode, err := parseSnapSpec(spec)
		if err != nil {
			return err
		}
		if s.IsInstalled(spec) {
			continue
		}
		byMode[mode] = append(byMode[mode], name)
	}
	if len(byMode) == 0 {
		return nil
	}
	// Sort modes for deterministic command order ("" sorts first, so the
	// strict group installs before any confinement-flagged groups).
	modes := make([]string, 0, len(byMode))
	for m := range byMode {
		modes = append(modes, m)
	}
	sort.Strings(modes)
	for _, mode := range modes {
		args := []string{"snap", "install"}
		args = append(args, byMode[mode]...)
		if flag := snapModes[mode]; flag != "" {
			args = append(args, flag)
		}
		cmd, rest := s.command(args)
		out, err := s.Runner(cmd, rest...)
		if err != nil {
			// Name the confinement group so a failure in one of several
			// mode batches points at the right command.
			confinement := mode
			if confinement == "" {
				confinement = "strict"
			}
			return fmt.Errorf("snap install (%s): %w: %s", confinement, err, strings.TrimSpace(string(out)))
		}
	}
	return nil
}

func (s *Snap) command(args []string) (string, []string) {
	if s.Sudo {
		return "sudo", args
	}
	return args[0], args[1:]
}
