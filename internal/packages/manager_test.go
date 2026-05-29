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
	calls     []call
	dpkgOK    map[string]bool // pkg -> Status: installed?
	rpmOK     map[string]bool
	flatpakOK map[string]bool // ref -> appears in `flatpak list` output
	brewOK    map[string]bool // formula -> appears in `brew list --formula` output
	caskOK    map[string]bool // cask -> appears in `brew list --cask` output
	snapOK    map[string]bool // snap name -> appears in `snap list` output
	failCmd   string          // if set, return error when arg[0]+args matches
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
	case name == "flatpak" && len(args) >= 1 && args[0] == "list":
		var b strings.Builder
		for ref := range f.flatpakOK {
			b.WriteString(ref)
			b.WriteByte('\n')
		}
		return []byte(b.String()), nil
	case name == "brew" && len(args) >= 2 && args[0] == "list" && args[1] == "--formula":
		// Bulk listing form: `brew list --formula -1` — dump every
		// installed formula, one per line. The cached IsInstalled relies
		// on this form rather than per-package shellouts.
		var b strings.Builder
		for formula := range f.brewOK {
			b.WriteString(formula)
			b.WriteByte('\n')
		}
		return []byte(b.String()), nil
	case name == "brew" && len(args) >= 2 && args[0] == "list" && args[1] == "--cask":
		// `brew list --cask -1` — the cask analog, loaded lazily and only
		// when a "/cask" spec is looked up.
		var b strings.Builder
		for cask := range f.caskOK {
			b.WriteString(cask)
			b.WriteByte('\n')
		}
		return []byte(b.String()), nil
	case name == "snap" && len(args) >= 1 && args[0] == "list":
		// `snap list` leads with a header row, which loadInstalled skips;
		// the first field of each subsequent row is the snap name.
		var b strings.Builder
		b.WriteString("Name  Version  Rev  Tracking  Publisher  Notes\n")
		for s := range f.snapOK {
			b.WriteString(s + "  1.0  1  latest/stable  scout  -\n")
		}
		return []byte(b.String()), nil
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
		{detect.Env{PackageManager: "brew", Distro: "macos"}, "brew"},
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

func TestFlatpakIsInstalled(t *testing.T) {
	f := &fakeRunner{flatpakOK: map[string]bool{"md.obsidian.Obsidian": true}}
	fp := &Flatpak{Runner: f.run}
	if !fp.IsInstalled("md.obsidian.Obsidian") {
		t.Errorf("Obsidian should report installed")
	}
	if fp.IsInstalled("us.zoom.Zoom") {
		t.Errorf("Zoom should report not installed")
	}
}

func TestFlatpakInstallFiltersInstalled(t *testing.T) {
	f := &fakeRunner{flatpakOK: map[string]bool{"md.obsidian.Obsidian": true}}
	fp := &Flatpak{Runner: f.run}
	if err := fp.Install([]string{"md.obsidian.Obsidian", "us.zoom.Zoom"}); err != nil {
		t.Fatalf("Install: %v", err)
	}
	last := f.calls[len(f.calls)-1]
	if last.name != "flatpak" {
		t.Fatalf("expected flatpak, got %s", last.name)
	}
	args := strings.Join(last.args, " ")
	if !strings.Contains(args, "install -y --noninteractive flathub us.zoom.Zoom") {
		t.Errorf("install args = %q, want only Zoom from flathub", args)
	}
	if strings.Contains(args, "Obsidian") {
		t.Errorf("Obsidian was installed; must not appear: %q", args)
	}
}

func TestFlatpakInstallNoopWhenAllInstalled(t *testing.T) {
	f := &fakeRunner{flatpakOK: map[string]bool{"md.obsidian.Obsidian": true}}
	fp := &Flatpak{Runner: f.run}
	if err := fp.Install([]string{"md.obsidian.Obsidian"}); err != nil {
		t.Fatalf("Install: %v", err)
	}
	for _, c := range f.calls {
		if c.name == "flatpak" && len(c.args) > 0 && c.args[0] == "install" {
			t.Errorf("flatpak install should not have been invoked: calls=%+v", f.calls)
		}
	}
}

func TestBrewIsInstalled(t *testing.T) {
	f := &fakeRunner{brewOK: map[string]bool{"fd": true}}
	b := &Brew{Runner: f.run}
	if !b.IsInstalled("fd") {
		t.Errorf("fd should report installed")
	}
	if b.IsInstalled("nope") {
		t.Errorf("nope should report not installed")
	}
}

func TestBrewInstallFiltersInstalled(t *testing.T) {
	f := &fakeRunner{brewOK: map[string]bool{"fd": true}}
	b := &Brew{Runner: f.run}
	if err := b.Install([]string{"fd", "ripgrep", "bat"}); err != nil {
		t.Fatalf("Install: %v", err)
	}
	last := f.calls[len(f.calls)-1]
	if last.name != "brew" {
		t.Fatalf("expected brew, got %s", last.name)
	}
	args := strings.Join(last.args, " ")
	if !strings.Contains(args, "install ripgrep bat") {
		t.Errorf("install args = %q, want ripgrep + bat", args)
	}
	if strings.Contains(args, " fd") || strings.HasSuffix(args, "fd") {
		t.Errorf("fd was installed; must not appear: %q", args)
	}
}

func TestFlatpakListIsCachedAcrossCalls(t *testing.T) {
	f := &fakeRunner{flatpakOK: map[string]bool{"md.obsidian.Obsidian": true}}
	fp := &Flatpak{Runner: f.run}
	// Bucket + Install pattern from apply.go — N IsInstalled calls then
	// an Install that internally calls filterUninstalled (another N
	// IsInstalled calls). Together: 2N checks, but only one `flatpak
	// list` shellout because the result is cached.
	refs := []string{"md.obsidian.Obsidian", "us.zoom.Zoom", "io.bassi.Amberol"}
	for _, r := range refs {
		fp.IsInstalled(r)
	}
	if err := fp.Install(refs); err != nil {
		t.Fatalf("Install: %v", err)
	}
	var lists int
	for _, c := range f.calls {
		if c.name == "flatpak" && len(c.args) > 0 && c.args[0] == "list" {
			lists++
		}
	}
	if lists != 1 {
		t.Errorf("flatpak list invoked %d times, want exactly 1 (cache should serve repeat lookups)", lists)
	}
}

func TestParseBrewSpec(t *testing.T) {
	cases := []struct {
		spec     string
		wantName string
		wantCask bool
		wantErr  bool
	}{
		{"wget", "wget", false, false},
		{"firefox/cask", "firefox", true, false},
		{"foo/bogus", "", false, true},
		{"org/tap/foo", "", false, true}, // tap-qualified: first "/" splits a non-cask remainder
		{"foo/", "", false, true},        // trailing slash, empty suffix
		{"", "", false, true},            // empty spec
		{"/cask", "", false, true},       // empty name before a valid suffix
	}
	for _, tc := range cases {
		name, cask, err := parseBrewSpec(tc.spec)
		if tc.wantErr {
			if err == nil {
				t.Errorf("parseBrewSpec(%q) err = nil, want error", tc.spec)
			}
			continue
		}
		if err != nil {
			t.Errorf("parseBrewSpec(%q) err = %v", tc.spec, err)
			continue
		}
		if name != tc.wantName || cask != tc.wantCask {
			t.Errorf("parseBrewSpec(%q) = (%q, %v), want (%q, %v)", tc.spec, name, cask, tc.wantName, tc.wantCask)
		}
	}
}

func TestBrewCaskIsInstalled(t *testing.T) {
	f := &fakeRunner{
		brewOK: map[string]bool{"fd": true},
		caskOK: map[string]bool{"firefox": true},
	}
	b := &Brew{Runner: f.run}
	if !b.IsInstalled("firefox/cask") {
		t.Errorf("firefox/cask should report installed")
	}
	if b.IsInstalled("chrome/cask") {
		t.Errorf("chrome/cask should report not installed")
	}
	// A formula name must not match a cask of the same name and vice versa.
	if b.IsInstalled("firefox") {
		t.Errorf("bare formula firefox should report not installed (it's a cask)")
	}
}

func TestBrewInstallBucketsFormulaeAndCasks(t *testing.T) {
	f := &fakeRunner{
		brewOK: map[string]bool{"fd": true},
		caskOK: map[string]bool{"firefox": true},
	}
	b := &Brew{Runner: f.run}
	// fd + firefox/cask are already installed; ripgrep (formula) + slack/cask
	// are not. Expect one `brew install ripgrep` and one `brew install --cask
	// slack`, nothing for the already-installed pair.
	if err := b.Install([]string{"fd", "ripgrep", "firefox/cask", "slack/cask"}); err != nil {
		t.Fatalf("Install: %v", err)
	}
	var formula, cask string
	for _, c := range f.calls {
		if c.name != "brew" || len(c.args) == 0 || c.args[0] != "install" {
			continue
		}
		joined := strings.Join(c.args, " ")
		if strings.Contains(joined, "--cask") {
			cask = joined
		} else {
			formula = joined
		}
	}
	if formula != "install ripgrep" {
		t.Errorf("formula install = %q, want `install ripgrep` (fd already installed)", formula)
	}
	if cask != "install --cask slack" {
		t.Errorf("cask install = %q, want `install --cask slack` (firefox already installed)", cask)
	}
}

func TestBrewInstallCasksContinueOnConflict(t *testing.T) {
	// 1password conflicts (e.g. /Applications/1Password.app already exists);
	// ghostty must still be installed, and the returned error must name the
	// failing cask and point at --adopt.
	f := &fakeRunner{failCmd: "brew install --cask 1password"}
	b := &Brew{Runner: f.run}
	err := b.Install([]string{"1password/cask", "ghostty/cask"})
	if err == nil {
		t.Fatal("expected an error for the conflicting cask")
	}
	if !strings.Contains(err.Error(), "1password") || !strings.Contains(err.Error(), "--adopt") {
		t.Errorf("error should name the failing cask and suggest --adopt, got: %v", err)
	}
	// Both casks must have been attempted individually — the conflict on the
	// first must NOT skip the second.
	var caskInstalls []string
	for _, c := range f.calls {
		joined := strings.Join(append([]string{c.name}, c.args...), " ")
		if strings.HasPrefix(joined, "brew install --cask ") {
			caskInstalls = append(caskInstalls, joined)
		}
	}
	if len(caskInstalls) != 2 {
		t.Fatalf("expected 2 individual cask installs, got %d: %v", len(caskInstalls), caskInstalls)
	}
	if caskInstalls[0] != "brew install --cask 1password" || caskInstalls[1] != "brew install --cask ghostty" {
		t.Errorf("cask installs = %v, want [brew install --cask 1password, brew install --cask ghostty]", caskInstalls)
	}
}

func TestBrewInstallNoopWhenAllInstalled(t *testing.T) {
	f := &fakeRunner{
		brewOK: map[string]bool{"fd": true},
		caskOK: map[string]bool{"firefox": true},
	}
	b := &Brew{Runner: f.run}
	if err := b.Install([]string{"fd", "firefox/cask"}); err != nil {
		t.Fatalf("Install: %v", err)
	}
	for _, c := range f.calls {
		if c.name == "brew" && len(c.args) > 0 && c.args[0] == "install" {
			t.Errorf("brew install should not have been invoked: calls=%+v", f.calls)
		}
	}
}

func TestBrewInstallUnknownSuffixErrors(t *testing.T) {
	f := &fakeRunner{}
	b := &Brew{Runner: f.run}
	err := b.Install([]string{"foo/bogus"})
	if err == nil {
		t.Fatal("Install with an unknown suffix should error")
	}
	if !strings.Contains(err.Error(), "bogus") || !strings.Contains(err.Error(), "/cask") {
		t.Errorf("error = %q, want it to name the bad suffix and mention /cask", err)
	}
	for _, c := range f.calls {
		if c.name == "brew" && len(c.args) > 0 && c.args[0] == "install" {
			t.Errorf("no brew install should run when a spec is invalid: calls=%+v", f.calls)
		}
	}
}

func TestBrewValidate(t *testing.T) {
	b := &Brew{Runner: (&fakeRunner{}).run}
	if err := b.Validate([]string{"wget", "firefox/cask"}); err != nil {
		t.Errorf("valid specs should pass Validate, got %v", err)
	}
	err := b.Validate([]string{"wget", "foo/bogus"})
	if err == nil || !strings.Contains(err.Error(), "bogus") {
		t.Errorf("Validate should reject foo/bogus with a clear message, got %v", err)
	}
}

func TestBrewListIsCachedAcrossCalls(t *testing.T) {
	f := &fakeRunner{brewOK: map[string]bool{"fd": true}}
	b := &Brew{Runner: f.run}
	formulae := []string{"fd", "ripgrep", "bat"}
	for _, name := range formulae {
		b.IsInstalled(name)
	}
	if err := b.Install(formulae); err != nil {
		t.Fatalf("Install: %v", err)
	}
	var lists int
	for _, c := range f.calls {
		if c.name == "brew" && len(c.args) > 0 && c.args[0] == "list" {
			lists++
		}
	}
	if lists != 1 {
		t.Errorf("brew list invoked %d times, want exactly 1", lists)
	}
}

func TestForBackend(t *testing.T) {
	cases := []struct {
		name string
		want string
	}{
		{"flatpak", "flatpak"},
		{"brew", "brew"},
		{"snap", "snap"},
		{"cargo", ""},
		{"", ""},
	}
	for _, tc := range cases {
		got := ForBackend(tc.name)
		if tc.want == "" {
			if got != nil {
				t.Errorf("ForBackend(%q) = %v, want nil", tc.name, got)
			}
			continue
		}
		if got == nil || got.Name() != tc.want {
			t.Errorf("ForBackend(%q).Name() = %v, want %q", tc.name, got, tc.want)
		}
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

func TestParseSnapSpec(t *testing.T) {
	cases := []struct {
		spec     string
		wantName string
		wantMode string
		wantErr  bool
	}{
		{"gimp", "gimp", "", false},
		{"aws-cli/classic", "aws-cli", "classic", false},
		{"foo/devmode", "foo", "devmode", false},
		{"foo/jailmode", "foo", "jailmode", false},
		{"foo/bogus", "", "", true},
		{"foo/", "", "", true}, // trailing slash, empty mode
	}
	for _, tc := range cases {
		name, mode, err := parseSnapSpec(tc.spec)
		if tc.wantErr {
			if err == nil {
				t.Errorf("parseSnapSpec(%q) err = nil, want error", tc.spec)
			}
			continue
		}
		if err != nil {
			t.Errorf("parseSnapSpec(%q) err = %v", tc.spec, err)
			continue
		}
		if name != tc.wantName || mode != tc.wantMode {
			t.Errorf("parseSnapSpec(%q) = (%q, %q), want (%q, %q)", tc.spec, name, mode, tc.wantName, tc.wantMode)
		}
	}
}

func TestSnapIsInstalled(t *testing.T) {
	f := &fakeRunner{snapOK: map[string]bool{"gimp": true, "code": true}}
	s := &Snap{Runner: f.run}
	if !s.IsInstalled("gimp") {
		t.Errorf("gimp should report installed")
	}
	if !s.IsInstalled("code/classic") {
		t.Errorf("code/classic should report installed — confinement suffix is ignored for the lookup")
	}
	if s.IsInstalled("spotify") {
		t.Errorf("spotify should report not installed")
	}
}

func TestSnapInstallStrictAndClassic(t *testing.T) {
	f := &fakeRunner{}
	s := &Snap{Runner: f.run}
	if err := s.Install([]string{"gimp", "aws-cli/classic"}); err != nil {
		t.Fatalf("Install: %v", err)
	}
	var installs []string
	for _, c := range f.calls {
		if c.name == "snap" && len(c.args) > 0 && c.args[0] == "install" {
			installs = append(installs, strings.Join(c.args, " "))
		}
	}
	if len(installs) != 2 {
		t.Fatalf("expected 2 snap install calls (one per mode), got %d: %v", len(installs), installs)
	}
	// Strict group sorts before the classic group (mode "" < "classic").
	if installs[0] != "install gimp" {
		t.Errorf("strict install = %q, want `install gimp`", installs[0])
	}
	if installs[1] != "install aws-cli --classic" {
		t.Errorf("classic install = %q, want `install aws-cli --classic`", installs[1])
	}
}

func TestSnapInstallGroupsByMode(t *testing.T) {
	f := &fakeRunner{}
	s := &Snap{Runner: f.run}
	if err := s.Install([]string{"a", "b", "c/classic", "d/classic"}); err != nil {
		t.Fatalf("Install: %v", err)
	}
	var strict, classic string
	for _, c := range f.calls {
		if c.name != "snap" || len(c.args) == 0 || c.args[0] != "install" {
			continue
		}
		joined := strings.Join(c.args, " ")
		if strings.Contains(joined, "--classic") {
			classic = joined
		} else {
			strict = joined
		}
	}
	if strict != "install a b" {
		t.Errorf("strict group = %q, want `install a b`", strict)
	}
	if classic != "install c d --classic" {
		t.Errorf("classic group = %q, want `install c d --classic`", classic)
	}
}

func TestSnapInstallFiltersInstalled(t *testing.T) {
	f := &fakeRunner{snapOK: map[string]bool{"gimp": true}}
	s := &Snap{Runner: f.run}
	if err := s.Install([]string{"gimp", "spotify"}); err != nil {
		t.Fatalf("Install: %v", err)
	}
	var installs []string
	for _, c := range f.calls {
		if c.name == "snap" && len(c.args) > 0 && c.args[0] == "install" {
			installs = append(installs, strings.Join(c.args, " "))
		}
	}
	if len(installs) != 1 || installs[0] != "install spotify" {
		t.Errorf("installs = %v, want only `install spotify` (gimp already installed)", installs)
	}
}

func TestSnapInstallNoopWhenAllInstalled(t *testing.T) {
	f := &fakeRunner{snapOK: map[string]bool{"gimp": true}}
	s := &Snap{Runner: f.run}
	if err := s.Install([]string{"gimp"}); err != nil {
		t.Fatalf("Install: %v", err)
	}
	for _, c := range f.calls {
		if c.name == "snap" && len(c.args) > 0 && c.args[0] == "install" {
			t.Errorf("snap install should not have been invoked: calls=%+v", f.calls)
		}
	}
}

func TestSnapInstallUnknownModeErrors(t *testing.T) {
	f := &fakeRunner{}
	s := &Snap{Runner: f.run}
	err := s.Install([]string{"foo/bogus"})
	if err == nil {
		t.Fatal("Install with unknown confinement mode should error")
	}
	if !strings.Contains(err.Error(), "bogus") || !strings.Contains(err.Error(), "confinement") {
		t.Errorf("error = %q, want it to name the bad mode and mention confinement", err)
	}
	for _, c := range f.calls {
		if c.name == "snap" && len(c.args) > 0 && c.args[0] == "install" {
			t.Errorf("no snap install should run when a spec is invalid: calls=%+v", f.calls)
		}
	}
}

func TestSnapValidate(t *testing.T) {
	s := &Snap{Runner: (&fakeRunner{}).run}
	if err := s.Validate([]string{"gimp", "aws-cli/classic", "x/devmode", "y/jailmode"}); err != nil {
		t.Errorf("valid specs should pass Validate, got %v", err)
	}
	err := s.Validate([]string{"gimp", "foo/bogus"})
	if err == nil || !strings.Contains(err.Error(), "bogus") {
		t.Errorf("Validate should reject foo/bogus with a clear message, got %v", err)
	}
}

func TestSnapInstallErrorNamesMode(t *testing.T) {
	// The classic group's install fails; the strict group (installed first)
	// succeeds. The error must identify the classic batch.
	f := &fakeRunner{failCmd: "snap install c --classic"}
	s := &Snap{Runner: f.run}
	err := s.Install([]string{"a", "c/classic"})
	if err == nil {
		t.Fatal("expected install error from the classic group")
	}
	if !strings.Contains(err.Error(), "classic") {
		t.Errorf("error should name the failing confinement group, got %q", err)
	}
}

func TestSnapInstallSudo(t *testing.T) {
	f := &fakeRunner{}
	s := &Snap{Runner: f.run, Sudo: true}
	if err := s.Install([]string{"gimp"}); err != nil {
		t.Fatalf("Install: %v", err)
	}
	last := f.calls[len(f.calls)-1]
	if last.name != "sudo" || last.args[0] != "snap" {
		t.Errorf("expected `sudo snap ...`, got %s %v", last.name, last.args)
	}
}

func TestSnapListIsCachedAcrossCalls(t *testing.T) {
	f := &fakeRunner{snapOK: map[string]bool{"gimp": true}}
	s := &Snap{Runner: f.run}
	specs := []string{"gimp", "spotify", "code/classic"}
	for _, spec := range specs {
		s.IsInstalled(spec)
	}
	if err := s.Install(specs); err != nil {
		t.Fatalf("Install: %v", err)
	}
	var lists int
	for _, c := range f.calls {
		if c.name == "snap" && len(c.args) > 0 && c.args[0] == "list" {
			lists++
		}
	}
	if lists != 1 {
		t.Errorf("snap list invoked %d times, want exactly 1", lists)
	}
}
