package config

import (
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/kurowski/homie/internal/detect"
)

func TestLoadHappyPath(t *testing.T) {
	c, err := Load(filepath.Join("testdata", "happy"), "")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if c.User.Name != "Scout Homes" {
		t.Errorf("User.Name = %q", c.User.Name)
	}
	if c.User.Email != "scout@homie.sh" {
		t.Errorf("User.Email = %q", c.User.Email)
	}
	if c.Profile.Name != "personal" {
		t.Errorf("Profile.Name = %q", c.Profile.Name)
	}
	if c.Profile.DefaultShell != "zsh" {
		t.Errorf("Profile.DefaultShell = %q", c.Profile.DefaultShell)
	}
	if c.Vars["EDITOR"] != "nvim" {
		t.Errorf("Vars[EDITOR] = %q", c.Vars["EDITOR"])
	}
	if !reflect.DeepEqual(c.Tags.Extra, []string{"laptop"}) {
		t.Errorf("Tags.Extra = %v", c.Tags.Extra)
	}
	if len(c.Warnings) != 0 {
		t.Errorf("expected no warnings, got %v", c.Warnings)
	}
}

func TestLoadMissingRequired(t *testing.T) {
	cases := []struct {
		dir   string
		field string
	}{
		{"missing-name", "user.name"},
		{"missing-email", "user.email"},
	}
	for _, tc := range cases {
		t.Run(tc.dir, func(t *testing.T) {
			_, err := Load(filepath.Join("testdata", tc.dir), "")
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !strings.Contains(err.Error(), tc.field) {
				t.Errorf("error %q should mention %q", err, tc.field)
			}
		})
	}
}

func TestLoadUnknownFieldWarns(t *testing.T) {
	c, err := Load(filepath.Join("testdata", "unknown-field"), "")
	if err != nil {
		t.Fatalf("Load should succeed despite unknown field: %v", err)
	}
	if len(c.Warnings) == 0 {
		t.Fatal("expected warnings for unknown field")
	}
	joined := strings.Join(c.Warnings, " ")
	if !strings.Contains(joined, "future") {
		t.Errorf("warnings should mention the unknown table, got %q", joined)
	}
}

func TestLoadMissingFile(t *testing.T) {
	_, err := Load(t.TempDir(), "")
	if err == nil {
		t.Fatal("expected error when homie.toml missing")
	}
}

func TestPackagesFor(t *testing.T) {
	c, err := Load(filepath.Join("testdata", "per-distro"), "")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	cases := []struct {
		distro string
		want   []string
	}{
		{"fedora", []string{"git", "zsh", "neovim", "util-linux-user"}},
		{"ubuntu", []string{"git", "zsh", "neovim", "fd-find"}},
		{"debian", []string{"git", "zsh", "neovim"}}, // no debian-specific entry
	}
	for _, tc := range cases {
		t.Run(tc.distro, func(t *testing.T) {
			got := c.PackagesFor(detect.Env{Distro: tc.distro})
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("PackagesFor(%s) = %v, want %v", tc.distro, got, tc.want)
			}
		})
	}
}

func TestAllTags(t *testing.T) {
	c := Config{
		Profile: Profile{Name: "personal"},
		Tags:    Tags{Extra: []string{"laptop", "personal"}}, // duplicate of Profile.Name
	}
	env := detect.Env{Tags: []string{"fedora", "amd64"}}
	got := c.AllTags(env)
	want := []string{"amd64", "fedora", "laptop", "personal"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("AllTags = %v, want %v", got, want)
	}
}

func TestAllTagsEmptyProfile(t *testing.T) {
	c := Config{Tags: Tags{Extra: []string{"laptop"}}}
	env := detect.Env{Tags: []string{"ubuntu"}}
	got := c.AllTags(env)
	want := []string{"laptop", "ubuntu"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("AllTags = %v, want %v", got, want)
	}
}

func TestLoadHostOverlay(t *testing.T) {
	dir := filepath.Join("testdata", "host-overlay")
	t.Run("base only when no hostname", func(t *testing.T) {
		c, err := Load(dir, "")
		if err != nil {
			t.Fatalf("Load: %v", err)
		}
		if c.Profile.Name != "" {
			t.Errorf("Profile.Name = %q, want empty (base has no profile)", c.Profile.Name)
		}
		if !reflect.DeepEqual(c.Tags.Extra, []string{"base-extra"}) {
			t.Errorf("Tags.Extra = %v", c.Tags.Extra)
		}
		if !reflect.DeepEqual(c.Packages["fedora"], []string{"git", "zsh"}) {
			t.Errorf("Packages[fedora] = %v", c.Packages["fedora"])
		}
		if c.Vars["WORK_EMAIL"] != "" {
			t.Errorf("Vars[WORK_EMAIL] should be empty without overlay, got %q", c.Vars["WORK_EMAIL"])
		}
	})

	t.Run("coach overlay sets personal profile and appends packages", func(t *testing.T) {
		c, err := Load(dir, "coach")
		if err != nil {
			t.Fatalf("Load: %v", err)
		}
		if c.Profile.Name != "personal" {
			t.Errorf("Profile.Name = %q, want personal", c.Profile.Name)
		}
		wantExtra := []string{"base-extra", "laptop"}
		if !reflect.DeepEqual(c.Tags.Extra, wantExtra) {
			t.Errorf("Tags.Extra = %v, want %v", c.Tags.Extra, wantExtra)
		}
		wantPkgs := []string{"git", "zsh", "steam"}
		if !reflect.DeepEqual(c.Packages["fedora"], wantPkgs) {
			t.Errorf("Packages[fedora] = %v, want %v", c.Packages["fedora"], wantPkgs)
		}
		if c.Vars["EDITOR"] != "nvim" {
			t.Errorf("base var EDITOR lost: %q", c.Vars["EDITOR"])
		}
	})

	t.Run("work overlay overrides vars and appends work packages", func(t *testing.T) {
		c, err := Load(dir, "uceap-dev01")
		if err != nil {
			t.Fatalf("Load: %v", err)
		}
		if c.Profile.Name != "work" {
			t.Errorf("Profile.Name = %q, want work", c.Profile.Name)
		}
		if c.Vars["WORK_EMAIL"] != "scout@work.example.com" {
			t.Errorf("Vars[WORK_EMAIL] = %q", c.Vars["WORK_EMAIL"])
		}
		if c.Vars["EDITOR"] != "nvim" {
			t.Errorf("base var EDITOR lost: %q", c.Vars["EDITOR"])
		}
		wantPkgs := []string{"git", "zsh", "kubectl", "helm"}
		if !reflect.DeepEqual(c.Packages["fedora"], wantPkgs) {
			t.Errorf("Packages[fedora] = %v, want %v", c.Packages["fedora"], wantPkgs)
		}
	})

	t.Run("unknown hostname falls back to base", func(t *testing.T) {
		c, err := Load(dir, "nope-no-such-host")
		if err != nil {
			t.Fatalf("Load: %v", err)
		}
		if c.Profile.Name != "" {
			t.Errorf("Profile.Name = %q, want empty (no overlay matched)", c.Profile.Name)
		}
	})
}
