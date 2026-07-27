package scaffold

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestCloneTarget — only a $HOME-relative location generalizes to the
// machines bootstrap.sh actually runs on. Everything else reports false
// so the caller keeps the $HOME/<repo> default; baking in an absolute
// path from the authoring machine (a /tmp build dir, a CI checkout) is
// how e2e caught this the first time.
func TestCloneTarget(t *testing.T) {
	cases := []struct {
		name, dir, home, want string
		ok                    bool
	}{
		{name: "directly under home", dir: "/home/scout/dotfiles", home: "/home/scout", want: "$HOME/dotfiles", ok: true},
		{name: "nested under home", dir: "/home/scout/Projects/dotfiles", home: "/home/scout", want: "$HOME/Projects/dotfiles", ok: true},
		{name: "outside home", dir: "/opt/dotfiles", home: "/home/scout"},
		{name: "build dir", dir: "/tmp/build123/userrepo-src", home: "/home/scout"},
		{name: "sibling of home", dir: "/home/other/dotfiles", home: "/home/scout"},
		{name: "home itself", dir: "/home/scout", home: "/home/scout"},
		{name: "unknown home", dir: "/home/scout/dotfiles", home: ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, ok := CloneTarget(c.dir, c.home)
			if got != c.want || ok != c.ok {
				t.Errorf("CloneTarget(%q, %q) = %q,%v; want %q,%v", c.dir, c.home, got, ok, c.want, c.ok)
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
