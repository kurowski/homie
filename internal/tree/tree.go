// Package tree models Homie's tag-conditional directory convention for
// the single `home/` tree (and its `home.tag-X[.tag-Y...]/` siblings).
// It describes which directories apply for a given tag set and which
// files in them are templates — what to do with each file is each
// consumer's concern (link symlinks plain files, render writes
// templates through text/template).
package tree

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// tagPart is the per-tag prefix inside a tag-conditional directory
// name — e.g. home.tag-work.tag-kde -> ["work", "kde"].
const tagPart = "tag-"

// HomeDir is the single base directory under a user environment repo
// that mirrors $HOME. Files inside are either symlinked (link) or
// rendered (render); the .tmpl suffix is the disambiguator. Sibling
// directories named `home.tag-X[.tag-Y...]` are additional
// tag-conditional trees.
const HomeDir = "home"

// TemplateExtension is the suffix that marks a file inside a home tree
// as a render template rather than a plain symlink source. The suffix
// is stripped from the rendered file's path in $HOME.
const TemplateExtension = ".tmpl"

// IsTemplate reports whether a filename belongs to render (true) or to
// link (false). Both walk the same home trees but partition files by
// this rule so each file has exactly one owner.
func IsTemplate(name string) bool {
	return strings.HasSuffix(name, TemplateExtension)
}

// Trees is the partition of <base>-shaped directories under a repo
// against an active tag set, produced by Classify. Active and Inactive
// are both absolute paths in lexical name order.
type Trees struct {
	// Active holds trees that apply on this host — the bare base
	// directory (if present) plus any `<base>.tag-X[...]` siblings
	// whose required tags are all in the active set.
	Active []string

	// Inactive holds tag-gated `<base>.tag-X[...]` siblings whose
	// required tags are NOT all in the active set. The bare base
	// directory is never here.
	Inactive []string
}

// Classify partitions tag-conditional sibling directories under
// repoDir against the given active-tag set. base is the bare directory
// name without a tag suffix (today, [HomeDir]). A missing repoDir
// returns the zero Trees with no error, so callers can treat it as a
// no-op.
func Classify(repoDir, base string, tags []string) (Trees, error) {
	entries, err := os.ReadDir(repoDir)
	if errors.Is(err, fs.ErrNotExist) {
		return Trees{}, nil
	}
	if err != nil {
		return Trees{}, fmt.Errorf("read %s: %w", repoDir, err)
	}

	activeSet := make(map[string]struct{}, len(tags))
	for _, t := range tags {
		activeSet[t] = struct{}{}
	}

	var t Trees
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		required, ok := ParseDir(e.Name(), base)
		if !ok {
			continue
		}
		full := filepath.Join(repoDir, e.Name())
		if allActive(required, activeSet) {
			t.Active = append(t.Active, full)
		} else {
			// Bare base dir has no required tags and is always active,
			// so it never lands here — only tag-gated siblings can be
			// inactive.
			t.Inactive = append(t.Inactive, full)
		}
	}
	return t, nil
}

// Active returns just the active subset from Classify. Most callers
// (link.Plan, render.Apply, doctor's template walk) only need this;
// they don't care about inactive siblings.
func Active(repoDir, base string, tags []string) ([]string, error) {
	t, err := Classify(repoDir, base, tags)
	return t.Active, err
}

// Resolved describes one file under home/ (or an active tag-gated
// sibling) that wins the override race for its $HOME target.
type Resolved struct {
	// Source is the absolute path of the winning file in the repo.
	Source string
	// Target is the absolute path in $HOME the file maps to. For
	// templates the .tmpl suffix is stripped.
	Target string
	// IsTemplate is true if Source ends in TemplateExtension. Lets
	// link skip the entry and render claim it.
	IsTemplate bool
}

// Resolve walks every active home tree and partitions files by their
// $HOME target, applying the more-specific-wins override rule.
// Specificity is the number of required tags in the source tree's
// directory name: bare HomeDir is 0, home.tag-X is 1, home.tag-X.tag-Y
// is 2.
//
// When two trees claim the same target:
//   - Differing specificity → the more-specific source wins silently.
//     Example: home/.gitconfig (spec 0) is overridden by
//     home.tag-work/.gitconfig.tmpl (spec 1) on a work host.
//   - Equal specificity → Resolve returns an error. The override is
//     ambiguous and the user should disambiguate by narrowing one
//     tree, collapsing into a single template, or both.
//
// Returned slice is sorted by Target for deterministic downstream
// behavior. Missing or empty home tree returns (nil, nil).
func Resolve(repoDir, home string, tags []string) ([]Resolved, error) {
	classify, err := Classify(repoDir, HomeDir, tags)
	if err != nil {
		return nil, err
	}

	type claim struct {
		source      string
		isTemplate  bool
		specificity int
	}
	claimed := make(map[string]claim)

	for _, root := range classify.Active {
		required, _ := ParseDir(filepath.Base(root), HomeDir)
		spec := len(required)

		err := filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if d.IsDir() {
				return nil
			}
			rel, err := filepath.Rel(root, path)
			if err != nil {
				return err
			}
			isTemplate := IsTemplate(d.Name())
			target := filepath.Join(home, rel)
			if isTemplate {
				target = filepath.Join(home, strings.TrimSuffix(rel, TemplateExtension))
			}
			existing, exists := claimed[target]
			switch {
			case !exists:
				claimed[target] = claim{source: path, isTemplate: isTemplate, specificity: spec}
			case spec > existing.specificity:
				claimed[target] = claim{source: path, isTemplate: isTemplate, specificity: spec}
			case spec < existing.specificity:
				// existing is more specific — keep it
			default:
				return fmt.Errorf("%s is claimed by both %s and %s at the same specificity — make one more specific (add a tag), or collapse them into a single template",
					RelTo(repoDir, target), RelTo(repoDir, existing.source), RelTo(repoDir, path))
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
	}

	out := make([]Resolved, 0, len(claimed))
	for target, c := range claimed {
		out = append(out, Resolved{Source: c.source, Target: target, IsTemplate: c.isTemplate})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Target < out[j].Target })
	return out, nil
}

// ParseDir returns the tags required by a tagged tree directory named
// like "<base>.tag-X.tag-Y", or (nil, true) for the bare "<base>"
// directory. ok is false if name doesn't follow either convention —
// useful for doctor to distinguish a malformed sibling like
// "dotfiles.backup" from a legitimate but inactive "dotfiles.tag-work".
func ParseDir(name, base string) (tags []string, ok bool) {
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
// messages readable — the user knows which repo they're in.
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
