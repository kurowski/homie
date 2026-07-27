package scaffold

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
)

// Provenance is the stamp `hm init` leaves on tool-owned files — the
// ones Homie keeps current across releases (today just bootstrap.sh)
// rather than handing over to the user on day one.
//
// The stamp answers the only question `hm init --update` needs to ask:
// is this file still exactly what some hm wrote, or has the user edited
// it since? Recording the digest rather than just the version makes that
// check version-agnostic — we can tell an untouched v0.1.0 bootstrap.sh
// from an edited one without carrying every old template in the binary.
//
// Deleting the stamp line is a supported way to opt out: an unstamped
// file reads as "yours now", and update leaves it alone unless forced.
type Provenance struct {
	Version string // hm version that generated the file ("dev" for local builds)
	Digest  string // sha256 of the file with the stamp line removed
}

// provenancePrefix marks the stamp line. It's a shell comment because
// every tool-owned file is a script; if a non-script ever joins the
// manifest this needs a per-entry comment syntax.
const provenancePrefix = "# hm:generated "

// digestOf hashes body with any stamp line removed, so the digest of a
// stamped file and of the same file pre-stamp are identical. Callers can
// pass either form.
func digestOf(body []byte) string {
	sum := sha256.Sum256(stripProvenance(body))
	return hex.EncodeToString(sum[:])
}

// stripProvenance returns body without its stamp line.
func stripProvenance(body []byte) []byte {
	lines := strings.Split(string(body), "\n")
	out := lines[:0]
	for _, l := range lines {
		if strings.HasPrefix(l, provenancePrefix) {
			continue
		}
		out = append(out, l)
	}
	return []byte(strings.Join(out, "\n"))
}

// stampProvenance returns body with a stamp line for version, replacing
// any stamp already there. The line goes directly under the shebang so
// it's the first thing a reader sees without displacing `#!`.
func stampProvenance(body []byte, version string) []byte {
	clean := stripProvenance(body)
	stamp := fmt.Sprintf("%sversion=%s sha256=%s", provenancePrefix, version, digestOf(clean))

	lines := strings.Split(string(clean), "\n")
	at := 0
	if len(lines) > 0 && strings.HasPrefix(lines[0], "#!") {
		at = 1
	}
	out := make([]string, 0, len(lines)+1)
	out = append(out, lines[:at]...)
	out = append(out, stamp)
	out = append(out, lines[at:]...)
	return []byte(strings.Join(out, "\n"))
}

// readProvenance extracts the stamp from body. ok is false when there
// isn't one — either a file Homie never wrote, or one whose owner
// deliberately removed the line.
func readProvenance(body []byte) (p Provenance, ok bool) {
	for _, l := range strings.Split(string(body), "\n") {
		if !strings.HasPrefix(l, provenancePrefix) {
			continue
		}
		for _, f := range strings.Fields(strings.TrimPrefix(l, provenancePrefix)) {
			k, v, found := strings.Cut(f, "=")
			if !found {
				continue
			}
			switch k {
			case "version":
				p.Version = v
			case "sha256":
				p.Digest = v
			}
		}
		return p, p.Version != "" && p.Digest != ""
	}
	return Provenance{}, false
}

// unchangedSince reports whether body is byte-identical to what hm wrote,
// per its own stamp.
func unchangedSince(body []byte) bool {
	p, ok := readProvenance(body)
	return ok && p.Digest == digestOf(body)
}
