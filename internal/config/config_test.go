package config

import (
	"os"
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
		"packages.feodra",    // base distro typo
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

func TestPackagesForBackend(t *testing.T) {
	c, err := Load(filepath.Join("testdata", "backends"), "")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	// Native still works alongside backends.
	if got := c.PackagesFor(detect.Env{Distro: "fedora"}); !reflect.DeepEqual(got, []string{"git"}) {
		t.Errorf("PackagesFor(fedora) = %v, want [git]", got)
	}

	cases := []struct {
		name    string
		backend string
		env     detect.Env
		want    []string
	}{
		{
			name:    "flatpak base resolves all + distro",
			backend: "flatpak",
			env:     detect.Env{Distro: "fedora"},
			want:    []string{"md.obsidian.Obsidian", "org.localsend.localsend_app"},
		},
		{
			name:    "flatpak picks up tag-keyed entry when tag active",
			backend: "flatpak",
			env:     detect.Env{Distro: "fedora", Tags: []string{"work"}},
			want:    []string{"md.obsidian.Obsidian", "org.localsend.localsend_app", "us.zoom.Zoom"},
		},
		{
			name:    "flatpak skips tag-keyed entry when tag inactive",
			backend: "flatpak",
			env:     detect.Env{Distro: "ubuntu"},
			want:    []string{"md.obsidian.Obsidian"},
		},
		{
			name:    "brew is distro-agnostic via all",
			backend: "brew",
			env:     detect.Env{Distro: "fedora"},
			want:    []string{"fd", "ripgrep"},
		},
		{
			name:    "unknown backend returns nil",
			backend: "cargo",
			env:     detect.Env{Distro: "fedora"},
			want:    nil,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := c.PackagesForBackend(tc.env, tc.backend)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("PackagesForBackend(%q) =\n  got: %v\n want: %v", tc.backend, got, tc.want)
			}
		})
	}
}

func TestUnknownBackendIsWarnedNotErrored(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "homie.toml"), []byte(`
[user]
name  = "Scout Homes"
email = "scout@homie.sh"

[packages.cargo]
all = ["cargo-edit"]

[packages."tag:work".npm]
all = ["typescript"]
`), 0o644); err != nil {
		t.Fatal(err)
	}
	c, err := Load(dir, "")
	if err != nil {
		t.Fatalf("Load should accept unknown backend tables as a forward-compat warning, got: %v", err)
	}
	// Unknown backends are parsed into Backends so doctor can warn about
	// them at apply time and so the file still round-trips.
	if got := c.Packages.Backends["cargo"].Base["all"]; !reflect.DeepEqual(got, []string{"cargo-edit"}) {
		t.Errorf("cargo.Base[all] = %v, want [cargo-edit]", got)
	}
	if got := c.Packages.Backends["npm"].ByTag["work"]["all"]; !reflect.DeepEqual(got, []string{"typescript"}) {
		t.Errorf("npm.ByTag[work][all] = %v, want [typescript]", got)
	}
	joined := strings.Join(c.Warnings, "\n")
	for _, want := range []string{"packages.cargo", `tag:work"].npm`} {
		if !strings.Contains(joined, want) {
			t.Errorf("expected a warning mentioning %q, got:\n%s", want, joined)
		}
	}
	if !strings.Contains(joined, "looks like a backend but isn't recognized") {
		t.Errorf("warning should clarify the backend isn't recognized, got:\n%s", joined)
	}
}

func TestMalformedTagKeyErrors(t *testing.T) {
	// Each of these is a structurally invalid [packages."..."] key and must
	// fail Load with a clear message rather than silently not matching.
	for _, key := range []string{
		"tag:",          // empty tag name
		"tag:X.foo",     // second segment isn't a tag: segment
		"tag:X.",        // trailing empty segment
		"tag:.tag:work", // empty first segment
	} {
		t.Run(key, func(t *testing.T) {
			dir := t.TempDir()
			body := `
[user]
name  = "Scout Homes"
email = "scout@homie.sh"

[packages."` + key + `"]
fedora = ["pkg"]
`
			if err := os.WriteFile(filepath.Join(dir, "homie.toml"), []byte(body), 0o644); err != nil {
				t.Fatal(err)
			}
			_, err := Load(dir, "")
			if err == nil {
				t.Fatalf("Load should reject malformed tag key %q", key)
			}
			if !strings.Contains(err.Error(), "malformed tag key") {
				t.Errorf("error for %q should be clear about the malformed key, got: %v", key, err)
			}
		})
	}
}

func TestDuplicateCanonicalTagKeyWarns(t *testing.T) {
	// Two blocks written in different tag orders resolve to the same
	// canonical key within one file — they overwrite rather than merge, so
	// the user gets a warning to combine them. Which one wins is unspecified
	// (it depends on TOML map iteration order); the contract is "warn, and
	// don't merge", not a particular winner.
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "homie.toml"), []byte(`
[user]
name  = "Scout Homes"
email = "scout@homie.sh"

[packages."tag:a.tag:b"]
all = ["first"]

[packages."tag:b.tag:a"]
all = ["second"]

[packages."tag:c.tag:d".snap]
all = ["s1"]

[packages."tag:d.tag:c".snap]
all = ["s2"]
`), 0o644); err != nil {
		t.Fatal(err)
	}
	c, err := Load(dir, "")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	joined := strings.Join(c.Warnings, "\n")
	if !strings.Contains(joined, "duplicates an earlier block") {
		t.Errorf("expected a duplicate-canonical-key warning, got:\n%s", joined)
	}
	// Native: exactly one block wins (overwrite, not merge). Don't assert
	// which — that's map-iteration-order dependent; assert the lists weren't
	// concatenated into ["first","second"].
	if got := c.PackagesFor(detect.Env{Tags: []string{"a", "b"}}); len(got) != 1 || (got[0] != "first" && got[0] != "second") {
		t.Errorf("native dup: PackagesFor = %v, want exactly one of [first]/[second] (overwrite, not merge)", got)
	}
	// Backend collision warns too.
	if !strings.Contains(joined, "snap") {
		t.Errorf("expected the backend duplicate to warn as well, got:\n%s", joined)
	}
}

func TestPackagesForChainedTags(t *testing.T) {
	c, err := Load(filepath.Join("testdata", "and-tag-packages"), "")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	cases := []struct {
		name    string
		env     detect.Env
		native  []string
		snap    []string
		flatpak []string
	}{
		{
			name:    "personal AND ubuntu active",
			env:     detect.Env{Distro: "ubuntu", Tags: []string{"personal", "ubuntu"}},
			native:  []string{"git", "base-and-pkg", "ubuntu-only"},
			snap:    nil, // desktop not active
			flatpak: nil, // work not active
		},
		{
			name:    "personal AND desktop active picks up the snap block",
			env:     detect.Env{Distro: "ubuntu", Tags: []string{"personal", "desktop"}},
			native:  []string{"git"}, // personal.ubuntu needs ubuntu tag, absent
			snap:    []string{"gimp", "spotify"},
			flatpak: nil,
		},
		{
			name:    "work AND ubuntu active picks up the flatpak block (order-independent key)",
			env:     detect.Env{Distro: "ubuntu", Tags: []string{"work", "ubuntu"}},
			native:  []string{"git", "kubectl"}, // single-tag work block
			snap:    nil,
			flatpak: []string{"us.zoom.Zoom"},
		},
		{
			name:    "only one of an AND pair active contributes nothing from that block",
			env:     detect.Env{Distro: "ubuntu", Tags: []string{"personal"}},
			native:  []string{"git"},
			snap:    nil,
			flatpak: nil,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := c.PackagesFor(tc.env); !reflect.DeepEqual(got, tc.native) {
				t.Errorf("PackagesFor =\n  got: %v\n want: %v", got, tc.native)
			}
			if got := c.PackagesForBackend(tc.env, "snap"); !reflect.DeepEqual(got, tc.snap) {
				t.Errorf("PackagesForBackend(snap) =\n  got: %v\n want: %v", got, tc.snap)
			}
			if got := c.PackagesForBackend(tc.env, "flatpak"); !reflect.DeepEqual(got, tc.flatpak) {
				t.Errorf("PackagesForBackend(flatpak) =\n  got: %v\n want: %v", got, tc.flatpak)
			}
		})
	}
}

func TestActiveTagBlocks(t *testing.T) {
	c, err := Load(filepath.Join("testdata", "and-tag-packages"), "")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	// personal+ubuntu: only the native personal.ubuntu AND-block applies.
	got := c.ActiveTagBlocks(detect.Env{Distro: "ubuntu", Tags: []string{"personal", "ubuntu"}})
	if want := []string{"tag:personal.tag:ubuntu"}; !reflect.DeepEqual(got, want) {
		t.Errorf("ActiveTagBlocks =\n  got: %v\n want: %v", got, want)
	}
	// work+ubuntu: the flatpak block (written tag:work.tag:ubuntu) shows in
	// canonical sorted display form; the single-tag work block is omitted.
	got = c.ActiveTagBlocks(detect.Env{Distro: "ubuntu", Tags: []string{"work", "ubuntu"}})
	if want := []string{"tag:ubuntu.tag:work"}; !reflect.DeepEqual(got, want) {
		t.Errorf("ActiveTagBlocks =\n  got: %v\n want: %v", got, want)
	}
	// No AND-pair fully active.
	if got := c.ActiveTagBlocks(detect.Env{Distro: "ubuntu", Tags: []string{"personal"}}); len(got) != 0 {
		t.Errorf("ActiveTagBlocks with no AND-pair satisfied = %v, want empty", got)
	}
}

func TestMergeBackends(t *testing.T) {
	base := Config{
		Packages: Packages{
			Backends: map[string]BackendPackages{
				"flatpak": {
					Base:  map[string][]string{"all": {"md.obsidian.Obsidian"}},
					ByTag: map[string]map[string][]string{"work": {"all": {"us.zoom.Zoom"}}},
				},
			},
		},
	}
	overlay := Config{
		Packages: Packages{
			Backends: map[string]BackendPackages{
				"flatpak": {
					Base:  map[string][]string{"all": {"md.obsidian.Obsidian", "io.bassi.Amberol"}}, // overlap on Obsidian
					ByTag: map[string]map[string][]string{"work": {"all": {"us.zoom.Zoom", "com.slack.Slack"}}},
				},
				"brew": {
					Base: map[string][]string{"all": {"fd"}},
				},
			},
		},
	}
	got := merge(base, overlay)
	if w, want := got.Packages.Backends["flatpak"].Base["all"], []string{"md.obsidian.Obsidian", "io.bassi.Amberol"}; !reflect.DeepEqual(w, want) {
		t.Errorf("flatpak.Base[all] = %v, want %v (Obsidian deduped)", w, want)
	}
	if w, want := got.Packages.Backends["flatpak"].ByTag["work"]["all"], []string{"us.zoom.Zoom", "com.slack.Slack"}; !reflect.DeepEqual(w, want) {
		t.Errorf("flatpak.ByTag[work][all] = %v, want %v", w, want)
	}
	if w, want := got.Packages.Backends["brew"].Base["all"], []string{"fd"}; !reflect.DeepEqual(w, want) {
		t.Errorf("brew.Base[all] = %v, want %v", w, want)
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
