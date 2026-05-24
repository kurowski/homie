package main

import (
	"github.com/spf13/cobra"
)

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show sync state, no changes",
	RunE:  notYetImplemented("status"),
}

func init() {
	rootCmd.AddCommand(statusCmd)
}
