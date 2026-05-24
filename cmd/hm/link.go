package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/kurowski/homie/internal/link"
	"github.com/kurowski/homie/internal/repo"
	"github.com/spf13/cobra"
)

var (
	linkDryRun bool
	linkHome   string // override $HOME, mostly for testing
)

var linkCmd = &cobra.Command{
	Use:   "link",
	Short: "Symlink dotfiles/ into $HOME",
	RunE:  runLink,
}

func init() {
	linkCmd.Flags().BoolVar(&linkDryRun, "dry-run", false, "report what would change, don't write anything")
	linkCmd.Flags().StringVar(&linkHome, "home", "", "override target home directory (default $HOME)")
	rootCmd.AddCommand(linkCmd)
}

func runLink(cmd *cobra.Command, args []string) error {
	repoDir, err := repo.Find()
	if err != nil {
		return err
	}
	home := linkHome
	if home == "" {
		home, err = os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("resolve home: %w", err)
		}
	}

	actions, err := link.Plan(repoDir, home)
	if err != nil {
		return err
	}
	w := cmd.OutOrStdout()
	if len(actions) == 0 {
		fmt.Fprintln(w, "No dotfiles to link.")
		return nil
	}

	if linkDryRun {
		printPlan(w, home, actions)
		return nil
	}

	res := link.Apply(actions, time.Now())
	printResult(w, home, res)
	if len(res.Errors) > 0 {
		return fmt.Errorf("%d error(s) during link", len(res.Errors))
	}
	return nil
}

func relTarget(home, target string) string {
	if rel, err := filepath.Rel(home, target); err == nil {
		return "~/" + rel
	}
	return target
}

func printPlan(w io.Writer, home string, actions []link.Action) {
	for _, a := range actions {
		fmt.Fprintf(w, "  %-8s %s\n", a.Kind, relTarget(home, a.Target))
	}
}

func printResult(w io.Writer, home string, res link.Result) {
	for _, a := range res.Created {
		fmt.Fprintf(w, "  create   %s\n", relTarget(home, a.Target))
	}
	for _, a := range res.Replaced {
		fmt.Fprintf(w, "  replace  %s\n", relTarget(home, a.Target))
	}
	for _, b := range res.Backed {
		fmt.Fprintf(w, "  backup   %s -> %s\n", relTarget(home, b.Action.Target), relTarget(home, b.Backup))
	}
	if len(res.Skipped) > 0 {
		fmt.Fprintf(w, "  skip     %d already in sync\n", len(res.Skipped))
	}
	for _, err := range res.Errors {
		fmt.Fprintf(w, "  error    %s\n", err)
	}
}
