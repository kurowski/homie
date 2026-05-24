package packages

import (
	"errors"
	"strings"
	"testing"

	"github.com/kurowski/homie/internal/detect"
)

// fakeRunner records every call and returns canned output for specific
// commands. Anything unmatched returns ("", nil) (success, no output).
type fakeRunner struct {
	calls   []call
	dpkgOK  map[string]bool // pkg -> Status: installed?
	rpmOK   map[string]bool
	failCmd string // if set, return error when arg[0]+args matches
}

type call struct {
	name string
	args []string
}

func (f *fakeRunner) run(name string, args ...string) ([]byte, error) {
	f.calls = append(f.calls, call{name: name, args: append([]string(nil), args...)})
	switch {
	case name == "dpkg" && len(args) == 2 && args[0] == "-s":
		if f.dpkgOK[args[1]] {
			return []byte("Status: install ok installed\n"), nil
		}
		return []byte("dpkg: package " + args[1] + " is not installed"), errors.New("exit 1")
	case name == "rpm" && len(args) == 2 && args[0] == "-q":
		if f.rpmOK[args[1]] {
			return []byte(args[1] + "-1.0\n"), nil
		}
		return []byte("package " + args[1] + " is not installed"), errors.New("exit 1")
	}
	if f.failCmd != "" && strings.HasPrefix(name+" "+strings.Join(args, " "), f.failCmd) {
		return []byte("boom"), errors.New("exit 1")
	}
	return nil, nil
}

func TestAptIsInstalled(t *testing.T) {
	f := &fakeRunner{dpkgOK: map[string]bool{"git": true}}
	a := &Apt{Runner: f.run}
	if !a.IsInstalled("git") {
		t.Errorf("git should report installed")
	}
	if a.IsInstalled("nope") {
		t.Errorf("nope should report not installed")
	}
}

func TestAptInstallFiltersInstalled(t *testing.T) {
	f := &fakeRunner{dpkgOK: map[string]bool{"git": true}}
	a := &Apt{Runner: f.run, Sudo: false}
	if err := a.Install([]string{"git", "zsh", "neovim"}); err != nil {
		t.Fatalf("Install: %v", err)
	}
	last := f.calls[len(f.calls)-1]
	if last.name != "apt-get" {
		t.Fatalf("expected apt-get, got %s", last.name)
	}
	args := strings.Join(last.args, " ")
	if !strings.Contains(args, "install -y zsh neovim") {
		t.Errorf("install args = %q, want only zsh and neovim", args)
	}
	if strings.Contains(args, "git") {
		t.Errorf("git was already installed, must not be in install args: %q", args)
	}
}

func TestAptInstallNoopWhenAllInstalled(t *testing.T) {
	f := &fakeRunner{dpkgOK: map[string]bool{"git": true, "zsh": true}}
	a := &Apt{Runner: f.run}
	if err := a.Install([]string{"git", "zsh"}); err != nil {
		t.Fatalf("Install: %v", err)
	}
	for _, c := range f.calls {
		if c.name == "apt-get" {
			t.Errorf("apt-get should not have been invoked: calls=%+v", f.calls)
		}
	}
}

func TestAptSudoPrefixWhenNotRoot(t *testing.T) {
	f := &fakeRunner{}
	a := &Apt{Runner: f.run, Sudo: true}
	if err := a.Install([]string{"zsh"}); err != nil {
		t.Fatalf("Install: %v", err)
	}
	last := f.calls[len(f.calls)-1]
	if last.name != "sudo" || last.args[0] != "apt-get" {
		t.Errorf("expected `sudo apt-get ...`, got %s %v", last.name, last.args)
	}
}

func TestDnfIsInstalled(t *testing.T) {
	f := &fakeRunner{rpmOK: map[string]bool{"git": true}}
	d := &Dnf{Runner: f.run}
	if !d.IsInstalled("git") {
		t.Errorf("git should report installed")
	}
	if d.IsInstalled("nope") {
		t.Errorf("nope should report not installed")
	}
}

func TestDnfInstallSudo(t *testing.T) {
	f := &fakeRunner{rpmOK: map[string]bool{"git": true}}
	d := &Dnf{Runner: f.run, Sudo: true}
	if err := d.Install([]string{"git", "tmux"}); err != nil {
		t.Fatalf("Install: %v", err)
	}
	last := f.calls[len(f.calls)-1]
	if last.name != "sudo" || last.args[0] != "dnf" {
		t.Errorf("expected `sudo dnf ...`, got %s %v", last.name, last.args)
	}
	args := strings.Join(last.args, " ")
	if !strings.Contains(args, "install -y tmux") {
		t.Errorf("install args = %q, want tmux", args)
	}
	if strings.Contains(args, "git") {
		t.Errorf("git was installed; must not appear: %q", args)
	}
}

func TestForPicksBackend(t *testing.T) {
	cases := []struct {
		env  detect.Env
		want string
	}{
		{detect.Env{PackageManager: "apt", IsRoot: true}, "apt"},
		{detect.Env{PackageManager: "dnf", IsRoot: false}, "dnf"},
		{detect.Env{PackageManager: "unknown", Distro: "arch"}, "noop"},
		{detect.Env{}, "noop"},
	}
	for _, tc := range cases {
		got := For(tc.env).Name()
		if got != tc.want {
			t.Errorf("For(%+v).Name() = %q, want %q", tc.env, got, tc.want)
		}
	}
}

func TestForRespectsRoot(t *testing.T) {
	asRoot := For(detect.Env{PackageManager: "apt", IsRoot: true}).(*Apt)
	if asRoot.Sudo {
		t.Errorf("root should not use sudo")
	}
	asUser := For(detect.Env{PackageManager: "apt", IsRoot: false}).(*Apt)
	if !asUser.Sudo {
		t.Errorf("non-root must use sudo")
	}
}

func TestNoop(t *testing.T) {
	n := &Noop{Distro: "arch"}
	if !n.IsAvailable() {
		t.Errorf("Noop.IsAvailable should be true so callers don't fail")
	}
	if n.IsInstalled("git") {
		t.Errorf("Noop.IsInstalled must report false — we cannot verify")
	}
	if err := n.Install([]string{"git", "zsh"}); err != nil {
		t.Errorf("Noop.Install should be a no-op, got %v", err)
	}
}
