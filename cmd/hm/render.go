package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/kurowski/homie/internal/config"
	"github.com/kurowski/homie/internal/detect"
	"github.com/kurowski/homie/internal/render"
	"github.com/kurowski/homie/internal/repo"
	"github.com/spf13/cobra"
)

var renderCmd = &cobra.Command{
	Use:   "render <path>",
	Short: "Render one template to stdout, no writes",
	Long: `Render a single .tmpl file to stdout using the exact same data
a real ` + "`hm home`" + ` would use on this host — active tags, [vars], user
identity, distro, and the hasTag helper — without writing anything to
$HOME.

Use it as a tight authoring loop for templates: edit, render, inspect,
repeat. Because nothing is written, it's safe to run from CI or an
automated agent to verify a template resolves correctly on this machine
before applying it for real.

The path is tried as given first (absolute or relative to the current
directory), then relative to the repo root, so
` + "`hm render home/.gitconfig.tmpl`" + ` works from anywhere.

Output is the raw rendered content, suitable for piping. A parse or
execution error exits non-zero.

To preview every active template at once, use ` + "`hm home --dry-run`" + `.
See https://homie.sh/docs/dotfiles/ for the template data reference.`,
	Args: cobra.ExactArgs(1),
	RunE: runRender,
}

func init() {
	rootCmd.AddCommand(renderCmd)
}

func runRender(cmd *cobra.Command, args []string) error {
	repoDir, err := repo.Find()
	if err != nil {
		return err
	}
	env := detect.Detect()
	cfg, err := config.Load(repoDir, env.Hostname)
	if err != nil {
		return err
	}
	path := args[0]
	raw, err := os.ReadFile(path)
	if err != nil && !filepath.IsAbs(path) {
		// Not found from the cwd — fall back to repo-relative, so
		// `hm render home/.gitconfig.tmpl` works from anywhere.
		if alt, altErr := os.ReadFile(filepath.Join(repoDir, path)); altErr == nil {
			raw, err = alt, nil
		}
	}
	if err != nil {
		return err
	}
	out, err := render.Render(string(raw), render.BuildData(cfg, env))
	if err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	fmt.Fprint(cmd.OutOrStdout(), out)
	return nil
}
