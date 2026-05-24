package main

import (
	"fmt"
	"os"

	"github.com/kurowski/homie/internal/config"
	"github.com/kurowski/homie/internal/detect"
	"github.com/kurowski/homie/internal/render"
	"github.com/kurowski/homie/internal/repo"
	"github.com/spf13/cobra"
)

var renderHome string

var renderCmd = &cobra.Command{
	Use:   "render",
	Short: "Render templates/ into $HOME",
	RunE:  runRender,
}

func init() {
	renderCmd.Flags().StringVar(&renderHome, "home", "", "override target home directory (default $HOME)")
	rootCmd.AddCommand(renderCmd)
}

func runRender(cmd *cobra.Command, args []string) error {
	repoDir, err := repo.Find()
	if err != nil {
		return err
	}
	home := renderHome
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

	res := render.Apply(repoDir, home, cfg, env)
	w := cmd.OutOrStdout()
	if len(res.Rendered) == 0 && len(res.Errors) == 0 {
		fmt.Fprintln(w, "No templates to render.")
		return nil
	}
	for _, a := range res.Rendered {
		fmt.Fprintf(w, "  render   %s\n", relTarget(home, a.Target))
	}
	for _, err := range res.Errors {
		fmt.Fprintf(w, "  error    %s\n", err)
	}
	if len(res.Errors) > 0 {
		return fmt.Errorf("%d error(s) during render", len(res.Errors))
	}
	return nil
}
