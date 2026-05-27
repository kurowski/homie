package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func resetHomeFlags() {
	homeTarget = ""
}

func runHomeCli(t *testing.T, args []string) (string, error) {
	t.Helper()
	resetHomeFlags()
	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetErr(buf)
	rootCmd.SetArgs(args)
	err := rootCmd.Execute()
	return buf.String(), err
}

// TestHomeMaterializesBothClasses pins the contract for `hm home`:
// plain files in home/ become symlinks, .tmpl files render. Runs the
// same shared phase apply uses, so any drift between the two paths
// shows up here.
func TestHomeMaterializesBothClasses(t *testing.T) {
	repo := fixtureRepo(t)
	home := t.TempDir()
	t.Setenv("HM_REPO", repo)

	out, err := runHomeCli(t, []string{"home", "--home", home})
	if err != nil {
		t.Fatalf("home: %v\noutput:\n%s", err, out)
	}

	// Plain file symlinked.
	link := filepath.Join(home, ".zshrc")
	info, err := os.Lstat(link)
	if err != nil {
		t.Fatalf(".zshrc missing: %v", err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Errorf(".zshrc should be a symlink, mode=%v", info.Mode())
	}
	target, err := os.Readlink(link)
	if err != nil {
		t.Fatalf("readlink: %v", err)
	}
	if target != filepath.Join(repo, "home", ".zshrc") {
		t.Errorf(".zshrc points to %s, want path under repo home/", target)
	}

	// Template rendered.
	gitconfig, err := os.ReadFile(filepath.Join(home, ".gitconfig"))
	if err != nil {
		t.Fatalf("gitconfig: %v", err)
	}
	if !strings.Contains(string(gitconfig), "name = Scout Homes") {
		t.Errorf("gitconfig missing name substitution: %q", gitconfig)
	}

	// Output uses the same `== home ==` phase header apply does, since
	// runHomeCmd routes through applyHomePhase + the same ui.UI.
	if !strings.Contains(out, "== home ==") {
		t.Errorf("expected `== home ==` phase header, got:\n%s", out)
	}
	// Action verbs for both classes show up under that header.
	if !strings.Contains(out, "create") {
		t.Errorf("expected a `create` action for the symlink, got:\n%s", out)
	}
	if !strings.Contains(out, "render") {
		t.Errorf("expected a `render` action for the template, got:\n%s", out)
	}
}

// TestHomeSecondRunIsIdempotent confirms a re-run produces no new
// work and reports the in-sync state.
func TestHomeSecondRunIsIdempotent(t *testing.T) {
	repo := fixtureRepo(t)
	home := t.TempDir()
	t.Setenv("HM_REPO", repo)

	if _, err := runHomeCli(t, []string{"home", "--home", home}); err != nil {
		t.Fatalf("first run: %v", err)
	}
	out, err := runHomeCli(t, []string{"home", "--home", home})
	if err != nil {
		t.Fatalf("second run: %v\n%s", err, out)
	}
	if !strings.Contains(out, "already in sync") {
		t.Errorf("expected `already in sync` skip line on second run:\n%s", out)
	}
	if !strings.Contains(out, "All phases completed cleanly") {
		t.Errorf("expected clean summary on second run:\n%s", out)
	}
}
