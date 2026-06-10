package main

import (
	"github.com/kurowski/homie/internal/config"
	"github.com/kurowski/homie/internal/detect"
	"github.com/kurowski/homie/internal/render"
	"github.com/kurowski/homie/internal/repo"
	"github.com/spf13/cobra"
)

var contextCmd = &cobra.Command{
	Use:   "context",
	Short: "Print the template data model as JSON",
	Long: `Print the exact data passed to every *.tmpl render on this host,
as JSON. The keys match the template fields one-to-one — a key named
"Email" means a template may reference {{ .Email }} — so one read tells
you (or an automated agent) every field a template can use, with the
values it would resolve to right now.

Output is always JSON, pretty-printed, nothing else on stdout — safe to
pipe into jq or feed to a tool.

Two things templates can use that aren't data fields, so they don't
appear here: the hasTag helper ({{ if hasTag "fedora" }}), which tests
membership in Tags, and the Sprig function library.

Pairs with the render preview commands: introspect the context with
` + "`hm context`" + `, then check a template with ` + "`hm render <path>`" + ` or
` + "`hm home --dry-run`" + `. See https://homie.sh/docs/dotfiles/ for the
template data reference.`,
	Args: cobra.NoArgs,
	RunE: runContext,
}

func init() {
	rootCmd.AddCommand(contextCmd)
}

func runContext(cmd *cobra.Command, args []string) error {
	repoDir, err := repo.Find()
	if err != nil {
		return err
	}
	env := detect.Detect()
	cfg, err := config.Load(repoDir, env.Hostname)
	if err != nil {
		return err
	}
	// render.Data has no json tags on purpose: encoding/json then uses
	// the Go field names, which are exactly the template field names.
	// Adding a field to Data automatically surfaces it here.
	return writeJSON(cmd.OutOrStdout(), render.BuildData(cfg, env))
}
