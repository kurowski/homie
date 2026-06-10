package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/kurowski/homie/internal/selfupdate"
	"github.com/spf13/cobra"
)

var selfupdateCmd = &cobra.Command{
	Use:     "selfupdate",
	Aliases: []string{"self-update"},
	Short:   "Update the hm binary to the latest release",
	Long: `Replace the running hm binary with the newest GitHub release.

The latest tag is resolved from github.com/kurowski/homie/releases, the
binary for this OS and architecture is downloaded together with the
release's SHA256SUMS, and the checksum is verified before the running
binary is atomically replaced — the same checks the install script
performs. Pass --check to only report whether a newer release exists.

Updating writes to the directory the binary lives in: an hm in
/usr/local/bin usually needs ` + "`sudo hm selfupdate`" + `, while the default
user install in ~/.local/bin needs no root. Two kinds of installs
refuse to self-update: a build from source (rebuild it, or reinstall a
release via install.sh), and a Homebrew-managed binary (use
` + "`brew upgrade`" + ` so the cellar stays consistent).

Unlike most commands, selfupdate needs no environment repo — it works
anywhere the binary does.`,
	Args: cobra.NoArgs,
	RunE: runSelfupdate,
}

func init() {
	selfupdateCmd.Flags().Bool("check", false, "report whether a newer release exists without installing it")
	rootCmd.AddCommand(selfupdateCmd)
}

func runSelfupdate(cmd *cobra.Command, args []string) error {
	w := cmd.OutOrStdout()
	check, _ := cmd.Flags().GetBool("check")

	if !selfupdate.IsReleaseVersion(version) {
		return fmt.Errorf("this hm was built from source (version %s) — rebuild it, or reinstall a release with install.sh", version)
	}
	exe, err := selfupdate.ExecutablePath()
	if err != nil {
		return err
	}
	if selfupdate.BrewManaged(exe) {
		return errors.New("this hm is managed by Homebrew — update it with `brew upgrade` instead")
	}

	u := selfupdate.New()
	latest, err := u.Latest()
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "  current  %s\n", version)
	fmt.Fprintf(w, "  latest   %s\n", latest)
	if latest == version {
		fmt.Fprintln(w, "\nAlready up to date.")
		return nil
	}
	if check {
		fmt.Fprintln(w, "\nUpdate available — run `hm selfupdate` to install it.")
		return nil
	}

	asset := selfupdate.AssetName()
	fmt.Fprintf(w, "  fetch    %s\n", asset)
	bin, err := u.Fetch(latest)
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "  verify   sha256 ok\n")
	if err := selfupdate.Apply(exe, bin); err != nil {
		if errors.Is(err, os.ErrPermission) {
			return fmt.Errorf("cannot replace %s: %w — try `sudo hm selfupdate`", exe, err)
		}
		return err
	}
	fmt.Fprintf(w, "  install  %s\n", exe)
	fmt.Fprintf(w, "\nUpdated %s to %s.\n", version, latest)
	return nil
}
