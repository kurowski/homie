//go:build e2e

// Package e2e drives the real curl|bash flow against Ubuntu, Debian, and
// Fedora containers. A tiny nginx sidecar serves the hm release artifacts,
// a bare clone of a scaffold-generated user repo, and the bootstrap.sh,
// all over HTTPS using the committed test CA + leaf cert under e2e/certs.
// Each distro container joins the nginx network with --network-alias
// github.com so the URLs inside bootstrap.sh resolve to nginx without
// modification.
//
// Build-tagged so `go test ./...` skips it. Opt in with:
//
//	go test -tags=e2e ./e2e/...      # or
//	make e2e
package e2e_test

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// distros enumerates the supported v1 distros plus a one-shot package
// query that verifies one of the scaffold packages was installed. tmux
// is in the scaffold's default `[packages].all` and lives in every
// distro's main repo, so it's a reliable proxy for "the apply step's
// package phase ran". The test CA is baked into each image at build
// time (see e2e/dockerfiles/*.Dockerfile), so nothing to set up here.
var distros = []struct {
	name         string
	packageCheck []string // command + args, exit 0 iff installed
}{
	{"ubuntu", []string{"dpkg", "-s", "tmux"}},
	{"debian", []string{"dpkg", "-s", "tmux"}},
	{"fedora", []string{"rpm", "-q", "tmux"}},
}

const (
	containerUser = "scout"
	containerHome = "/home/scout"
)

func TestApplyAcrossDistros(t *testing.T) {
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker not on PATH; skipping e2e")
	}

	binary := buildHmBinary(t)
	w := prepWebserver(t, binary)

	network := makeNetwork(t)
	t.Cleanup(func() { _, _ = dockerRun("network", "rm", network) })

	webserverCID := startWebserver(t, w, network)
	t.Cleanup(func() { _, _ = dockerRun("rm", "-f", webserverCID) })

	for _, d := range distros {
		d := d
		t.Run(d.name, func(t *testing.T) {
			t.Parallel()
			image := buildImage(t, d.name)
			cid := runContainer(t, image, network)
			t.Cleanup(func() { _, _ = dockerRun("rm", "-f", cid) })

			// The full curl|bash flow. bootstrap.sh:
			//   1. downloads hm-linux-<arch> + SHA256SUMS, verifies hash
			//   2. runs `hm bootstrap` to install git + ca-certificates
			//   3. git-clones the user repo
			//   4. execs `hm apply`
			bootstrapURL := fmt.Sprintf(
				"https://raw.githubusercontent.com/%s/%s/main/bootstrap.sh",
				userrepoOwner, userrepoName,
			)
			out, err := dockerExecOutput(cid, "bash", "-c",
				"curl -fsSL "+bootstrapURL+" | bash 2>&1")
			if err != nil {
				t.Fatalf("bootstrap (curl|bash) failed: %v\n%s", err, out)
			}
			if !strings.Contains(out, "All phases completed cleanly") {
				t.Errorf("bootstrap summary not clean:\n%s", out)
			}

			// Per-distro post-state assertions, all against scaffold
			// defaults: link phase symlinked .zshrc, render phase
			// produced ~/.gitconfig from .gitconfig.tmpl, script phase
			// ran 01-shell.sh and printed the login shell.
			mustExec(t, cid, d.packageCheck...)
			assertSymlink(t, cid,
				containerHome+"/.zshrc",
				containerHome+"/"+userrepoName+"/dotfiles/.zshrc",
			)
			assertContains(t, cid, containerHome+"/.gitconfig", "name = Scout Homes")
			assertContains(t, cid, containerHome+"/.gitconfig", "email = scout@homie.sh")
			assertContains(t, cid, containerHome+"/.gitconfig", "editor = nvim")
			if !strings.Contains(out, "login shell:") {
				t.Errorf("01-shell.sh output not found in apply log:\n%s", out)
			}

			// Idempotency — second apply from the cloned repo must
			// succeed and report the link as already in sync.
			// bootstrap.sh installed hm under ~/.local/bin; that
			// dir is only on PATH inside bootstrap.sh's own shell,
			// not a fresh docker exec, so spell out the absolute
			// path.
			out, err = dockerExecOutput(cid, "bash", "-c",
				"cd "+containerHome+"/"+userrepoName+
					" && "+containerHome+"/.local/bin/hm apply 2>&1")
			if err != nil {
				t.Fatalf("second apply failed: %v\n%s", err, out)
			}
			if !strings.Contains(out, "All phases completed cleanly") {
				t.Errorf("second apply summary not clean:\n%s", out)
			}
			if !strings.Contains(out, "already in sync") {
				t.Errorf("second apply should report dotfile skip:\n%s", out)
			}
		})
	}
}

// buildHmBinary compiles ./cmd/hm as linux/<host-arch> static. We run
// inside docker on the host's architecture, so GOARCH = runtime.GOARCH.
func buildHmBinary(t *testing.T) string {
	t.Helper()
	root := repoRoot(t)
	out := filepath.Join(t.TempDir(), "hm")

	cmd := exec.Command("go", "build",
		"-trimpath",
		"-ldflags=-s -w -X main.version=e2e",
		"-o", out,
		"./cmd/hm",
	)
	cmd.Dir = root
	cmd.Env = append(os.Environ(),
		"GOOS=linux",
		"GOARCH="+runtime.GOARCH,
		"CGO_ENABLED=0",
	)
	if buf, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build hm: %v\n%s", err, buf)
	}
	return out
}

// buildImage builds (or rebuilds, cached) the e2e image for distro.
func buildImage(t *testing.T, distro string) string {
	t.Helper()
	root := repoRoot(t)
	image := "homie-e2e-" + distro
	dockerfile := filepath.Join(root, "e2e", "dockerfiles", distro+".Dockerfile")

	out, err := exec.Command("docker", "build",
		"--quiet",
		"-t", image,
		"-f", dockerfile,
		filepath.Join(root, "e2e"),
	).CombinedOutput()
	if err != nil {
		t.Fatalf("docker build %s: %v\n%s", distro, err, out)
	}
	return image
}

// makeNetwork creates a fresh user-defined bridge network for this run.
// The nginx alias trick (resolving github.com to our webserver) only
// works on user-defined networks, not the default bridge.
func makeNetwork(t *testing.T) string {
	t.Helper()
	name := fmt.Sprintf("homie-e2e-%d", time.Now().UnixNano())
	out, err := exec.Command("docker", "network", "create", name).CombinedOutput()
	if err != nil {
		t.Fatalf("docker network create: %v\n%s", err, out)
	}
	return name
}

// runContainer starts a distro container on the supplied network and
// returns its ID. The image already declares the scout user + WORKDIR;
// we just need to keep it alive long enough to exec commands into.
func runContainer(t *testing.T, image, network string) string {
	t.Helper()
	out, err := exec.Command("docker", "run", "-d", "--rm",
		"--network", network,
		image,
	).CombinedOutput()
	if err != nil {
		t.Fatalf("docker run %s: %v\n%s", image, err, out)
	}
	cid := strings.TrimSpace(string(out))
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		status, _ := dockerRun("inspect", "-f", "{{.State.Status}}", cid)
		if strings.TrimSpace(status) == "running" {
			return cid
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("container %s never reached running state", cid)
	return ""
}

// dockerExecOutput runs argv as the container's default user and
// returns combined output + error.
func dockerExecOutput(cid string, argv ...string) (string, error) {
	args := append([]string{"exec", cid}, argv...)
	out, err := exec.Command("docker", args...).CombinedOutput()
	return string(out), err
}

// mustExec runs argv inside the container and fails the test if it
// returns non-zero.
func mustExec(t *testing.T, cid string, argv ...string) {
	t.Helper()
	out, err := dockerExecOutput(cid, argv...)
	if err != nil {
		t.Errorf("exec %v: %v\n%s", argv, err, out)
	}
}

// assertSymlink fails the test unless path inside cid is a symlink
// pointing to want.
func assertSymlink(t *testing.T, cid, path, want string) {
	t.Helper()
	out, err := dockerExecOutput(cid, "readlink", path)
	if err != nil {
		t.Errorf("readlink %s: %v\n%s", path, err, out)
		return
	}
	if strings.TrimSpace(out) != want {
		t.Errorf("symlink %s = %q, want %q", path, strings.TrimSpace(out), want)
	}
}

// assertContains reads path inside cid and fails unless it contains substr.
func assertContains(t *testing.T, cid, path, substr string) {
	t.Helper()
	out, err := dockerExecOutput(cid, "cat", path)
	if err != nil {
		t.Errorf("cat %s: %v\n%s", path, err, out)
		return
	}
	if !strings.Contains(out, substr) {
		t.Errorf("%s missing %q, got:\n%s", path, substr, out)
	}
}

func dockerRun(args ...string) (string, error) {
	out, err := exec.Command("docker", args...).CombinedOutput()
	return string(out), err
}

// repoRoot walks up from the test binary's CWD until a go.mod is found.
// We don't hard-code "../" because go test can be invoked with various
// working directories.
func repoRoot(t *testing.T) string {
	t.Helper()
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for dir := cwd; ; {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("no go.mod found above %s", cwd)
		}
		dir = parent
	}
}
