package doctor

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kurowski/homie/internal/config"
	"github.com/kurowski/homie/internal/detect"
	"github.com/kurowski/homie/internal/packages"
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

// fakeValidatingMgr is a fakeMgr that also implements packages.Validator,
// so doctor's malformed-spec path can be exercised without snap internals.
type fakeValidatingMgr struct {
	fakeMgr
	validateErr error
}

func (f *fakeValidatingMgr) Validate(_ []string) error { return f.validateErr }

func makeRepo(t *testing.T) (repo, home string) {
	t.Helper()
	root := t.TempDir()
	repo = filepath.Join(root, "repo")
	home = filepath.Join(root, "home")
	if err := os.MkdirAll(filepath.Join(repo, "home"), 0o755); err != nil {
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
	src := filepath.Join(repo, "home", ".zshrc")
	if err := os.WriteFile(src, []byte("# zsh"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(src, filepath.Join(home, ".zshrc")); err != nil {
		t.Fatal(err)
	}

	// Rendered template — content matches what render.Render would
	// produce for the empty template.
	tmpl := "static body\n"
	if err := os.WriteFile(filepath.Join(repo, "home", "x.tmpl"), []byte(tmpl), 0o644); err != nil {
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

	r := Run(repo, home, sampleCfg(), env, mgr, nil)
	if len(r.Findings) != 0 {
		t.Fatalf("expected clean report, got: %+v", r.Findings)
	}
	if r.HasErrors() {
		t.Errorf("expected no errors")
	}
}

func TestRunReportsBrokenLink(t *testing.T) {
	repo, home := makeRepo(t)
	src := filepath.Join(repo, "home", ".zshrc")
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
		&fakeMgr{name: "apt", available: true, installed: map[string]bool{"git": true, "zsh": true}}, nil)
	if !r.HasErrors() {
		t.Fatalf("expected error for broken symlink, got %+v", r.Findings)
	}
	msgs := strings.Join(messagesByArea(r, "link"), "\n")
	if !strings.Contains(msgs, "broken symlink") {
		t.Errorf("expected broken-symlink message in: %s", msgs)
	}
}

func TestRunReportsBrokenLinkIntoTaggedTree(t *testing.T) {
	repo, home := makeRepo(t)
	// Source under a tag-gated tree; verifies findBrokenLinks's
	// taggedPrefix matches `home.tag-work/...`, not just `home/...`.
	srcDir := filepath.Join(repo, "home.tag-work")
	if err := os.MkdirAll(srcDir, 0o755); err != nil {
		t.Fatal(err)
	}
	src := filepath.Join(srcDir, "work-only")
	if err := os.WriteFile(src, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(src, filepath.Join(home, "work-only")); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(src); err != nil {
		t.Fatal(err)
	}

	env := detect.Env{Distro: "ubuntu", PackageManager: "apt", Hostname: "test"}
	r := Run(repo, home, sampleCfg(), env,
		&fakeMgr{name: "apt", available: true, installed: map[string]bool{"git": true, "zsh": true}}, nil)
	msgs := strings.Join(messagesByArea(r, "link"), "\n")
	if !strings.Contains(msgs, "broken symlink") || !strings.Contains(msgs, "work-only") {
		t.Errorf("expected broken-symlink message for tag-gated source, got:\n%s", msgs)
	}
}

func TestRunReportsBackendPackageFindings(t *testing.T) {
	repo, home := makeRepo(t)
	cfg := config.Config{
		User:    config.User{Name: "Scout Homes", Email: "scout@homie.sh"},
		Profile: config.Profile{Name: "personal", DefaultShell: "zsh"},
		Packages: config.Packages{
			Base: map[string][]string{"all": {"git", "zsh"}},
			Backends: map[string]config.BackendPackages{
				"flatpak": {Base: map[string][]string{"all": {"md.obsidian.Obsidian", "us.zoom.Zoom"}}},
				"brew":    {Base: map[string][]string{"all": {"fd"}}},
				"cargo":   {Base: map[string][]string{"all": {"cargo-edit"}}},
			},
		},
	}
	env := detect.Env{Distro: "ubuntu", PackageManager: "apt", Hostname: "test"}
	native := &fakeMgr{name: "apt", available: true, installed: map[string]bool{"git": true, "zsh": true}}

	// flatpak: tool available, one of two installed -> warn missing.
	// brew:    tool not available -> warn (skipping).
	// cargo:   unknown backend -> warn (no manager).
	lookup := func(name string) packages.Manager {
		switch name {
		case "flatpak":
			return &fakeMgr{name: "flatpak", available: true, installed: map[string]bool{"md.obsidian.Obsidian": true}}
		case "brew":
			return &fakeMgr{name: "brew", available: false}
		}
		return nil
	}
	r := Run(repo, home, cfg, env, native, lookup)
	msgs := strings.Join(messagesByArea(r, "packages"), "\n")
	for _, want := range []string{
		"flatpak: 1 not installed: us.zoom.Zoom",
		"brew not on PATH",
		`backend "cargo" is declared`,
	} {
		if !strings.Contains(msgs, want) {
			t.Errorf("missing finding %q in:\n%s", want, msgs)
		}
	}
	if r.HasErrors() {
		t.Errorf("backend warnings should not register as errors")
	}
}

func TestRunReportsBackendValidationError(t *testing.T) {
	repo, home := makeRepo(t)
	cfg := sampleCfg()
	cfg.Packages.Backends = map[string]config.BackendPackages{
		"snap": {Base: map[string][]string{"all": {"foo/bogus"}}},
	}
	env := detect.Env{Distro: "ubuntu", PackageManager: "apt", Hostname: "test"}
	native := &fakeMgr{name: "apt", available: true, installed: map[string]bool{"git": true, "zsh": true}}
	lookup := func(name string) packages.Manager {
		return &fakeValidatingMgr{
			fakeMgr:     fakeMgr{name: name, available: true},
			validateErr: errors.New(`unknown confinement mode "bogus"`),
		}
	}
	r := Run(repo, home, cfg, env, native, lookup)
	if !r.HasErrors() {
		t.Fatal("a malformed backend spec should be an error finding, not 'not installed'")
	}
	msgs := strings.Join(messagesByArea(r, "packages"), "\n")
	if !strings.Contains(msgs, "bogus") {
		t.Errorf("validation error should be surfaced in packages findings:\n%s", msgs)
	}
}

func TestRunReportsUnlinkedDotfile(t *testing.T) {
	repo, home := makeRepo(t)
	// Source exists in home/ but hasn't been symlinked into $HOME yet
	// — link.Plan classifies as KindCreate, doctor reports as a warning.
	if err := os.WriteFile(filepath.Join(repo, "home", ".zshrc"), []byte("# zsh"), 0o644); err != nil {
		t.Fatal(err)
	}
	env := detect.Env{Distro: "ubuntu", PackageManager: "apt", Hostname: "test"}
	r := Run(repo, home, sampleCfg(), env,
		&fakeMgr{name: "apt", available: true, installed: map[string]bool{"git": true, "zsh": true}}, nil)
	msgs := strings.Join(messagesByArea(r, "link"), "\n")
	if !strings.Contains(msgs, "not yet linked") {
		t.Errorf("expected not-yet-linked finding: %s", msgs)
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
	r := Run(repo, home, sampleCfg(), env, mgr, nil)
	msgs := strings.Join(messagesByArea(r, "packages"), "\n")
	if !strings.Contains(msgs, "zsh") {
		t.Errorf("expected zsh in missing packages, got: %s", msgs)
	}
}

func TestRunReportsUnrenderedTemplate(t *testing.T) {
	repo, home := makeRepo(t)
	if err := os.WriteFile(filepath.Join(repo, "home", "x.tmpl"), []byte("body"), 0o644); err != nil {
		t.Fatal(err)
	}
	env := detect.Env{Distro: "ubuntu", PackageManager: "apt", Hostname: "test"}
	r := Run(repo, home, sampleCfg(), env,
		&fakeMgr{name: "apt", available: true, installed: map[string]bool{"git": true, "zsh": true}}, nil)
	msgs := strings.Join(messagesByArea(r, "render"), "\n")
	if !strings.Contains(msgs, "not yet rendered") {
		t.Errorf("expected not-yet-rendered finding: %s", msgs)
	}
}

func TestRunReportsStaleTemplate(t *testing.T) {
	repo, home := makeRepo(t)
	if err := os.WriteFile(filepath.Join(repo, "home", "x.tmpl"), []byte("new"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, "x"), []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	env := detect.Env{Distro: "ubuntu", PackageManager: "apt", Hostname: "test"}
	r := Run(repo, home, sampleCfg(), env,
		&fakeMgr{name: "apt", available: true, installed: map[string]bool{"git": true, "zsh": true}}, nil)
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
		&fakeMgr{name: "apt", available: true, installed: map[string]bool{"git": true, "zsh": true}}, nil)
	msgs := strings.Join(messagesByArea(r, "scripts"), "\n")
	if !strings.Contains(msgs, "not executable") {
		t.Errorf("expected non-executable finding: %s", msgs)
	}
}

func TestRunReportsInactiveScriptTree(t *testing.T) {
	repo, home := makeRepo(t)
	// scripts.tag-work is inactive on a personal host (no work tag), so
	// doctor should note it informationally under the "scripts" area,
	// mirroring the home-tree behavior.
	if err := os.MkdirAll(filepath.Join(repo, "scripts.tag-work"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "scripts.tag-work", "01.sh"), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	env := detect.Env{Distro: "ubuntu", PackageManager: "apt", Hostname: "test"}
	r := Run(repo, home, sampleCfg(), env,
		&fakeMgr{name: "apt", available: true, installed: map[string]bool{"git": true, "zsh": true}}, nil)

	var found bool
	for _, f := range r.Findings {
		if f.Severity == SeverityInfo && f.Area == "scripts" && strings.Contains(f.Message, "scripts.tag-work") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected an info finding naming scripts.tag-work, got: %v", messagesByArea(r, "scripts"))
	}
	if r.HasErrors() {
		t.Errorf("an inactive script tree is informational, not an error")
	}
}

func TestRunReportsScriptCollision(t *testing.T) {
	repo, home := makeRepo(t)
	if err := os.WriteFile(filepath.Join(repo, "scripts", "05.sh"), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(repo, "scripts.tag-fedora"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "scripts.tag-fedora", "05.sh"), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	// fedora tag active → both trees claim 05.sh → hard error.
	env := detect.Env{Distro: "fedora", PackageManager: "dnf", Hostname: "test", Tags: []string{"fedora"}}
	r := Run(repo, home, sampleCfg(), env,
		&fakeMgr{name: "dnf", available: true, installed: map[string]bool{"git": true, "zsh": true}}, nil)

	if !r.HasErrors() {
		t.Fatal("expected an error finding for the script collision")
	}
	msgs := strings.Join(messagesByArea(r, "scripts"), "\n")
	if !strings.Contains(msgs, "05.sh") {
		t.Errorf("collision error should name 05.sh, got: %s", msgs)
	}
}

func TestRunReportsActiveAndTagBlock(t *testing.T) {
	repo, home := makeRepo(t)
	cfg := sampleCfg() // profile "personal"
	cfg.Packages.ByTag = map[string]map[string][]string{
		"personal.ubuntu": {"all": {"aws-cli"}}, // multi-tag AND block
		"work":            {"all": {"kubectl"}}, // single-tag, not reported
	}
	// personal (profile) + ubuntu (tag) both active → the AND block applies.
	env := detect.Env{Distro: "ubuntu", PackageManager: "apt", Hostname: "test", Tags: []string{"ubuntu"}}
	r := Run(repo, home, cfg, env,
		&fakeMgr{name: "apt", available: true, installed: map[string]bool{"git": true, "zsh": true, "aws-cli": true}}, nil)

	var infos []string
	for _, f := range r.Findings {
		if f.Severity == SeverityInfo && f.Area == "packages" {
			infos = append(infos, f.Message)
		}
	}
	joined := strings.Join(infos, "\n")
	if !strings.Contains(joined, "tag:personal.tag:ubuntu") {
		t.Errorf("expected an info finding for the active AND block, got:\n%s", joined)
	}
	if strings.Contains(joined, `"work"`) {
		t.Errorf("single-tag blocks should not be surfaced as AND blocks, got:\n%s", joined)
	}
	if r.HasErrors() {
		t.Errorf("an active AND block is informational, not an error")
	}
}

func TestRunReportsUnknownDistro(t *testing.T) {
	repo, home := makeRepo(t)
	env := detect.Env{Distro: "unknown", PackageManager: "unknown", Hostname: "test"}
	r := Run(repo, home, sampleCfg(), env, &fakeMgr{name: "noop"}, nil)
	msgs := strings.Join(messagesByArea(r, "env"), "\n")
	if !strings.Contains(msgs, "not recognized") {
		t.Errorf("expected unknown-distro warning: %s", msgs)
	}
}

func TestRunReportsInactiveTaggedTreeDirs(t *testing.T) {
	repo, home := makeRepo(t)
	// home.tag-work exists with both a symlink-shaped file and a
	// template-shaped file — the directory is inactive on this host
	// (no work tag), so we expect exactly one info finding for the dir,
	// emitted under the "home" area rather than separate link/render
	// areas (templates and dotfiles now share the same tree).
	mk := func(rel string) {
		path := filepath.Join(repo, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	mk("home.tag-work/.config/work-only")
	mk("home.tag-work/.work.tmpl")

	env := detect.Env{Distro: "ubuntu", PackageManager: "apt", Hostname: "test"}
	r := Run(repo, home, sampleCfg(), env,
		&fakeMgr{name: "apt", available: true, installed: map[string]bool{"git": true, "zsh": true}}, nil)

	var infos []string
	for _, f := range r.Findings {
		if f.Severity != SeverityInfo {
			continue
		}
		if f.Area != "home" {
			t.Errorf("inactive tree info finding should be in area=home, got area=%q msg=%q", f.Area, f.Message)
		}
		infos = append(infos, f.Message)
	}
	if len(infos) != 1 {
		t.Fatalf("expected exactly 1 info finding for home.tag-work, got %d: %v", len(infos), infos)
	}
	if !strings.Contains(infos[0], "home.tag-work") {
		t.Errorf("expected finding to name home.tag-work, got: %s", infos[0])
	}
	// Info findings must not flip HasErrors or count as warnings.
	if r.HasErrors() {
		t.Errorf("info findings should not register as errors")
	}
	if _, warns := r.Counts(); warns > 0 {
		t.Errorf("info findings should not count as warnings, got %d warns", warns)
	}
}

func TestRunReportsMissingHostname(t *testing.T) {
	repo, home := makeRepo(t)
	env := detect.Env{Distro: "ubuntu", PackageManager: "apt"} // Hostname unset
	r := Run(repo, home, sampleCfg(), env,
		&fakeMgr{name: "apt", available: true, installed: map[string]bool{"git": true, "zsh": true}}, nil)
	msgs := strings.Join(messagesByArea(r, "env"), "\n")
	if !strings.Contains(msgs, "hostname unavailable") {
		t.Errorf("expected hostname-unavailable warning: %s", msgs)
	}
}
