package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kurowski/homie/internal/detect"
)

// writeRepo writes a homie.toml (and optional hosts overlay) into a temp
// repo dir and returns its path.
func writeRepo(t *testing.T, body string, overlays map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "homie.toml"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	for host, content := range overlays {
		path := filepath.Join(dir, HostsDir, host+".toml")
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

const externalsHeader = `
[user]
name  = "Scout Homes"
email = "scout@homie.sh"
`

func TestExternalsParseAndResolve(t *testing.T) {
	dir := writeRepo(t, externalsHeader+`
[externals."~/.zsh/plugins/zsh-autosuggestions"]
repo = "https://github.com/zsh-users/zsh-autosuggestions"

[externals."~/.config/nvim"]
repo = "https://github.com/example/astronvim-template"
ref  = "v4.0.0"

[externals."tag:desktop"."~/.config/theme"]
repo = "https://github.com/example/theme"
`, nil)
	cfg, err := Load(dir, "")
	if err != nil {
		t.Fatal(err)
	}

	// Without the desktop tag, only the two base entries resolve.
	got, err := cfg.ExternalsFor(detect.Env{})
	if err != nil {
		t.Fatal(err)
	}
	want := []External{
		{Dest: "~/.config/nvim", Repo: "https://github.com/example/astronvim-template", Ref: "v4.0.0"},
		{Dest: "~/.zsh/plugins/zsh-autosuggestions", Repo: "https://github.com/zsh-users/zsh-autosuggestions"},
	}
	assertExternals(t, got, want)

	// With the tag active, the gated entry joins, sorted by dest.
	got, err = cfg.ExternalsFor(detect.Env{Tags: []string{"desktop"}})
	if err != nil {
		t.Fatal(err)
	}
	want = append([]External{want[0], {Dest: "~/.config/theme", Repo: "https://github.com/example/theme"}}, want[1])
	assertExternals(t, got, want)
}

func TestExternalsMissingRepoErrors(t *testing.T) {
	dir := writeRepo(t, externalsHeader+`
[externals."~/.config/nvim"]
ref = "v4.0.0"
`, nil)
	_, err := Load(dir, "")
	if err == nil || !strings.Contains(err.Error(), "repo is required") {
		t.Fatalf("want repo-required error, got: %v", err)
	}
}

func TestExternalsUnknownKeyWarns(t *testing.T) {
	dir := writeRepo(t, externalsHeader+`
[externals."~/.config/nvim"]
repo   = "https://github.com/example/x"
branch = "main"
`, nil)
	cfg, err := Load(dir, "")
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, w := range cfg.Warnings {
		if strings.Contains(w, `unknown key "branch"`) {
			found = true
		}
	}
	if !found {
		t.Errorf("expected a warning about the unknown key, got: %v", cfg.Warnings)
	}
}

// TestExternalsMoreSpecificWins: a tag-gated entry overrides a base
// entry for the same destination; two active blocks at the same
// specificity with different settings conflict.
func TestExternalsMoreSpecificWins(t *testing.T) {
	dir := writeRepo(t, externalsHeader+`
[externals."~/.config/nvim"]
repo = "https://github.com/example/nvim-base"

[externals."tag:work"."~/.config/nvim"]
repo = "https://github.com/example/nvim-work"
ref  = "stable"
`, nil)
	cfg, err := Load(dir, "")
	if err != nil {
		t.Fatal(err)
	}

	got, err := cfg.ExternalsFor(detect.Env{Tags: []string{"work"}})
	if err != nil {
		t.Fatal(err)
	}
	assertExternals(t, got, []External{
		{Dest: "~/.config/nvim", Repo: "https://github.com/example/nvim-work", Ref: "stable"},
	})

	// Same destination from two single-tag blocks, both active, with
	// different repos: no winner, must error.
	dir = writeRepo(t, externalsHeader+`
[externals."tag:work"."~/.config/nvim"]
repo = "https://github.com/example/nvim-work"

[externals."tag:desktop"."~/.config/nvim"]
repo = "https://github.com/example/nvim-desktop"
`, nil)
	cfg, err = Load(dir, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := cfg.ExternalsFor(detect.Env{Tags: []string{"work", "desktop"}}); err == nil {
		t.Fatal("conflicting same-specificity entries should error")
	}
	// Only one of the two active: no conflict.
	if _, err := cfg.ExternalsFor(detect.Env{Tags: []string{"work"}}); err != nil {
		t.Fatalf("single active block should resolve, got: %v", err)
	}

	// Identical specs at the same specificity collapse silently — no
	// winner is needed when there's nothing to choose between.
	dir = writeRepo(t, externalsHeader+`
[externals."tag:work"."~/.config/nvim"]
repo = "https://github.com/example/nvim"

[externals."tag:desktop"."~/.config/nvim"]
repo = "https://github.com/example/nvim"
`, nil)
	cfg, err = Load(dir, "")
	if err != nil {
		t.Fatal(err)
	}
	got, err = cfg.ExternalsFor(detect.Env{Tags: []string{"work", "desktop"}})
	if err != nil {
		t.Fatalf("identical duplicates should collapse, got: %v", err)
	}
	assertExternals(t, got, []External{
		{Dest: "~/.config/nvim", Repo: "https://github.com/example/nvim"},
	})
}

// TestExternalsHostOverlayReplaces: hosts/<name>.toml entries replace
// per destination (specs aren't lists, so there's nothing to append).
func TestExternalsHostOverlayReplaces(t *testing.T) {
	dir := writeRepo(t, externalsHeader+`
[externals."~/.config/nvim"]
repo = "https://github.com/example/nvim"
ref  = "v1"
`, map[string]string{"coach": `
[externals."~/.config/nvim"]
repo = "https://github.com/example/nvim"
ref  = "v2"

[externals."~/.zsh/plugins/extra"]
repo = "https://github.com/example/extra"
`})
	cfg, err := Load(dir, "coach")
	if err != nil {
		t.Fatal(err)
	}
	got, err := cfg.ExternalsFor(detect.Env{})
	if err != nil {
		t.Fatal(err)
	}
	assertExternals(t, got, []External{
		{Dest: "~/.config/nvim", Repo: "https://github.com/example/nvim", Ref: "v2"},
		{Dest: "~/.zsh/plugins/extra", Repo: "https://github.com/example/extra"},
	})
}

func assertExternals(t *testing.T, got, want []External) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("got %d externals, want %d: %+v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("externals[%d] = %+v, want %+v", i, got[i], want[i])
		}
	}
}
