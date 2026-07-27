package scaffold

import (
	"os"
	"os/exec"
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
	if err := Run(dir, sampleAnswers, testVersion); err != nil {
		t.Fatalf("Run: %v", err)
	}
	want := map[string]os.FileMode{
		"homie.toml":           0o644,
		"bootstrap.sh":         0o755,
		"README.md":            0o644,
		".gitignore":           0o644,
		"home/.zshrc":          0o644,
		"home/.gitconfig.tmpl": 0o644,
		"scripts/01-shell.sh":  0o755,
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
	if err := Run(dir, sampleAnswers, testVersion); err != nil {
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

// TestBootstrapHandsChildrenTheTerminal guards the fix for #46. Under
// `curl ... | bash` stdin is the pipe bash is reading the script from, so
// every `sudo` in `hm bootstrap` and in the user's setup scripts dies with
// "a terminal is required to read the password" unless bootstrap.sh passes
// the controlling terminal down. Note the redirect can't be a script-wide
// `exec </dev/tty` — bash still needs that stdin to read the rest of itself.
func TestBootstrapHandsChildrenTheTerminal(t *testing.T) {
	dir := t.TempDir()
	if err := Run(dir, sampleAnswers, testVersion); err != nil {
		t.Fatalf("Run: %v", err)
	}
	boot, err := os.ReadFile(filepath.Join(dir, "bootstrap.sh"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"if (: </dev/tty) 2>/dev/null; then", // probe, in a subshell
		"withtty hm bootstrap",
		`exec hm apply <"$tty_in"`,
	} {
		if !strings.Contains(string(boot), want) {
			t.Errorf("bootstrap.sh missing %q:\n%s", want, boot)
		}
	}
	if strings.Contains(string(boot), "\nexec </dev/tty") {
		t.Errorf("bootstrap.sh redirects its own stdin, truncating the piped script:\n%s", boot)
	}
}

// TestBootstrapIsValidBash catches syntax errors in the rendered template —
// the substitutions land inside shell quoting, so a bad edit is only visible
// after rendering.
func TestBootstrapIsValidBash(t *testing.T) {
	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("bash not on PATH")
	}
	dir := t.TempDir()
	if err := Run(dir, sampleAnswers, testVersion); err != nil {
		t.Fatalf("Run: %v", err)
	}
	out, err := exec.Command(bash, "-n", filepath.Join(dir, "bootstrap.sh")).CombinedOutput()
	if err != nil {
		t.Errorf("bash -n bootstrap.sh: %v\n%s", err, out)
	}
}

func TestRunRoundtripsThroughConfigLoad(t *testing.T) {
	dir := t.TempDir()
	if err := Run(dir, sampleAnswers, testVersion); err != nil {
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
	if err := Run(dir, sampleAnswers, testVersion); err == nil {
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
	if err := Run(dir, min, testVersion); err != nil {
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
		if err := Run(t.TempDir(), a, testVersion); err == nil {
			t.Errorf("case %d: expected required-field error, got nil", i)
		}
	}
}
