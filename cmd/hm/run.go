package main

import (
	"github.com/spf13/cobra"
)

var runCmd = &cobra.Command{
	Use:   "run",
	Short: "Run scripts/ in order",
	RunE:  notYetImplemented("run"),
}

func init() {
	rootCmd.AddCommand(runCmd)
}
