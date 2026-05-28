package runner

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kurowski/homie/internal/config"
)

func writeScript(t *testing.T, repo, name, body string) {
	t.Helper()
	writeTreeScript(t, repo, ScriptsDir, name, body)
}

// writeTreeScript writes a script into an arbitrary script tree under the
// repo (e.g. "scripts", "scripts.tag-fedora", "scripts.tag-a.tag-b").
func writeTreeScript(t *testing.T, repo, treeDir, name, body string) {
	t.Helper()
	dir := filepath.Join(repo, treeDir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
}

func TestRunNoScriptsDir(t *testing.T) {
	res := Run(t.TempDir(), t.TempDir(), config.Config{}, nil, PhasePost, new(bytes.Buffer))
	if len(res.Ran) != 0 || len(res.Errors) != 0 {
		t.Errorf("expected empty result, got %+v", res)
	}
}

func TestRunLexicalOrderAndEnv(t *testing.T) {
	repo := t.TempDir()
	home := t.TempDir()
	marker := filepath.Join(home, "marker")

	// Each script appends "<n>:<envvar>" so the marker file ends up
	// recording both the order they ran in and the env they saw.
	writeScript(t, repo, "02-second.sh", `printf "2:$HM_REPO=$HM_HOME=$EDITOR\n" >> "$HM_HOME/marker"`)
	writeScript(t, repo, "01-first.sh", `printf "1:$HM_TAGS\n" >> "$HM_HOME/marker"`)
	writeScript(t, repo, "10-third.sh", `printf "3:$WORK_EMAIL\n" >> "$HM_HOME/marker"`)
	// Non-.sh file should be ignored.
	if err := os.WriteFile(filepath.Join(repo, ScriptsDir, "README"), []byte("notes"), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := config.Config{
		Vars: map[string]string{
			"EDITOR":     "nvim",
			"WORK_EMAIL": "scout@work.example.com",
		},
	}
	res := Run(repo, home, cfg, []string{"fedora", "amd64", "personal"}, PhasePost, new(bytes.Buffer))
	if len(res.Errors) != 0 {
		t.Fatalf("errors: %v", res.Errors)
	}
	if len(res.Ran) != 3 {
		t.Fatalf("Ran = %d, want 3", len(res.Ran))
	}

	data, err := os.ReadFile(marker)
	if err != nil {
		t.Fatalf("marker: %v", err)
	}
	got := strings.TrimSpace(string(data))
	want := strings.Join([]string{
		"1:fedora,amd64,personal",
		"2:" + repo + "=" + home + "=nvim",
		"3:scout@work.example.com",
	}, "\n")
	if got != want {
		t.Errorf("marker contents:\ngot:\n%s\nwant:\n%s", got, want)
	}

	// Ran should be in lexical order too.
	gotOrder := []string{
		filepath.Base(res.Ran[0].Path),
		filepath.Base(res.Ran[1].Path),
		filepath.Base(res.Ran[2].Path),
	}
	wantOrder := []string{"01-first.sh", "02-second.sh", "10-third.sh"}
	if strings.Join(gotOrder, ",") != strings.Join(wantOrder, ",") {
		t.Errorf("script order = %v, want %v", gotOrder, wantOrder)
	}
}

func TestRunCollectsErrorsAndKeepsGoing(t *testing.T) {
	repo := t.TempDir()
	home := t.TempDir()
	writeScript(t, repo, "01-ok.sh", `echo first`)
	writeScript(t, repo, "02-fail.sh", `echo failing; exit 7`)
	writeScript(t, repo, "03-ok.sh", `echo third > "$HM_HOME/third"`)

	out := new(bytes.Buffer)
	res := Run(repo, home, config.Config{}, nil, PhasePost, out)
	if len(res.Ran) != 3 {
		t.Errorf("Ran = %d, want 3 (one error mid-run should not abort)", len(res.Ran))
	}
	if len(res.Errors) != 1 {
		t.Errorf("Errors = %d, want 1", len(res.Errors))
	}
	// The third script must still have run, proving error didn't abort.
	if _, err := os.Stat(filepath.Join(home, "third")); err != nil {
		t.Errorf("third script should have run after the failing one: %v", err)
	}
	// The failing script's stderr should have made it into out.
	if !strings.Contains(out.String(), "failing") {
		t.Errorf("output should include failing script's stdout, got %q", out.String())
	}
}

func TestRunPhaseFilter(t *testing.T) {
	repo := t.TempDir()
	home := t.TempDir()
	writeScript(t, repo, "pre-01-repos.sh", `printf "pre1\n" >> "$HM_HOME/log"`)
	writeScript(t, repo, "pre-02-keys.sh", `printf "pre2\n" >> "$HM_HOME/log"`)
	writeScript(t, repo, "01-tools.sh", `printf "post1\n" >> "$HM_HOME/log"`)
	writeScript(t, repo, "02-shell.sh", `printf "post2\n" >> "$HM_HOME/log"`)

	pre := Run(repo, home, config.Config{}, nil, PhasePre, new(bytes.Buffer))
	if len(pre.Errors) != 0 {
		t.Fatalf("pre errors: %v", pre.Errors)
	}
	if got := scriptNames(pre); !equal(got, []string{"pre-01-repos.sh", "pre-02-keys.sh"}) {
		t.Errorf("pre Ran = %v, want [pre-01-repos.sh pre-02-keys.sh]", got)
	}

	post := Run(repo, home, config.Config{}, nil, PhasePost, new(bytes.Buffer))
	if len(post.Errors) != 0 {
		t.Fatalf("post errors: %v", post.Errors)
	}
	if got := scriptNames(post); !equal(got, []string{"01-tools.sh", "02-shell.sh"}) {
		t.Errorf("post Ran = %v, want [01-tools.sh 02-shell.sh]", got)
	}

	// Marker file confirms both phases executed in order: pre1, pre2, post1, post2.
	data, err := os.ReadFile(filepath.Join(home, "log"))
	if err != nil {
		t.Fatalf("log: %v", err)
	}
	want := "pre1\npre2\npost1\npost2\n"
	if string(data) != want {
		t.Errorf("log =\n%s\nwant:\n%s", data, want)
	}
}

func scriptNames(res Result) []string {
	out := make([]string, 0, len(res.Ran))
	for _, r := range res.Ran {
		out = append(out, filepath.Base(r.Path))
	}
	return out
}

func equal(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestRunMergesTagTreesInUnifiedOrder(t *testing.T) {
	repo := t.TempDir()
	home := t.TempDir()

	// Three trees; tag set activates fedora but not ubuntu. Filenames
	// interleave across trees, so a correct merge orders by filename:
	// 01 (plain) < 02 (fedora) < 03 (plain); the ubuntu 02b never runs.
	writeScript(t, repo, "01-a.sh", `printf "a\n" >> "$HM_HOME/log"`)
	writeScript(t, repo, "03-c.sh", `printf "c\n" >> "$HM_HOME/log"`)
	writeTreeScript(t, repo, "scripts.tag-fedora", "02-b.sh", `printf "b\n" >> "$HM_HOME/log"`)
	writeTreeScript(t, repo, "scripts.tag-ubuntu", "02b-skip.sh", `printf "SKIP\n" >> "$HM_HOME/log"`)

	res := Run(repo, home, config.Config{}, []string{"fedora"}, PhasePost, new(bytes.Buffer))
	if len(res.Errors) != 0 {
		t.Fatalf("errors: %v", res.Errors)
	}
	if got := scriptNames(res); !equal(got, []string{"01-a.sh", "02-b.sh", "03-c.sh"}) {
		t.Errorf("order = %v, want [01-a.sh 02-b.sh 03-c.sh] (ubuntu tree inactive)", got)
	}
	data, err := os.ReadFile(filepath.Join(home, "log"))
	if err != nil {
		t.Fatalf("log: %v", err)
	}
	if string(data) != "a\nb\nc\n" {
		t.Errorf("log = %q, want \"a\\nb\\nc\\n\"", data)
	}
}

func TestRunTagTreeAndSemantics(t *testing.T) {
	repo := t.TempDir()
	writeTreeScript(t, repo, "scripts.tag-work.tag-kde", "01-x.sh", `echo x`)

	// Only one of the two required tags active → tree inactive.
	one := Run(repo, t.TempDir(), config.Config{}, []string{"work"}, PhasePost, new(bytes.Buffer))
	if len(one.Ran) != 0 {
		t.Errorf("with only 'work' active, the work.kde tree must not run; ran %v", scriptNames(one))
	}
	// Both tags active → tree runs.
	both := Run(repo, t.TempDir(), config.Config{}, []string{"work", "kde"}, PhasePost, new(bytes.Buffer))
	if got := scriptNames(both); !equal(got, []string{"01-x.sh"}) {
		t.Errorf("with both tags active, ran = %v, want [01-x.sh]", got)
	}
}

func TestRunTagTreeRespectsPhase(t *testing.T) {
	repo := t.TempDir()
	writeTreeScript(t, repo, "scripts.tag-fedora", "pre-01-repos.sh", `echo pre`)
	writeTreeScript(t, repo, "scripts.tag-fedora", "10-tool.sh", `echo post`)

	pre := Run(repo, t.TempDir(), config.Config{}, []string{"fedora"}, PhasePre, new(bytes.Buffer))
	if got := scriptNames(pre); !equal(got, []string{"pre-01-repos.sh"}) {
		t.Errorf("pre phase ran = %v, want [pre-01-repos.sh]", got)
	}
	post := Run(repo, t.TempDir(), config.Config{}, []string{"fedora"}, PhasePost, new(bytes.Buffer))
	if got := scriptNames(post); !equal(got, []string{"10-tool.sh"}) {
		t.Errorf("post phase ran = %v, want [10-tool.sh]", got)
	}
}

func TestRunFilenameCollisionIsError(t *testing.T) {
	repo := t.TempDir()
	writeScript(t, repo, "05-foo.sh", `echo plain`)
	writeTreeScript(t, repo, "scripts.tag-fedora", "05-foo.sh", `echo fedora`)

	// Tag inactive → only the plain copy applies → no collision.
	ok := Run(repo, t.TempDir(), config.Config{}, nil, PhasePost, new(bytes.Buffer))
	if len(ok.Errors) != 0 {
		t.Fatalf("no collision expected when the tag tree is inactive: %v", ok.Errors)
	}
	if got := scriptNames(ok); !equal(got, []string{"05-foo.sh"}) {
		t.Errorf("ran = %v, want [05-foo.sh]", got)
	}

	// Tag active → both trees claim 05-foo.sh → hard error, nothing runs.
	clash := Run(repo, t.TempDir(), config.Config{}, []string{"fedora"}, PhasePost, new(bytes.Buffer))
	if len(clash.Errors) != 1 {
		t.Fatalf("expected one collision error, got %v", clash.Errors)
	}
	if len(clash.Ran) != 0 {
		t.Errorf("no script should run when a collision is detected; ran %v", scriptNames(clash))
	}
	if !strings.Contains(clash.Errors[0].Error(), "05-foo.sh") {
		t.Errorf("collision error %q should name the clashing filename", clash.Errors[0])
	}
}

func TestPlanReportsOrderWithoutRunning(t *testing.T) {
	repo := t.TempDir()
	writeScript(t, repo, "02-b.sh", `echo b`)
	writeTreeScript(t, repo, "scripts.tag-fedora", "01-a.sh", `echo a`)

	paths, err := Plan(repo, []string{"fedora"}, PhasePost)
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	var names []string
	for _, p := range paths {
		names = append(names, filepath.Base(p))
	}
	if !equal(names, []string{"01-a.sh", "02-b.sh"}) {
		t.Errorf("Plan order = %v, want [01-a.sh 02-b.sh]", names)
	}
}

func TestRunStreamsOutput(t *testing.T) {
	repo := t.TempDir()
	writeScript(t, repo, "00-talk.sh", `echo hello`)
	out := new(bytes.Buffer)
	res := Run(repo, t.TempDir(), config.Config{}, nil, PhasePost, out)
	if len(res.Errors) != 0 {
		t.Fatalf("errors: %v", res.Errors)
	}
	if !strings.Contains(out.String(), "hello") {
		t.Errorf("output = %q, want to contain hello", out.String())
	}
}
