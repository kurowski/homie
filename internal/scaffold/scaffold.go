// Package scaffold generates a fresh user environment repo. It's the
// guts of `hm init`: take a few answers, materialize a working
// homie.toml + bootstrap.sh + sample dotfile/template/script so the
// user has a runnable repo immediately.
//
// The source files live under files/ and are embedded into the binary
// so `hm init` has no runtime dependency on the Homie tool repo's
// layout. Files ending in .scaffold are rendered as Go text/templates
// against Answers; everything else is copied verbatim.
package scaffold

import (
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"text/template"
)

//go:embed all:files
var content embed.FS

// Answers carries the inputs for a scaffold run.
type Answers struct {
	Name         string // user's full name
	Email        string // primary email
	GitHubUser   string // GitHub username — used in bootstrap.sh
	GitHubRepo   string // repo name on GitHub (defaults to "dotfiles")
	Profile      string // "personal" | "work" | "devcontainer" | ...
	DefaultShell string // "zsh" | "bash" | "fish"
}

// entry describes one file in the scaffold output.
type entry struct {
	src      string      // path inside the embedded files/ tree
	dst      string      // relative output path
	mode     os.FileMode // mode to write the output with
	rendered bool        // if true, run through text/template with Answers
}

var manifest = []entry{
	{"files/homie.toml", "homie.toml", 0o644, true},
	{"files/bootstrap.sh", "bootstrap.sh", 0o755, true},
	{"files/README.md", "README.md", 0o644, true},
	// The leading-dot files have to be named without the dot in the
	// embed source so plain `//go:embed` would include them too; we use
	// `all:` so it doesn't actually matter, but renaming `gitignore` to
	// `.gitignore` at write time keeps git happy.
	{"files/gitignore", ".gitignore", 0o644, false},
	{"files/dotfiles/.zshrc", "dotfiles/.zshrc", 0o644, false},
	{"files/templates/.gitconfig.tmpl", "templates/.gitconfig.tmpl", 0o644, false},
	{"files/scripts/01-shell.sh", "scripts/01-shell.sh", 0o755, false},
}

// Run materializes a new user environment repo at targetDir. The
// directory is created if missing; existing files are NOT overwritten
// — running scaffold against a non-empty dir errors out so we don't
// clobber user work.
func Run(targetDir string, a Answers) error {
	if err := a.fillDefaults(); err != nil {
		return err
	}
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		return fmt.Errorf("create %s: %w", targetDir, err)
	}
	for _, e := range manifest {
		out := filepath.Join(targetDir, e.dst)
		if _, err := os.Stat(out); err == nil {
			return fmt.Errorf("%s already exists — refusing to overwrite", out)
		}
		raw, err := fs.ReadFile(content, e.src)
		if err != nil {
			return fmt.Errorf("read embedded %s: %w", e.src, err)
		}
		body := raw
		if e.rendered {
			body, err = render(string(raw), a)
			if err != nil {
				return fmt.Errorf("render %s: %w", e.src, err)
			}
		}
		if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
			return fmt.Errorf("mkdir %s: %w", filepath.Dir(out), err)
		}
		if err := os.WriteFile(out, body, e.mode); err != nil {
			return fmt.Errorf("write %s: %w", out, err)
		}
	}
	return nil
}

func (a *Answers) fillDefaults() error {
	if a.Name == "" {
		return fmt.Errorf("name is required")
	}
	if a.Email == "" {
		return fmt.Errorf("email is required")
	}
	if a.GitHubUser == "" {
		return fmt.Errorf("github user is required")
	}
	if a.GitHubRepo == "" {
		a.GitHubRepo = "dotfiles"
	}
	if a.Profile == "" {
		a.Profile = "personal"
	}
	if a.DefaultShell == "" {
		a.DefaultShell = "zsh"
	}
	return nil
}

func render(body string, a Answers) ([]byte, error) {
	t, err := template.New("scaffold").Option("missingkey=error").Parse(body)
	if err != nil {
		return nil, err
	}
	var buf []byte
	w := newBuffer(&buf)
	if err := t.Execute(w, a); err != nil {
		return nil, err
	}
	return buf, nil
}

// newBuffer wraps a *[]byte as an io.Writer so we can use text/template
// without pulling in bytes.Buffer for one allocation.
func newBuffer(b *[]byte) *byteBuffer { return &byteBuffer{b: b} }

type byteBuffer struct{ b *[]byte }

func (w *byteBuffer) Write(p []byte) (int, error) {
	*w.b = append(*w.b, p...)
	return len(p), nil
}
