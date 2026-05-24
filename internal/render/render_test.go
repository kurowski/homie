package render

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kurowski/homie/internal/config"
	"github.com/kurowski/homie/internal/detect"
)

var sampleData = Data{
	Name:        "Scout Homes",
	Email:       "scout@homie.sh",
	Profile:     "personal",
	Distro:      "fedora",
	Arch:        "amd64",
	IsContainer: false,
	IsRoot:      false,
	Tags:        []string{"fedora", "amd64", "personal", "laptop"},
	Vars:        map[string]any{"EDITOR": "nvim", "WORK_EMAIL": "scout@work.example.com"},
}

func TestRender(t *testing.T) {
	cases := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{
			name:  "basic substitution",
			input: "name = {{ .Name }}",
			want:  "name = Scout Homes",
		},
		{
			name:  "vars map access",
			input: "editor = {{ .Vars.EDITOR }}",
			want:  "editor = nvim",
		},
		{
			name:  "hasTag true",
			input: `{{ if hasTag "fedora" }}on fedora{{ else }}elsewhere{{ end }}`,
			want:  "on fedora",
		},
		{
			name:  "hasTag false",
			input: `{{ if hasTag "ubuntu" }}on ubuntu{{ else }}elsewhere{{ end }}`,
			want:  "elsewhere",
		},
		{
			name:  "sprig upper",
			input: `{{ "hello" | upper }}`,
			want:  "HELLO",
		},
		{
			// missingkey=error means `default` can't rescue a missing map key —
			// the lookup errors before `default` ever sees the value. Use
			// hasKey or sprig's `dig` for optional vars.
			name:  "sprig dig with default",
			input: `{{ dig "MISSING" "fallback" .Vars }}`,
			want:  "fallback",
		},
		{
			name:  "hasKey on present var",
			input: `{{ if hasKey .Vars "EDITOR" }}{{ .Vars.EDITOR }}{{ else }}none{{ end }}`,
			want:  "nvim",
		},
		{
			name:  "hasKey on missing var",
			input: `{{ if hasKey .Vars "MISSING" }}{{ .Vars.MISSING }}{{ else }}none{{ end }}`,
			want:  "none",
		},
		{
			name:  "range over tags",
			input: `{{ range .Tags }}{{ . }},{{ end }}`,
			want:  "fedora,amd64,personal,laptop,",
		},
		{
			name:  "container/root flags",
			input: `container={{ .IsContainer }} root={{ .IsRoot }}`,
			want:  "container=false root=false",
		},
		{
			name:    "missing field errors",
			input:   `{{ .Bogus }}`,
			wantErr: true,
		},
		{
			name:    "unparseable template",
			input:   `{{ .Name`,
			wantErr: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Render(tc.input, sampleData)
			if tc.wantErr {
				if err == nil {
					t.Errorf("expected error, got %q", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestBuildData(t *testing.T) {
	cfg := config.Config{
		User:    config.User{Name: "Scout Homes", Email: "scout@homie.sh"},
		Profile: config.Profile{Name: "personal", DefaultShell: "zsh"},
		Tags:    config.Tags{Extra: []string{"laptop"}},
		Vars:    map[string]string{"EDITOR": "nvim"},
	}
	env := detect.Env{
		Distro: "fedora", Arch: "amd64",
		IsContainer: false, IsRoot: true,
		Tags: []string{"fedora", "amd64", "root"},
	}
	d := BuildData(cfg, env)
	if d.Name != "Scout Homes" || d.Email != "scout@homie.sh" {
		t.Errorf("identity wrong: %+v", d)
	}
	if d.Profile != "personal" {
		t.Errorf("Profile = %q", d.Profile)
	}
	if d.IsRoot != true {
		t.Errorf("IsRoot should propagate from env")
	}
	wantTags := []string{"amd64", "fedora", "laptop", "personal", "root"} // sorted, deduped
	if strings.Join(d.Tags, ",") != strings.Join(wantTags, ",") {
		t.Errorf("Tags = %v, want %v", d.Tags, wantTags)
	}
}

// writeTemplate writes a .tmpl file under repo/templates/<rel> and returns
// the resulting target path under home.
func writeTemplate(t *testing.T, repo, rel, body string, mode os.FileMode) {
	t.Helper()
	path := filepath.Join(repo, TemplatesDir, rel)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), mode); err != nil {
		t.Fatal(err)
	}
}

func TestApplyWritesFiles(t *testing.T) {
	repo := t.TempDir()
	home := t.TempDir()
	writeTemplate(t, repo, ".gitconfig.tmpl", `[user]
name = {{ .Name }}
email = {{ .Email }}
`, 0o644)
	writeTemplate(t, repo, "bin/say-hi.sh.tmpl", `#!/usr/bin/env bash
echo hello {{ .Name }}
`, 0o755)

	cfg := config.Config{User: config.User{Name: "Scout", Email: "scout@homie.sh"}}
	env := detect.Env{Distro: "fedora", Arch: "amd64", Tags: []string{"fedora", "amd64"}}
	res := Apply(repo, home, cfg, env)
	if len(res.Errors) != 0 {
		t.Fatalf("unexpected errors: %v", res.Errors)
	}
	if len(res.Rendered) != 2 {
		t.Fatalf("rendered count = %d, want 2", len(res.Rendered))
	}

	out, err := os.ReadFile(filepath.Join(home, ".gitconfig"))
	if err != nil {
		t.Fatalf("read gitconfig: %v", err)
	}
	if !strings.Contains(string(out), "name = Scout") {
		t.Errorf(".gitconfig content = %q", out)
	}

	// .tmpl suffix must be stripped from the target.
	scriptPath := filepath.Join(home, "bin", "say-hi.sh")
	info, err := os.Stat(scriptPath)
	if err != nil {
		t.Fatalf("script not written: %v", err)
	}
	if info.Mode().Perm()&0o100 == 0 {
		t.Errorf("executable bit not preserved: mode=%v", info.Mode())
	}
}

func TestApplySkipsNonTmpl(t *testing.T) {
	repo := t.TempDir()
	home := t.TempDir()
	// File without .tmpl extension should be ignored.
	tplDir := filepath.Join(repo, TemplatesDir)
	if err := os.MkdirAll(tplDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tplDir, "README"), []byte("notes"), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := config.Config{User: config.User{Name: "Scout", Email: "scout@homie.sh"}}
	res := Apply(repo, home, cfg, detect.Env{})
	if len(res.Rendered) != 0 || len(res.Errors) != 0 {
		t.Errorf("expected no-op, got %+v", res)
	}
}

func TestApplyNoTemplatesDir(t *testing.T) {
	res := Apply(t.TempDir(), t.TempDir(), config.Config{}, detect.Env{})
	if len(res.Rendered) != 0 || len(res.Errors) != 0 {
		t.Errorf("expected empty result, got %+v", res)
	}
}

func TestApplyCollectsErrors(t *testing.T) {
	repo := t.TempDir()
	home := t.TempDir()
	writeTemplate(t, repo, "good.tmpl", `hello {{ .Name }}`, 0o644)
	writeTemplate(t, repo, "bad.tmpl", `oops {{ .Bogus }}`, 0o644)

	cfg := config.Config{User: config.User{Name: "Scout", Email: "scout@homie.sh"}}
	res := Apply(repo, home, cfg, detect.Env{})
	if len(res.Rendered) != 1 {
		t.Errorf("Rendered = %d, want 1 (the good template)", len(res.Rendered))
	}
	if len(res.Errors) != 1 {
		t.Errorf("Errors = %d, want 1 (the bad template)", len(res.Errors))
	}
	// The good output should still exist even though the other template failed.
	if _, err := os.Stat(filepath.Join(home, "good")); err != nil {
		t.Errorf("good template should still have rendered: %v", err)
	}
}

func TestApplyOverwritesStaleSymlink(t *testing.T) {
	repo := t.TempDir()
	home := t.TempDir()
	writeTemplate(t, repo, "config.tmpl", `name = {{ .Name }}`, 0o644)

	// An old symlink at the target pointing to a sibling file.
	elsewhere := filepath.Join(t.TempDir(), "old")
	if err := os.WriteFile(elsewhere, []byte("old data"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(elsewhere, filepath.Join(home, "config")); err != nil {
		t.Fatal(err)
	}

	cfg := config.Config{User: config.User{Name: "Scout", Email: "scout@homie.sh"}}
	res := Apply(repo, home, cfg, detect.Env{})
	if len(res.Errors) != 0 {
		t.Fatalf("errors: %v", res.Errors)
	}

	// Old symlink target must NOT have been written through.
	old, err := os.ReadFile(elsewhere)
	if err != nil {
		t.Fatal(err)
	}
	if string(old) != "old data" {
		t.Errorf("write followed symlink: elsewhere now contains %q", old)
	}

	// Target should be a regular file, not a symlink.
	info, err := os.Lstat(filepath.Join(home, "config"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		t.Errorf("target should be a regular file, got symlink")
	}
	out, _ := os.ReadFile(filepath.Join(home, "config"))
	if string(out) != "name = Scout" {
		t.Errorf("rendered content = %q", out)
	}
}
