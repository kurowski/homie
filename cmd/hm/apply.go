package main

import (
	"github.com/spf13/cobra"
)

var applyCmd = &cobra.Command{
	Use:   "apply",
	Short: "Full reconciliation: detect → packages → link → render → scripts",
	RunE:  notYetImplemented("apply"),
}

func init() {
	rootCmd.AddCommand(applyCmd)
}
