package ui

import (
	"io"
	"os"

	"golang.org/x/term"
)

// New returns the appropriate UI for the current process. The TUI is
// only used when stdout is an interactive terminal AND noTTY is false.
// Anything else (pipes, CI, --no-tty) gets the Plain UI so logs stay
// clean.
func New(out io.Writer, noTTY bool) UI {
	if noTTY {
		return NewPlain(out)
	}
	if f, ok := out.(*os.File); ok && term.IsTerminal(int(f.Fd())) {
		return NewTUI(out)
	}
	return NewPlain(out)
}
