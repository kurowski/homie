package main

import (
	"bytes"
	"encoding/json"
	"slices"
	"strings"
	"testing"
)

func resetStatusFlags() {
	statusHome = ""
	statusJSON = false
}

func runStatusCmd(t *testing.T, args []string) (string, error) {
	t.Helper()
	resetStatusFlags()
	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetErr(buf)
	rootCmd.SetArgs(args)
	err := rootCmd.Execute()
	return buf.String(), err
}

// TestStatusJSON pins the machine-readable contract: stdout is one JSON
// document and nothing else, with environment, repo, and health fields
// populated from the same data the text path prints.
func TestStatusJSON(t *testing.T) {
	repo := fixtureRepo(t)
	home := t.TempDir()
	t.Setenv("HM_REPO", repo)

	out, err := runStatusCmd(t, []string{"status", "--json", "--home", home})
	if err != nil {
		t.Fatalf("status --json: %v\noutput:\n%s", err, out)
	}

	var doc statusOutput
	if err := json.Unmarshal([]byte(out), &doc); err != nil {
		t.Fatalf("output is not valid JSON: %v\noutput:\n%s", err, out)
	}
	if doc.Environment.Distro == "" || doc.Environment.Arch == "" {
		t.Errorf("environment not populated: %+v", doc.Environment)
	}
	if doc.Repo == nil {
		t.Fatalf("repo is null, expected populated repo:\n%s", out)
	}
	if doc.Repo.Path != repo {
		t.Errorf("repo.path = %q, want %q", doc.Repo.Path, repo)
	}
	if doc.Repo.User.Name != "Scout Homes" || doc.Repo.User.Email != "scout@homie.sh" {
		t.Errorf("repo.user = %+v", doc.Repo.User)
	}
	if doc.Repo.Profile != "personal" {
		t.Errorf("repo.profile = %q, want %q", doc.Repo.Profile, "personal")
	}
	if !slices.Contains(doc.Repo.Tags, "personal") {
		t.Errorf("repo.tags = %v, want to contain %q", doc.Repo.Tags, "personal")
	}
	// The fixture declares no packages, backends, or warnings; the
	// fields must still be present as []/{} — never null, never absent
	// — so consumers can iterate without an absent-vs-empty check.
	for _, want := range []string{`"packages": []`, `"backend_packages": {}`, `"warnings": []`} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %s:\n%s", want, out)
		}
	}
	if doc.Health == nil {
		t.Errorf("health missing from output:\n%s", out)
	}
}

// TestStatusBadHMRepoErrors: $HM_REPO pointing at a directory with no
// homie.toml is a misconfiguration, not "no repo" — both output modes
// must error rather than report the benign not-found state.
func TestStatusBadHMRepoErrors(t *testing.T) {
	t.Setenv("HM_REPO", t.TempDir())

	for _, args := range [][]string{
		{"status", "--json"},
		{"status"},
	} {
		if out, err := runStatusCmd(t, args); err == nil {
			t.Errorf("%v: expected error for bad HM_REPO\noutput:\n%s", args, out)
		}
	}
}

// TestStatusJSONNoRepo: a missing environment repo is data, not an
// error — the document carries "repo": null and the command exits zero.
func TestStatusJSONNoRepo(t *testing.T) {
	t.Setenv("HM_REPO", "")
	t.Chdir(t.TempDir())

	out, err := runStatusCmd(t, []string{"status", "--json"})
	if err != nil {
		t.Fatalf("status --json without a repo: %v\noutput:\n%s", err, out)
	}
	var doc statusOutput
	if err := json.Unmarshal([]byte(out), &doc); err != nil {
		t.Fatalf("output is not valid JSON: %v\noutput:\n%s", err, out)
	}
	if doc.Repo != nil {
		t.Errorf("repo = %+v, want null", doc.Repo)
	}
}
