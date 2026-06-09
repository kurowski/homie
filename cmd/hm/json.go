package main

import (
	"encoding/json"
	"io"
)

// writeJSON pretty-prints v to w with a trailing newline. Shared by the
// machine-readable outputs (`hm status --json`, `hm doctor --json`,
// `hm context`) so they all emit the same 2-space-indented style.
func writeJSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}
