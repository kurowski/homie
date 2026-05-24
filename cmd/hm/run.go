package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/kurowski/homie/internal/config"
	"github.com/kurowski/homie/internal/detect"
	"github.com/kurowski/homie/internal/repo"
	"github.com/kurowski/homie/internal/runner"
	"github.com/spf13/cobra"
)

var runHome string

var runCmd = &cobra.Command{
	Use:   "run",
	Short: "Run scripts/ in order",
	RunE:  runRun,
}

func init() {
	runCmd.Flags().StringVar(&runHome, "home", "", "override target home directory (default $HOME)")
	rootCmd.AddCommand(runCmd)
}

func runRun(cmd *cobra.Command, args []string) error {
	repoDir, err := repo.Find()
	if err != nil {
		return err
	}
	home := runHome
	if home == "" {
		home, err = os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("resolve home: %w", err)
		}
	}
	cfg, err := config.Load(repoDir)
	if err != nil {
		return err
	}
	env := detect.Detect()

	w := cmd.OutOrStdout()
	res := runner.Run(repoDir, home, cfg, cfg.AllTags(env), w)
	if len(res.Ran) == 0 {
		fmt.Fprintln(w, "No scripts to run.")
		return nil
	}
	for _, r := range res.Ran {
		status := "ok"
		if r.Err != nil {
			status = "fail"
		}
		fmt.Fprintf(w, "  %-5s %s\n", status, filepath.Base(r.Path))
	}
	if len(res.Errors) > 0 {
		return fmt.Errorf("%d script(s) failed", len(res.Errors))
	}
	return nil
}
