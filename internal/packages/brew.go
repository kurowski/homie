package packages

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"sync"
)

// Brew is the Homebrew package manager. It serves two roles:
//
//   - the native manager on macOS, returned by For(env) when
//     env.PackageManager == "brew";
//   - an opt-in backend on Linux (Universal Blue, Bluefin, Bazzite and
//     other immutable distros where dnf is discouraged), returned by
//     ForBackend("brew").
//
// Casks (GUI apps, macOS-only) are encoded as a "/cask" suffix on the
// package name: "wget" is a formula, "firefox/cask" is a cask. A bare name
// is always a formula, so the Linux backend — which only ever names
// formulae — is unaffected. brew formula/cask names never contain a slash,
// so "/" is an unambiguous delimiter.
type Brew struct {
	Runner Runner

	// Installed formulae and casks are cached separately, each behind its
	// own sync.Once, so a formula-only repo never shells out to
	// `brew list --cask`. See Flatpak for the caching rationale.
	formulaOnce sync.Once
	formulae    map[string]struct{}
	caskOnce    sync.Once
	casks       map[string]struct{}
}

// brewCaskSuffix is the only recognized package-name suffix. A spec with
// any other suffix is a hard error (see parseBrewSpec).
const brewCaskSuffix = "cask"

// parseBrewSpec splits a package spec into its bare name and whether it's a
// cask. A spec with no "/" is a formula. A "/cask" suffix marks a cask. Any
// other suffix — including a tap-qualified name like "org/tap/foo", whose
// first "/" splits off a non-"cask" remainder — is a hard error so a typo
// or unsupported form fails loudly rather than silently installing the
// wrong thing.
func parseBrewSpec(spec string) (name string, cask bool, err error) {
	name, suffix, found := strings.Cut(spec, "/")
	if name == "" {
		// Catches both "" and "/cask" (empty name before the suffix).
		return "", false, fmt.Errorf("brew package %q has an empty name", spec)
	}
	if !found {
		return spec, false, nil
	}
	if suffix != brewCaskSuffix {
		return "", false, fmt.Errorf("brew package %q has unknown suffix %q — the only valid suffix is /cask; tap-qualified names (e.g. org/tap/foo) aren't supported, install those from a scripts/pre-*.sh", spec, suffix)
	}
	return name, true, nil
}

// Name returns "brew".
func (b *Brew) Name() string { return "brew" }

// Validate reports the first spec with an unknown suffix, or nil if every
// spec parses. Implements [Validator] so apply and doctor can flag a typo
// like "foo/bogus" before attempting (or pretending to skip) an install.
func (b *Brew) Validate(specs []string) error {
	for _, spec := range specs {
		if _, _, err := parseBrewSpec(spec); err != nil {
			return err
		}
	}
	return nil
}

// IsAvailable reports whether the brew command-line tool is on PATH.
// If it isn't, the apply phase logs a warning and skips — brew is optional
// (the default macOS manager and an opt-in Linux backend), and a host
// without it shouldn't fail outright.
func (b *Brew) IsAvailable() bool {
	_, err := exec.LookPath("brew")
	return err == nil
}

// IsInstalled reports whether the named formula (or "name/cask") is
// currently installed. The matching installed set is loaded lazily on first
// use. A spec with an invalid suffix reports not-installed so it flows into
// Install, where the suffix is validated and the error surfaced.
func (b *Brew) IsInstalled(spec string) bool {
	name, cask, err := parseBrewSpec(spec)
	if err != nil {
		return false
	}
	if cask {
		b.caskOnce.Do(b.loadCasks)
		_, ok := b.casks[name]
		return ok
	}
	b.formulaOnce.Do(b.loadFormulae)
	_, ok := b.formulae[name]
	return ok
}

func (b *Brew) loadFormulae() {
	b.formulae = b.loadList("--formula")
}

func (b *Brew) loadCasks() {
	b.casks = b.loadList("--cask")
}

// loadList runs `brew list <kind> -1` and returns the names as a set. A
// failure (e.g. brew not installed) yields an empty set, so callers treat
// everything as not-installed and the install attempt surfaces the real
// error.
func (b *Brew) loadList(kind string) map[string]struct{} {
	set := make(map[string]struct{})
	out, err := b.Runner("brew", "list", kind, "-1")
	if err != nil {
		return set
	}
	sc := bufio.NewScanner(bytes.NewReader(out))
	for sc.Scan() {
		if name := strings.TrimSpace(sc.Text()); name != "" {
			set[name] = struct{}{}
		}
	}
	return set
}

// Install installs the formulae and casks not already present. Every spec is
// validated up front, so a bad suffix fails the batch with a clear message
// before any shellout.
//
// Formulae install in a single `brew install A B C` (they batch cleanly).
// Casks install one at a time: a single conflict — most often
// /Applications/<App>.app already present from an App Store or direct
// download — makes a batched `brew install --cask A B C` abort and silently
// skip the rest, so an unrelated cask later in the same [packages].macos
// list never gets installed. Installing each cask separately keeps one
// conflict from blocking the others; failures are collected (not aborted on)
// and each points at `--adopt` to take over the existing app.
//
// Like Snap (and unlike flatpak), this can't reuse filterUninstalled: every
// spec must be parsed (to split name from the /cask suffix) and bucketed in
// the same pass, so the not-installed filter is folded into that loop.
func (b *Brew) Install(specs []string) error {
	var formulae, casks []string
	for _, spec := range specs {
		name, cask, err := parseBrewSpec(spec)
		if err != nil {
			return err
		}
		if b.IsInstalled(spec) {
			continue
		}
		if cask {
			casks = append(casks, name)
		} else {
			formulae = append(formulae, name)
		}
	}
	var errs []error
	if len(formulae) > 0 {
		if err := b.run(nil, formulae); err != nil {
			errs = append(errs, err)
		}
	}
	for _, c := range casks {
		if err := b.run([]string{"--cask"}, []string{c}); err != nil {
			// The hint is conditional: cask installs also fail on network
			// errors, typos, and checksum mismatches, where --adopt is
			// irrelevant. /Applications conflicts are just the common case.
			errs = append(errs, fmt.Errorf("%w\n    if this failed because /Applications/<App>.app already exists, retry with: brew install --cask --adopt %s", err, c))
		}
	}
	return errors.Join(errs...)
}

// run executes one `brew install [flags] <names>` invocation. The error
// names the exact command (including the package names) so a failure in a
// per-cask loop points at the offending cask rather than a bare
// "brew install --cask:" repeated per line.
func (b *Brew) run(flags, names []string) error {
	args := []string{"install"}
	args = append(args, flags...)
	args = append(args, names...)
	out, err := b.Runner("brew", args...)
	if err != nil {
		return fmt.Errorf("brew %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return nil
}
