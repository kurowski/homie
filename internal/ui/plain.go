package ui

import (
	"fmt"
	"io"
)

// Plain prints one line per event with no ANSI styling. It's what `hm`
// uses when stdout isn't a terminal or when --no-tty is set, so log
// capture stays readable.
type Plain struct {
	out io.Writer
}

// NewPlain returns a Plain UI writing to out.
func NewPlain(out io.Writer) *Plain {
	return &Plain{out: out}
}

// Phase writes a section header like "== link ==".
func (p *Plain) Phase(name string) {
	fmt.Fprintf(p.out, "\n== %s ==\n", name)
}

// Action writes "  <kind>  <target>" with kind padded for alignment.
func (p *Plain) Action(kind, target string) {
	fmt.Fprintf(p.out, "  %-8s %s\n", kind, target)
}

// Info writes "  <msg>".
func (p *Plain) Info(msg string) {
	fmt.Fprintf(p.out, "  %s\n", msg)
}

// Warn writes "  warning  <msg>".
func (p *Plain) Warn(msg string) {
	fmt.Fprintf(p.out, "  warning  %s\n", msg)
}

// Summary writes a closing section. Clean runs say so; failed ones
// list each collected error.
func (p *Plain) Summary(errs []error) {
	fmt.Fprintln(p.out, "\n== summary ==")
	if len(errs) == 0 {
		fmt.Fprintln(p.out, "  All phases completed cleanly.")
		return
	}
	for _, e := range errs {
		fmt.Fprintf(p.out, "  error    %s\n", e)
	}
}

// Writer returns the same writer Plain prints to, so script output
// interleaves with the phase lines in the natural order.
func (p *Plain) Writer() io.Writer {
	return p.out
}

// Close is a no-op for Plain.
func (p *Plain) Close() error { return nil }
