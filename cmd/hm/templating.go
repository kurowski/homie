package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

var templateCmd = &cobra.Command{
	Use:     "templating",
	Aliases: []string{"template"},
	Short:  "Show the template data model and functions",
	Hidden: true,
	Long: `Templates use Go's text/template syntax extended with Sprig functions
(https://masterminds.github.io/sprig/) and a Homie-specific hasTag function.
They render from '<repo>/home/' (and active tag-conditional sibling trees)
with a .tmpl suffix, and the output is written to $HOME with the suffix
stripped.

=== Template Data ===

All of the following fields are available as {{ .<Field> }}:

  .Name          string         e.g. "Scout Homes"
  .Email         string         e.g. "scout@homie.sh"
  .Profile       string         the active profile name ("" if none)
  .DefaultShell  string         e.g. "/usr/bin/zsh"
  .Distro        string         e.g. "ubuntu", "fedora", "debian", "macos"
  .Arch          string         e.g. "amd64", "arm64"
  .IsContainer   bool
  .IsRoot        bool
  .Vars          map[string]any all keys from [vars] in homie.toml
  .Tags          []string       active tags from [tags] in homie.toml

=== Template Functions ===

  hasTag "<tag>"
      Returns true when <tag> is active (declared in [tags] and
      the current host). Useful with {{ if ... }} blocks:
        {{ if hasTag "work" }}...{{ else }}...{{ end }}

  All ~100 Sprig functions
      https://masterminds.github.io/sprig/
      Strings, math, list manipulation, filesystem, type coercion,
      cryptography, encoding, date formatting, etc.

For optional vars, use {{ if hasKey .Vars "KEY" }}...{{ end }} or
{{ dig "KEY" "fallback" .Vars }}. Missing fields (as opposed to
lookups with rich fallback helpers like dig, hasKey, default) will
error immediately so typos never produce hidden output.

=== More Info ===

  hm home --help       # how template and link files are partitioned
  hm doctor            # check for stale rendered files
  homie.sh/docs/dotfiles  # the full dotfiles/ template model
`,
	Run: func(cmd *cobra.Command, args []string) {
		_, _ = fmt.Fprint(cmd.OutOrStdout(), cmd.Long)
	},
}

func init() {
	rootCmd.AddCommand(templateCmd)
}
