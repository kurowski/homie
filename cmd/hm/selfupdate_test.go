package main

import (
	"bytes"
	"strings"
	"testing"
)

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
		buf := new(bytes.Buffer)
		rootCmd.SetOut(buf)
		rootCmd.SetErr(buf)
		rootCmd.SetArgs(tc.args)
		err := rootCmd.Execute()
		if err == nil {
			t.Errorf("%v with version %q did not refuse", tc.args, tc.version)
			continue
		}
		if !strings.Contains(err.Error(), "built from source") {
			t.Errorf("%v with version %q: error %q does not explain the source-build refusal", tc.args, tc.version, err)
		}
	}
}
