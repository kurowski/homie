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

func TestPackagesForWithTagKeyed(t *testing.T) {
	c, err := Load(filepath.Join("testdata", "tag-packages"), "")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	cases := []struct {
		name string
		env  detect.Env
		want []string
	}{
		{
			name: "no matching tags, base only",
			env:  detect.Env{Distro: "fedora"}, // no env.Tags, no profile
			want: []string{"git", "zsh", "util-linux-user"},
		},
		{
			name: "work tag picks up tag:work for the distro",
			env:  detect.Env{Distro: "fedora", Tags: []string{"work"}},
			want: []string{"git", "zsh", "util-linux-user", "kubectl", "helm"},
		},
		{
			name: "work tag on ubuntu picks up the ubuntu-specific tag list",
			env:  detect.Env{Distro: "ubuntu", Tags: []string{"work"}},
			want: []string{"git", "zsh", "fd-find", "kubectl", "terraform"},
		},
		{
			name: "multiple matching tags including auto-detected distro tag",
			env:  detect.Env{Distro: "fedora", Tags: []string{"fedora", "work", "personal"}},
			// AllTags sorts: ["fedora", "personal", "work"]. tag:fedora
			// overlaps base "git" so dedup keeps it once, contributing only
			// "neovim".
			want: []string{"git", "zsh", "util-linux-user", "neovim", "steam", "kubectl", "helm"},
		},
		{
			name: "unknown tag contributes zero",
			env:  detect.Env{Distro: "fedora", Tags: []string{"no-such-tag"}},
			want: []string{"git", "zsh", "util-linux-user"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := c.PackagesFor(tc.env)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("PackagesFor =\n  got: %v\n want: %v", got, tc.want)
			}
		})
	}
}

func TestPackagesWarnsOnTypos(t *testing.T) {
	c, err := Load(filepath.Join("testdata", "packages-typos"), "")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	joined := strings.Join(c.Warnings, "\n")
	for _, want := range []string{
		"packages.feodra",  // base distro typo
		`empty tag name`,   // [packages."tag:"]
		`tag:work"].ubunntu`, // sub-table distro typo
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("warnings missing %q\n  got:\n%s", want, joined)
		}
	}
	// Filename context is preserved on each warning so users can trace
	// the source when they see a doctor finding.
	for _, w := range c.Warnings {
		if !strings.Contains(w, "homie.toml") {
			t.Errorf("warning missing filename: %q", w)
		}
	}
}

func TestPackagesForOrderIsDeterministic(t *testing.T) {
	c, err := Load(filepath.Join("testdata", "tag-packages"), "")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	env := detect.Env{Distro: "fedora", Tags: []string{"fedora", "work", "personal"}}
	first := c.PackagesFor(env)
	for i := 0; i < 5; i++ {
		got := c.PackagesFor(env)
		if !reflect.DeepEqual(got, first) {
			t.Fatalf("iteration %d differed:\n got: %v\nwant: %v", i, got, first)
		}
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
		if !reflect.DeepEqual(c.Packages.Base["fedora"], []string{"git", "zsh"}) {
			t.Errorf("Packages[fedora] = %v", c.Packages.Base["fedora"])
		}
		if c.Vars["WORK_EMAIL"] != "" {
			t.Errorf("Vars[WORK_EMAIL] should be empty without overlay, got %q", c.Vars["WORK_EMAIL"])
		}
	})

	t.Run("coach overlay sets personal profile, appends packages, dedupes overlap", func(t *testing.T) {
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
		// coach.toml lists ["zsh", "steam"]; base has ["git", "zsh"].
		// Merged result must dedupe `zsh` and preserve insertion order.
		wantPkgs := []string{"git", "zsh", "steam"}
		if !reflect.DeepEqual(c.Packages.Base["fedora"], wantPkgs) {
			t.Errorf("Packages[fedora] = %v, want %v", c.Packages.Base["fedora"], wantPkgs)
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
		if !reflect.DeepEqual(c.Packages.Base["fedora"], wantPkgs) {
			t.Errorf("Packages[fedora] = %v, want %v", c.Packages.Base["fedora"], wantPkgs)
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

func TestMergeTagKeyedPackages(t *testing.T) {
	base := Config{
		Packages: Packages{
			Base: map[string][]string{"fedora": {"git"}},
			ByTag: map[string]map[string][]string{
				"work": {"fedora": {"kubectl"}},
			},
		},
	}
	overlay := Config{
		Packages: Packages{
			Base: map[string][]string{"fedora": {"zsh"}},
			ByTag: map[string]map[string][]string{
				"work":     {"fedora": {"kubectl", "helm"}}, // overlap on kubectl
				"personal": {"fedora": {"steam"}},
			},
		},
	}
	got := merge(base, overlay)

	if want := []string{"git", "zsh"}; !reflect.DeepEqual(got.Packages.Base["fedora"], want) {
		t.Errorf("Base[fedora] = %v, want %v", got.Packages.Base["fedora"], want)
	}
	if want := []string{"kubectl", "helm"}; !reflect.DeepEqual(got.Packages.ByTag["work"]["fedora"], want) {
		t.Errorf("ByTag[work][fedora] = %v, want %v (kubectl deduped)", got.Packages.ByTag["work"]["fedora"], want)
	}
	if want := []string{"steam"}; !reflect.DeepEqual(got.Packages.ByTag["personal"]["fedora"], want) {
		t.Errorf("ByTag[personal][fedora] = %v, want %v", got.Packages.ByTag["personal"]["fedora"], want)
	}
}
