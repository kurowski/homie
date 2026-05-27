package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeFile creates parent dirs and writes the file atomically.
func writeFile(t *testing.T, path, content string, mode os.FileMode) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), mode); err != nil {
		t.Fatal(err)
	}
}

// resetApplyFlags zeroes the cobra-managed globals so test ordering
// doesn't matter.
func resetApplyFlags() {
	applyHome = ""
	applySkipPackages = false
	applySkipScripts = false
}

// fixtureRepo builds a user environment repo with one of each artifact
// type and returns its path.
func fixtureRepo(t *testing.T) string {
	t.Helper()
	repo := t.TempDir()
	writeFile(t, filepath.Join(repo, "homie.toml"), `
[user]
name  = "Scout Homes"
email = "scout@homie.sh"

[profile]
name = "personal"

[vars]
EDITOR = "nvim"
`, 0o644)
	writeFile(t, filepath.Join(repo, "home", ".zshrc"), "# zshrc from repo\n", 0o644)
	writeFile(t, filepath.Join(repo, "home", ".gitconfig.tmpl"), `[user]
name = {{ .Name }}
email = {{ .Email }}
editor = {{ .Vars.EDITOR }}
`, 0o644)
	writeFile(t, filepath.Join(repo, "scripts", "01-marker.sh"),
		`touch "$HM_HOME/script-ran-marker"`, 0o755)
	return repo
}

func runApplyCmd(t *testing.T, args []string) (string, error) {
	t.Helper()
	resetApplyFlags()
	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetErr(buf)
	rootCmd.SetArgs(args)
	err := rootCmd.Execute()
	return buf.String(), err
}

func TestApplyEndToEnd(t *testing.T) {
	repo := fixtureRepo(t)
	home := t.TempDir()
	t.Setenv("HM_REPO", repo)

	out, err := runApplyCmd(t, []string{"apply", "--home", home, "--skip-packages"})
	if err != nil {
		t.Fatalf("apply: %v\noutput:\n%s", err, out)
	}

	// 1. Dotfile got symlinked into home.
	link := filepath.Join(home, ".zshrc")
	info, err := os.Lstat(link)
	if err != nil {
		t.Fatalf("zshrc missing: %v", err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Errorf(".zshrc should be a symlink, mode=%v", info.Mode())
	}
	target, err := os.Readlink(link)
	if err != nil {
		t.Fatalf("readlink: %v", err)
	}
	if target != filepath.Join(repo, "home", ".zshrc") {
		t.Errorf(".zshrc points to %s, want repo home file", target)
	}

	// 2. Template rendered with substitutions.
	gitconfig, err := os.ReadFile(filepath.Join(home, ".gitconfig"))
	if err != nil {
		t.Fatalf("gitconfig: %v", err)
	}
	if !strings.Contains(string(gitconfig), "name = Scout Homes") {
		t.Errorf("gitconfig missing name substitution: %q", gitconfig)
	}
	if !strings.Contains(string(gitconfig), "editor = nvim") {
		t.Errorf("gitconfig missing EDITOR var: %q", gitconfig)
	}

	// 3. Script ran (wrote marker).
	if _, err := os.Stat(filepath.Join(home, "script-ran-marker")); err != nil {
		t.Errorf("script-ran-marker missing: %v", err)
	}

	// 4. Summary line indicates clean run.
	if !strings.Contains(out, "All phases completed cleanly") {
		t.Errorf("summary missing clean line in output:\n%s", out)
	}
}

func TestApplyRunsPreScriptsBeforePackages(t *testing.T) {
	repo := fixtureRepo(t)
	home := t.TempDir()
	t.Setenv("HM_REPO", repo)
	// pre-*.sh records the time it ran; the post-script in fixtureRepo
	// records its own. Pre must run strictly before post.
	writeFile(t, filepath.Join(repo, "scripts", "pre-00-repos.sh"),
		`date +%s%N > "$HM_HOME/pre-marker"`, 0o755)

	out, err := runApplyCmd(t, []string{"apply", "--home", home, "--skip-packages"})
	if err != nil {
		t.Fatalf("apply: %v\n%s", err, out)
	}
	preRaw, err := os.ReadFile(filepath.Join(home, "pre-marker"))
	if err != nil {
		t.Fatalf("pre-marker missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(home, "script-ran-marker")); err != nil {
		t.Errorf("post-script marker missing: %v", err)
	}
	if len(strings.TrimSpace(string(preRaw))) == 0 {
		t.Errorf("pre-marker empty")
	}
	// Phase labels appear in the right order in the output stream.
	// Use the full bracketed form so "== scripts ==" doesn't match the
	// "== pre-scripts ==" header (substring trap).
	pre := strings.Index(out, "== pre-scripts ==")
	pkgs := strings.Index(out, "== packages ==")
	post := strings.Index(out, "== scripts ==")
	if pre < 0 || pkgs < 0 || post < 0 {
		t.Fatalf("phase labels missing in output:\n%s", out)
	}
	if pre >= pkgs || pkgs >= post {
		t.Errorf("phase ordering wrong: pre=%d packages=%d scripts=%d\n%s", pre, pkgs, post, out)
	}
}

func TestApplyIsIdempotent(t *testing.T) {
	repo := fixtureRepo(t)
	home := t.TempDir()
	t.Setenv("HM_REPO", repo)

	// First apply seeds the home.
	if _, err := runApplyCmd(t, []string{"apply", "--home", home, "--skip-packages", "--skip-scripts"}); err != nil {
		t.Fatalf("first apply: %v", err)
	}
	// Second apply must not error and must still report clean.
	out, err := runApplyCmd(t, []string{"apply", "--home", home, "--skip-packages", "--skip-scripts"})
	if err != nil {
		t.Fatalf("second apply: %v\n%s", err, out)
	}
	if !strings.Contains(out, "All phases completed cleanly") {
		t.Errorf("second run not clean:\n%s", out)
	}
	if !strings.Contains(out, "already in sync") {
		t.Errorf("second run should report dotfile skip:\n%s", out)
	}
}
