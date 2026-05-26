package scaffold

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kurowski/homie/internal/config"
)

var sampleAnswers = Answers{
	Name:         "Scout Homes",
	Email:        "scout@homie.sh",
	GitHubUser:   "scouthomes",
	GitHubRepo:   "dotfiles",
	Profile:      "personal",
	DefaultShell: "zsh",
}

func TestRunWritesAllFiles(t *testing.T) {
	dir := t.TempDir()
	if err := Run(dir, sampleAnswers); err != nil {
		t.Fatalf("Run: %v", err)
	}
	want := map[string]os.FileMode{
		"homie.toml":                0o644,
		"bootstrap.sh":              0o755,
		"README.md":                 0o644,
		".gitignore":                0o644,
		"dotfiles/.zshrc":           0o644,
		"templates/.gitconfig.tmpl": 0o644,
		"scripts/01-shell.sh":       0o755,
	}
	for rel, mode := range want {
		info, err := os.Stat(filepath.Join(dir, rel))
		if err != nil {
			t.Errorf("%s missing: %v", rel, err)
			continue
		}
		if info.Mode().Perm() != mode {
			t.Errorf("%s mode = %v, want %v", rel, info.Mode().Perm(), mode)
		}
	}
}

func TestRunSubstitutesAnswers(t *testing.T) {
	dir := t.TempDir()
	if err := Run(dir, sampleAnswers); err != nil {
		t.Fatalf("Run: %v", err)
	}

	toml, err := os.ReadFile(filepath.Join(dir, "homie.toml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(toml), `name  = "Scout Homes"`) {
		t.Errorf("homie.toml missing Name substitution: %s", toml)
	}
	if !strings.Contains(string(toml), `email = "scout@homie.sh"`) {
		t.Errorf("homie.toml missing Email substitution: %s", toml)
	}

	boot, err := os.ReadFile(filepath.Join(dir, "bootstrap.sh"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(boot), "https://github.com/scouthomes/dotfiles") {
		t.Errorf("bootstrap.sh missing GitHub URL: %s", boot)
	}
}

func TestRunRoundtripsThroughConfigLoad(t *testing.T) {
	dir := t.TempDir()
	if err := Run(dir, sampleAnswers); err != nil {
		t.Fatalf("Run: %v", err)
	}
	cfg, err := config.Load(dir, "")
	if err != nil {
		t.Fatalf("config.Load on scaffolded repo: %v", err)
	}
	if cfg.User.Name != "Scout Homes" || cfg.User.Email != "scout@homie.sh" {
		t.Errorf("Loaded config mismatched: %+v", cfg.User)
	}
	if cfg.Profile.Name != "personal" || cfg.Profile.DefaultShell != "zsh" {
		t.Errorf("Loaded profile mismatched: %+v", cfg.Profile)
	}
}

func TestRunRefusesToOverwrite(t *testing.T) {
	dir := t.TempDir()
	// Pre-create homie.toml so scaffold sees a collision.
	if err := os.WriteFile(filepath.Join(dir, "homie.toml"), []byte("pre-existing"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := Run(dir, sampleAnswers); err == nil {
		t.Errorf("expected Run to refuse overwrite, got nil")
	}
	body, _ := os.ReadFile(filepath.Join(dir, "homie.toml"))
	if string(body) != "pre-existing" {
		t.Errorf("user's existing homie.toml was modified: %s", body)
	}
}

func TestRunFillsDefaults(t *testing.T) {
	dir := t.TempDir()
	min := Answers{Name: "Scout Homes", Email: "scout@homie.sh", GitHubUser: "scouthomes"}
	if err := Run(dir, min); err != nil {
		t.Fatalf("Run: %v", err)
	}
	toml, _ := os.ReadFile(filepath.Join(dir, "homie.toml"))
	if !strings.Contains(string(toml), `name          = "personal"`) {
		t.Errorf("expected default profile=personal: %s", toml)
	}
	if !strings.Contains(string(toml), `default_shell = "zsh"`) {
		t.Errorf("expected default shell=zsh: %s", toml)
	}
	boot, _ := os.ReadFile(filepath.Join(dir, "bootstrap.sh"))
	if !strings.Contains(string(boot), "scouthomes/dotfiles") {
		t.Errorf("expected default repo name=dotfiles: %s", boot)
	}
}

func TestRunRequiresIdentity(t *testing.T) {
	cases := []Answers{
		{Email: "scout@homie.sh", GitHubUser: "scouthomes"},
		{Name: "Scout", GitHubUser: "scouthomes"},
		{Name: "Scout", Email: "scout@homie.sh"},
	}
	for i, a := range cases {
		if err := Run(t.TempDir(), a); err == nil {
			t.Errorf("case %d: expected required-field error, got nil", i)
		}
	}
}
