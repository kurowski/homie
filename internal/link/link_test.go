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
	actions, err := Plan(repo, home)
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

	actions, err := Plan(repo, home)
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
	actions, _ := Plan(repo, home)
	Apply(actions, time.Now())

	// Second apply must be all skips.
	actions, _ = Plan(repo, home)
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

	actions, _ := Plan(repo, home)
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

	actions, _ := Plan(repo, home)
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

	actions, err := Plan(repo, home)
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
