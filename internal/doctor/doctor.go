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
	r.checkTagBlocks(cfg, env)
	r.checkScripts(repoDir, cfg, env)
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

// checkTagBlocks surfaces which multi-tag (AND) [packages] blocks are
// active for the current tag set, so the user can confirm an AND-condition
// like [packages."tag:personal.tag:ubuntu"] fired on this host. Single-tag
// blocks aren't reported — they're unremarkable.
func (r *Report) checkTagBlocks(cfg config.Config, env detect.Env) {
	for _, key := range cfg.ActiveTagBlocks(env) {
		r.add(SeverityInfo, "packages",
			fmt.Sprintf(`[packages."%s"] is active (all tags satisfied)`, key))
	}
}

func (r *Report) checkLinks(repoDir, home string, cfg config.Config, env detect.Env) {
	actions, err := link.Plan(repoDir, home, cfg.AllTags(env))
	if err != nil {
		r.add(SeverityError, "link", fmt.Sprintf("plan home: %v", err))
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
				fmt.Sprintf("%s not yet linked — run `hm apply` or `hm home`", a.Target))
		}
	}
	// Detect broken symlinks: a homie-managed symlink whose source file
	// has been removed from the repo. Plan only surfaces files still in
	// the active trees, so we scan the directories homie actually
	// mirrors and flag symlinks into any home* tree under repoDir whose
	// target is missing.
	broken := findBrokenLinks(repoDir, home, cfg.AllTags(env))
	sort.Strings(broken)
	for _, p := range broken {
		r.add(SeverityError, "link",
			fmt.Sprintf("%s is a broken symlink (source no longer in repo)", p))
	}

	// Surface tag-gated home trees that won't apply on this host so the
	// user can confirm their multi-tag layout matches expectations.
	// Reported under area "home" rather than "link" because the trees
	// hold both symlink sources and templates; the finding is about the
	// directory itself, not the link mechanism.
	active := cfg.AllTags(env)
	for _, p := range inactiveTreeDirs(repoDir, tree.HomeDir, active) {
		r.add(SeverityInfo, "home",
			fmt.Sprintf("%s is not active on this host (tags not satisfied)", p))
	}
}

// findBrokenLinks returns paths of homie-managed symlinks whose source
// file has been removed from the repo. A symlink is homie-managed when
// it points into the bare home/ dir or a sibling home.tag-X/... under
// repoDir.
//
// We deliberately do NOT walk $HOME — on macOS that includes ~/Library,
// cloud-sync stub trees (iCloud, OneDrive, Creative Cloud, Dropbox) and
// other huge or fault-prone subtrees that can take minutes or hang on
// stat. Instead we walk the repo's small home* trees to discover the
// set of $HOME directories homie mirrors, then list each one directly.
// $HOME itself is always included so a broken symlink at the root is
// caught even if the tree currently has nothing else there.
//
// taggedPrefix intentionally matches `home.<anything>`, not just dirs
// that pass tree.ParseDir. A stale link into a renamed-away `home.backup/`
// is still a broken homie-shaped link the user probably wants to clean
// up — tightening this to tree.ParseDir would silently lose those reports.
func findBrokenLinks(repoDir, home string, tags []string) []string {
	homeBase := filepath.Join(repoDir, tree.HomeDir)
	plainPrefix := homeBase + string(os.PathSeparator)
	taggedPrefix := homeBase + "."

	var out []string
	for _, dir := range homieMirroredDirs(repoDir, home, tags) {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue // unreadable dirs are not our problem
		}
		for _, e := range entries {
			path := filepath.Join(dir, e.Name())
			info, err := os.Lstat(path)
			if err != nil || info.Mode()&os.ModeSymlink == 0 {
				continue
			}
			dest, err := os.Readlink(path)
			if err != nil {
				continue
			}
			if !strings.HasPrefix(dest, plainPrefix) && !strings.HasPrefix(dest, taggedPrefix) {
				continue
			}
			if _, err := os.Stat(dest); errors.Is(err, fs.ErrNotExist) {
				out = append(out, path)
			}
		}
	}
	return out
}

// homieMirroredDirs returns the set of $HOME directories that any home*
// tree in the repo mirrors — i.e., the only directories a homie-managed
// symlink can live in. Both active and inactive tag-gated trees
// contribute, since a stale symlink may point into a tree that has
// since gone inactive on this host. $HOME itself is always included.
func homieMirroredDirs(repoDir, home string, tags []string) []string {
	trees, _ := tree.Classify(repoDir, tree.HomeDir, tags)
	roots := append([]string{}, trees.Active...)
	roots = append(roots, trees.Inactive...)

	seen := map[string]struct{}{home: {}}
	for _, root := range roots {
		_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil || !d.IsDir() {
				return nil
			}
			rel, err := filepath.Rel(root, path)
			if err != nil {
				return nil
			}
			seen[filepath.Join(home, rel)] = struct{}{}
			return nil
		})
	}
	out := make([]string, 0, len(seen))
	for p := range seen {
		out = append(out, p)
	}
	return out
}

// inactiveTreeDirs returns the bare names of tag-gated tree directories
// (<base>.tag-X[.tag-Y...]) under repoDir whose tag set is NOT
// satisfied by the active set. Result is sorted for stable reporting.
// Thin wrapper over tree.Classify — the partitioning rule lives there.
func inactiveTreeDirs(repoDir, base string, activeTags []string) []string {
	t, _ := tree.Classify(repoDir, base, activeTags)
	out := make([]string, 0, len(t.Inactive))
	for _, p := range t.Inactive {
		out = append(out, filepath.Base(p))
	}
	sort.Strings(out)
	return out
}

func (r *Report) checkTemplates(repoDir, home string, cfg config.Config, env detect.Env) {
	active := cfg.AllTags(env)
	roots, err := tree.Active(repoDir, tree.HomeDir, active)
	if err != nil {
		r.add(SeverityError, "render", err.Error())
		return
	}
	data := render.BuildData(cfg, env)
	for _, src := range roots {
		_ = filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() || !tree.IsTemplate(d.Name()) {
				return nil
			}
			rel, _ := filepath.Rel(src, path)
			target := filepath.Join(home, strings.TrimSuffix(rel, tree.TemplateExtension))

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
					fmt.Sprintf("%s not yet rendered — run `hm apply` or `hm home`", target))
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
	// No inactive-trees finding here — checkLinks already emits one per
	// home.tag-X dir under the "home" area; emitting again from this
	// method would double-report (templates now share the same trees as
	// dotfiles).
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
		// Surface malformed specs (e.g. a bad snap confinement suffix) as
		// a precise error rather than letting them fall through IsInstalled
		// and report as "not installed".
		if v, ok := mgr.(packages.Validator); ok {
			if err := v.Validate(want); err != nil {
				r.add(SeverityError, "packages", fmt.Sprintf("%s: %v", name, err))
				continue
			}
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
	want := cfg.PackagesFor(env)
	if !mgr.IsAvailable() {
		// brew is the default macOS manager but optional: with no packages
		// declared a dotfiles-only host is a clean run, and a missing brew
		// when packages *are* declared is a warning, not an error.
		if mgr.Name() == "brew" {
			if len(want) == 0 {
				return
			}
			r.add(SeverityWarn, "packages",
				fmt.Sprintf("brew not on PATH — %d package(s) declared will not install (install brew or add a scripts/pre-*.sh)", len(want)))
			return
		}
		r.add(SeverityError, "packages",
			fmt.Sprintf("package manager %s is not on PATH", mgr.Name()))
		return
	}
	if v, ok := mgr.(packages.Validator); ok {
		if err := v.Validate(want); err != nil {
			r.add(SeverityError, "packages", fmt.Sprintf("%s: %v", mgr.Name(), err))
			return
		}
	}
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

func (r *Report) checkScripts(repoDir string, cfg config.Config, env detect.Env) {
	tags := cfg.AllTags(env)
	// Walk both phases so every active *.sh (pre and post) is checked.
	// runner.Plan applies the same tag-tree merge and filename-collision
	// rule as `hm apply`, so a collision surfaces here too.
	for _, phase := range []runner.Phase{runner.PhasePre, runner.PhasePost} {
		paths, err := runner.Plan(repoDir, tags, phase)
		if err != nil {
			r.add(SeverityError, "scripts", err.Error())
			continue
		}
		for _, p := range paths {
			info, err := os.Stat(p)
			if err != nil {
				continue
			}
			if info.Mode()&0o111 == 0 {
				r.add(SeverityWarn, "scripts",
					fmt.Sprintf("%s is not executable (chmod +x recommended)", tree.RelTo(repoDir, p)))
			}
		}
	}
	// Surface tag-gated script trees that won't run on this host, mirroring
	// the home-tree behavior in checkLinks.
	for _, p := range inactiveTreeDirs(repoDir, runner.ScriptsDir, tags) {
		r.add(SeverityInfo, "scripts",
			fmt.Sprintf("%s is not active on this host (tags not satisfied)", p))
	}
}
