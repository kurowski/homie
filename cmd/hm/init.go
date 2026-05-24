package main

import (
	"github.com/spf13/cobra"
)

var initCmd = &cobra.Command{
	Use:   "init [target-dir]",
	Short: "Scaffold a new user environment repo",
	Args:  cobra.MaximumNArgs(1),
	RunE:  notYetImplemented("init"),
}

func init() {
	rootCmd.AddCommand(initCmd)
}
