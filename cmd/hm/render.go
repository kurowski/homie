package main

import (
	"github.com/spf13/cobra"
)

var renderCmd = &cobra.Command{
	Use:   "render",
	Short: "Render templates/ into $HOME",
	RunE:  notYetImplemented("render"),
}

func init() {
	rootCmd.AddCommand(renderCmd)
}
