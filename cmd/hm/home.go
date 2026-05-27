package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/kurowski/homie/internal/config"
	"github.com/kurowski/homie/internal/detect"
	"github.com/kurowski/homie/internal/repo"
	"github.com/kurowski/homie/internal/ui"
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

	// Route through the same phase apply uses so the output is
	// byte-identical between `hm home` and `hm apply`'s home section.
	noTTY, _ := cmd.Root().PersistentFlags().GetBool("no-tty")
	u := ui.New(cmd.OutOrStdout(), noTTY)
	defer func() { _ = u.Close() }()

	errs := applyHomePhase(u, repoDir, home, cfg, env)
	u.Summary(errs)
	if len(errs) > 0 {
		return fmt.Errorf("%d error(s) during home", len(errs))
	}
	return nil
}

// relTarget formats target as a ~/-relative path when it's inside
// home, otherwise returns the absolute path. Shared by apply.go and
// home.go for consistent action-line formatting.
func relTarget(home, target string) string {
	if rel, err := filepath.Rel(home, target); err == nil {
		return "~/" + rel
	}
	return target
}
