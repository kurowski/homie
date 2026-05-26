// Package doctor performs a read-only audit of a user environment repo
// and the host it's about to be applied to. It walks the same directories
// as `hm apply` but never writes — surfacing issues like broken symlinks,
// missing packages, or unrendered templates so users can fix them before
// (or independently of) running apply.
package doctor

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/kurowski/homie/internal/config"
	"github.com/kurowski/homie/internal/detect"
	"github.com/kurowski/homie/internal/link"
	"github.com/kurowski/homie/internal/packages"
	"github.com/kurowski/homie/internal/render"
	"github.com/kurowski/homie/internal/runner"
	"github.com/kurowski/homie/internal/tree"
)

// Severity classifies a finding. Errors cause `hm doctor` to exit 1;
// warnings are advisory.
type Severity string

const (
	SeverityError Severity = "error"
	SeverityWarn  Severity = "warn"
	// SeverityInfo is for context the user might want to confirm but
	// shouldn't act on — for example, tag-gated tree directories that
	// aren't active on this host. Info findings don't count toward
	// HasErrors or the warn count.
	SeverityInfo Severity = "info"
)

// Finding is one issue surfaced by Run.
type Finding struct {
	Severity Severity
	Area     string // env | config | link | render | packages | scripts
	Message  string
}

// Report aggregates findings from a single run.
type Report struct {
	Findings []Finding
}

// HasErrors reports whether any finding is at error severity. Useful
// for `hm doctor`'s exit code in CI.
func (r Report) HasErrors() bool {
	for _, f := range r.Findings {
		if f.Severity == SeverityError {
			return true
		}
	}
	return false
}

// Counts returns the number of errors and warnings.
func (r Report) Counts() (errs, warns int) {
	for _, f := range r.Findings {
		switch f.Severity {
		case SeverityError:
			errs++
		case SeverityWarn:
			warns++
		}
	}
	return
}

// Run walks the user repo and host, returning everything that looks off.
// The Manager is injected so tests can drive package checks without
// shelling out — `cmd/hm/doctor.go` passes packages.For(env).
func Run(repoDir, home string, cfg config.Config, env detect.Env, mgr packages.Manager, backendLookup BackendManagerLookup) Report {
	var r Report
	r.checkEnv(env)
	r.checkConfig(cfg)
	r.checkLinks(repoDir, home, cfg, env)
	r.checkTemplates(repoDir, home, cfg, env)
	r.checkPackages(cfg, env, mgr)
	r.checkBackendPackages(cfg, env, backendLookup)
	r.checkScripts(repoDir)
	return r
}

func (r *Report) add(sev Severity, area, msg string) {
	r.Findings = append(r.Findings, Finding{Severity: sev, Area: area, Message: msg})
}

func (r *Report) checkEnv(env detect.Env) {
	if env.Distro == "unknown" {
		r.add(SeverityWarn, "env",
			"distro not recognized — see homie.sh/contributing to add support")
	}
	if env.Hostname == "" {
		r.add(SeverityWarn, "env",
			"hostname unavailable — no host tag will be added and hosts/<name>.toml overlays will not load")
	}
}

func (r *Report) checkConfig(cfg config.Config) {
	for _, w := range cfg.Warnings {
		r.add(SeverityWarn, "config", w)
	}
}

func (r *Report) checkLinks(repoDir, home string, cfg config.Config, env detect.Env) {
	actions, err := link.Plan(repoDir, home, cfg.AllTags(env))
	if err != nil {
		r.add(SeverityError, "link", fmt.Sprintf("plan dotfiles: %v", err))
		return
	}
	for _, a := range actions {
		switch a.Kind {
		case link.KindReplace:
			actual, _ := os.Readlink(a.Target)
			r.add(SeverityError, "link",
				fmt.Sprintf("%s is a symlink to %s, expected %s", a.Target, actual, a.Source))
		case link.KindBackup:
			r.add(SeverityWarn, "link",
				fmt.Sprintf("%s exists as a real file — `hm apply` would back it up", a.Target))
		case link.KindCreate:
			r.add(SeverityWarn, "link",
				fmt.Sprintf("%s not yet linked — run `hm apply` or `hm link`", a.Target))
		}
	}
	// Detect broken symlinks: a homie-managed symlink whose source file
	// has been removed from the repo. Plan only surfaces files still in
	// the active trees, so we walk $HOME for symlinks pointing into any
	// dotfiles* tree under repoDir and flag any whose target is missing.
	broken := findBrokenLinks(home, filepath.Join(repoDir, link.DotfilesDir))
	sort.Strings(broken)
	for _, p := range broken {
		r.add(SeverityError, "link",
			fmt.Sprintf("%s is a broken symlink (source no longer in repo)", p))
	}

	// Surface tag-gated dotfile trees that won't apply on this host so
	// the user can confirm their multi-tag layout matches expectations.
	active := cfg.AllTags(env)
	for _, p := range inactiveTreeDirs(repoDir, link.DotfilesDir, active) {
		r.add(SeverityInfo, "link",
			fmt.Sprintf("%s is not active on this host (tags not satisfied)", p))
	}
}

// findBrokenLinks walks home and returns paths of symlinks that point
// into any homie dotfile tree (the bare dotfiles/ dir or a sibling
// dotfiles.tag-X/...) but whose target file no longer exists.
//
// dotfilesBase is the absolute path of <repoDir>/dotfiles. A symlink
// dest matches when it starts with that path followed by either a path
// separator (plain) or "." (tag-gated sibling).
//
// taggedPrefix intentionally matches `dotfiles.<anything>`, not just
// dirs that pass ParseTreeDir. A stale link into a renamed-away
// `dotfiles.backup/` is still a broken homie-shaped link the user
// probably wants to clean up — tightening this to ParseTreeDir would
// silently lose those reports.
func findBrokenLinks(home, dotfilesBase string) []string {
	var out []string
	plainPrefix := dotfilesBase + string(os.PathSeparator)
	taggedPrefix := dotfilesBase + "."
	_ = filepath.WalkDir(home, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // unreadable subtrees are not our problem
		}
		if d.IsDir() {
			return nil
		}
		info, err := os.Lstat(path)
		if err != nil || info.Mode()&os.ModeSymlink == 0 {
			return nil
		}
		dest, err := os.Readlink(path)
		if err != nil {
			return nil
		}
		if !strings.HasPrefix(dest, plainPrefix) && !strings.HasPrefix(dest, taggedPrefix) {
			return nil
		}
		if _, err := os.Stat(dest); errors.Is(err, fs.ErrNotExist) {
			out = append(out, path)
		}
		return nil
	})
	return out
}

// inactiveTreeDirs returns the names of tag-gated tree directories
// (<base>.tag-X[.tag-Y...]) under repoDir whose tag set is NOT satisfied
// by the active set. Result is sorted by directory name for stable
// reporting. Used by doctor to inform the user which tag-gated trees
// won't apply on this host.
func inactiveTreeDirs(repoDir, base string, activeTags []string) []string {
	entries, err := os.ReadDir(repoDir)
	if err != nil {
		return nil
	}
	active := make(map[string]struct{}, len(activeTags))
	for _, t := range activeTags {
		active[t] = struct{}{}
	}

	var out []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		required, ok := tree.ParseDir(e.Name(), base)
		if !ok || len(required) == 0 {
			continue // not a tagged tree, or the bare base dir
		}
		// "Inactive" means at least one required tag isn't in the
		// active set — the same test ActiveTrees applies, computed
		// directly here so we make one ReadDir pass instead of two.
		satisfied := true
		for _, t := range required {
			if _, ok := active[t]; !ok {
				satisfied = false
				break
			}
		}
		if !satisfied {
			out = append(out, e.Name())
		}
	}
	sort.Strings(out)
	return out
}

func (r *Report) checkTemplates(repoDir, home string, cfg config.Config, env detect.Env) {
	active := cfg.AllTags(env)
	roots, err := tree.Active(repoDir, render.TemplatesDir, active)
	if err != nil {
		r.add(SeverityError, "render", err.Error())
		return
	}
	data := render.BuildData(cfg, env)
	for _, src := range roots {
		_ = filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() || !strings.HasSuffix(d.Name(), render.Extension) {
				return nil
			}
			rel, _ := filepath.Rel(src, path)
			target := filepath.Join(home, strings.TrimSuffix(rel, render.Extension))

			raw, err := os.ReadFile(path)
			if err != nil {
				r.add(SeverityError, "render", fmt.Sprintf("read %s: %v", path, err))
				return nil
			}
			want, err := render.Render(string(raw), data)
			if err != nil {
				r.add(SeverityError, "render", fmt.Sprintf("%s: %v", path, err))
				return nil
			}
			got, err := os.ReadFile(target)
			if errors.Is(err, fs.ErrNotExist) {
				r.add(SeverityWarn, "render",
					fmt.Sprintf("%s not yet rendered — run `hm apply` or `hm render`", target))
				return nil
			}
			if err != nil {
				r.add(SeverityError, "render", fmt.Sprintf("read %s: %v", target, err))
				return nil
			}
			if string(got) != want {
				r.add(SeverityWarn, "render",
					fmt.Sprintf("%s is stale — re-render to pick up template/var changes", target))
			}
			return nil
		})
	}
	for _, p := range inactiveTreeDirs(repoDir, render.TemplatesDir, active) {
		r.add(SeverityInfo, "render",
			fmt.Sprintf("%s is not active on this host (tags not satisfied)", p))
	}
}

// BackendManagerLookup resolves a backend name (e.g. "flatpak", "brew")
// to a Manager, or nil if the name is unknown. Injected so doctor tests
// can supply fakes without invoking the real packages.ForBackend.
type BackendManagerLookup func(name string) packages.Manager

func (r *Report) checkBackendPackages(cfg config.Config, env detect.Env, lookup BackendManagerLookup) {
	if lookup == nil {
		return
	}
	// Iterate deterministically so warnings appear in a stable order.
	names := make([]string, 0, len(cfg.Packages.Backends))
	for n := range cfg.Packages.Backends {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, name := range names {
		want := cfg.PackagesForBackend(env, name)
		if len(want) == 0 {
			continue
		}
		mgr := lookup(name)
		if mgr == nil {
			r.add(SeverityWarn, "packages",
				fmt.Sprintf("backend %q is declared in homie.toml but homie has no Manager for it", name))
			continue
		}
		if !mgr.IsAvailable() {
			r.add(SeverityWarn, "packages",
				fmt.Sprintf("%s not on PATH — %d package(s) declared for it will not install", name, len(want)))
			continue
		}
		var missing []string
		for _, p := range want {
			if !mgr.IsInstalled(p) {
				missing = append(missing, p)
			}
		}
		if len(missing) > 0 {
			r.add(SeverityWarn, "packages",
				fmt.Sprintf("%s: %d not installed: %s", name, len(missing), strings.Join(missing, ", ")))
		}
	}
}

func (r *Report) checkPackages(cfg config.Config, env detect.Env, mgr packages.Manager) {
	if mgr == nil || mgr.Name() == "noop" {
		return // distro check already covers this
	}
	if !mgr.IsAvailable() {
		r.add(SeverityError, "packages",
			fmt.Sprintf("package manager %s is not on PATH", mgr.Name()))
		return
	}
	want := cfg.PackagesFor(env)
	var missing []string
	for _, p := range want {
		if !mgr.IsInstalled(p) {
			missing = append(missing, p)
		}
	}
	if len(missing) > 0 {
		r.add(SeverityWarn, "packages",
			fmt.Sprintf("%d not installed: %s", len(missing), strings.Join(missing, ", ")))
	}
}

func (r *Report) checkScripts(repoDir string) {
	dir := filepath.Join(repoDir, runner.ScriptsDir)
	entries, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return
	}
	if err != nil {
		r.add(SeverityError, "scripts", fmt.Sprintf("read %s: %v", dir, err))
		return
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), runner.Extension) {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		if info.Mode()&0o111 == 0 {
			r.add(SeverityWarn, "scripts",
				fmt.Sprintf("%s is not executable (chmod +x recommended)", e.Name()))
		}
	}
}
