package main

import (
	"github.com/spf13/cobra"
)

var doctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "Check for broken symlinks, missing deps, common problems",
	RunE:  notYetImplemented("doctor"),
}

func init() {
	rootCmd.AddCommand(doctorCmd)
}
