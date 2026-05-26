// Package tree models Homie's tag-conditional directory convention,
// shared by the dotfile (`dotfiles/`, `dotfiles.tag-X/`) and template
// (`templates/`, `templates.tag-X/`) trees. It only describes which
// directories apply for a given tag set — what to do with the files
// inside is each consumer's concern (link symlinks them, render writes
// them through text/template).
package tree

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// tagPart is the per-tag prefix inside a tag-conditional directory
// name — e.g. dotfiles.tag-work.tag-kde -> ["work", "kde"].
const tagPart = "tag-"

// Active returns the absolute paths of trees that apply for the given
// tag set. The plain base directory (if it exists) is always first;
// tag-gated siblings whose required tag set is satisfied follow in
// lexical name order.
//
// base is the bare directory name without a tag suffix — "dotfiles" for
// dotfile trees, "templates" for template trees. Symmetric naming keeps
// the convention shared across the two.
func Active(repoDir, base string, tags []string) ([]string, error) {
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
		required, ok := ParseDir(e.Name(), base)
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
