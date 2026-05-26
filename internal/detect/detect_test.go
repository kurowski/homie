package detect

import (
	"reflect"
	"sort"
	"testing"
	"testing/fstest"
)

func osRelease(id string) *fstest.MapFile {
	return &fstest.MapFile{Data: []byte("NAME=\"Test\"\nID=" + id + "\nVERSION_ID=1\n")}
}

func TestDetect(t *testing.T) {
	cases := []struct {
		name     string
		fsys     fstest.MapFS
		env      map[string]string
		uid      int
		tty      bool
		arch     string
		hostname string
		hostErr  error
		want     Env
	}{
		{
			name:     "fedora amd64 user terminal",
			fsys:     fstest.MapFS{"etc/os-release": osRelease("fedora")},
			uid:      1000,
			tty:      true,
			arch:     "amd64",
			hostname: "coach",
			want: Env{
				Distro: "fedora", PackageManager: "dnf", Arch: "amd64",
				Hostname:      "coach",
				IsInteractive: true,
				Tags:          []string{"amd64", "coach", "fedora"},
			},
		},
		{
			name:     "ubuntu apt as root",
			fsys:     fstest.MapFS{"etc/os-release": osRelease("ubuntu")},
			uid:      0,
			arch:     "arm64",
			hostname: "build-01",
			want: Env{
				Distro: "ubuntu", PackageManager: "apt", Arch: "arm64",
				Hostname: "build-01",
				IsRoot:   true,
				Tags:     []string{"arm64", "build-01", "root", "ubuntu"},
			},
		},
		{
			name:     "debian apt",
			fsys:     fstest.MapFS{"etc/os-release": osRelease("debian")},
			uid:      1000,
			arch:     "amd64",
			hostname: "deb",
			want: Env{
				Distro: "debian", PackageManager: "apt", Arch: "amd64",
				Hostname: "deb",
				Tags:     []string{"amd64", "deb", "debian"},
			},
		},
		{
			name:     "unknown distro stays unknown",
			fsys:     fstest.MapFS{"etc/os-release": osRelease("arch")},
			uid:      1000,
			arch:     "amd64",
			hostname: "weird",
			want: Env{
				Distro: "unknown", PackageManager: "unknown", Arch: "amd64",
				Hostname: "weird",
				Tags:     []string{"amd64", "weird"},
			},
		},
		{
			name:     "missing os-release",
			fsys:     fstest.MapFS{},
			uid:      1000,
			arch:     "amd64",
			hostname: "noos",
			want: Env{
				Distro: "unknown", PackageManager: "unknown", Arch: "amd64",
				Hostname: "noos",
				Tags:     []string{"amd64", "noos"},
			},
		},
		{
			name:     "quoted id",
			fsys:     fstest.MapFS{"etc/os-release": &fstest.MapFile{Data: []byte(`ID="fedora"` + "\n")}},
			uid:      1000,
			arch:     "amd64",
			hostname: "q",
			want: Env{
				Distro: "fedora", PackageManager: "dnf", Arch: "amd64",
				Hostname: "q",
				Tags:     []string{"amd64", "fedora", "q"},
			},
		},
		{
			name: "docker via /.dockerenv",
			fsys: fstest.MapFS{
				"etc/os-release": osRelease("ubuntu"),
				".dockerenv":     &fstest.MapFile{},
			},
			uid:      1000,
			arch:     "amd64",
			hostname: "ctr1",
			want: Env{
				Distro: "ubuntu", PackageManager: "apt", Arch: "amd64",
				Hostname:    "ctr1",
				IsContainer: true,
				Tags:        []string{"amd64", "container", "ctr1", "ubuntu"},
			},
		},
		{
			name: "podman via /run/.containerenv",
			fsys: fstest.MapFS{
				"etc/os-release":    osRelease("fedora"),
				"run/.containerenv": &fstest.MapFile{},
			},
			uid:      1000,
			arch:     "amd64",
			hostname: "pod1",
			want: Env{
				Distro: "fedora", PackageManager: "dnf", Arch: "amd64",
				Hostname:    "pod1",
				IsContainer: true,
				Tags:        []string{"amd64", "container", "fedora", "pod1"},
			},
		},
		{
			name: "cgroup signal",
			fsys: fstest.MapFS{
				"etc/os-release": osRelease("debian"),
				"proc/1/cgroup":  &fstest.MapFile{Data: []byte("0::/system.slice/docker-abc.scope\n")},
			},
			uid:      1000,
			arch:     "amd64",
			hostname: "cg",
			want: Env{
				Distro: "debian", PackageManager: "apt", Arch: "amd64",
				Hostname:    "cg",
				IsContainer: true,
				Tags:        []string{"amd64", "cg", "container", "debian"},
			},
		},
		{
			name:     "codespaces env var",
			fsys:     fstest.MapFS{"etc/os-release": osRelease("ubuntu")},
			env:      map[string]string{"CODESPACES": "true"},
			uid:      1000,
			arch:     "amd64",
			hostname: "codespace",
			want: Env{
				Distro: "ubuntu", PackageManager: "apt", Arch: "amd64",
				Hostname:    "codespace",
				IsContainer: true,
				Tags:        []string{"amd64", "codespace", "container", "ubuntu"},
			},
		},
		{
			name:     "remote_containers env var",
			fsys:     fstest.MapFS{"etc/os-release": osRelease("debian")},
			env:      map[string]string{"REMOTE_CONTAINERS": "true"},
			uid:      1000,
			arch:     "amd64",
			hostname: "devc",
			want: Env{
				Distro: "debian", PackageManager: "apt", Arch: "amd64",
				Hostname:    "devc",
				IsContainer: true,
				Tags:        []string{"amd64", "container", "debian", "devc"},
			},
		},
		{
			name:     "fqdn is truncated to short hostname",
			fsys:     fstest.MapFS{"etc/os-release": osRelease("fedora")},
			uid:      1000,
			arch:     "amd64",
			hostname: "coach.lan",
			want: Env{
				Distro: "fedora", PackageManager: "dnf", Arch: "amd64",
				Hostname: "coach",
				Tags:     []string{"amd64", "coach", "fedora"},
			},
		},
		{
			name:     "hostname error means no tag",
			fsys:     fstest.MapFS{"etc/os-release": osRelease("fedora")},
			uid:      1000,
			arch:     "amd64",
			hostname: "ignored",
			hostErr:  errSentinel,
			want: Env{
				Distro: "fedora", PackageManager: "dnf", Arch: "amd64",
				Tags: []string{"amd64", "fedora"},
			},
		},
		{
			name:     "hostname with path separator is rejected",
			fsys:     fstest.MapFS{"etc/os-release": osRelease("fedora")},
			uid:      1000,
			arch:     "amd64",
			hostname: "../etc/passwd",
			want: Env{
				Distro: "fedora", PackageManager: "dnf", Arch: "amd64",
				Tags: []string{"amd64", "fedora"},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d := Detector{
				FS:             tc.fsys,
				Getenv:         func(k string) string { return tc.env[k] },
				Geteuid:        func() int { return tc.uid },
				IsTTY:          func() bool { return tc.tty },
				Arch:           tc.arch,
				LookupHostname: func() (string, error) { return tc.hostname, tc.hostErr },
			}
			got := d.Detect()
			// sort tags to make comparison order-independent
			sort.Strings(got.Tags)
			sort.Strings(tc.want.Tags)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("\n got: %+v\nwant: %+v", got, tc.want)
			}
		})
	}
}

var errSentinel = stubError("boom")

type stubError string

func (e stubError) Error() string { return string(e) }

func TestDetectDefaults(t *testing.T) {
	// Verify the zero-value Detector picks up real defaults without panicking
	// against the host filesystem.
	env := Detect()
	if env.Arch == "" {
		t.Errorf("Arch should be populated from runtime.GOARCH, got empty")
	}
	if env.Distro == "" {
		t.Errorf("Distro should never be empty, got empty")
	}
}
