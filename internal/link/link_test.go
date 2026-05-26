package link

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"
)

// setup builds a fake repo with a dotfiles/ tree and returns (repoDir, homeDir).
func setup(t *testing.T, files map[string]string) (string, string) {
	t.Helper()
	repo := t.TempDir()
	home := t.TempDir()
	for rel, content := range files {
		path := filepath.Join(repo, DotfilesDir, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return repo, home
}

func readSymlink(t *testing.T, p string) string {
	t.Helper()
	target, err := os.Readlink(p)
	if err != nil {
		t.Fatalf("readlink %s: %v", p, err)
	}
	return target
}

func TestPlanWithoutDotfilesDir(t *testing.T) {
	repo := t.TempDir()
	home := t.TempDir()
	actions, err := Plan(repo, home, nil)
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	if len(actions) != 0 {
		t.Errorf("expected 0 actions when no dotfiles/, got %d", len(actions))
	}
}

func TestApplyCreate(t *testing.T) {
	repo, home := setup(t, map[string]string{
		".zshrc":             "# zshrc\n",
		".config/git/config": "[user]\nname = Scout\n",
	})

	actions, err := Plan(repo, home, nil)
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	if len(actions) != 2 {
		t.Fatalf("expected 2 actions, got %d", len(actions))
	}
	for _, a := range actions {
		if a.Kind != KindCreate {
			t.Errorf("action for %s: kind=%s, want create", a.Target, a.Kind)
		}
	}

	res := Apply(actions, time.Now())
	if len(res.Errors) != 0 {
		t.Errorf("unexpected errors: %v", res.Errors)
	}
	if len(res.Created) != 2 {
		t.Errorf("Created count = %d, want 2", len(res.Created))
	}

	// Both symlinks should exist and point at the repo.
	for _, rel := range []string{".zshrc", ".config/git/config"} {
		target := filepath.Join(home, rel)
		want := filepath.Join(repo, DotfilesDir, rel)
		if got := readSymlink(t, target); got != want {
			t.Errorf("%s: symlink -> %s, want %s", target, got, want)
		}
	}
}

func TestApplyIsIdempotent(t *testing.T) {
	repo, home := setup(t, map[string]string{".zshrc": "x"})

	// First apply.
	actions, _ := Plan(repo, home, nil)
	Apply(actions, time.Now())

	// Second apply must be all skips.
	actions, _ = Plan(repo, home, nil)
	if len(actions) != 1 || actions[0].Kind != KindSkip {
		t.Fatalf("second Plan: %+v, want one skip", actions)
	}
	res := Apply(actions, time.Now())
	if len(res.Skipped) != 1 {
		t.Errorf("Skipped = %d, want 1", len(res.Skipped))
	}
	if len(res.Created)+len(res.Replaced)+len(res.Backed) != 0 {
		t.Errorf("idempotent apply should do no work, got %+v", res)
	}
}

func TestApplyReplacesStaleSymlink(t *testing.T) {
	repo, home := setup(t, map[string]string{".zshrc": "new"})

	// Existing symlink at the target pointing somewhere else.
	stale := filepath.Join(t.TempDir(), "elsewhere")
	if err := os.WriteFile(stale, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(home, ".zshrc")
	if err := os.Symlink(stale, target); err != nil {
		t.Fatal(err)
	}

	actions, _ := Plan(repo, home, nil)
	if actions[0].Kind != KindReplace {
		t.Fatalf("expected KindReplace, got %s", actions[0].Kind)
	}
	res := Apply(actions, time.Now())
	if len(res.Errors) != 0 {
		t.Errorf("unexpected errors: %v", res.Errors)
	}
	if len(res.Replaced) != 1 {
		t.Errorf("Replaced = %d, want 1", len(res.Replaced))
	}
	want := filepath.Join(repo, DotfilesDir, ".zshrc")
	if got := readSymlink(t, target); got != want {
		t.Errorf("symlink target = %s, want %s", got, want)
	}
}

func TestApplyBacksUpRealFile(t *testing.T) {
	repo, home := setup(t, map[string]string{".zshrc": "from repo"})

	// Real file already at the target.
	target := filepath.Join(home, ".zshrc")
	if err := os.WriteFile(target, []byte("user wrote this"), 0o644); err != nil {
		t.Fatal(err)
	}

	actions, _ := Plan(repo, home, nil)
	if actions[0].Kind != KindBackup {
		t.Fatalf("expected KindBackup, got %s", actions[0].Kind)
	}

	now := time.Date(2026, 5, 24, 9, 30, 0, 0, time.UTC)
	res := Apply(actions, now)
	if len(res.Errors) != 0 {
		t.Errorf("unexpected errors: %v", res.Errors)
	}
	if len(res.Backed) != 1 {
		t.Fatalf("Backed = %d, want 1", len(res.Backed))
	}

	backup := res.Backed[0].Backup
	if !strings.Contains(backup, ".homie-backup-2026-05-24-093000") {
		t.Errorf("backup path %q missing timestamp", backup)
	}
	data, err := os.ReadFile(backup)
	if err != nil {
		t.Fatalf("backup unreadable: %v", err)
	}
	if string(data) != "user wrote this" {
		t.Errorf("backup content = %q, want preserved user data", data)
	}

	// Target now resolves through the symlink to the repo content.
	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read target: %v", err)
	}
	if string(got) != "from repo" {
		t.Errorf("target content = %q, want from repo", got)
	}
}

func TestPlanTagGatedDirs(t *testing.T) {
	repo := t.TempDir()
	home := t.TempDir()
	mk := func(rel, content string) {
		path := filepath.Join(repo, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	mk("dotfiles/.zshrc", "base")
	mk("dotfiles.tag-work/.config/work-only", "work")
	mk("dotfiles.tag-personal/.config/personal-only", "personal")
	mk("dotfiles.tag-work.tag-kde/.config/plasma/rc", "work-and-kde")
	mk("dotfiles.backup/leftover", "ignored") // not a tag-gated tree; ignored
	mk("dotfiles.tag-/empty", "ignored")      // empty tag name; not recognized

	cases := []struct {
		name        string
		tags        []string
		wantTargets []string
	}{
		{
			name: "no tags: base only",
			tags: nil,
			wantTargets: []string{
				filepath.Join(home, ".zshrc"),
			},
		},
		{
			name: "work tag adds work tree",
			tags: []string{"work"},
			wantTargets: []string{
				filepath.Join(home, ".zshrc"),
				filepath.Join(home, ".config/work-only"),
			},
		},
		{
			name: "work + kde activates the two-tag tree",
			tags: []string{"work", "kde"},
			wantTargets: []string{
				filepath.Join(home, ".zshrc"),
				filepath.Join(home, ".config/work-only"),
				filepath.Join(home, ".config/plasma/rc"),
			},
		},
		{
			name: "only kde does not activate work-and-kde",
			tags: []string{"kde"},
			wantTargets: []string{
				filepath.Join(home, ".zshrc"),
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			actions, err := Plan(repo, home, tc.tags)
			if err != nil {
				t.Fatalf("Plan: %v", err)
			}
			got := make([]string, 0, len(actions))
			for _, a := range actions {
				got = append(got, a.Target)
			}
			sort.Strings(got)
			want := append([]string(nil), tc.wantTargets...)
			sort.Strings(want)
			if strings.Join(got, ",") != strings.Join(want, ",") {
				t.Errorf("targets:\n got: %v\nwant: %v", got, want)
			}
		})
	}
}

func TestPlanCollidingTreesError(t *testing.T) {
	repo := t.TempDir()
	home := t.TempDir()
	mk := func(rel, content string) {
		path := filepath.Join(repo, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	mk("dotfiles/.zshrc", "base")
	mk("dotfiles.tag-work/.zshrc", "work override")

	_, err := Plan(repo, home, []string{"work"})
	if err == nil {
		t.Fatal("expected collision error, got nil")
	}
	if !strings.Contains(err.Error(), ".zshrc") {
		t.Errorf("error missing target path: %v", err)
	}
}

func TestParseTreeDir(t *testing.T) {
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
			got, ok := ParseTreeDir(tc.name, tc.base)
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

func TestPlanMixedKinds(t *testing.T) {
	repo, home := setup(t, map[string]string{
		"a": "1",
		"b": "2",
		"c": "3",
	})

	// a: no destination — Create
	// b: already correctly linked — Skip
	if err := os.MkdirAll(home, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(repo, DotfilesDir, "b"), filepath.Join(home, "b")); err != nil {
		t.Fatal(err)
	}
	// c: real file at destination — Backup
	if err := os.WriteFile(filepath.Join(home, "c"), []byte("user"), 0o644); err != nil {
		t.Fatal(err)
	}

	actions, err := Plan(repo, home, nil)
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}

	got := make(map[string]Kind)
	for _, a := range actions {
		got[filepath.Base(a.Target)] = a.Kind
	}
	want := map[string]Kind{"a": KindCreate, "b": KindSkip, "c": KindBackup}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s: kind=%s, want %s", k, got[k], v)
		}
	}

	keys := make([]string, 0, len(got))
	for k := range got {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	if strings.Join(keys, ",") != "a,b,c" {
		t.Errorf("plan covered %v, want a,b,c", keys)
	}
}
