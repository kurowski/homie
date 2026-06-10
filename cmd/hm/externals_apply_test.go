package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// sourceGitRepo creates a real local git repo with one commit, usable
// as an [externals] clone source without touching the network.
func sourceGitRepo(t *testing.T) string {
	t.Helper()
	src := filepath.Join(t.TempDir(), "plug-src")
	writeFile(t, filepath.Join(src, "plugin.zsh"), "# plugin\n", 0o644)
	for _, args := range [][]string{
		{"init", "-b", "main", src},
		{"-C", src, "add", "plugin.zsh"},
		{"-C", src, "-c", "user.name=Scout Homes", "-c", "user.email=scout@homie.sh", "commit", "-m", "init"},
	} {
		if out, err := exec.Command("git", args...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	return src
}

// TestApplyExternalsEndToEnd drives the externals phase through real
// git against a local source repo: first apply clones, second apply is
// an idempotent skip.
func TestApplyExternalsEndToEnd(t *testing.T) {
	src := sourceGitRepo(t)
	repo := fixtureRepo(t)
	home := t.TempDir()
	t.Setenv("HM_REPO", repo)

	f, err := os.OpenFile(filepath.Join(repo, "homie.toml"), os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString("\n[externals.\"~/.zsh/plug\"]\nrepo = \"" + src + "\"\n"); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}

	out, err := runApplyCmd(t, []string{"apply", "--home", home, "--skip-packages", "--skip-scripts"})
	if err != nil {
		t.Fatalf("apply: %v\noutput:\n%s", err, out)
	}
	if !strings.Contains(out, "clone") {
		t.Errorf("first apply should report a clone:\n%s", out)
	}
	if _, err := os.Stat(filepath.Join(home, ".zsh", "plug", "plugin.zsh")); err != nil {
		t.Errorf("clone content missing: %v", err)
	}

	out, err = runApplyCmd(t, []string{"apply", "--home", home, "--skip-packages", "--skip-scripts"})
	if err != nil {
		t.Fatalf("second apply: %v\noutput:\n%s", err, out)
	}
	if !strings.Contains(out, "up to date") {
		t.Errorf("second apply should skip the converged checkout:\n%s", out)
	}
}

// TestApplySkipExternals: the flag announces the phase and does nothing.
func TestApplySkipExternals(t *testing.T) {
	src := sourceGitRepo(t)
	repo := fixtureRepo(t)
	home := t.TempDir()
	t.Setenv("HM_REPO", repo)

	f, err := os.OpenFile(filepath.Join(repo, "homie.toml"), os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString("\n[externals.\"~/.zsh/plug\"]\nrepo = \"" + src + "\"\n"); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}

	out, err := runApplyCmd(t, []string{"apply", "--home", home, "--skip-packages", "--skip-scripts", "--skip-externals"})
	if err != nil {
		t.Fatalf("apply: %v\noutput:\n%s", err, out)
	}
	if !strings.Contains(out, "skipped (--skip-externals)") {
		t.Errorf("expected the skip notice:\n%s", out)
	}
	if _, err := os.Stat(filepath.Join(home, ".zsh")); !os.IsNotExist(err) {
		t.Errorf("nothing should have been cloned, stat err = %v", err)
	}
}
