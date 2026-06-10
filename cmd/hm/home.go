package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/kurowski/homie/internal/config"
	"github.com/kurowski/homie/internal/detect"
	"github.com/kurowski/homie/internal/render"
	"github.com/kurowski/homie/internal/repo"
	"github.com/kurowski/homie/internal/tree"
	"github.com/kurowski/homie/internal/ui"
	"github.com/spf13/cobra"
)

var (
	homeTarget string
	homeDryRun bool
)

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

With --dry-run, nothing is written: the plan (every link and render
target with its source) is printed instead, followed by the full
rendered content of each template. Use it to preview what a template
resolves to on this host before applying for real; ` + "`hm render <path>`" + `
does the same for a single file.

This is one phase of ` + "`hm apply`" + `. Run it alone to refresh dotfiles
without touching packages or scripts. See https://homie.sh/docs/dotfiles/
for the full model.`,
	RunE: runHomeCmd,
}

func init() {
	homeCmd.Flags().StringVar(&homeTarget, "home", "", "override target home directory (default $HOME)")
	homeCmd.Flags().BoolVar(&homeDryRun, "dry-run", false, "print the plan and rendered template content, write nothing")
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

	if homeDryRun {
		return dryRunHome(cmd.OutOrStdout(), repoDir, home, cfg, env)
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

// dryRunHome prints what the home phase would do — every resolved link
// and render target with its winning source — then the full rendered
// content of each template, writing nothing to $HOME. Output is plain
// (no TUI): it's an inspection surface meant for reading and piping.
func dryRunHome(w io.Writer, repoDir, home string, cfg config.Config, env detect.Env) error {
	resolved, err := tree.Resolve(repoDir, home, cfg.AllTags(env))
	if err != nil {
		return err
	}
	if len(resolved) == 0 {
		fmt.Fprintln(w, "nothing in home/")
		return nil
	}
	for _, r := range resolved {
		verb := "link"
		if r.IsTemplate {
			verb = "render"
		}
		fmt.Fprintf(w, "%-6s %s <- %s\n", verb, relTarget(home, r.Target), tree.RelTo(repoDir, r.Source))
	}
	data := render.BuildData(cfg, env)
	var failed []string
	for _, r := range resolved {
		if !r.IsTemplate {
			continue
		}
		fmt.Fprintf(w, "\n--- %s (rendered from %s)\n", relTarget(home, r.Target), tree.RelTo(repoDir, r.Source))
		out, err := renderSource(r.Source, data)
		if err != nil {
			fmt.Fprintf(w, "error: %v\n", err)
			failed = append(failed, tree.RelTo(repoDir, r.Source))
			continue
		}
		fmt.Fprint(w, out)
		if !strings.HasSuffix(out, "\n") {
			fmt.Fprintln(w)
		}
	}
	// Name the failures in the returned error: the inline error lines
	// above live on stdout among the rendered content, so this is what
	// makes a CI run's stderr say which templates broke.
	if len(failed) > 0 {
		return fmt.Errorf("%d template(s) failed to render: %s", len(failed), strings.Join(failed, ", "))
	}
	return nil
}

func renderSource(source string, data render.Data) (string, error) {
	raw, err := os.ReadFile(source)
	if err != nil {
		return "", err
	}
	return render.Render(string(raw), data)
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
