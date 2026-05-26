package doctor

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kurowski/homie/internal/config"
	"github.com/kurowski/homie/internal/detect"
)

// fakeMgr is a minimal packages.Manager for doctor tests.
type fakeMgr struct {
	name      string
	available bool
	installed map[string]bool
}

func (f *fakeMgr) Name() string              { return f.name }
func (f *fakeMgr) IsAvailable() bool         { return f.available }
func (f *fakeMgr) IsInstalled(p string) bool { return f.installed[p] }
func (f *fakeMgr) Install(_ []string) error  { return nil }

func makeRepo(t *testing.T) (repo, home string) {
	t.Helper()
	root := t.TempDir()
	repo = filepath.Join(root, "repo")
	home = filepath.Join(root, "home")
	if err := os.MkdirAll(filepath.Join(repo, "dotfiles"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(repo, "templates"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(repo, "scripts"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(home, 0o755); err != nil {
		t.Fatal(err)
	}
	return
}

func sampleCfg() config.Config {
	return config.Config{
		User:    config.User{Name: "Scout Homes", Email: "scout@homie.sh"},
		Profile: config.Profile{Name: "personal", DefaultShell: "zsh"},
		Packages: config.Packages{
			Base: map[string][]string{"all": {"git", "zsh"}},
		},
	}
}

func messagesByArea(r Report, area string) []string {
	var out []string
	for _, f := range r.Findings {
		if f.Area == area {
			out = append(out, f.Message)
		}
	}
	return out
}

func TestRunCleanRepoNoFindings(t *testing.T) {
	repo, home := makeRepo(t)

	// Linked dotfile.
	src := filepath.Join(repo, "dotfiles", ".zshrc")
	if err := os.WriteFile(src, []byte("# zsh"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(src, filepath.Join(home, ".zshrc")); err != nil {
		t.Fatal(err)
	}

	// Rendered template — content matches what render.Render would
	// produce for the empty template.
	tmpl := "static body\n"
	if err := os.WriteFile(filepath.Join(repo, "templates", "x.tmpl"), []byte(tmpl), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, "x"), []byte(tmpl), 0o644); err != nil {
		t.Fatal(err)
	}

	// Executable script.
	if err := os.WriteFile(filepath.Join(repo, "scripts", "01.sh"), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	mgr := &fakeMgr{
		name:      "apt",
		available: true,
		installed: map[string]bool{"git": true, "zsh": true},
	}
	env := detect.Env{Distro: "ubuntu", PackageManager: "apt", Arch: "amd64", Hostname: "test"}

	r := Run(repo, home, sampleCfg(), env, mgr)
	if len(r.Findings) != 0 {
		t.Fatalf("expected clean report, got: %+v", r.Findings)
	}
	if r.HasErrors() {
		t.Errorf("expected no errors")
	}
}

func TestRunReportsBrokenLink(t *testing.T) {
	repo, home := makeRepo(t)
	src := filepath.Join(repo, "dotfiles", ".zshrc")
	if err := os.WriteFile(src, []byte("ok"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(src, filepath.Join(home, ".zshrc")); err != nil {
		t.Fatal(err)
	}
	// Now remove the source — link is broken.
	if err := os.Remove(src); err != nil {
		t.Fatal(err)
	}

	env := detect.Env{Distro: "ubuntu", PackageManager: "apt", Hostname: "test"}
	r := Run(repo, home, sampleCfg(), env,
		&fakeMgr{name: "apt", available: true, installed: map[string]bool{"git": true, "zsh": true}})
	if !r.HasErrors() {
		t.Fatalf("expected error for broken symlink, got %+v", r.Findings)
	}
	msgs := strings.Join(messagesByArea(r, "link"), "\n")
	if !strings.Contains(msgs, "broken symlink") {
		t.Errorf("expected broken-symlink message in: %s", msgs)
	}
}

func TestRunReportsMissingPackages(t *testing.T) {
	repo, home := makeRepo(t)
	env := detect.Env{Distro: "ubuntu", PackageManager: "apt", Hostname: "test"}
	mgr := &fakeMgr{
		name:      "apt",
		available: true,
		installed: map[string]bool{"git": true}, // zsh missing
	}
	r := Run(repo, home, sampleCfg(), env, mgr)
	msgs := strings.Join(messagesByArea(r, "packages"), "\n")
	if !strings.Contains(msgs, "zsh") {
		t.Errorf("expected zsh in missing packages, got: %s", msgs)
	}
}

func TestRunReportsUnrenderedTemplate(t *testing.T) {
	repo, home := makeRepo(t)
	if err := os.WriteFile(filepath.Join(repo, "templates", "x.tmpl"), []byte("body"), 0o644); err != nil {
		t.Fatal(err)
	}
	env := detect.Env{Distro: "ubuntu", PackageManager: "apt", Hostname: "test"}
	r := Run(repo, home, sampleCfg(), env,
		&fakeMgr{name: "apt", available: true, installed: map[string]bool{"git": true, "zsh": true}})
	msgs := strings.Join(messagesByArea(r, "render"), "\n")
	if !strings.Contains(msgs, "not yet rendered") {
		t.Errorf("expected not-yet-rendered finding: %s", msgs)
	}
}

func TestRunReportsStaleTemplate(t *testing.T) {
	repo, home := makeRepo(t)
	if err := os.WriteFile(filepath.Join(repo, "templates", "x.tmpl"), []byte("new"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, "x"), []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	env := detect.Env{Distro: "ubuntu", PackageManager: "apt", Hostname: "test"}
	r := Run(repo, home, sampleCfg(), env,
		&fakeMgr{name: "apt", available: true, installed: map[string]bool{"git": true, "zsh": true}})
	msgs := strings.Join(messagesByArea(r, "render"), "\n")
	if !strings.Contains(msgs, "stale") {
		t.Errorf("expected stale finding: %s", msgs)
	}
}

func TestRunReportsNonExecutableScript(t *testing.T) {
	repo, home := makeRepo(t)
	if err := os.WriteFile(filepath.Join(repo, "scripts", "01.sh"), []byte("#!/bin/sh\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	env := detect.Env{Distro: "ubuntu", PackageManager: "apt", Hostname: "test"}
	r := Run(repo, home, sampleCfg(), env,
		&fakeMgr{name: "apt", available: true, installed: map[string]bool{"git": true, "zsh": true}})
	msgs := strings.Join(messagesByArea(r, "scripts"), "\n")
	if !strings.Contains(msgs, "not executable") {
		t.Errorf("expected non-executable finding: %s", msgs)
	}
}

func TestRunReportsUnknownDistro(t *testing.T) {
	repo, home := makeRepo(t)
	env := detect.Env{Distro: "unknown", PackageManager: "unknown", Hostname: "test"}
	r := Run(repo, home, sampleCfg(), env, &fakeMgr{name: "noop"})
	msgs := strings.Join(messagesByArea(r, "env"), "\n")
	if !strings.Contains(msgs, "not recognized") {
		t.Errorf("expected unknown-distro warning: %s", msgs)
	}
}

func TestRunReportsMissingHostname(t *testing.T) {
	repo, home := makeRepo(t)
	env := detect.Env{Distro: "ubuntu", PackageManager: "apt"} // Hostname unset
	r := Run(repo, home, sampleCfg(), env,
		&fakeMgr{name: "apt", available: true, installed: map[string]bool{"git": true, "zsh": true}})
	msgs := strings.Join(messagesByArea(r, "env"), "\n")
	if !strings.Contains(msgs, "hostname unavailable") {
		t.Errorf("expected hostname-unavailable warning: %s", msgs)
	}
}
