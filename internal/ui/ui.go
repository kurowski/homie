// Package ui presents Homie's reconciliation phases to the user.
//
// There are two implementations: a plain line-oriented one suitable for
// CI logs and piped output, and a Bubble Tea TUI for interactive use.
// Pick the right one with New(); commands shouldn't care which they
// get. The interface is intentionally small so the two implementations
// can stay in sync without elaborate ceremony.
package ui

import "io"

// UI is the surface every Homie command writes through.
type UI interface {
	// Phase announces the start of a new reconciliation phase
	// (packages, link, render, scripts). Calling Phase again ends the
	// previous phase implicitly.
	Phase(name string)

	// Action records a single unit of work within the current phase —
	// kind is a short verb ("create", "replace", "install", "render",
	// "skip"), target is the path or item it acted on.
	Action(kind, target string)

	// Info adds a neutral note within the current phase.
	Info(msg string)

	// Warn adds a non-fatal warning within the current phase.
	Warn(msg string)

	// Summary closes out the run with the collected errors. Nil or
	// empty means the run was clean.
	Summary(errors []error)

	// Writer returns an io.Writer for streaming subprocess output
	// (currently used by the script runner). Plain UIs return the same
	// underlying writer they print to; the TUI sends each chunk into
	// its model so script output renders inline.
	Writer() io.Writer

	// Suspend hands the terminal back to the OS so a child process can own
	// stdin/stdout/stderr — used around interactive scripts (e.g. a sudo
	// password prompt) so the live TUI isn't fighting the script for the
	// terminal. Resume restores the UI afterward. Plain UIs don't hold the
	// terminal, so both are no-ops there. Safe to pair; best-effort.
	Suspend() error
	Resume() error

	// Close releases any resources held by the UI. Safe to call once.
	Close() error
}
