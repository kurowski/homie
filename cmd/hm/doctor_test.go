package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func resetDoctorFlags() {
	doctorHome = ""
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
	// Yank the source dotfile out from under the symlink in $HOME.
	if err := os.Remove(filepath.Join(repo, "dotfiles", ".zshrc")); err != nil {
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
