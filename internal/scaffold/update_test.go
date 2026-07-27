package scaffold

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// testVersion stands in for the ldflags-injected hm version.
const testVersion = "v9.9.9"

// bootstrapPath is the only tool-owned file today; the tests read it
// directly rather than through the manifest so a second tool-owned file
// doesn't silently change what they assert.
const bootstrapPath = "bootstrap.sh"

func seedRepo(t *testing.T, version string) string {
	t.Helper()
	dir := t.TempDir()
	if err := Run(dir, sampleAnswers, version); err != nil {
		t.Fatalf("Run: %v", err)
	}
	return dir
}

func readBootstrap(t *testing.T, dir string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(dir, bootstrapPath))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func only(t *testing.T, rs []Result) Result {
	t.Helper()
	if len(rs) != 1 {
		t.Fatalf("expected 1 tool-owned result, got %d: %+v", len(rs), rs)
	}
	return rs[0]
}

func TestRunStampsToolOwnedFilesOnly(t *testing.T) {
	dir := seedRepo(t, testVersion)

	p, ok := readProvenance([]byte(readBootstrap(t, dir)))
	if !ok {
		t.Fatalf("bootstrap.sh has no provenance stamp:\n%s", readBootstrap(t, dir))
	}
	if p.Version != testVersion {
		t.Errorf("stamped version = %q, want %q", p.Version, testVersion)
	}

	// The seeds are the user's from day one — stamping them would imply
	// Homie intends to come back and rewrite them.
	for _, seed := range []string{"homie.toml", "README.md", "home/.zshrc", "scripts/01-shell.sh"} {
		b, err := os.ReadFile(filepath.Join(dir, seed))
		if err != nil {
			t.Fatal(err)
		}
		if _, ok := readProvenance(b); ok {
			t.Errorf("%s carries a provenance stamp; only tool-owned files should", seed)
		}
	}
}

func TestUpdateIsNoOpOnAFreshRepo(t *testing.T) {
	dir := seedRepo(t, testVersion)
	before := readBootstrap(t, dir)

	got := only(t, mustUpdate(t, dir, testVersion, false))
	if got.State != StateCurrent {
		t.Errorf("state = %q, want %q", got.State, StateCurrent)
	}
	if after := readBootstrap(t, dir); after != before {
		t.Errorf("update rewrote an already-current file")
	}
}

func TestUpdateRefreshesAnUntouchedOlderGeneration(t *testing.T) {
	dir := seedRepo(t, "v0.1.0")
	// Simulate an older release whose template lacked a line we now ship.
	path := filepath.Join(dir, bootstrapPath)
	old := strings.Replace(readBootstrap(t, dir), "withtty hm bootstrap", "hm bootstrap", 1)
	writeStamped(t, path, old, "v0.1.0")

	got := only(t, mustUpdate(t, dir, testVersion, false))
	if got.State != StateUpdated {
		t.Fatalf("state = %q, want %q", got.State, StateUpdated)
	}
	if got.From != "v0.1.0" || got.To != testVersion {
		t.Errorf("From/To = %q/%q, want v0.1.0/%s", got.From, got.To, testVersion)
	}
	if body := readBootstrap(t, dir); !strings.Contains(body, "withtty hm bootstrap") {
		t.Errorf("update didn't bring the file current:\n%s", body)
	}
}

func TestUpdateSkipsLocallyEditedFiles(t *testing.T) {
	dir := seedRepo(t, "v0.1.0")
	path := filepath.Join(dir, bootstrapPath)
	edited := readBootstrap(t, dir) + "\n# my own tweak\n"
	if err := os.WriteFile(path, []byte(edited), 0o755); err != nil {
		t.Fatal(err)
	}

	got := only(t, mustUpdate(t, dir, testVersion, false))
	if got.State != StateCustomized || !got.Skipped() {
		t.Fatalf("state = %q, want %q", got.State, StateCustomized)
	}
	if got.From != "v0.1.0" {
		t.Errorf("From = %q, want the version that wrote it (v0.1.0)", got.From)
	}
	if readBootstrap(t, dir) != edited {
		t.Error("update overwrote a locally edited file without --force")
	}
	if !strings.Contains(string(got.Want), "withtty hm bootstrap") {
		t.Error("Want should carry the content update would have written, for the diff")
	}
}

// TestUpdateSkipsUnstampedFiles covers every repo scaffolded before
// stamps existed, plus anyone who deleted the line to opt out. Both mean
// "this file is the user's" and must not be clobbered.
func TestUpdateSkipsUnstampedFiles(t *testing.T) {
	dir := seedRepo(t, testVersion)
	path := filepath.Join(dir, bootstrapPath)
	unstamped := string(stripProvenance([]byte(readBootstrap(t, dir))))
	if err := os.WriteFile(path, []byte(unstamped), 0o755); err != nil {
		t.Fatal(err)
	}

	got := only(t, mustUpdate(t, dir, testVersion, false))
	if got.State != StateCustomized {
		t.Fatalf("state = %q, want %q", got.State, StateCustomized)
	}
	if got.From != "" {
		t.Errorf("From = %q, want empty for an unstamped file", got.From)
	}
	if readBootstrap(t, dir) != unstamped {
		t.Error("update overwrote an unstamped file without --force")
	}
}

func TestUpdateForceOverwritesEdits(t *testing.T) {
	dir := seedRepo(t, "v0.1.0")
	path := filepath.Join(dir, bootstrapPath)
	if err := os.WriteFile(path, []byte("# mine now\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	got := only(t, mustUpdate(t, dir, testVersion, true))
	if got.State != StateForced {
		t.Fatalf("state = %q, want %q", got.State, StateForced)
	}
	body := readBootstrap(t, dir)
	if strings.Contains(body, "mine now") || !strings.Contains(body, "withtty hm bootstrap") {
		t.Errorf("--force didn't take Homie's version:\n%s", body)
	}
}

func TestUpdateRecreatesADeletedFile(t *testing.T) {
	dir := seedRepo(t, testVersion)
	if err := os.Remove(filepath.Join(dir, bootstrapPath)); err != nil {
		t.Fatal(err)
	}

	got := only(t, mustUpdate(t, dir, testVersion, false))
	if got.State != StateCreated {
		t.Errorf("state = %q, want %q", got.State, StateCreated)
	}
	if info, err := os.Stat(filepath.Join(dir, bootstrapPath)); err != nil {
		t.Fatal(err)
	} else if info.Mode().Perm() != 0o755 {
		t.Errorf("recreated bootstrap.sh mode = %v, want 0755", info.Mode().Perm())
	}
}

// TestUpdateLeavesSeedsAlone is the load-bearing guarantee: a refresh
// must never touch the files the user owns.
func TestUpdateLeavesSeedsAlone(t *testing.T) {
	dir := seedRepo(t, "v0.1.0")
	seeds := map[string]string{
		"homie.toml":          "# hand-written since\n",
		"home/.zshrc":         "export EDITOR=nvim\n",
		"scripts/01-shell.sh": "#!/usr/bin/env bash\necho mine\n",
	}
	for rel, body := range seeds {
		if err := os.WriteFile(filepath.Join(dir, rel), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	mustUpdate(t, dir, testVersion, true) // even --force stays out of the seeds

	for rel, want := range seeds {
		got, err := os.ReadFile(filepath.Join(dir, rel))
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != want {
			t.Errorf("%s was modified by update:\ngot:  %q\nwant: %q", rel, got, want)
		}
	}
}

// TestUpdateIsIdempotent — a repeatable path has to be safe to run twice.
func TestUpdateIsIdempotent(t *testing.T) {
	dir := seedRepo(t, "v0.1.0")
	mustUpdate(t, dir, testVersion, false)
	first := readBootstrap(t, dir)

	got := only(t, mustUpdate(t, dir, testVersion, false))
	if got.State != StateCurrent {
		t.Errorf("second run state = %q, want %q", got.State, StateCurrent)
	}
	if readBootstrap(t, dir) != first {
		t.Error("second update changed the file again")
	}
}

func TestStampRoundTrip(t *testing.T) {
	body := []byte("#!/usr/bin/env bash\necho hi\n")
	stamped := stampProvenance(body, testVersion)

	if !strings.HasPrefix(string(stamped), "#!/usr/bin/env bash\n"+provenancePrefix) {
		t.Errorf("stamp should sit directly under the shebang:\n%s", stamped)
	}
	if !unchangedSince(stamped) {
		t.Error("a freshly stamped file should verify as unchanged")
	}
	if string(stripProvenance(stamped)) != string(body) {
		t.Errorf("strip didn't restore the original:\n%q", stripProvenance(stamped))
	}
	// Re-stamping replaces rather than accumulates.
	again := stampProvenance(stamped, "v1.0.0")
	if n := strings.Count(string(again), provenancePrefix); n != 1 {
		t.Errorf("stamp count = %d, want 1:\n%s", n, again)
	}
	if p, _ := readProvenance(again); p.Version != "v1.0.0" {
		t.Errorf("re-stamp kept the old version %q", p.Version)
	}
	// An edit anywhere else has to break verification.
	if unchangedSince([]byte(string(stamped) + "tampered\n")) {
		t.Error("edited content still verified as unchanged")
	}
}

func mustUpdate(t *testing.T, dir, version string, force bool) []Result {
	t.Helper()
	rs, err := Update(dir, sampleAnswers, version, force)
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	return rs
}

// writeStamped writes body with a stamp claiming version, i.e. what an
// older hm would have left behind.
func writeStamped(t *testing.T, path, body, version string) {
	t.Helper()
	if err := os.WriteFile(path, stampProvenance([]byte(body), version), 0o755); err != nil {
		t.Fatal(err)
	}
}
