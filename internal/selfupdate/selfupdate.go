// Package selfupdate replaces the running hm binary with a released
// one from GitHub. It performs the same steps as install.sh — download
// the os/arch binary plus the release's SHA256SUMS, verify, install —
// so the two update paths can't drift apart in what they check.
package selfupdate

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"
)

// DefaultBaseURL is the GitHub repository the released binaries are
// published under.
const DefaultBaseURL = "https://github.com/kurowski/homie"

// releaseTag matches the plain-semver tags Homie releases under. No
// pre-release or build suffixes — anything else is a local build.
var releaseTag = regexp.MustCompile(`^v\d+\.\d+\.\d+$`)

// IsReleaseVersion reports whether v is a released tag. Local builds
// carry "dev", "(devel)", or a git-describe string like
// v0.4.0-1-gabc1234-dirty, and must not be clobbered by an update.
func IsReleaseVersion(v string) bool {
	return releaseTag.MatchString(v)
}

// brewCellars are the cellar directories of the three Homebrew
// layouts: macOS arm64, macOS amd64, and Linux.
var brewCellars = []string{
	"/opt/homebrew/Cellar/",
	"/usr/local/Cellar/",
	"/home/linuxbrew/.linuxbrew/Cellar/",
}

// BrewManaged reports whether path points into a Homebrew cellar,
// where the formula owns the file and `brew upgrade` is the right
// updater. Callers pass a symlink-resolved path: every Homebrew layout
// links bin/hm into its Cellar, so the resolved path is the reliable
// signal. Anchored to the known cellar prefixes so an unrelated
// directory that happens to be named Cellar doesn't match.
func BrewManaged(path string) bool {
	for _, cellar := range brewCellars {
		if strings.HasPrefix(path, cellar) {
			return true
		}
	}
	return false
}

// AssetName is the release asset for this OS and architecture, named
// by the release workflow as hm-<os>-<arch>.
func AssetName() string {
	return "hm-" + runtime.GOOS + "-" + runtime.GOARCH
}

// ExecutablePath returns the running binary's real path, resolving any
// symlink (e.g. a ~/bin/hm -> ~/.local/bin/hm link) so Apply replaces
// the actual file rather than the link.
func ExecutablePath() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	return filepath.EvalSymlinks(exe)
}

// Updater resolves and downloads released binaries. BaseURL is the
// GitHub repository URL; tests point it at an httptest server.
type Updater struct {
	BaseURL string
	Client  *http.Client
}

// New returns an Updater against the real release repository.
func New() *Updater {
	return &Updater{
		BaseURL: DefaultBaseURL,
		Client:  &http.Client{Timeout: 5 * time.Minute},
	}
}

// Latest resolves the newest release tag without the GitHub API:
// <repo>/releases/latest answers with a redirect to
// <repo>/releases/tag/<tag>, and reading the Location header avoids
// the API's unauthenticated rate limit.
func (u *Updater) Latest() (string, error) {
	client := *u.Client
	client.CheckRedirect = func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}
	resp, err := client.Get(u.BaseURL + "/releases/latest")
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	loc := resp.Header.Get("Location")
	if resp.StatusCode < 300 || resp.StatusCode > 399 || loc == "" {
		return "", fmt.Errorf("expected a redirect from %s/releases/latest, got %s", u.BaseURL, resp.Status)
	}
	parsed, err := url.Parse(loc)
	if err != nil {
		return "", err
	}
	tag := path.Base(parsed.Path)
	if !IsReleaseVersion(tag) {
		return "", fmt.Errorf("could not resolve a release tag from redirect to %q", loc)
	}
	return tag, nil
}

// Fetch downloads the release's SHA256SUMS and then the binary for
// this OS/arch, returning the binary only after its checksum verifies.
// The checklist comes first: it's tiny, so a half-published release
// (no SHA256SUMS, no entry for this asset) fails fast before the
// multi-megabyte binary download.
func (u *Updater) Fetch(tag string) ([]byte, error) {
	base := u.BaseURL + "/releases/download/" + tag + "/"
	asset := AssetName()
	sums, err := u.get(base + "SHA256SUMS")
	if err != nil {
		return nil, err
	}
	want, err := sumFor(sums, asset)
	if err != nil {
		return nil, err
	}
	bin, err := u.get(base + asset)
	if err != nil {
		return nil, err
	}
	if got := sha256.Sum256(bin); hex.EncodeToString(got[:]) != want {
		return nil, fmt.Errorf("checksum mismatch for %s", asset)
	}
	return bin, nil
}

func (u *Updater) get(url string) ([]byte, error) {
	resp, err := u.Client.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET %s: %s", url, resp.Status)
	}
	return io.ReadAll(resp.Body)
}

// sumFor finds the checksum for name in a sha256sum-format checklist
// ("<hex>  <name>" per line; SHA256SUMS carries one line per published
// os/arch).
func sumFor(sums []byte, name string) (string, error) {
	for line := range strings.SplitSeq(string(sums), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 && fields[1] == name {
			return fields[0], nil
		}
	}
	return "", fmt.Errorf("SHA256SUMS has no entry for %s", name)
}

// Apply atomically replaces the binary at target with data. The new
// bytes are written to a temp file in target's own directory — same
// filesystem, so the rename is atomic — and renamed into place, which
// is safe on Linux and macOS even while target is the running
// executable. The replacement keeps target's permission bits.
func Apply(target string, data []byte) error {
	mode := os.FileMode(0o755)
	if fi, err := os.Stat(target); err == nil {
		mode = fi.Mode().Perm()
	}
	tmp, err := os.CreateTemp(filepath.Dir(target), ".hm-selfupdate-*")
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(tmp.Name()) }() // already gone if the rename landed
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Chmod(mode); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), target)
}
