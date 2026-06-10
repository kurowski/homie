package selfupdate

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// newServer fakes the two GitHub endpoints the updater touches: the
// /releases/latest redirect and the per-tag asset downloads.
func newServer(t *testing.T, tag string, assets map[string][]byte) *Updater {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/releases/latest", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/releases/tag/"+tag, http.StatusFound)
	})
	for name, data := range assets {
		mux.HandleFunc("/releases/download/"+tag+"/"+name, func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write(data)
		})
	}
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return &Updater{BaseURL: srv.URL, Client: srv.Client()}
}

// sumsFor builds a SHA256SUMS body covering the given assets, in the
// same "<hex>  <name>" format the release workflow's sha256sum emits.
func sumsFor(assets map[string][]byte) []byte {
	var b strings.Builder
	for name, data := range assets {
		sum := sha256.Sum256(data)
		fmt.Fprintf(&b, "%s  %s\n", hex.EncodeToString(sum[:]), name)
	}
	return []byte(b.String())
}

func TestLatest(t *testing.T) {
	u := newServer(t, "v1.2.3", nil)
	tag, err := u.Latest()
	if err != nil {
		t.Fatalf("Latest: %v", err)
	}
	if tag != "v1.2.3" {
		t.Errorf("Latest = %q, want v1.2.3", tag)
	}
}

func TestLatestNoRedirect(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	t.Cleanup(srv.Close)
	u := &Updater{BaseURL: srv.URL, Client: srv.Client()}
	if _, err := u.Latest(); err == nil {
		t.Error("Latest succeeded against a server with no releases")
	}
}

func TestLatestRedirectWithoutTag(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/releases/latest", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/releases", http.StatusFound)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	u := &Updater{BaseURL: srv.URL, Client: srv.Client()}
	if _, err := u.Latest(); err == nil {
		t.Error("Latest accepted a redirect that carries no release tag")
	}
}

func TestFetch(t *testing.T) {
	bin := []byte("fake hm binary")
	assets := map[string][]byte{
		AssetName():     bin,
		"hm-other-arch": []byte("some other binary"),
	}
	assets["SHA256SUMS"] = sumsFor(assets)
	u := newServer(t, "v1.2.3", assets)

	got, err := u.Fetch("v1.2.3")
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if string(got) != string(bin) {
		t.Errorf("Fetch returned %q, want %q", got, bin)
	}
}

func TestFetchChecksumMismatch(t *testing.T) {
	assets := map[string][]byte{AssetName(): []byte("fake hm binary")}
	assets["SHA256SUMS"] = sumsFor(map[string][]byte{AssetName(): []byte("different bytes")})
	u := newServer(t, "v1.2.3", assets)

	if _, err := u.Fetch("v1.2.3"); err == nil || !strings.Contains(err.Error(), "checksum mismatch") {
		t.Errorf("Fetch = %v, want checksum mismatch", err)
	}
}

func TestFetchMissingChecksumEntry(t *testing.T) {
	assets := map[string][]byte{
		AssetName():  []byte("fake hm binary"),
		"SHA256SUMS": sumsFor(map[string][]byte{"hm-other-arch": []byte("x")}),
	}
	u := newServer(t, "v1.2.3", assets)

	if _, err := u.Fetch("v1.2.3"); err == nil || !strings.Contains(err.Error(), "no entry") {
		t.Errorf("Fetch = %v, want missing-entry error", err)
	}
}

func TestApplyReplacesAndPreservesMode(t *testing.T) {
	target := filepath.Join(t.TempDir(), "hm")
	if err := os.WriteFile(target, []byte("old"), 0o700); err != nil {
		t.Fatal(err)
	}

	if err := Apply(target, []byte("new")); err != nil {
		t.Fatalf("Apply: %v", err)
	}

	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "new" {
		t.Errorf("target content = %q, want %q", got, "new")
	}
	fi, err := os.Stat(target)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm() != 0o700 {
		t.Errorf("target mode = %v, want 0700", fi.Mode().Perm())
	}
	leftovers, _ := filepath.Glob(filepath.Join(filepath.Dir(target), ".hm-selfupdate-*"))
	if len(leftovers) > 0 {
		t.Errorf("temp files left behind: %v", leftovers)
	}
}

func TestIsReleaseVersion(t *testing.T) {
	for v, want := range map[string]bool{
		"v0.4.0":                 true,
		"v12.34.56":              true,
		"dev":                    false,
		"(devel)":                false,
		"0.4.0":                  false,
		"v0.4.0-1-g64ad4cc":      false,
		"v0.4.0-1-g64ad4cc-dirty": false,
	} {
		if got := IsReleaseVersion(v); got != want {
			t.Errorf("IsReleaseVersion(%q) = %v, want %v", v, got, want)
		}
	}
}

func TestBrewManaged(t *testing.T) {
	for p, want := range map[string]bool{
		"/opt/homebrew/Cellar/hm/0.4.0/bin/hm":          true,
		"/usr/local/Cellar/hm/0.4.0/bin/hm":             true,
		"/home/linuxbrew/.linuxbrew/Cellar/hm/0.4.0/bin/hm": true,
		"/usr/local/bin/hm":                             false,
		"/home/scout/.local/bin/hm":                     false,
	} {
		if got := BrewManaged(p); got != want {
			t.Errorf("BrewManaged(%q) = %v, want %v", p, got, want)
		}
	}
}
