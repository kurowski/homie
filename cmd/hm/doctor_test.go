package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kurowski/homie/internal/doctor"
)

func resetDoctorFlags() {
	doctorHome = ""
	doctorJSON = false
}

func runDoctorCmd(t *testing.T, args []string) (string, error) {
	t.Helper()
	resetDoctorFlags()
	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetErr(buf)
	rootCmd.SetArgs(args)
	err := rootCmd.Execute()
	return buf.String(), err
}

func TestDoctorAfterApplyReportsClean(t *testing.T) {
	repo := fixtureRepo(t)
	home := t.TempDir()
	t.Setenv("HM_REPO", repo)

	// Bring home into a fully-synced state first.
	if _, err := runApplyCmd(t, []string{"apply", "--home", home, "--skip-packages", "--skip-scripts"}); err != nil {
		t.Fatalf("apply: %v", err)
	}

	out, err := runDoctorCmd(t, []string{"doctor", "--home", home})
	if err != nil {
		t.Fatalf("doctor: %v\n%s", err, out)
	}
	if !strings.Contains(out, "All checks passed") {
		t.Errorf("expected clean doctor report, got:\n%s", out)
	}
}

func TestDoctorReportsBrokenSymlinkAsError(t *testing.T) {
	repo := fixtureRepo(t)
	home := t.TempDir()
	t.Setenv("HM_REPO", repo)

	if _, err := runApplyCmd(t, []string{"apply", "--home", home, "--skip-packages", "--skip-scripts"}); err != nil {
		t.Fatalf("apply: %v", err)
	}
	// Yank the source file out from under the symlink in $HOME.
	if err := os.Remove(filepath.Join(repo, "home", ".zshrc")); err != nil {
		t.Fatal(err)
	}

	out, err := runDoctorCmd(t, []string{"doctor", "--home", home})
	if err == nil {
		t.Fatalf("expected non-nil error for broken-symlink case, output:\n%s", out)
	}
	if !strings.Contains(out, "broken symlink") {
		t.Errorf("expected broken-symlink in output:\n%s", out)
	}
}

// TestDoctorJSONClean pins the machine-readable contract for a healthy
// host: stdout is one JSON document, findings is [] (never null), and
// the exit is zero.
func TestDoctorJSONClean(t *testing.T) {
	repo := fixtureRepo(t)
	home := t.TempDir()
	t.Setenv("HM_REPO", repo)

	if _, err := runApplyCmd(t, []string{"apply", "--home", home, "--skip-packages", "--skip-scripts"}); err != nil {
		t.Fatalf("apply: %v", err)
	}

	out, err := runDoctorCmd(t, []string{"doctor", "--json", "--home", home})
	if err != nil {
		t.Fatalf("doctor --json: %v\noutput:\n%s", err, out)
	}
	var doc doctorOutput
	if err := json.Unmarshal([]byte(out), &doc); err != nil {
		t.Fatalf("output is not valid JSON: %v\noutput:\n%s", err, out)
	}
	if doc.Errors != 0 {
		t.Errorf("errors = %d, want 0\noutput:\n%s", doc.Errors, out)
	}
	if doc.Findings == nil || !strings.Contains(out, `"findings": []`) {
		t.Errorf("findings should serialize as [], got:\n%s", out)
	}
}

// TestDoctorJSONReportsErrors: error findings appear as structured
// records and the command still exits non-zero — but stdout must remain
// pure JSON, with no styled error printed on top.
func TestDoctorJSONReportsErrors(t *testing.T) {
	repo := fixtureRepo(t)
	home := t.TempDir()
	t.Setenv("HM_REPO", repo)

	if _, err := runApplyCmd(t, []string{"apply", "--home", home, "--skip-packages", "--skip-scripts"}); err != nil {
		t.Fatalf("apply: %v", err)
	}
	if err := os.Remove(filepath.Join(repo, "home", ".zshrc")); err != nil {
		t.Fatal(err)
	}

	out, err := runDoctorCmd(t, []string{"doctor", "--json", "--home", home})
	if err == nil {
		t.Fatalf("expected non-nil error for broken-symlink case, output:\n%s", out)
	}
	var doc doctorOutput
	if err := json.Unmarshal([]byte(out), &doc); err != nil {
		t.Fatalf("output is not valid JSON: %v\noutput:\n%s", err, out)
	}
	if doc.Errors == 0 {
		t.Errorf("errors = 0, want > 0\noutput:\n%s", out)
	}
	found := false
	for _, f := range doc.Findings {
		if f.Severity == doctor.SeverityError && f.Area == "link" &&
			strings.Contains(f.Message, "broken symlink") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected a link/error broken-symlink finding, got:\n%s", out)
	}
}
