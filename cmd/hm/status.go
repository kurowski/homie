package main

import (
	"fmt"
	"strings"

	"github.com/kurowski/homie/internal/detect"
	"github.com/spf13/cobra"
)

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show sync state, no changes",
	RunE:  runStatus,
}

func init() {
	rootCmd.AddCommand(statusCmd)
}

func runStatus(cmd *cobra.Command, args []string) error {
	env := detect.Detect()
	w := cmd.OutOrStdout()
	fmt.Fprintln(w, "Environment:")
	fmt.Fprintf(w, "  Distro:          %s\n", env.Distro)
	fmt.Fprintf(w, "  Package manager: %s\n", env.PackageManager)
	fmt.Fprintf(w, "  Arch:            %s\n", env.Arch)
	fmt.Fprintf(w, "  Container:       %v\n", env.IsContainer)
	fmt.Fprintf(w, "  Root:            %v\n", env.IsRoot)
	fmt.Fprintf(w, "  Interactive:     %v\n", env.IsInteractive)
	fmt.Fprintf(w, "  Tags:            %s\n", strings.Join(env.Tags, ", "))
	return nil
}
