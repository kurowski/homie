package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

func notYetImplemented(name string) func(*cobra.Command, []string) error {
	return func(cmd *cobra.Command, args []string) error {
		return fmt.Errorf("%s: not yet implemented", name)
	}
}
