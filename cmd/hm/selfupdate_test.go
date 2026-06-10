package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kurowski/homie/internal/selfupdate"
)

// execRoot runs rootCmd with args and captured output, restoring the
// command's out/err/args state afterward so nothing leaks into other
// tests.
func execRoot(t *testing.T, args []string) (string, error) {
	t.Helper()
	t.Cleanup(func() {
		rootCmd.SetOut(nil)
		rootCmd.SetErr(nil)
		rootCmd.SetArgs(nil)
	})
	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetErr(buf)
	rootCmd.SetArgs(args)
	err := rootCmd.Execute()
	return buf.String(), err
}

// TestSelfupdateRefusesNonReleaseBuild covers the guard that keeps a
// local build from being clobbered: any version that isn't a plain
// release tag — `go build` ("dev") or `make build` (git describe) —
// must refuse before touching the network. Both spellings of the
// command resolve to the same guard.
func TestSelfupdateRefusesNonReleaseBuild(t *testing.T) {
	old := version
	t.Cleanup(func() { version = old })

	for _, tc := range []struct {
		version string
		args    []string
	}{
		{"dev", []string{"selfupdate"}},
		{"v0.4.0-1-g64ad4cc-dirty", []string{"selfupdate"}},
		{"dev", []string{"self-update"}},
	} {
		version = tc.version
		_, err := execRoot(t, tc.args)
		if err == nil {
			t.Errorf("%v with version %q did not refuse", tc.args, tc.version)
			continue
		}
		if !strings.Contains(err.Error(), "built from source") {
			t.Errorf("%v with version %q: error %q does not explain the source-build refusal", tc.args, tc.version, err)
		}
	}
}

// TestSelfupdateEndToEnd runs the whole command against a fake release
// server: resolve the latest tag from the redirect, download SHA256SUMS
// and the binary, verify, and swap a scratch file standing in for the
// running executable.
func TestSelfupdateEndToEnd(t *testing.T) {
	bin := []byte("new hm binary")
	asset := selfupdate.AssetName()
	sum := sha256.Sum256(bin)
	sums := fmt.Sprintf("%s  %s\n", hex.EncodeToString(sum[:]), asset)

	mux := http.NewServeMux()
	mux.HandleFunc("/releases/latest", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/releases/tag/v9.9.9", http.StatusFound)
	})
	mux.HandleFunc("/releases/download/v9.9.9/SHA256SUMS", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(sums))
	})
	mux.HandleFunc("/releases/download/v9.9.9/"+asset, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(bin)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	target := filepath.Join(t.TempDir(), "hm")
	if err := os.WriteFile(target, []byte("old hm binary"), 0o755); err != nil {
		t.Fatal(err)
	}

	oldVersion, oldNew, oldExe := version, newUpdater, executablePath
	t.Cleanup(func() { version, newUpdater, executablePath = oldVersion, oldNew, oldExe })
	version = "v0.0.1"
	newUpdater = func() *selfupdate.Updater {
		return &selfupdate.Updater{BaseURL: srv.URL, Client: srv.Client()}
	}
	executablePath = func() (string, error) { return target, nil }

	out, err := execRoot(t, []string{"selfupdate"})
	if err != nil {
		t.Fatalf("selfupdate: %v\n%s", err, out)
	}
	if !strings.Contains(out, "Updated v0.0.1 to v9.9.9.") {
		t.Errorf("output does not report the update:\n%s", out)
	}
	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(bin) {
		t.Errorf("target content = %q, want the downloaded binary", got)
	}
}
