package ui

import (
	"errors"
	"strings"
	"testing"
)

// The TUI's rendering is hard to test without spinning up a real
// program (terminal teardown semantics are tricky). The model itself
// is straightforward, though — Update/View are pure given the messages
// they receive, so we drive them directly.

func TestModelHandlesFullPhaseLifecycle(t *testing.T) {
	m := initialModel()

	step := func(msg interface{}) {
		t.Helper()
		next, _ := m.Update(msg)
		m = next.(tuiModel)
	}

	step(phaseStartMsg{name: "link"})
	step(actionMsg{kind: "create", target: "~/.zshrc"})
	step(actionMsg{kind: "skip", target: "3 already in sync"})
	step(phaseStartMsg{name: "render"})
	step(actionMsg{kind: "render", target: "~/.gitconfig"})
	step(warnMsg{text: "missing optional var"})
	step(streamMsg{text: "script said hi\n"})
	step(summaryMsg{errors: nil})

	view := m.View()
	mustContain(t, view, "link")
	mustContain(t, view, "~/.zshrc")
	mustContain(t, view, "render")
	mustContain(t, view, "~/.gitconfig")
	mustContain(t, view, "missing optional var")
	mustContain(t, view, "script said hi")
	mustContain(t, view, "All phases completed cleanly")

	// Earlier phase must be marked done (✓) once the next one started.
	if !strings.Contains(view, "✓") {
		t.Errorf("expected completed-phase checkmark in view:\n%s", view)
	}
}

func TestModelSummaryRendersErrors(t *testing.T) {
	m := initialModel()
	next, _ := m.Update(phaseStartMsg{name: "packages"})
	m = next.(tuiModel)
	next, _ = m.Update(summaryMsg{errors: []error{errors.New("apt-get exploded")}})
	m = next.(tuiModel)
	view := m.View()
	mustContain(t, view, "apt-get exploded")
	if strings.Contains(view, "All phases completed cleanly") {
		t.Errorf("summary with errors should not claim clean: %s", view)
	}
}

func TestModelActionsBeforePhaseAreSafe(t *testing.T) {
	// Defensive: if Action is called before Phase, the model shouldn't
	// panic with a nil deref. The action is dropped.
	m := initialModel()
	next, _ := m.Update(actionMsg{kind: "create", target: "/tmp/x"})
	m = next.(tuiModel)
	if len(m.phases) != 0 {
		t.Errorf("action without a phase shouldn't create one: %+v", m.phases)
	}
}
