package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func resetInitFlags() {
	initName = ""
	initEmail = ""
	initGitHubUser = ""
	initGitHubRepo = "dotfiles"
	initProfile = "personal"
	initShell = "zsh"
}

func TestInitNonInteractiveScaffold(t *testing.T) {
	resetInitFlags()
	dir := t.TempDir()
	target := filepath.Join(dir, "myrepo")

	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetErr(buf)
	rootCmd.SetArgs([]string{
		"init",
		"--name", "Scout Homes",
		"--email", "scout@homie.sh",
		"--github-user", "scouthomes",
		target,
	})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("init failed: %v\noutput:\n%s", err, buf.String())
	}

	// All scaffold files should exist.
	for _, rel := range []string{"homie.toml", "bootstrap.sh", "dotfiles/.zshrc", "scripts/01-shell.sh"} {
		if _, err := os.Stat(filepath.Join(target, rel)); err != nil {
			t.Errorf("expected %s in scaffold: %v", rel, err)
		}
	}

	if !strings.Contains(buf.String(), "Next steps") {
		t.Errorf("expected next-steps hints in output: %s", buf.String())
	}
}

func TestInitThenApplySucceeds(t *testing.T) {
	resetInitFlags()
	resetApplyFlags()
	root := t.TempDir()
	repo := filepath.Join(root, "myrepo")
	home := filepath.Join(root, "home")
	if err := os.MkdirAll(home, 0o755); err != nil {
		t.Fatal(err)
	}

	// Scaffold.
	rootCmd.SetOut(new(bytes.Buffer))
	rootCmd.SetErr(new(bytes.Buffer))
	rootCmd.SetArgs([]string{
		"init",
		"--name", "Scout Homes",
		"--email", "scout@homie.sh",
		"--github-user", "scouthomes",
		repo,
	})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("init: %v", err)
	}

	// Apply against the freshly scaffolded repo.
	t.Setenv("HM_REPO", repo)
	out := new(bytes.Buffer)
	rootCmd.SetOut(out)
	rootCmd.SetErr(out)
	rootCmd.SetArgs([]string{
		"apply", "--home", home, "--skip-packages", "--skip-scripts",
	})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("apply: %v\noutput:\n%s", err, out.String())
	}

	// The sample dotfile should now be symlinked into home.
	if _, err := os.Lstat(filepath.Join(home, ".zshrc")); err != nil {
		t.Errorf(".zshrc should be linked: %v", err)
	}
	// The sample template should have rendered.
	gitconfig, err := os.ReadFile(filepath.Join(home, ".gitconfig"))
	if err != nil {
		t.Fatalf("gitconfig: %v", err)
	}
	if !strings.Contains(string(gitconfig), "name = Scout Homes") {
		t.Errorf(".gitconfig missing substitution: %s", gitconfig)
	}
}
