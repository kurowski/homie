//go:build e2e

package e2e_test

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// webserver is the on-disk layout consumed by the nginx container. Built
// once per test run; mounted read-only into nginx. certsDir points at
// the committed fixtures under e2e/certs so nginx and the distro
// containers share the same trust chain across runs.
type webserver struct {
	rootDir  string // tmpdir containing nginx.conf + content/
	certsDir string // <module>/e2e/certs
}

const (
	userrepoOwner = "scouthomes"
	userrepoName  = "dotfiles"
	hmOwner       = "kurowski"
	hmName        = "homie"
)

// prepWebserver builds everything nginx will serve:
//   - the hm release artifacts (binary + SHA256SUMS) at the same path
//     shape used by GitHub Releases
//   - a bare git repo of the user environment, produced by running the
//     real `hm init` binary
//   - bootstrap.sh exposed at /<owner>/<repo>/main/bootstrap.sh on the
//     raw.githubusercontent.com vhost
//   - nginx.conf wired against the committed test CA + leaf cert
func prepWebserver(t *testing.T, hmBinary string) *webserver {
	t.Helper()

	root := t.TempDir()
	w := &webserver{
		rootDir:  root,
		certsDir: filepath.Join(repoRoot(t), "e2e", "certs"),
	}

	// 1. Run the real `hm init` (the same binary we serve to test
	// containers) so the entire user repo — homie.toml, bootstrap.sh,
	// .zshrc, .gitconfig.tmpl, 01-shell.sh — comes from the production
	// command path. The test asserts post-state against these scaffold
	// defaults verbatim; nothing is overlaid.
	repoSrc := filepath.Join(root, "userrepo-src")
	runOrFail(t, "", hmBinary, "init",
		"--name", "Scout Homes",
		"--email", "scout@homie.sh",
		"--github-user", userrepoOwner,
		"--github-repo", userrepoName,
		"--profile", "personal",
		"--shell", "bash",
		repoSrc,
	)

	// 2. Init the user repo as git and make a bare clone — that bare
	// clone is what nginx exposes as `https://github.com/<owner>/<repo>.git`.
	gitInit(t, repoSrc)
	bareDir := filepath.Join(root, "content", "github", userrepoOwner, userrepoName+".git")
	if err := os.MkdirAll(filepath.Dir(bareDir), 0o755); err != nil {
		t.Fatal(err)
	}
	runOrFail(t, "", "git", "clone", "--bare", repoSrc, bareDir)
	// Dumb-HTTP serving needs server-info up to date.
	runOrFail(t, bareDir, "git", "update-server-info")

	// 3. Release artifacts at the github.com path GitHub uses.
	relDir := filepath.Join(root, "content", "github",
		hmOwner, hmName, "releases", "latest", "download")
	if err := os.MkdirAll(relDir, 0o755); err != nil {
		t.Fatal(err)
	}
	binName := "hm-linux-" + runtime.GOARCH
	binDst := filepath.Join(relDir, binName)
	if err := copyFile(hmBinary, binDst, 0o755); err != nil {
		t.Fatalf("copy hm binary: %v", err)
	}
	sum, err := sha256Hex(binDst)
	if err != nil {
		t.Fatalf("sha256 hm binary: %v", err)
	}
	sumLine := fmt.Sprintf("%s  %s\n", sum, binName)
	if err := os.WriteFile(filepath.Join(relDir, "SHA256SUMS"), []byte(sumLine), 0o644); err != nil {
		t.Fatal(err)
	}

	// 4. Raw view of bootstrap.sh — the URL pattern used by the
	// curl|bash invocation we're testing.
	rawDir := filepath.Join(root, "content", "raw", userrepoOwner, userrepoName, "main")
	if err := os.MkdirAll(rawDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := copyFile(
		filepath.Join(repoSrc, "bootstrap.sh"),
		filepath.Join(rawDir, "bootstrap.sh"),
		0o755,
	); err != nil {
		t.Fatalf("copy bootstrap.sh: %v", err)
	}

	// 5. nginx.conf — two HTTPS server blocks on one cert.
	if err := os.WriteFile(filepath.Join(root, "nginx.conf"), []byte(nginxConf), 0o644); err != nil {
		t.Fatal(err)
	}

	return w
}

// gitInit makes dir a git repo with one initial commit on main. We set
// committer/author via -c so the test doesn't depend on host git config.
func gitInit(t *testing.T, dir string) {
	t.Helper()
	id := []string{
		"-c", "init.defaultBranch=main",
		"-c", "user.name=Scout Homes",
		"-c", "user.email=scout@homie.sh",
	}
	runOrFail(t, dir, "git", append(id, "init")...)
	runOrFail(t, dir, "git", append(id, "add", "-A")...)
	runOrFail(t, dir, "git", append(id, "commit", "-m", "initial")...)
}

// nginxConf serves:
//   - https://raw.githubusercontent.com/<owner>/<repo>/main/...  → /var/www/raw
//   - https://github.com/<owner>/homie/releases/latest/download/...
//   - https://github.com/<owner>/<repo>.git/...                  (dumb HTTP)
//
// Both server blocks share one cert; nginx picks via SNI. The "dumb HTTP"
// path is just static-file serving with the right MIME types so `git clone`
// falls back from smart HTTP and reads the bare repo over plain GETs.
const nginxConf = `
events {}

http {
    default_type application/octet-stream;
    types {
        text/plain                              txt sh;
        application/x-git-packed-objects-toc    .idx;
        application/x-git-packed-objects        .pack;
    }
    sendfile on;
    access_log /dev/stdout;
    error_log  /dev/stderr;

    server {
        listen 443 ssl;
        server_name raw.githubusercontent.com;
        ssl_certificate     /etc/nginx/certs/server.crt;
        ssl_certificate_key /etc/nginx/certs/server.key;

        root /var/www/raw;
        location / {
            try_files $uri =404;
        }
    }

    server {
        listen 443 ssl default_server;
        server_name github.com;
        ssl_certificate     /etc/nginx/certs/server.crt;
        ssl_certificate_key /etc/nginx/certs/server.key;

        root /var/www/github;

        # Bare git repos served via dumb HTTP. update-server-info was
        # run at prep time so info/refs is present.
        location ~ \.git(/.*)?$ {
            autoindex off;
        }

        location / {
            try_files $uri =404;
        }
    }
}
`

// startWebserver launches the nginx container on `network` with hostname
// aliases for the two domains we impersonate. Returns the container ID.
func startWebserver(t *testing.T, w *webserver, network string) string {
	t.Helper()

	args := []string{
		"run", "-d", "--rm",
		"--network", network,
		"--network-alias", "github.com",
		"--network-alias", "raw.githubusercontent.com",
		"-v", filepath.Join(w.rootDir, "nginx.conf") + ":/etc/nginx/nginx.conf:ro",
		"-v", w.certsDir + ":/etc/nginx/certs:ro",
		"-v", filepath.Join(w.rootDir, "content", "github") + ":/var/www/github:ro",
		"-v", filepath.Join(w.rootDir, "content", "raw") + ":/var/www/raw:ro",
		"nginx:alpine",
	}
	out, err := exec.Command("docker", args...).CombinedOutput()
	if err != nil {
		t.Fatalf("docker run nginx: %v\n%s", err, out)
	}
	return strings.TrimSpace(string(out))
}

func copyFile(src, dst string, mode os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}

func sha256Hex(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func runOrFail(t *testing.T, dir, name string, args ...string) {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%s %v: %v\n%s", name, args, err, out)
	}
}
