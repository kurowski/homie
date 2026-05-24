package main

import (
	"github.com/spf13/cobra"
)

var installCmd = &cobra.Command{
	Use:   "install",
	Short: "Install packages declared in homie.toml",
	RunE:  notYetImplemented("install"),
}

func init() {
	rootCmd.AddCommand(installCmd)
}
