// Package render evaluates Go text/template files (those ending in
// .tmpl) under <repo>/home/ and any active home.tag-X[.tag-Y...]
// siblings, and writes the output to corresponding paths under $HOME
// with the .tmpl suffix stripped.
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
	"text/template"

	"github.com/Masterminds/sprig/v3"
	"github.com/kurowski/homie/internal/config"
	"github.com/kurowski/homie/internal/detect"
	"github.com/kurowski/homie/internal/tree"
)

// Data is what's available inside a template via {{ .Name }} etc.
// Vars is map[string]any (not map[string]string) so Sprig helpers like
// hasKey and dig work — they require the any-valued type.
//
// No json tags: `hm context` marshals this struct directly so the JSON
// keys are the template field names verbatim. Keep it that way — a new
// field is then discoverable via `hm context` with no extra wiring.
// A new field DOES need adding to the two hand-written field tables:
// cmd/hm/templating.go (`hm help templating`) and the docs site's
// dotfiles page.
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
			// Custom funcs are documented in cmd/hm/templating.go
			// (`hm help templating`) and the docs site — keep in sync.
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
	Rendered []Action // actually written this run
	Skipped  []Action // already in sync; no write performed
	Errors   []error
}

// Apply renders every template resolved out of the active home trees
// into <home> with the .tmpl suffix stripped. Non-template files in
// the same trees are owned by link.Plan and ignored here.
//
// Collision handling lives in tree.Resolve: more-specific tree wins
// per target, same-specificity collisions return an error from
// Resolve. If Resolve errors, Apply returns it as the single entry in
// Result.Errors with no rendering performed; per-file render errors
// (template parse failures, write errors, etc.) are still collected
// individually so one bad template doesn't block the rest.
func Apply(repoDir, home string, cfg config.Config, env detect.Env) Result {
	var res Result
	resolved, err := tree.Resolve(repoDir, home, cfg.AllTags(env))
	if err != nil {
		res.Errors = append(res.Errors, err)
		return res
	}
	data := BuildData(cfg, env)
	for _, r := range resolved {
		if !r.IsTemplate {
			continue // link.Plan owns this file
		}
		written, err := renderFile(r.Source, r.Target, data)
		if err != nil {
			res.Errors = append(res.Errors, fmt.Errorf("%s: %w", r.Source, err))
			continue
		}
		action := Action{Source: r.Source, Target: r.Target}
		if written {
			res.Rendered = append(res.Rendered, action)
		} else {
			res.Skipped = append(res.Skipped, action)
		}
	}
	return res
}

// renderFile reports whether the target was (re)written. A return of
// (false, nil) means the target was already in sync with the rendered
// output and the source's mode — nothing on disk changed.
func renderFile(source, target string, data Data) (bool, error) {
	raw, err := os.ReadFile(source)
	if err != nil {
		return false, err
	}
	info, err := os.Stat(source)
	if err != nil {
		return false, err
	}
	out, err := Render(string(raw), data)
	if err != nil {
		return false, err
	}
	outBytes := []byte(out)
	if inSync(target, outBytes, info.Mode()) {
		return false, nil
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return false, err
	}
	// Remove any existing file or symlink so we don't write through a stale
	// symlink to wherever it points.
	if err := os.Remove(target); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return false, err
	}
	if err := os.WriteFile(target, outBytes, info.Mode().Perm()); err != nil {
		return false, err
	}
	return true, nil
}

// inSync reports whether target is a regular file whose contents and
// permission bits already match the rendered output. Symlinks count as
// out-of-sync — they must be replaced with the real rendered file.
func inSync(target string, want []byte, mode os.FileMode) bool {
	info, err := os.Lstat(target)
	if err != nil || info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return false
	}
	if info.Mode().Perm() != mode.Perm() {
		return false
	}
	have, err := os.ReadFile(target)
	if err != nil {
		return false
	}
	return bytes.Equal(have, want)
}
