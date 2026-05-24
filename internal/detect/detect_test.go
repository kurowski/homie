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
		name string
		fsys fstest.MapFS
		env  map[string]string
		uid  int
		tty  bool
		arch string
		want Env
	}{
		{
			name: "fedora amd64 user terminal",
			fsys: fstest.MapFS{"etc/os-release": osRelease("fedora")},
			uid:  1000,
			tty:  true,
			arch: "amd64",
			want: Env{
				Distro: "fedora", PackageManager: "dnf", Arch: "amd64",
				IsInteractive: true,
				Tags:          []string{"amd64", "fedora"},
			},
		},
		{
			name: "ubuntu apt as root",
			fsys: fstest.MapFS{"etc/os-release": osRelease("ubuntu")},
			uid:  0,
			arch: "arm64",
			want: Env{
				Distro: "ubuntu", PackageManager: "apt", Arch: "arm64",
				IsRoot: true,
				Tags:   []string{"arm64", "root", "ubuntu"},
			},
		},
		{
			name: "debian apt",
			fsys: fstest.MapFS{"etc/os-release": osRelease("debian")},
			uid:  1000,
			arch: "amd64",
			want: Env{
				Distro: "debian", PackageManager: "apt", Arch: "amd64",
				Tags: []string{"amd64", "debian"},
			},
		},
		{
			name: "unknown distro stays unknown",
			fsys: fstest.MapFS{"etc/os-release": osRelease("arch")},
			uid:  1000,
			arch: "amd64",
			want: Env{
				Distro: "unknown", PackageManager: "unknown", Arch: "amd64",
				Tags: []string{"amd64"},
			},
		},
		{
			name: "missing os-release",
			fsys: fstest.MapFS{},
			uid:  1000,
			arch: "amd64",
			want: Env{
				Distro: "unknown", PackageManager: "unknown", Arch: "amd64",
				Tags: []string{"amd64"},
			},
		},
		{
			name: "quoted id",
			fsys: fstest.MapFS{
				"etc/os-release": &fstest.MapFile{Data: []byte(`ID="fedora"` + "\n")},
			},
			uid:  1000,
			arch: "amd64",
			want: Env{
				Distro: "fedora", PackageManager: "dnf", Arch: "amd64",
				Tags: []string{"amd64", "fedora"},
			},
		},
		{
			name: "docker via /.dockerenv",
			fsys: fstest.MapFS{
				"etc/os-release": osRelease("ubuntu"),
				".dockerenv":     &fstest.MapFile{},
			},
			uid:  1000,
			arch: "amd64",
			want: Env{
				Distro: "ubuntu", PackageManager: "apt", Arch: "amd64",
				IsContainer: true,
				Tags:        []string{"amd64", "container", "ubuntu"},
			},
		},
		{
			name: "podman via /run/.containerenv",
			fsys: fstest.MapFS{
				"etc/os-release":    osRelease("fedora"),
				"run/.containerenv": &fstest.MapFile{},
			},
			uid:  1000,
			arch: "amd64",
			want: Env{
				Distro: "fedora", PackageManager: "dnf", Arch: "amd64",
				IsContainer: true,
				Tags:        []string{"amd64", "container", "fedora"},
			},
		},
		{
			name: "cgroup signal",
			fsys: fstest.MapFS{
				"etc/os-release": osRelease("debian"),
				"proc/1/cgroup":  &fstest.MapFile{Data: []byte("0::/system.slice/docker-abc.scope\n")},
			},
			uid:  1000,
			arch: "amd64",
			want: Env{
				Distro: "debian", PackageManager: "apt", Arch: "amd64",
				IsContainer: true,
				Tags:        []string{"amd64", "container", "debian"},
			},
		},
		{
			name: "codespaces env var",
			fsys: fstest.MapFS{"etc/os-release": osRelease("ubuntu")},
			env:  map[string]string{"CODESPACES": "true"},
			uid:  1000,
			arch: "amd64",
			want: Env{
				Distro: "ubuntu", PackageManager: "apt", Arch: "amd64",
				IsContainer: true,
				Tags:        []string{"amd64", "container", "ubuntu"},
			},
		},
		{
			name: "remote_containers env var",
			fsys: fstest.MapFS{"etc/os-release": osRelease("debian")},
			env:  map[string]string{"REMOTE_CONTAINERS": "true"},
			uid:  1000,
			arch: "amd64",
			want: Env{
				Distro: "debian", PackageManager: "apt", Arch: "amd64",
				IsContainer: true,
				Tags:        []string{"amd64", "container", "debian"},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d := Detector{
				FS:      tc.fsys,
				Getenv:  func(k string) string { return tc.env[k] },
				Geteuid: func() int { return tc.uid },
				IsTTY:   func() bool { return tc.tty },
				Arch:    tc.arch,
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
