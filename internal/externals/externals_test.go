package externals

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fakeGit scripts git responses keyed by the joined argument list.
// Each key holds a FIFO queue so repeated invocations (rev-parse HEAD
// before and after a pull) can return different values. Unscripted
// calls succeed with empty output.
type fakeGit struct {
	calls []string
	resp  map[string][]gitResp
}

type gitResp struct {
	out string
	err error
}

func (f *fakeGit) run(name string, args ...string) ([]byte, error) {
	if name != "git" {
		return nil, errors.New("unexpected command " + name)
	}
	key := strings.Join(args, " ")
	f.calls = append(f.calls, key)
	q := f.resp[key]
	if len(q) == 0 {
		return nil, nil
	}
	f.resp[key] = q[1:]
	return []byte(q[0].out), q[0].err
}

func (f *fakeGit) called(substr string) bool {
	for _, c := range f.calls {
		if strings.Contains(c, substr) {
			return true
		}
	}
	return false
}

// gitDir creates dest with a .git marker so Sync takes the update path.
func gitDir(t *testing.T, home string) string {
	t.Helper()
	dest := filepath.Join(home, "plug")
	if err := os.MkdirAll(filepath.Join(dest, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	return dest
}

const repoURL = "https://github.com/example/plug"

func TestSyncCloneWhenMissing(t *testing.T) {
	home := t.TempDir()
	git := &fakeGit{}

	a, err := Sync(home, Spec{Dest: "~/plug", Repo: repoURL}, git.run)
	if err != nil {
		t.Fatal(err)
	}
	if a.Kind != KindClone || a.Dest != filepath.Join(home, "plug") {
		t.Errorf("action = %+v", a)
	}
	want := "clone --depth 1 " + repoURL + " " + a.Dest
	if len(git.calls) != 1 || git.calls[0] != want {
		t.Errorf("calls = %v, want [%s]", git.calls, want)
	}
}

// TestSyncCloneWhenPinned: a pinned clone is full (a SHA pin can point
// anywhere in history) and ends detached at the ref.
func TestSyncCloneWhenPinned(t *testing.T) {
	home := t.TempDir()
	git := &fakeGit{}

	a, err := Sync(home, Spec{Dest: "$HOME/plug", Repo: repoURL, Ref: "v1.5.0"}, git.run)
	if err != nil {
		t.Fatal(err)
	}
	if a.Kind != KindClone || !strings.Contains(a.Detail, "@ v1.5.0") {
		t.Errorf("action = %+v", a)
	}
	dest := filepath.Join(home, "plug")
	wantCalls := []string{
		"clone " + repoURL + " " + dest,
		"-C " + dest + " checkout --detach v1.5.0",
	}
	if strings.Join(git.calls, "\n") != strings.Join(wantCalls, "\n") {
		t.Errorf("calls = %v, want %v", git.calls, wantCalls)
	}
}

func TestSyncRefusesNonGitDest(t *testing.T) {
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, "plug"), 0o755); err != nil {
		t.Fatal(err)
	}
	git := &fakeGit{}

	_, err := Sync(home, Spec{Dest: "~/plug", Repo: repoURL}, git.run)
	if err == nil || !strings.Contains(err.Error(), "not a git repository") {
		t.Fatalf("want refusal, got: %v", err)
	}
	if len(git.calls) != 0 {
		t.Errorf("must not touch git for a non-git dest, called: %v", git.calls)
	}
}

func TestSyncRefusesRemoteMismatch(t *testing.T) {
	home := t.TempDir()
	dest := gitDir(t, home)
	git := &fakeGit{resp: map[string][]gitResp{
		"-C " + dest + " remote get-url origin": {{out: "https://github.com/other/repo\n"}},
	}}

	_, err := Sync(home, Spec{Dest: "~/plug", Repo: repoURL}, git.run)
	if err == nil || !strings.Contains(err.Error(), "homie.toml declares") {
		t.Fatalf("want remote-mismatch error, got: %v", err)
	}
}

// TestSyncTrackSkipAndUpdate covers the unpinned update path: a pull
// that doesn't move HEAD is a skip, one that does is an update. The
// trailing .git on the configured remote must not count as a mismatch.
func TestSyncTrackSkipAndUpdate(t *testing.T) {
	for _, tc := range []struct {
		name       string
		heads      []gitResp // rev-parse HEAD responses, FIFO
		wantKind   Kind
		wantDetail string
	}{
		{"skip", []gitResp{{out: "aaaaaaaaaa"}, {out: "aaaaaaaaaa"}}, KindSkip, "up to date"},
		{"update", []gitResp{{out: "aaaaaaaaaa"}, {out: "bbbbbbbbbb"}}, KindUpdate, "aaaaaaa -> bbbbbbb"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home := t.TempDir()
			dest := gitDir(t, home)
			git := &fakeGit{resp: map[string][]gitResp{
				"-C " + dest + " remote get-url origin": {{out: repoURL + ".git"}},
				"-C " + dest + " rev-parse HEAD":        tc.heads,
			}}

			a, err := Sync(home, Spec{Dest: "~/plug", Repo: repoURL}, git.run)
			if err != nil {
				t.Fatal(err)
			}
			if a.Kind != tc.wantKind || a.Detail != tc.wantDetail {
				t.Errorf("action = %+v, want kind %s detail %q", a, tc.wantKind, tc.wantDetail)
			}
			if !git.called("pull --ff-only") {
				t.Errorf("expected a pull, calls: %v", git.calls)
			}
		})
	}
}

// TestSyncTrackReattachesDetachedHead: removing a pin leaves a detached
// HEAD behind; the next unpinned run re-attaches to the remote default
// branch before pulling.
func TestSyncTrackReattachesDetachedHead(t *testing.T) {
	home := t.TempDir()
	dest := gitDir(t, home)
	git := &fakeGit{resp: map[string][]gitResp{
		"-C " + dest + " remote get-url origin":              {{out: repoURL}},
		"-C " + dest + " rev-parse HEAD":                     {{out: "aaaaaaaaaa"}, {out: "bbbbbbbbbb"}},
		"-C " + dest + " symbolic-ref -q HEAD":               {{err: errors.New("exit 1")}},
		"-C " + dest + " rev-parse --abbrev-ref origin/HEAD": {{out: "origin/main"}},
	}}

	a, err := Sync(home, Spec{Dest: "~/plug", Repo: repoURL}, git.run)
	if err != nil {
		t.Fatal(err)
	}
	if a.Kind != KindUpdate {
		t.Errorf("action = %+v", a)
	}
	if !git.called("checkout main") {
		t.Errorf("expected re-attach to main, calls: %v", git.calls)
	}
}

// TestSyncPinned covers the pinned update path: converged means skip
// with no network, a changed pin checks out the new ref, and an
// unresolvable ref fetches before failing.
func TestSyncPinned(t *testing.T) {
	t.Run("already at ref skips without fetch", func(t *testing.T) {
		home := t.TempDir()
		dest := gitDir(t, home)
		git := &fakeGit{resp: map[string][]gitResp{
			"-C " + dest + " remote get-url origin":     {{out: repoURL}},
			"-C " + dest + " rev-parse HEAD":            {{out: "cccccccccc"}},
			"-C " + dest + " rev-parse v1.5.0^{commit}": {{out: "cccccccccc"}},
		}}

		a, err := Sync(home, Spec{Dest: "~/plug", Repo: repoURL, Ref: "v1.5.0"}, git.run)
		if err != nil {
			t.Fatal(err)
		}
		if a.Kind != KindSkip || a.Detail != "pinned at v1.5.0" {
			t.Errorf("action = %+v", a)
		}
		if git.called("fetch --tags origin") {
			t.Errorf("converged pin must not touch the network, calls: %v", git.calls)
		}
	})

	t.Run("moved pin checks out new ref", func(t *testing.T) {
		home := t.TempDir()
		dest := gitDir(t, home)
		git := &fakeGit{resp: map[string][]gitResp{
			"-C " + dest + " remote get-url origin":     {{out: repoURL}},
			"-C " + dest + " rev-parse HEAD":            {{out: "cccccccccc"}},
			"-C " + dest + " rev-parse v1.6.0^{commit}": {{out: "dddddddddd"}},
		}}

		a, err := Sync(home, Spec{Dest: "~/plug", Repo: repoURL, Ref: "v1.6.0"}, git.run)
		if err != nil {
			t.Fatal(err)
		}
		if a.Kind != KindUpdate || a.Detail != "ccccccc -> v1.6.0" {
			t.Errorf("action = %+v", a)
		}
		if !git.called("checkout --detach v1.6.0") {
			t.Errorf("expected detached checkout, calls: %v", git.calls)
		}
	})

	t.Run("unknown ref fetches then resolves", func(t *testing.T) {
		home := t.TempDir()
		dest := gitDir(t, home)
		git := &fakeGit{resp: map[string][]gitResp{
			"-C " + dest + " remote get-url origin": {{out: repoURL}},
			"-C " + dest + " rev-parse HEAD":        {{out: "cccccccccc"}},
			"-C " + dest + " rev-parse v2.0.0^{commit}": {
				{err: errors.New("unknown revision")},
				{out: "eeeeeeeeee"},
			},
		}}

		a, err := Sync(home, Spec{Dest: "~/plug", Repo: repoURL, Ref: "v2.0.0"}, git.run)
		if err != nil {
			t.Fatal(err)
		}
		if a.Kind != KindUpdate {
			t.Errorf("action = %+v", a)
		}
		if !git.called("fetch --tags origin") {
			t.Errorf("expected a fetch for the unknown ref, calls: %v", git.calls)
		}
	})

	t.Run("ref missing after fetch errors", func(t *testing.T) {
		home := t.TempDir()
		dest := gitDir(t, home)
		git := &fakeGit{resp: map[string][]gitResp{
			"-C " + dest + " remote get-url origin": {{out: repoURL}},
			"-C " + dest + " rev-parse HEAD":        {{out: "cccccccccc"}},
			"-C " + dest + " rev-parse vX^{commit}": {
				{err: errors.New("unknown revision")},
				{err: errors.New("unknown revision")},
			},
		}}

		_, err := Sync(home, Spec{Dest: "~/plug", Repo: repoURL, Ref: "vX"}, git.run)
		if err == nil || !strings.Contains(err.Error(), `ref "vX" not found`) {
			t.Fatalf("want ref-not-found error, got: %v", err)
		}
	})
}

// TestApplyCollectsErrorsAndContinues: a failing spec lands in Errors
// without blocking the rest.
func TestApplyCollectsErrorsAndContinues(t *testing.T) {
	home := t.TempDir()
	git := &fakeGit{}

	res := Apply(home, []Spec{
		{Dest: "relative/path", Repo: repoURL}, // invalid dest
		{Dest: "~/ok", Repo: repoURL},
	}, git.run)
	if len(res.Errors) != 1 || !strings.Contains(res.Errors[0].Error(), "must start with") {
		t.Errorf("errors = %v", res.Errors)
	}
	if len(res.Actions) != 1 || res.Actions[0].Kind != KindClone {
		t.Errorf("actions = %+v", res.Actions)
	}
}

func TestExpand(t *testing.T) {
	home := "/home/scout"
	for _, tc := range []struct {
		in, want string
		wantErr  bool
	}{
		{in: "~/.zsh/plug", want: "/home/scout/.zsh/plug"},
		{in: "$HOME/.zsh/plug", want: "/home/scout/.zsh/plug"},
		{in: "/opt/thing", want: "/opt/thing"},
		{in: ".zsh/plug", wantErr: true},
		{in: "~", wantErr: true},
	} {
		got, err := expand(tc.in, home)
		if tc.wantErr {
			if err == nil {
				t.Errorf("expand(%q) should error, got %q", tc.in, got)
			}
			continue
		}
		if err != nil || got != tc.want {
			t.Errorf("expand(%q) = %q, %v; want %q", tc.in, got, err, tc.want)
		}
	}
}
