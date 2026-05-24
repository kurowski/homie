package main

import (
	"github.com/spf13/cobra"
)

var linkCmd = &cobra.Command{
	Use:   "link",
	Short: "Symlink dotfiles/ into $HOME",
	RunE:  notYetImplemented("link"),
}

func init() {
	rootCmd.AddCommand(linkCmd)
}
