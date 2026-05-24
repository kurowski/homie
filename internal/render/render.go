// Package render evaluates Go text/template files under <repo>/templates/
// and writes the output to corresponding paths under $HOME.
//
// Templates use Go's text/template syntax extended with Sprig
// (https://masterminds.github.io/sprig/) plus a hasTag helper. Unlike
// internal/link, the output is a real file — not a symlink — because
// content is generated from data, not stored as canonical source.
package render

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/Masterminds/sprig/v3"
	"github.com/kurowski/homie/internal/config"
	"github.com/kurowski/homie/internal/detect"
)

// TemplatesDir is the directory under the user repo that holds .tmpl files.
const TemplatesDir = "templates"

// Extension is the suffix stripped from template filenames during rendering.
const Extension = ".tmpl"

// Data is what's available inside a template via {{ .Name }} etc.
// Vars is map[string]any (not map[string]string) so Sprig helpers like
// hasKey and dig work — they require the any-valued type.
type Data struct {
	Name         string
	Email        string
	Profile      string
	DefaultShell string
	Distro       string
	Arch         string
	IsContainer  bool
	IsRoot       bool
	Tags         []string
	Vars         map[string]any
}

// BuildData composes Data from a Config and an Env.
func BuildData(cfg config.Config, env detect.Env) Data {
	vars := make(map[string]any, len(cfg.Vars))
	for k, v := range cfg.Vars {
		vars[k] = v
	}
	return Data{
		Name:         cfg.User.Name,
		Email:        cfg.User.Email,
		Profile:      cfg.Profile.Name,
		DefaultShell: cfg.Profile.DefaultShell,
		Distro:       env.Distro,
		Arch:         env.Arch,
		IsContainer:  env.IsContainer,
		IsRoot:       env.IsRoot,
		Tags:         cfg.AllTags(env),
		Vars:         vars,
	}
}

// Render parses input as a Go text/template and executes it with data.
// Missing fields fail loudly (missingkey=error) so typos surface immediately
// instead of producing "<no value>" in the output. For optional vars in
// templates, use `{{ if hasKey .Vars "X" }}...{{ end }}` or sprig's
// `{{ dig "X" "fallback" .Vars }}` — `default` cannot rescue a missing key
// under missingkey=error.
func Render(input string, data Data) (string, error) {
	tmpl, err := template.New("homie").
		Funcs(sprig.TxtFuncMap()).
		Funcs(template.FuncMap{
			"hasTag": hasTagFn(data.Tags),
		}).
		Option("missingkey=error").
		Parse(input)
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", err
	}
	return buf.String(), nil
}

func hasTagFn(tags []string) func(string) bool {
	set := make(map[string]struct{}, len(tags))
	for _, t := range tags {
		set[t] = struct{}{}
	}
	return func(name string) bool {
		_, ok := set[name]
		return ok
	}
}

// Action records a template that was rendered.
type Action struct {
	Source string // absolute path of the .tmpl
	Target string // absolute path of the rendered output
}

// Result aggregates the outcome of Apply.
type Result struct {
	Rendered []Action
	Errors   []error
}

// Apply walks <repoDir>/templates and renders every *.tmpl into <home> with
// the .tmpl suffix stripped. If the templates directory is missing, Apply
// returns an empty result with no error. Non-fatal per-file errors are
// collected in Result.Errors so the rest of `hm apply` can continue.
func Apply(repoDir, home string, cfg config.Config, env detect.Env) Result {
	var res Result
	src := filepath.Join(repoDir, TemplatesDir)
	info, err := os.Stat(src)
	if errors.Is(err, fs.ErrNotExist) {
		return res
	}
	if err != nil {
		res.Errors = append(res.Errors, fmt.Errorf("stat %s: %w", src, err))
		return res
	}
	if !info.IsDir() {
		res.Errors = append(res.Errors, fmt.Errorf("%s is not a directory", src))
		return res
	}

	data := BuildData(cfg, env)
	_ = filepath.WalkDir(src, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			res.Errors = append(res.Errors, walkErr)
			return nil
		}
		if d.IsDir() || !strings.HasSuffix(d.Name(), Extension) {
			return nil
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			res.Errors = append(res.Errors, err)
			return nil
		}
		target := filepath.Join(home, strings.TrimSuffix(rel, Extension))
		if err := renderFile(path, target, data); err != nil {
			res.Errors = append(res.Errors, fmt.Errorf("%s: %w", path, err))
			return nil
		}
		res.Rendered = append(res.Rendered, Action{Source: path, Target: target})
		return nil
	})
	return res
}

func renderFile(source, target string, data Data) error {
	raw, err := os.ReadFile(source)
	if err != nil {
		return err
	}
	info, err := os.Stat(source)
	if err != nil {
		return err
	}
	out, err := Render(string(raw), data)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	// Remove any existing file or symlink so we don't write through a stale
	// symlink to wherever it points.
	if err := os.Remove(target); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return os.WriteFile(target, []byte(out), info.Mode())
}
