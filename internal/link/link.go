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
	"time"
)

// DotfilesDir is the directory under the user repo that holds files to
// be symlinked into $HOME.
const DotfilesDir = "dotfiles"

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

// Plan walks <repoDir>/dotfiles and returns one Action per regular file.
// If the dotfiles directory does not exist, Plan returns an empty slice
// with no error (the user repo may legitimately have no dotfiles).
func Plan(repoDir, home string) ([]Action, error) {
	src := filepath.Join(repoDir, DotfilesDir)
	info, err := os.Stat(src)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("stat %s: %w", src, err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("%s is not a directory", src)
	}

	var actions []Action
	err = filepath.WalkDir(src, func(path string, d fs.DirEntry, walkErr error) error {
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
	return actions, nil
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
