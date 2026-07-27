package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// scaffoldRepo creates a repo with `hm init` and turns it into a git repo
// with an origin remote, which is where --update reads the GitHub
// coordinates from.
func scaffoldRepo(t *testing.T, origin string) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH")
	}
	resetInitFlags()
	dir := filepath.Join(t.TempDir(), "dotfiles")

	rootCmd.SetOut(new(bytes.Buffer))
	rootCmd.SetErr(new(bytes.Buffer))
	rootCmd.SetArgs([]string{
		"init",
		"--name", "Scout Homes",
		"--email", "scout@homie.sh",
		"--github-user", "scouthomes",
		dir,
	})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("init: %v", err)
	}
	for _, args := range [][]string{
		{"init", "-b", "main"},
		{"remote", "add", "origin", origin},
	} {
		cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	return dir
}

func runUpdate(t *testing.T, dir string, extra ...string) string {
	t.Helper()
	resetInitFlags()
	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetErr(buf)
	rootCmd.SetArgs(append([]string{"init", "--update", dir}, extra...))
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("init --update: %v\noutput:\n%s", err, buf.String())
	}
	return buf.String()
}

// TestInitOutsideHomeKeepsThePortableDefault is the e2e regression in
// unit form: the harness scaffolds into a temp dir on the host and
// expects the container to clone to $HOME/dotfiles. A repo built
// somewhere outside $HOME says nothing about where it will live on the
// machines bootstrap.sh runs on.
func TestInitOutsideHomeKeepsThePortableDefault(t *testing.T) {
	resetInitFlags()
	// Pin $HOME somewhere unrelated rather than assuming t.TempDir()
	// falls outside it — with TMPDIR under $HOME it doesn't, which is
	// how this test first failed for the wrong reason.
	t.Setenv("HOME", t.TempDir())
	dir := filepath.Join(t.TempDir(), "userrepo-src")
	rootCmd.SetOut(new(bytes.Buffer))
	rootCmd.SetErr(new(bytes.Buffer))
	rootCmd.SetArgs([]string{
		"init", "--name", "Scout Homes", "--email", "scout@homie.sh",
		"--github-user", "scouthomes", dir,
	})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("init: %v", err)
	}
	body, err := os.ReadFile(filepath.Join(dir, "bootstrap.sh"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), `REPO_DIR="${HM_REPO:-$HOME/dotfiles}"`) {
		t.Errorf("expected the portable $HOME/<repo> default, got:\n%s", firstLines(string(body), 40))
	}
}

// TestInitUnderHomeDerivesTheLocation is the other half: a repo built
// where it will live yields a $HOME-relative destination, nesting and
// all, so bootstrap.sh clones to the same place on the next machine.
func TestInitUnderHomeDerivesTheLocation(t *testing.T) {
	resetInitFlags()
	home := t.TempDir()
	t.Setenv("HOME", home)
	dir := filepath.Join(home, "Projects", "dotfiles")

	rootCmd.SetOut(new(bytes.Buffer))
	rootCmd.SetErr(new(bytes.Buffer))
	rootCmd.SetArgs([]string{
		"init", "--name", "Scout Homes", "--email", "scout@homie.sh",
		"--github-user", "scouthomes", dir,
	})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("init: %v", err)
	}
	body, err := os.ReadFile(filepath.Join(dir, "bootstrap.sh"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), `REPO_DIR="${HM_REPO:-$HOME/Projects/dotfiles}"`) {
		t.Errorf("expected the derived nested location:\n%s", firstLines(string(body), 40))
	}
}

func TestInitRepoDirFlagWins(t *testing.T) {
	resetInitFlags()
	dir := filepath.Join(t.TempDir(), "repo")
	rootCmd.SetOut(new(bytes.Buffer))
	rootCmd.SetErr(new(bytes.Buffer))
	rootCmd.SetArgs([]string{
		"init", "--name", "Scout Homes", "--email", "scout@homie.sh",
		"--github-user", "scouthomes", "--repo-dir", "/opt/dotfiles", dir,
	})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("init: %v", err)
	}
	body, _ := os.ReadFile(filepath.Join(dir, "bootstrap.sh"))
	if !strings.Contains(string(body), `REPO_DIR="${HM_REPO:-/opt/dotfiles}"`) {
		t.Errorf("--repo-dir should override the derived value:\n%s", firstLines(string(body), 40))
	}
}

func firstLines(s string, n int) string {
	lines := strings.SplitN(s, "\n", n+1)
	if len(lines) > n {
		lines = lines[:n]
	}
	return strings.Join(lines, "\n")
}

func TestInitUpdateIsCleanOnAFreshRepo(t *testing.T) {
	dir := scaffoldRepo(t, "https://github.com/scouthomes/dotfiles.git")
	out := runUpdate(t, dir)
	if !strings.Contains(out, "current") {
		t.Errorf("expected a no-op report, got:\n%s", out)
	}
}

// TestInitUpdateDerivesAnswersFromTheRepo is the point of the whole
// command: no prompts, no re-answering. Deleting the file forces a
// re-render, so the GitHub URL in the result proves where the answers
// came from — homie.toml for identity, origin for the coordinates.
func TestInitUpdateDerivesAnswersFromTheRepo(t *testing.T) {
	dir := scaffoldRepo(t, "git@github.com:otheruser/otherrepo.git")
	if err := os.Remove(filepath.Join(dir, "bootstrap.sh")); err != nil {
		t.Fatal(err)
	}

	out := runUpdate(t, dir)
	if !strings.Contains(out, "created") {
		t.Errorf("expected 'created', got:\n%s", out)
	}
	body, err := os.ReadFile(filepath.Join(dir, "bootstrap.sh"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), "https://github.com/otheruser/otherrepo.git") {
		t.Errorf("bootstrap.sh should point at the origin remote:\n%s", body)
	}
}

func TestInitUpdateStopsOnLocalEdits(t *testing.T) {
	dir := scaffoldRepo(t, "https://github.com/scouthomes/dotfiles.git")
	path := filepath.Join(dir, "bootstrap.sh")
	edited := "#!/usr/bin/env bash\n# entirely mine\n"
	if err := os.WriteFile(path, []byte(edited), 0o755); err != nil {
		t.Fatal(err)
	}

	out := runUpdate(t, dir)
	if !strings.Contains(out, "skipped") {
		t.Errorf("expected the edited file to be skipped:\n%s", out)
	}
	if !strings.Contains(out, "--force") {
		t.Errorf("expected the output to name the escape hatch:\n%s", out)
	}
	if body, _ := os.ReadFile(path); string(body) != edited {
		t.Errorf("update clobbered a local edit:\n%s", body)
	}
	// The diff is what makes "stop" actionable rather than just a refusal.
	if !strings.Contains(out, "entirely mine") {
		t.Errorf("expected a diff of the pending change:\n%s", out)
	}
}

func TestInitUpdateForceTakesHomiesVersion(t *testing.T) {
	dir := scaffoldRepo(t, "https://github.com/scouthomes/dotfiles.git")
	path := filepath.Join(dir, "bootstrap.sh")
	if err := os.WriteFile(path, []byte("#!/usr/bin/env bash\n# entirely mine\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	out := runUpdate(t, dir, "--force")
	if !strings.Contains(out, "forced") {
		t.Errorf("expected a forced report:\n%s", out)
	}
	body, _ := os.ReadFile(path)
	if strings.Contains(string(body), "entirely mine") {
		t.Errorf("--force should have replaced the file:\n%s", body)
	}
	if !strings.Contains(string(body), "withtty hm bootstrap") {
		t.Errorf("--force didn't write the current template:\n%s", body)
	}
}

func TestInitUpdateNeedsAHomieRepo(t *testing.T) {
	resetInitFlags()
	dir := t.TempDir() // no homie.toml
	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetErr(buf)
	rootCmd.SetArgs([]string{"init", "--update", dir})
	if err := rootCmd.Execute(); err == nil {
		t.Errorf("expected --update to refuse a non-repo, got nil\noutput:\n%s", buf.String())
	}
}

func TestGithubFromRemote(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH")
	}
	cases := map[string]struct{ user, repo string }{
		"https://github.com/scouthomes/dotfiles.git": {"scouthomes", "dotfiles"},
		"https://github.com/scouthomes/dotfiles":     {"scouthomes", "dotfiles"},
		"git@github.com:scouthomes/dotfiles.git":     {"scouthomes", "dotfiles"},
		"ssh://git@github.com/scouthomes/dotfiles":   {"scouthomes", "dotfiles"},
	}
	for url, want := range cases {
		dir := t.TempDir()
		for _, args := range [][]string{{"init", "-b", "main"}, {"remote", "add", "origin", url}} {
			if out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput(); err != nil {
				t.Fatalf("git %v: %v\n%s", args, err, out)
			}
		}
		user, repo, err := githubFromRemote(dir)
		if err != nil {
			t.Errorf("%s: %v", url, err)
			continue
		}
		if user != want.user || repo != want.repo {
			t.Errorf("%s → %s/%s, want %s/%s", url, user, repo, want.user, want.repo)
		}
	}
}

func TestGithubFromRemoteWithoutOrigin(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH")
	}
	dir := t.TempDir()
	if out, err := exec.Command("git", "-C", dir, "init", "-b", "main").CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}
	if _, _, err := githubFromRemote(dir); err == nil {
		t.Error("expected an error when there's no origin remote")
	}
}
