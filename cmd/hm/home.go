package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/kurowski/homie/internal/config"
	"github.com/kurowski/homie/internal/detect"
	"github.com/kurowski/homie/internal/link"
	"github.com/kurowski/homie/internal/render"
	"github.com/kurowski/homie/internal/repo"
	"github.com/spf13/cobra"
)

var homeTarget string

var homeCmd = &cobra.Command{
	Use:   "home",
	Short: "Materialize home/ into $HOME (symlinks + rendered templates)",
	Long: `Walk the home/ tree (and any active home.tag-X[.tag-Y...]/
siblings) and place every file under $HOME.

Files are partitioned by suffix:

  - Plain files become symlinks. ` + "`home/.zshrc`" + ` → ` + "`~/.zshrc`" + ` is a real
    symlink into the repo, so editing ~/.zshrc edits the repo file
    directly. Conflicts (a real file already at the destination) are
    backed up to <path>.homie-backup-<timestamp> before linking.

  - Files ending in .tmpl are rendered through Go text/template + Sprig
    and written as real files with the .tmpl suffix stripped.
    ` + "`home/.gitconfig.tmpl`" + ` → ` + "`~/.gitconfig`" + `. The output is regenerated
    on every apply.

When two trees claim the same target, the more-specific tree (more
required tags in its directory name) wins. Same-specificity collisions
are an error — disambiguate by adding a tag or merging the files.

This is one phase of ` + "`hm apply`" + `. Run it alone to refresh dotfiles
without touching packages or scripts. See https://homie.sh/docs/dotfiles/
for the full model.`,
	RunE: runHomeCmd,
}

func init() {
	homeCmd.Flags().StringVar(&homeTarget, "home", "", "override target home directory (default $HOME)")
	rootCmd.AddCommand(homeCmd)
}

func runHomeCmd(cmd *cobra.Command, args []string) error {
	repoDir, err := repo.Find()
	if err != nil {
		return err
	}
	home := homeTarget
	if home == "" {
		home, err = os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("resolve home: %w", err)
		}
	}
	env := detect.Detect()
	cfg, err := config.Load(repoDir, env.Hostname)
	if err != nil {
		return err
	}

	errs := doHome(cmd.OutOrStdout(), repoDir, home, cfg, env)
	if len(errs) > 0 {
		return fmt.Errorf("%d error(s) during home", len(errs))
	}
	return nil
}

// doHome runs the link phase followed by the render phase against the
// same home tree and streams their actions to w. Shared by `hm home`
// and `hm apply`'s home phase so the output is identical either way.
func doHome(w io.Writer, repoDir, home string, cfg config.Config, env detect.Env) []error {
	var errs []error

	actions, err := link.Plan(repoDir, home, cfg.AllTags(env))
	if err != nil {
		fmt.Fprintf(w, "  error    %s\n", err)
		return []error{err}
	}
	linkRes := link.Apply(actions, time.Now())
	for _, a := range linkRes.Created {
		fmt.Fprintf(w, "  create   %s\n", relTarget(home, a.Target))
	}
	for _, a := range linkRes.Replaced {
		fmt.Fprintf(w, "  replace  %s\n", relTarget(home, a.Target))
	}
	for _, b := range linkRes.Backed {
		fmt.Fprintf(w, "  backup   %s -> %s\n", relTarget(home, b.Action.Target), relTarget(home, b.Backup))
	}
	if n := len(linkRes.Skipped); n > 0 {
		fmt.Fprintf(w, "  skip     %d symlinks already in sync\n", n)
	}
	errs = append(errs, linkRes.Errors...)

	renderRes := render.Apply(repoDir, home, cfg, env)
	for _, a := range renderRes.Rendered {
		fmt.Fprintf(w, "  render   %s\n", relTarget(home, a.Target))
	}
	if n := len(renderRes.Skipped); n > 0 {
		fmt.Fprintf(w, "  skip     %d templates already in sync\n", n)
	}
	for _, e := range renderRes.Errors {
		fmt.Fprintf(w, "  error    %s\n", e)
	}
	errs = append(errs, renderRes.Errors...)

	if len(linkRes.Created)+len(linkRes.Replaced)+len(linkRes.Backed)+len(linkRes.Skipped)+
		len(renderRes.Rendered)+len(renderRes.Skipped) == 0 && len(errs) == 0 {
		fmt.Fprintln(w, "Nothing in home/ to materialize.")
	}
	return errs
}

func relTarget(home, target string) string {
	if rel, err := filepath.Rel(home, target); err == nil {
		return "~/" + rel
	}
	return target
}
