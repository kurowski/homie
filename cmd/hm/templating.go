package main

import (
	"github.com/spf13/cobra"
)

// templatingCmd is a help-only topic: it has no Run function, so cobra
// lists it under "Additional help topics" and `hm help templating`
// resolves to this reference instead of dead-ending. The point is to
// keep the template reference reachable offline, where the docs site
// isn't — keep the field table in sync with render.Data and the docs
// site's dotfiles page.
var templatingCmd = &cobra.Command{
	Use:     "templating",
	Aliases: []string{"template", "templates"},
	Short:   "Template data fields and helper functions",
	Long: `Files ending in .tmpl under home/ (and any active
home.tag-X[.tag-Y...]/ tree) render through Go text/template and land
in $HOME with the suffix stripped. This topic is the reference for what
a template can use; see ` + "`hm help home`" + ` for how files are partitioned
and overridden.

Data fields — the same set ` + "`hm context`" + ` prints with live values:

  .Name          string          [user].name
  .Email         string          [user].email
  .Profile       string          [profile].name
  .DefaultShell  string          [profile].default_shell
  .Distro        string          ubuntu | debian | fedora | macos | unknown
  .Arch          string          amd64 | arm64
  .IsContainer   bool            true in containers (docker, devcontainer, ...)
  .IsRoot        bool            true when running as root
  .Tags          []string        all active tags (auto + profile + extra)
  .Vars          map[string]any  the [vars] table: {{ .Vars.EDITOR }}

Custom function — Homie's own, not part of Sprig:

  hasTag "<name>"   true when the named tag is active on this host.
                    The idiomatic host-conditional branch:
                    {{ if hasTag "work" }}...{{ else }}...{{ end }}

Sprig: all ~100 functions from https://masterminds.github.io/sprig/
are bundled — default, hasKey, dig, quote, upper, join, and the rest.

Missing fields fail loudly. Homie sets missingkey=error, so a typo like
{{ .Eamil }} or an absent {{ .Vars.X }} is a render error instead of
"<no value>" in the output. default cannot rescue a missing key — the
lookup errors before default ever runs. For optional vars use:

  {{ if hasKey .Vars "X" }}{{ .Vars.X }}{{ end }}
  {{ dig "X" "fallback" .Vars }}

Preview tools: ` + "`hm context`" + ` prints the data as JSON, ` + "`hm render <path>`" + `
renders one template to stdout, ` + "`hm home --dry-run`" + ` previews every
active template. Full guide: https://homie.sh/docs/dotfiles/`,
}

func init() {
	rootCmd.AddCommand(templatingCmd)
}
