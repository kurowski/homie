package ui

import (
	"fmt"
	"io"
	"strings"
	"sync"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// TUI renders apply phases as a live-updating Bubble Tea program. Each
// phase shows a spinner while running and collapses to a checkmark
// header with its action lines underneath once the next phase begins.
//
// The program runs in its own goroutine; UI methods send messages into
// it. Close() blocks until the program exits cleanly, which is also
// what restores the terminal cursor.
type TUI struct {
	prog   *tea.Program
	out    io.Writer
	mu     sync.Mutex
	closed bool
	done   chan struct{}
}

// NewTUI starts a Bubble Tea program writing to out and returns a UI
// that posts events into it.
func NewTUI(out io.Writer) *TUI {
	m := initialModel()
	prog := tea.NewProgram(m, tea.WithOutput(out))
	t := &TUI{prog: prog, out: out, done: make(chan struct{})}
	go func() {
		_, _ = prog.Run()
		close(t.done)
	}()
	return t
}

// Phase sends a phaseStartMsg.
func (t *TUI) Phase(name string) { t.prog.Send(phaseStartMsg{name: name}) }

// Action sends an actionMsg.
func (t *TUI) Action(kind, target string) {
	t.prog.Send(actionMsg{kind: kind, target: target})
}

// Info sends an infoMsg.
func (t *TUI) Info(msg string) { t.prog.Send(infoMsg{text: msg}) }

// Warn sends a warnMsg.
func (t *TUI) Warn(msg string) { t.prog.Send(warnMsg{text: msg}) }

// Summary sends a summaryMsg, which also tells the program to quit
// once it has rendered. Close() waits for that.
func (t *TUI) Summary(errs []error) { t.prog.Send(summaryMsg{errors: errs}) }

// Writer returns an io.Writer that streams chunks into the program as
// streamMsg events. Used by the script runner so output appears under
// the active phase.
func (t *TUI) Writer() io.Writer { return &streamWriter{prog: t.prog} }

// Suspend releases the terminal so a child process (an interactive script)
// can own stdin/stdout/stderr; the Bubble Tea program stops reading input
// and restores cooked mode until Resume. Resume re-takes the terminal and
// redraws. Both are best-effort — a failure to hand off shouldn't abort the
// run.
func (t *TUI) Suspend() error { return t.prog.ReleaseTerminal() }
func (t *TUI) Resume() error  { return t.prog.RestoreTerminal() }

// Close blocks until the Bubble Tea program exits. Idempotent.
func (t *TUI) Close() error {
	t.mu.Lock()
	if t.closed {
		t.mu.Unlock()
		return nil
	}
	t.closed = true
	t.mu.Unlock()

	// If Summary was never called, force a quit so we don't hang.
	t.prog.Send(forceQuitMsg{})
	<-t.done
	return nil
}

// streamWriter pipes subprocess output into the bubbletea model.
type streamWriter struct{ prog *tea.Program }

func (w *streamWriter) Write(p []byte) (int, error) {
	w.prog.Send(streamMsg{text: string(p)})
	return len(p), nil
}

// --- bubbletea model ----------------------------------------------------

type phaseStartMsg struct{ name string }
type actionMsg struct{ kind, target string }
type infoMsg struct{ text string }
type warnMsg struct{ text string }
type streamMsg struct{ text string }
type summaryMsg struct{ errors []error }
type forceQuitMsg struct{}

type phase struct {
	name    string
	actions []actionLine
	notes   []note
	stream  []string
	done    bool
}

type actionLine struct{ kind, target string }
type note struct {
	level string // "info", "warn"
	text  string
}

type tuiModel struct {
	phases  []*phase
	spinner spinner.Model
	summary *summaryMsg
}

func initialModel() tuiModel {
	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("205"))
	return tuiModel{spinner: sp}
}

func (m tuiModel) Init() tea.Cmd { return m.spinner.Tick }

func (m tuiModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case phaseStartMsg:
		if len(m.phases) > 0 {
			m.phases[len(m.phases)-1].done = true
		}
		m.phases = append(m.phases, &phase{name: msg.name})
		return m, nil

	case actionMsg:
		if p := m.currentPhase(); p != nil {
			p.actions = append(p.actions, actionLine(msg))
		}
		return m, nil

	case infoMsg:
		if p := m.currentPhase(); p != nil {
			p.notes = append(p.notes, note{level: "info", text: msg.text})
		}
		return m, nil

	case warnMsg:
		if p := m.currentPhase(); p != nil {
			p.notes = append(p.notes, note{level: "warn", text: msg.text})
		}
		return m, nil

	case streamMsg:
		if p := m.currentPhase(); p != nil {
			p.stream = append(p.stream, msg.text)
		}
		return m, nil

	case summaryMsg:
		if p := m.currentPhase(); p != nil {
			p.done = true
		}
		m.summary = &msg
		return m, tea.Quit

	case forceQuitMsg:
		return m, tea.Quit

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m tuiModel) currentPhase() *phase {
	if len(m.phases) == 0 {
		return nil
	}
	return m.phases[len(m.phases)-1]
}

var (
	headerDone   = lipgloss.NewStyle().Foreground(lipgloss.Color("42")).Bold(true)
	headerActive = lipgloss.NewStyle().Foreground(lipgloss.Color("205")).Bold(true)
	kindStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	warnStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
	errorStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("203"))
	streamStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("244")).Faint(true)
	okStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("42")).Bold(true)
)

func (m tuiModel) View() string {
	var sb strings.Builder
	for _, p := range m.phases {
		if p.done {
			sb.WriteString(headerDone.Render("✓ "+p.name) + "\n")
		} else {
			sb.WriteString(m.spinner.View() + " " + headerActive.Render(p.name) + "\n")
		}
		for _, a := range p.actions {
			sb.WriteString("    " + kindStyle.Render(fmt.Sprintf("%-8s", a.kind)) + " " + a.target + "\n")
		}
		for _, n := range p.notes {
			style := kindStyle
			if n.level == "warn" {
				style = warnStyle
			}
			sb.WriteString("    " + style.Render(n.text) + "\n")
		}
		for _, s := range p.stream {
			trimmed := strings.TrimRight(s, "\n")
			if trimmed == "" {
				continue
			}
			for _, line := range strings.Split(trimmed, "\n") {
				sb.WriteString("    " + streamStyle.Render(line) + "\n")
			}
		}
	}
	if m.summary != nil {
		sb.WriteString("\n")
		if len(m.summary.errors) == 0 {
			sb.WriteString(okStyle.Render("All phases completed cleanly.") + "\n")
		} else {
			for _, e := range m.summary.errors {
				sb.WriteString(errorStyle.Render("  error    "+e.Error()) + "\n")
			}
		}
	}
	return sb.String()
}
