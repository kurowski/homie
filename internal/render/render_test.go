package render

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kurowski/homie/internal/config"
	"github.com/kurowski/homie/internal/detect"
	"github.com/kurowski/homie/internal/tree"
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

// writeTemplate writes a .tmpl file under repo/home/<rel> and returns
// the resulting target path under home.
func writeTemplate(t *testing.T, repo, rel, body string, mode os.FileMode) {
	t.Helper()
	path := filepath.Join(repo, tree.HomeDir, rel)
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
	tplDir := filepath.Join(repo, tree.HomeDir)
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

func TestApplyOnlyPicksTemplates(t *testing.T) {
	// A unified home/ tree containing a mix of plain and .tmpl files —
	// Apply should write only the rendered (suffix-stripped) outputs.
	// The plain files are link's responsibility and must not appear
	// as render outputs.
	repo := t.TempDir()
	home := t.TempDir()
	mk := func(rel, body string) {
		path := filepath.Join(repo, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	mk("home/.zshrc", "# plain — not for render")
	mk("home/.gitconfig.tmpl", "name = {{ .Name }}")
	mk("home/bin/runme", "#!/bin/sh\necho hi")
	mk("home/bin/runme.sh.tmpl", "#!/bin/sh\necho {{ .Name }}")

	cfg := config.Config{User: config.User{Name: "Scout", Email: "scout@homie.sh"}}
	res := Apply(repo, home, cfg, detect.Env{})
	if len(res.Errors) != 0 {
		t.Fatalf("errors: %v", res.Errors)
	}
	// Both .tmpl files render to suffix-stripped paths.
	for _, rel := range []string{".gitconfig", "bin/runme.sh"} {
		if _, err := os.Stat(filepath.Join(home, rel)); err != nil {
			t.Errorf("expected rendered %s: %v", rel, err)
		}
	}
	// The plain files in the tree must NOT show up as render outputs.
	if _, err := os.Stat(filepath.Join(home, ".zshrc")); !os.IsNotExist(err) {
		t.Errorf("render should not write .zshrc — that's link's job")
	}
	if _, err := os.Stat(filepath.Join(home, "bin/runme")); !os.IsNotExist(err) {
		t.Errorf("render should not write bin/runme — that's link's job")
	}
}

func TestApplyTagGatedTemplates(t *testing.T) {
	repo := t.TempDir()
	mk := func(rel, body string) {
		path := filepath.Join(repo, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	mk("home/base.tmpl", `base {{ .Name }}`)
	mk("home.tag-work/work.tmpl", `work {{ .Name }}`)
	mk("home.tag-personal/personal.tmpl", `personal {{ .Name }}`)

	cfg := config.Config{User: config.User{Name: "Scout", Email: "scout@homie.sh"}}

	t.Run("no tags renders base only", func(t *testing.T) {
		home := t.TempDir()
		res := Apply(repo, home, cfg, detect.Env{})
		if len(res.Errors) != 0 {
			t.Fatalf("errors: %v", res.Errors)
		}
		if got, err := os.ReadFile(filepath.Join(home, "base")); err != nil {
			t.Fatalf("base: %v", err)
		} else if string(got) != "base Scout" {
			t.Errorf("base content = %q", got)
		}
		if _, err := os.Stat(filepath.Join(home, "work")); !os.IsNotExist(err) {
			t.Errorf("work template should NOT be rendered without the tag")
		}
	})

	t.Run("work tag renders the work tree too", func(t *testing.T) {
		home := t.TempDir()
		res := Apply(repo, home, cfg, detect.Env{Tags: []string{"work"}})
		if len(res.Errors) != 0 {
			t.Fatalf("errors: %v", res.Errors)
		}
		for _, want := range []string{"base", "work"} {
			if _, err := os.Stat(filepath.Join(home, want)); err != nil {
				t.Errorf("missing rendered file %s: %v", want, err)
			}
		}
		if _, err := os.Stat(filepath.Join(home, "personal")); !os.IsNotExist(err) {
			t.Errorf("personal template should NOT be rendered on a work host")
		}
	})
}

func TestApplyTemplateOverrideAcrossTrees(t *testing.T) {
	repo := t.TempDir()
	home := t.TempDir()
	mk := func(rel, body string) {
		path := filepath.Join(repo, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	// Same target from two trees of different specificity — the
	// more-specific work tree wins. Resolve, no collision error.
	mk("home/conf.tmpl", `base {{ .Name }}`)
	mk("home.tag-work/conf.tmpl", `work {{ .Name }}`)

	cfg := config.Config{User: config.User{Name: "Scout", Email: "scout@homie.sh"}}
	res := Apply(repo, home, cfg, detect.Env{Tags: []string{"work"}})

	if len(res.Errors) != 0 {
		t.Fatalf("expected no errors (work overrides base), got %v", res.Errors)
	}
	got, err := os.ReadFile(filepath.Join(home, "conf"))
	if err != nil {
		t.Fatalf("conf: %v", err)
	}
	if string(got) != "work Scout" {
		t.Errorf("conf = %q, want %q (work tree should win the override)", got, "work Scout")
	}
}

func TestApplyEqualSpecificityIsAmbiguous(t *testing.T) {
	repo := t.TempDir()
	home := t.TempDir()
	mk := func(rel, body string) {
		path := filepath.Join(repo, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	// Two tag dirs at the same specificity claim the same target —
	// Resolve refuses to guess and returns an error.
	mk("home.tag-work/conf.tmpl", `work {{ .Name }}`)
	mk("home.tag-personal/conf.tmpl", `personal {{ .Name }}`)

	cfg := config.Config{User: config.User{Name: "Scout", Email: "scout@homie.sh"}}
	res := Apply(repo, home, cfg, detect.Env{Tags: []string{"work", "personal"}})

	if len(res.Errors) != 1 {
		t.Fatalf("expected exactly 1 ambiguity error, got %d: %v", len(res.Errors), res.Errors)
	}
	if !strings.Contains(res.Errors[0].Error(), "same specificity") {
		t.Errorf("error should mention specificity tie: %v", res.Errors[0])
	}
	// Neither template should have rendered — Resolve fails before any
	// write happens.
	if _, err := os.Stat(filepath.Join(home, "conf")); !os.IsNotExist(err) {
		t.Errorf("conf should not exist when Resolve errored")
	}
}

func TestApplyNoHomeDir(t *testing.T) {
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

func TestApplySkipsAlreadyInSync(t *testing.T) {
	repo := t.TempDir()
	home := t.TempDir()
	writeTemplate(t, repo, ".gitconfig.tmpl", `[user]
name = {{ .Name }}
`, 0o644)

	cfg := config.Config{User: config.User{Name: "Scout", Email: "scout@homie.sh"}}
	env := detect.Env{}

	first := Apply(repo, home, cfg, env)
	if len(first.Errors) != 0 || len(first.Rendered) != 1 || len(first.Skipped) != 0 {
		t.Fatalf("first apply: rendered=%d skipped=%d errors=%v",
			len(first.Rendered), len(first.Skipped), first.Errors)
	}

	target := filepath.Join(home, ".gitconfig")
	beforeStat, err := os.Stat(target)
	if err != nil {
		t.Fatal(err)
	}

	// Bump mtime backward so we can detect any rewrite by comparing mtimes.
	past := beforeStat.ModTime().Add(-time.Hour)
	if err := os.Chtimes(target, past, past); err != nil {
		t.Fatal(err)
	}

	second := Apply(repo, home, cfg, env)
	if len(second.Errors) != 0 || len(second.Rendered) != 0 || len(second.Skipped) != 1 {
		t.Fatalf("second apply: rendered=%d skipped=%d errors=%v",
			len(second.Rendered), len(second.Skipped), second.Errors)
	}
	afterStat, err := os.Stat(target)
	if err != nil {
		t.Fatal(err)
	}
	if !afterStat.ModTime().Equal(past) {
		t.Errorf("target was rewritten (mtime changed): before=%v after=%v", past, afterStat.ModTime())
	}
}

func TestApplyRewritesWhenContentDiffers(t *testing.T) {
	repo := t.TempDir()
	home := t.TempDir()
	writeTemplate(t, repo, "config.tmpl", `name = {{ .Name }}`, 0o644)

	target := filepath.Join(home, "config")
	if err := os.WriteFile(target, []byte("name = Stale"), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := config.Config{User: config.User{Name: "Scout", Email: "scout@homie.sh"}}
	res := Apply(repo, home, cfg, detect.Env{})
	if len(res.Errors) != 0 || len(res.Rendered) != 1 || len(res.Skipped) != 0 {
		t.Fatalf("rendered=%d skipped=%d errors=%v",
			len(res.Rendered), len(res.Skipped), res.Errors)
	}
	out, _ := os.ReadFile(target)
	if string(out) != "name = Scout" {
		t.Errorf("content = %q, want %q", out, "name = Scout")
	}
}

func TestApplyRewritesWhenModeDiffers(t *testing.T) {
	repo := t.TempDir()
	home := t.TempDir()
	writeTemplate(t, repo, "bin/foo.sh.tmpl", `#!/bin/sh
echo {{ .Name }}
`, 0o755)

	// Run once so the file exists with correct mode.
	cfg := config.Config{User: config.User{Name: "Scout", Email: "scout@homie.sh"}}
	if first := Apply(repo, home, cfg, detect.Env{}); len(first.Errors) != 0 {
		t.Fatalf("first apply errors: %v", first.Errors)
	}

	// Strip the executable bit; next apply should restore it.
	target := filepath.Join(home, "bin", "foo.sh")
	if err := os.Chmod(target, 0o644); err != nil {
		t.Fatal(err)
	}

	res := Apply(repo, home, cfg, detect.Env{})
	if len(res.Errors) != 0 || len(res.Rendered) != 1 || len(res.Skipped) != 0 {
		t.Fatalf("rendered=%d skipped=%d errors=%v",
			len(res.Rendered), len(res.Skipped), res.Errors)
	}
	info, _ := os.Stat(target)
	if info.Mode().Perm()&0o100 == 0 {
		t.Errorf("executable bit not restored: mode=%v", info.Mode())
	}
}
