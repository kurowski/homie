// Package link mirrors a repo's dotfiles/ directory into $HOME as symlinks.
//
// Plan walks the source tree and classifies what needs to happen at each
// destination. Apply executes the plan, backing up real files that would
// otherwise be overwritten. Apply collects non-fatal errors instead of
// aborting at the first one, so a single misbehaving file doesn't block
// the rest of `hm apply`.
package link

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// DotfilesDir is the base directory under the user repo that holds
// files to be symlinked into $HOME. Sibling directories named
// `dotfiles.tag-X[.tag-Y...]` are additional, tag-conditional trees;
// see Plan.
const DotfilesDir = "dotfiles"

// tagPart is the per-tag prefix inside a tag-conditional dotfiles
// directory name — e.g. dotfiles.tag-work.tag-kde -> ["work", "kde"].
const tagPart = "tag-"

// Kind describes what should happen at a target path.
type Kind string

const (
	KindCreate  Kind = "create"  // nothing at the target — create the symlink
	KindSkip    Kind = "skip"    // correct symlink already exists
	KindReplace Kind = "replace" // a symlink exists but points elsewhere
	KindBackup  Kind = "backup"  // a real file/dir exists — back it up first
)

// Action describes one symlink the planner wants to materialize.
type Action struct {
	Kind   Kind
	Source string // absolute path inside the repo
	Target string // absolute path inside $HOME
}

// Result is the outcome of applying a set of actions.
type Result struct {
	Created  []Action
	Skipped  []Action
	Replaced []Action
	Backed   []BackupRecord
	Errors   []error
}

// BackupRecord records a file that was moved aside before its symlink
// was created.
type BackupRecord struct {
	Action Action
	Backup string // absolute path of the backup
}

// Plan walks the active dotfile trees under repoDir and returns one
// Action per regular file. Trees are:
//   - <repoDir>/dotfiles (always)
//   - <repoDir>/dotfiles.tag-<a>[.tag-<b>...] when every named tag is
//     present in the given tags slice
//
// If no dotfile tree exists, Plan returns an empty slice with no error.
// If two trees contribute the same target path, Plan fails fast and
// returns an error — overriding by tag should be expressed in templates,
// not by stacking dotfile trees. This is stricter than render.Apply,
// which records per-file collision errors and continues; the asymmetry
// matches each package's existing error model (Plan/Apply split here,
// per-file collection there). Roots are visited in lexical order so any
// collision is deterministic.
func Plan(repoDir, home string, tags []string) ([]Action, error) {
	roots, err := ActiveTrees(repoDir, DotfilesDir, tags)
	if err != nil {
		return nil, err
	}

	var actions []Action
	bySource := make(map[string]string) // target -> source that claimed it
	for _, src := range roots {
		err := filepath.WalkDir(src, func(path string, d fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if d.IsDir() {
				return nil
			}
			rel, err := filepath.Rel(src, path)
			if err != nil {
				return err
			}
			target := filepath.Join(home, rel)
			if prev, ok := bySource[target]; ok {
				return fmt.Errorf("%s is claimed by both %s and %s — move one into the other tree or use a template",
					RelTo(repoDir, target), RelTo(repoDir, prev), RelTo(repoDir, path))
			}
			bySource[target] = path
			kind, err := classify(path, target)
			if err != nil {
				return err
			}
			actions = append(actions, Action{Kind: kind, Source: path, Target: target})
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	return actions, nil
}

// ActiveTrees returns the absolute paths of dotfile/template trees that
// apply for the given tags. The plain base directory (if it exists) is
// always first; tag-gated siblings whose required tag set is satisfied
// follow in lexical name order.
//
// base is the bare directory name without a tag suffix — "dotfiles" for
// dotfile trees, "templates" for template trees. Symmetric naming
// (templates.tag-X) keeps the convention shared across the two.
func ActiveTrees(repoDir, base string, tags []string) ([]string, error) {
	entries, err := os.ReadDir(repoDir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", repoDir, err)
	}

	active := make(map[string]struct{}, len(tags))
	for _, t := range tags {
		active[t] = struct{}{}
	}

	var roots []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		required, ok := ParseTreeDir(e.Name(), base)
		if !ok {
			continue
		}
		if !allActive(required, active) {
			continue
		}
		roots = append(roots, filepath.Join(repoDir, e.Name()))
	}
	return roots, nil
}

// ParseTreeDir returns the tags required by a tagged tree directory
// named like "<base>.tag-X.tag-Y", or (nil, true) for the bare "<base>"
// directory. ok is false if name doesn't follow either convention —
// useful for doctor to distinguish a malformed sibling like
// "dotfiles.backup" from a legitimate but inactive "dotfiles.tag-work".
func ParseTreeDir(name, base string) (tags []string, ok bool) {
	if name == base {
		return nil, true
	}
	rest, hasPrefix := strings.CutPrefix(name, base+".")
	if !hasPrefix {
		return nil, false
	}
	parts := strings.Split(rest, ".")
	tags = make([]string, 0, len(parts))
	for _, p := range parts {
		tag, hasTag := strings.CutPrefix(p, tagPart)
		if !hasTag || tag == "" {
			return nil, false
		}
		tags = append(tags, tag)
	}
	return tags, true
}

// RelTo returns p as a repo-relative path when it's inside repoDir,
// otherwise the absolute path unchanged. Used to keep collision error
// messages readable — the user knows which repo they're in. Exported so
// render can format its parallel collision messages the same way.
func RelTo(repoDir, p string) string {
	rel, err := filepath.Rel(repoDir, p)
	if err != nil || strings.HasPrefix(rel, "..") {
		return p
	}
	return rel
}

func allActive(required []string, active map[string]struct{}) bool {
	for _, t := range required {
		if _, ok := active[t]; !ok {
			return false
		}
	}
	return true
}

// classify decides what action is needed at target given that source is
// the canonical file in the repo.
func classify(source, target string) (Kind, error) {
	info, err := os.Lstat(target)
	if errors.Is(err, fs.ErrNotExist) {
		return KindCreate, nil
	}
	if err != nil {
		return "", err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		actual, err := os.Readlink(target)
		if err != nil {
			return "", err
		}
		if actual == source {
			return KindSkip, nil
		}
		return KindReplace, nil
	}
	return KindBackup, nil
}

// Apply executes the given actions. Now controls the clock for backup
// filenames (use time.Now in production).
func Apply(actions []Action, now time.Time) Result {
	var res Result
	for _, a := range actions {
		switch a.Kind {
		case KindSkip:
			res.Skipped = append(res.Skipped, a)
		case KindCreate:
			if err := doCreate(a); err != nil {
				res.Errors = append(res.Errors, fmt.Errorf("create %s: %w", a.Target, err))
				continue
			}
			res.Created = append(res.Created, a)
		case KindReplace:
			if err := os.Remove(a.Target); err != nil {
				res.Errors = append(res.Errors, fmt.Errorf("remove stale symlink %s: %w", a.Target, err))
				continue
			}
			if err := doCreate(a); err != nil {
				res.Errors = append(res.Errors, fmt.Errorf("relink %s: %w", a.Target, err))
				continue
			}
			res.Replaced = append(res.Replaced, a)
		case KindBackup:
			backup := backupPath(a.Target, now)
			if err := os.Rename(a.Target, backup); err != nil {
				res.Errors = append(res.Errors, fmt.Errorf("back up %s: %w", a.Target, err))
				continue
			}
			if err := doCreate(a); err != nil {
				res.Errors = append(res.Errors, fmt.Errorf("link %s: %w", a.Target, err))
				continue
			}
			res.Backed = append(res.Backed, BackupRecord{Action: a, Backup: backup})
		}
	}
	return res
}

func doCreate(a Action) error {
	if err := os.MkdirAll(filepath.Dir(a.Target), 0o755); err != nil {
		return err
	}
	return os.Symlink(a.Source, a.Target)
}

func backupPath(target string, now time.Time) string {
	return target + ".homie-backup-" + now.UTC().Format("2006-01-02-150405")
}
