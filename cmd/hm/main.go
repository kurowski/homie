package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var version = "dev"

var rootCmd = &cobra.Command{
	Use:   "hm",
	Short: "Homie — generic Linux environment manager",
	Long: `Homie (hm) manages dotfiles and provisions Linux environments from a
user-owned git repo of config. Symlinks rather than copies, no state file,
TOML config, ordered scripts.

See https://homie.sh for documentation.`,
	Version:       version,
	SilenceUsage:  true,
	SilenceErrors: true,
}

func init() {
	rootCmd.PersistentFlags().Bool("verbose", false, "verbose output")
	rootCmd.PersistentFlags().Bool("no-tty", false, "force plain output, no TUI")
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}
}
