package ui

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

func TestPlainPhaseAndActions(t *testing.T) {
	buf := new(bytes.Buffer)
	u := NewPlain(buf)
	u.Phase("link")
	u.Action("create", "~/.zshrc")
	u.Action("backup", "~/.gitconfig -> ~/.gitconfig.homie-backup-...")
	u.Info("3 already in sync")
	u.Phase("render")
	u.Warn("missing optional var EDITOR")
	u.Summary(nil)

	got := buf.String()
	mustContain(t, got, "== link ==")
	mustContain(t, got, "  create   ~/.zshrc")
	mustContain(t, got, "  backup   ~/.gitconfig")
	mustContain(t, got, "  3 already in sync")
	mustContain(t, got, "== render ==")
	mustContain(t, got, "  warning  missing optional var EDITOR")
	mustContain(t, got, "== summary ==")
	mustContain(t, got, "All phases completed cleanly")
}

func TestPlainSummaryWithErrors(t *testing.T) {
	buf := new(bytes.Buffer)
	u := NewPlain(buf)
	u.Phase("packages")
	u.Summary([]error{errors.New("apt-get failed"), errors.New("rpm broken")})

	got := buf.String()
	if strings.Contains(got, "All phases completed cleanly") {
		t.Errorf("error summary should not say clean: %s", got)
	}
	mustContain(t, got, "error    apt-get failed")
	mustContain(t, got, "error    rpm broken")
}

func TestPlainWriterIsSameOut(t *testing.T) {
	buf := new(bytes.Buffer)
	u := NewPlain(buf)
	if _, err := u.Writer().Write([]byte("script said hi\n")); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "script said hi") {
		t.Errorf("script output should flow through Writer to the same buffer: %q", buf.String())
	}
}

func TestPlainCloseIsNoop(t *testing.T) {
	u := NewPlain(new(bytes.Buffer))
	if err := u.Close(); err != nil {
		t.Errorf("Close should be no-op: %v", err)
	}
}

func mustContain(t *testing.T, s, sub string) {
	t.Helper()
	if !strings.Contains(s, sub) {
		t.Errorf("output missing %q\n--- full output ---\n%s", sub, s)
	}
}
