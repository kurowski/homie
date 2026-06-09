package main

import (
	"bytes"
	"encoding/json"
	"slices"
	"testing"
)

func runContextCmd(t *testing.T) (string, error) {
	t.Helper()
	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetErr(buf)
	rootCmd.SetArgs([]string{"context"})
	err := rootCmd.Execute()
	return buf.String(), err
}

// TestContextJSON pins the command's core contract: the JSON keys are
// the template field names verbatim ("Name" means {{ .Name }} works),
// with the values a render would use on this host.
func TestContextJSON(t *testing.T) {
	repo := fixtureRepo(t)
	t.Setenv("HM_REPO", repo)

	out, err := runContextCmd(t)
	if err != nil {
		t.Fatalf("context: %v\noutput:\n%s", err, out)
	}

	var doc map[string]any
	if err := json.Unmarshal([]byte(out), &doc); err != nil {
		t.Fatalf("output is not valid JSON: %v\noutput:\n%s", err, out)
	}
	if doc["Name"] != "Scout Homes" || doc["Email"] != "scout@homie.sh" {
		t.Errorf("Name/Email = %v/%v", doc["Name"], doc["Email"])
	}
	vars, ok := doc["Vars"].(map[string]any)
	if !ok || vars["EDITOR"] != "nvim" {
		t.Errorf("Vars = %v, want EDITOR=nvim", doc["Vars"])
	}
	// Every field of render.Data must appear as a key, so a template
	// author can discover the whole data model from one read.
	for _, field := range []string{
		"Name", "Email", "Profile", "DefaultShell",
		"Distro", "Arch", "IsContainer", "IsRoot", "Tags", "Vars",
	} {
		if _, ok := doc[field]; !ok {
			t.Errorf("missing template field %q in context output:\n%s", field, out)
		}
	}
	tags, _ := doc["Tags"].([]any)
	if !slices.Contains(tags, any("personal")) {
		t.Errorf("Tags = %v, want to contain %q", doc["Tags"], "personal")
	}
}

// TestContextNoRepoErrors: without an environment repo there is no
// config to build a context from, so the command must fail.
func TestContextNoRepoErrors(t *testing.T) {
	t.Setenv("HM_REPO", "")
	t.Chdir(t.TempDir())

	if out, err := runContextCmd(t); err == nil {
		t.Errorf("expected error without a repo\noutput:\n%s", out)
	}
}
