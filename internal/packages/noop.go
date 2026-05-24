package packages

// Noop is the Manager returned when we don't recognize the host's
// package manager. It silently no-ops Install so `hm apply` can still
// finish — the caller is expected to surface the unsupported-distro
// warning to the user via the UI.
//
// TODO(contrib): replace Noop with a real backend for additional
// distros. See homie.sh/contributing for the checklist.
type Noop struct {
	Distro string
}

// Name returns "noop".
func (n *Noop) Name() string { return "noop" }

// IsAvailable always returns true so callers don't treat the absence of
// a real package manager as an error condition.
func (n *Noop) IsAvailable() bool { return true }

// IsInstalled conservatively returns false so any caller that branches
// on it won't claim a package is present that we can't verify.
func (n *Noop) IsInstalled(string) bool { return false }

// Install is a no-op. The unsupported-distro warning is the caller's
// responsibility.
func (n *Noop) Install([]string) error { return nil }
