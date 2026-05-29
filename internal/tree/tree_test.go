package tree

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func mkResolveFixture(t *testing.T, files map[string]string) (repo, home string) {
	t.Helper()
	repo = t.TempDir()
	home = t.TempDir()
	for rel, body := range files {
		path := filepath.Join(repo, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return
}

func resolveTargets(rs []Resolved) map[string]Resolved {
	out := make(map[string]Resolved, len(rs))
	for _, r := range rs {
		out[filepath.Base(r.Target)] = r
	}
	return out
}

func TestResolveOverridesByDirectorySpecificity(t *testing.T) {
	repo, home := mkResolveFixture(t, map[string]string{
		// spec 0 base
		"home/.zshrc":     "base zshrc",
		"home/.gitconfig": "base gitconfig",
		"home/keep":       "unique to base",
		// spec 1 — overrides base for shared targets
		"home.tag-work/.zshrc":          "work zshrc",
		"home.tag-work/.gitconfig.tmpl": "work gitconfig template",
		"home.tag-work/work-only":       "unique to work",
	})

	got, err := Resolve(repo, home, []string{"work"})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	byName := resolveTargets(got)

	// .zshrc: same-class override
	if r := byName[".zshrc"]; r.Source != filepath.Join(repo, "home.tag-work", ".zshrc") {
		t.Errorf(".zshrc winner = %s, want home.tag-work/.zshrc", r.Source)
	}
	// .gitconfig: cross-class override (plain → template wins)
	if r := byName[".gitconfig"]; !r.IsTemplate || r.Source != filepath.Join(repo, "home.tag-work", ".gitconfig.tmpl") {
		t.Errorf(".gitconfig winner = %+v, want template from home.tag-work", r)
	}
	// keep: only in base, no contender — base wins
	if r := byName["keep"]; r.Source != filepath.Join(repo, "home", "keep") {
		t.Errorf("keep winner = %s, want base", r.Source)
	}
	// work-only: only in work tree
	if r := byName["work-only"]; r.Source != filepath.Join(repo, "home.tag-work", "work-only") {
		t.Errorf("work-only winner = %s, want work tree", r.Source)
	}
}

func TestResolveSpecificityDepth(t *testing.T) {
	// spec 0 < spec 1 < spec 2 — the deepest active tree wins.
	repo, home := mkResolveFixture(t, map[string]string{
		"home/.foo":                       "base",
		"home.tag-work/.foo":              "work",
		"home.tag-work.tag-kde/.foo.tmpl": "work-kde template",
	})
	got, err := Resolve(repo, home, []string{"work", "kde"})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 entry, got %d: %+v", len(got), got)
	}
	wantSrc := filepath.Join(repo, "home.tag-work.tag-kde", ".foo.tmpl")
	if got[0].Source != wantSrc || !got[0].IsTemplate {
		t.Errorf("winner = %+v, want template from home.tag-work.tag-kde", got[0])
	}
}

func TestResolveAmbiguousErrors(t *testing.T) {
	cases := []struct {
		name  string
		files map[string]string
		tags  []string
	}{
		{
			name: "within bare tree, plain and template",
			files: map[string]string{
				"home/.gitconfig":      "plain",
				"home/.gitconfig.tmpl": "template",
			},
		},
		{
			name: "sibling tag dirs at the same specificity",
			files: map[string]string{
				"home.tag-work/.foo":     "work",
				"home.tag-personal/.foo": "personal",
			},
			tags: []string{"work", "personal"},
		},
		{
			name: "sibling tag dirs, cross-class",
			files: map[string]string{
				"home.tag-work/.foo":          "work plain",
				"home.tag-personal/.foo.tmpl": "personal template",
			},
			tags: []string{"work", "personal"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo, home := mkResolveFixture(t, tc.files)
			_, err := Resolve(repo, home, tc.tags)
			if err == nil {
				t.Fatal("expected ambiguity error, got nil")
			}
			if !strings.Contains(err.Error(), "same specificity") {
				t.Errorf("error should mention specificity: %v", err)
			}
		})
	}
}

func TestResolveSortedByTarget(t *testing.T) {
	repo, home := mkResolveFixture(t, map[string]string{
		"home/z":      "z",
		"home/a":      "a",
		"home/m.tmpl": "m",
	})
	got, err := Resolve(repo, home, nil)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	prev := ""
	for _, r := range got {
		if prev != "" && r.Target <= prev {
			t.Errorf("not sorted by Target: %v", got)
			break
		}
		prev = r.Target
	}
}

func TestParseDir(t *testing.T) {
	cases := []struct {
		name     string
		base     string
		wantTags []string
		wantOK   bool
	}{
		{name: "dotfiles", base: "dotfiles", wantTags: nil, wantOK: true},
		{name: "dotfiles.tag-work", base: "dotfiles", wantTags: []string{"work"}, wantOK: true},
		{name: "dotfiles.tag-work.tag-kde", base: "dotfiles", wantTags: []string{"work", "kde"}, wantOK: true},
		{name: "dotfiles.backup", base: "dotfiles", wantOK: false},
		{name: "dotfiles.tag-", base: "dotfiles", wantOK: false},
		{name: "templates.tag-work", base: "templates", wantTags: []string{"work"}, wantOK: true},
		{name: "something-else", base: "dotfiles", wantOK: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := ParseDir(tc.name, tc.base)
			if ok != tc.wantOK {
				t.Fatalf("ok=%v, want %v", ok, tc.wantOK)
			}
			if !ok {
				return
			}
			if strings.Join(got, ",") != strings.Join(tc.wantTags, ",") {
				t.Errorf("tags = %v, want %v", got, tc.wantTags)
			}
		})
	}
}
