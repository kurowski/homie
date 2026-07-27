package scaffold

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCloneTarget(t *testing.T) {
	cases := []struct {
		name, dir, home, want string
	}{
		{"directly under home", "/home/scout/dotfiles", "/home/scout", "$HOME/dotfiles"},
		{"nested under home", "/home/scout/Projects/dotfiles", "/home/scout", "$HOME/Projects/dotfiles"},
		{"outside home", "/opt/dotfiles", "/home/scout", "/opt/dotfiles"},
		{"sibling of home", "/home/other/dotfiles", "/home/scout", "/home/other/dotfiles"},
		{"home itself", "/home/scout", "/home/scout", "/home/scout"},
		{"unknown home", "/home/scout/dotfiles", "", "/home/scout/dotfiles"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := CloneTarget(c.dir, c.home); got != c.want {
				t.Errorf("CloneTarget(%q, %q) = %q, want %q", c.dir, c.home, got, c.want)
			}
		})
	}
}

// TestRepoDirRendersIntoBootstrap covers the case that motivated the
// field: a repo kept somewhere other than $HOME/<repo>. Before, that
// meant hand-editing the generated file — which under the provenance
// rule would cost the user every future `--update`.
func TestRepoDirRendersIntoBootstrap(t *testing.T) {
	dir := t.TempDir()
	a := sampleAnswers
	a.RepoDir = "$HOME/Projects/dotfiles"
	if err := Run(dir, a, testVersion); err != nil {
		t.Fatalf("Run: %v", err)
	}
	body, err := os.ReadFile(filepath.Join(dir, bootstrapPath))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), `REPO_DIR="${HM_REPO:-$HOME/Projects/dotfiles}"`) {
		t.Errorf("bootstrap.sh didn't take RepoDir:\n%s", body)
	}
	// A nested destination is exactly when git clone needs the parent.
	if !strings.Contains(string(body), `mkdir -p "$(dirname "$REPO_DIR")"`) {
		t.Errorf("bootstrap.sh should create the clone parent:\n%s", body)
	}
}

func TestRepoDirDefaultsToHomeSlashRepo(t *testing.T) {
	dir := t.TempDir()
	a := sampleAnswers
	a.RepoDir = ""
	if err := Run(dir, a, testVersion); err != nil {
		t.Fatalf("Run: %v", err)
	}
	body, _ := os.ReadFile(filepath.Join(dir, bootstrapPath))
	if !strings.Contains(string(body), `REPO_DIR="${HM_REPO:-$HOME/dotfiles}"`) {
		t.Errorf("expected the $HOME/<repo> default:\n%s", body)
	}
}
