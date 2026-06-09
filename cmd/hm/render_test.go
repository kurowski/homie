package main

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"
)

func runRenderCli(t *testing.T, args []string) (string, error) {
	t.Helper()
	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetErr(buf)
	rootCmd.SetArgs(args)
	err := rootCmd.Execute()
	return buf.String(), err
}

// TestRenderToStdout pins the contract for `hm render <path>`: the raw
// rendered content goes to stdout with no UI chrome, using the same
// data a real `hm home` would. The repo-relative path exercises the
// fallback — the test's cwd is not the repo.
func TestRenderToStdout(t *testing.T) {
	repo := fixtureRepo(t)
	t.Setenv("HM_REPO", repo)

	out, err := runRenderCli(t, []string{"render", "home/.gitconfig.tmpl"})
	if err != nil {
		t.Fatalf("render: %v\noutput:\n%s", err, out)
	}
	want := "[user]\nname = Scout Homes\nemail = scout@homie.sh\neditor = nvim\n"
	if out != want {
		t.Errorf("rendered output = %q, want %q", out, want)
	}
}

// TestRenderAbsolutePath confirms an absolute path works as given.
func TestRenderAbsolutePath(t *testing.T) {
	repo := fixtureRepo(t)
	t.Setenv("HM_REPO", repo)

	out, err := runRenderCli(t, []string{"render", filepath.Join(repo, "home", ".gitconfig.tmpl")})
	if err != nil {
		t.Fatalf("render: %v\noutput:\n%s", err, out)
	}
	if !strings.Contains(out, "name = Scout Homes") {
		t.Errorf("expected rendered content, got:\n%s", out)
	}
}

// TestRenderErrorsExitNonZero covers the two failure modes an authoring
// loop depends on: a missing file and a template that fails to execute.
func TestRenderErrorsExitNonZero(t *testing.T) {
	repo := fixtureRepo(t)
	t.Setenv("HM_REPO", repo)

	if out, err := runRenderCli(t, []string{"render", "home/.does-not-exist.tmpl"}); err == nil {
		t.Errorf("expected error for missing file\noutput:\n%s", out)
	}

	writeFile(t, filepath.Join(repo, "home", ".broken.tmpl"), "{{ .NoSuchField }}\n", 0o644)
	if out, err := runRenderCli(t, []string{"render", "home/.broken.tmpl"}); err == nil {
		t.Errorf("expected error for broken template\noutput:\n%s", out)
	}
}
