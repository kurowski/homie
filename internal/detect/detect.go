// Package detect inspects the runtime environment Homie is running in:
// Linux distro, package manager, CPU arch, container/root/TTY state.
// All fields are derived from injectable sources so tests don't depend on
// the real filesystem or process state.
package detect

import (
	"bufio"
	"io/fs"
	"os"
	"runtime"
	"strings"

	"golang.org/x/term"
)

// Env is the resolved view of the runtime environment.
type Env struct {
	Distro         string   // ubuntu, debian, fedora, unknown
	PackageManager string   // apt, dnf, unknown
	Arch           string   // amd64, arm64, ...
	Hostname       string   // short hostname (everything before the first dot)
	IsContainer    bool
	IsRoot         bool
	IsInteractive  bool
	Tags           []string // distro, arch, hostname, plus container/root when applicable
}

// Detector reads the environment. Zero value uses the real OS sources.
// Tests override fields to swap in fake filesystems, env, etc.
type Detector struct {
	FS             fs.FS                  // root filesystem (paths are relative — no leading slash)
	Getenv         func(string) string    // env lookup
	Geteuid        func() int             // effective uid
	IsTTY          func() bool            // TTY check for stdout
	Arch           string                 // GOARCH override
	LookupHostname func() (string, error) // os.Hostname override
}

// Detect runs the default detector against the real OS environment.
func Detect() Env { return Detector{}.Detect() }

// Detect runs this configured detector.
func (d Detector) Detect() Env {
	d = d.withDefaults()

	env := Env{Arch: d.Arch}
	env.Distro = parseDistro(d.FS)
	env.PackageManager = packageManagerFor(env.Distro)
	env.IsContainer = detectContainer(d.FS, d.Getenv)
	env.IsRoot = d.Geteuid() == 0
	env.IsInteractive = d.IsTTY()
	env.Hostname = shortHostname(d.LookupHostname)
	env.Tags = autoTags(env)
	return env
}

// shortHostname returns the hostname truncated at the first dot, so an FQDN
// like "coach.lan" tags as "coach" — what users mean when they write
// `hasTag "coach"`. Returns "" if the underlying call fails or the value
// contains a path separator (defense against a malformed hostname being
// interpolated into a hosts/<name>.toml lookup).
func shortHostname(hostFn func() (string, error)) string {
	h, err := hostFn()
	if err != nil {
		return ""
	}
	h = strings.TrimSpace(h)
	if i := strings.IndexByte(h, '.'); i >= 0 {
		h = h[:i]
	}
	if strings.ContainsAny(h, `/\`) {
		return ""
	}
	return h
}

func (d Detector) withDefaults() Detector {
	if d.FS == nil {
		d.FS = os.DirFS("/")
	}
	if d.Getenv == nil {
		d.Getenv = os.Getenv
	}
	if d.Geteuid == nil {
		d.Geteuid = os.Geteuid
	}
	if d.IsTTY == nil {
		d.IsTTY = func() bool { return term.IsTerminal(int(os.Stdout.Fd())) }
	}
	if d.Arch == "" {
		d.Arch = runtime.GOARCH
	}
	if d.LookupHostname == nil {
		d.LookupHostname = os.Hostname
	}
	return d
}

func parseDistro(root fs.FS) string {
	f, err := root.Open("etc/os-release")
	if err != nil {
		return "unknown"
	}
	defer func() { _ = f.Close() }()

	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		if !strings.HasPrefix(line, "ID=") {
			continue
		}
		id := strings.Trim(strings.TrimPrefix(line, "ID="), `"'`)
		switch id {
		case "ubuntu", "debian", "fedora":
			return id
		default:
			// TODO(contrib): add support for additional distros here.
			// See https://homie.sh/contributing.
			return "unknown"
		}
	}
	if err := sc.Err(); err != nil {
		return "unknown"
	}
	return "unknown"
}

func packageManagerFor(distro string) string {
	switch distro {
	case "ubuntu", "debian":
		return "apt"
	case "fedora":
		return "dnf"
	default:
		// TODO(contrib): add support for additional package managers here.
		return "unknown"
	}
}

func detectContainer(root fs.FS, getenv func(string) string) bool {
	if getenv("REMOTE_CONTAINERS") != "" || getenv("CODESPACES") != "" {
		return true
	}
	for _, path := range []string{".dockerenv", "run/.containerenv"} {
		if _, err := fs.Stat(root, path); err == nil {
			return true
		}
	}
	if data, err := fs.ReadFile(root, "proc/1/cgroup"); err == nil {
		s := string(data)
		for _, needle := range []string{"docker", "containerd", "kubepods", "libpod"} {
			if strings.Contains(s, needle) {
				return true
			}
		}
	}
	return false
}

func autoTags(env Env) []string {
	tags := make([]string, 0, 5)
	if env.Distro != "" && env.Distro != "unknown" {
		tags = append(tags, env.Distro)
	}
	if env.Arch != "" {
		tags = append(tags, env.Arch)
	}
	if env.Hostname != "" {
		tags = append(tags, env.Hostname)
	}
	if env.IsContainer {
		tags = append(tags, "container")
	}
	if env.IsRoot {
		tags = append(tags, "root")
	}
	return tags
}
