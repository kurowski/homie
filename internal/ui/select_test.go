package ui

import (
	"bytes"
	"testing"
)

func TestNewNoTTYReturnsPlain(t *testing.T) {
	u := New(new(bytes.Buffer), true)
	if _, ok := u.(*Plain); !ok {
		t.Errorf("--no-tty should pick Plain, got %T", u)
	}
}

func TestNewNonTerminalReturnsPlain(t *testing.T) {
	// A bytes.Buffer isn't a *os.File, so the terminal check fails and
	// we should fall back to Plain. This is what happens when stdout
	// is piped.
	u := New(new(bytes.Buffer), false)
	if _, ok := u.(*Plain); !ok {
		t.Errorf("non-terminal writer should pick Plain, got %T", u)
	}
}
