package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/kurowski/homie/internal/config"
	"github.com/kurowski/homie/internal/repo"
	"github.com/kurowski/homie/internal/scaffold"
	"github.com/spf13/cobra"
)

// runInitUpdate is `hm init --update`: re-render the generated files in
// an existing environment repo. Where a fresh init asks the user for
// everything, update derives it — the answers are already in the repo,
// and a refresh you have to re-answer isn't a repeatable path.
func runInitUpdate(cmd *cobra.Command, args []string) error {
	stdout := cmd.OutOrStdout()

	dir, err := updateTarget(args)
	if err != nil {
		return err
	}
	answers, err := deriveAnswers(cmd, dir)
	if err != nil {
		return err
	}

	results, err := scaffold.Update(dir, answers, version, initForce)
	if err != nil {
		return err
	}

	var changed, skipped int
	for _, r := range results {
		fmt.Fprintf(stdout, "  %-14s %s\n", r.Path, describe(r))
		switch r.State {
		case scaffold.StateCustomized:
			skipped++
		case scaffold.StateCurrent:
		default:
			changed++
		}
	}

	for _, r := range results {
		if !r.Skipped() {
			continue
		}
		fmt.Fprintf(stdout, "\nWhat --update would write to %s:\n\n", r.Path)
		if err := showDiff(stdout, filepath.Join(dir, r.Path), r); err != nil {
			fmt.Fprintf(stdout, "  (couldn't render a diff: %v)\n", err)
		}
	}

	fmt.Fprintln(stdout)
	switch {
	case skipped > 0 && changed == 0:
		fmt.Fprintf(stdout, "Nothing written. Take Homie's version with `hm init --update --force`,\n")
		fmt.Fprintf(stdout, "or keep yours — deleting the hm:generated line opts the file out for good.\n")
	case skipped > 0:
		fmt.Fprintf(stdout, "Wrote %d file(s), skipped %d you've edited. Review with `git diff`, then commit.\n", changed, skipped)
	case changed > 0:
		fmt.Fprintf(stdout, "Wrote %d file(s) in %s. Review with `git diff`, then commit.\n", changed, dir)
	default:
		fmt.Fprintf(stdout, "Everything already current for hm %s.\n", version)
	}
	return nil
}

// describe renders one Result as a status column.
func describe(r scaffold.Result) string {
	switch r.State {
	case scaffold.StateCurrent:
		return fmt.Sprintf("current   (hm %s)", r.To)
	case scaffold.StateCreated:
		return fmt.Sprintf("created   (hm %s)", r.To)
	case scaffold.StateUpdated, scaffold.StateForced:
		verb := "updated"
		if r.State == scaffold.StateForced {
			verb = "forced "
		}
		return fmt.Sprintf("%s   %s → %s", verb, versionOrUnknown(r.From), r.To)
	default:
		if r.From == "" {
			return "skipped   no hm:generated stamp — treating it as yours"
		}
		return fmt.Sprintf("skipped   edited since hm %s wrote it", r.From)
	}
}

func versionOrUnknown(v string) string {
	if v == "" {
		return "(unstamped)"
	}
	return v
}

// updateTarget resolves which repo to refresh: the argument if given,
// otherwise the same walk-up every other command uses, so `hm init
// --update` works from anywhere inside the repo.
func updateTarget(args []string) (string, error) {
	if len(args) == 1 {
		abs, err := filepath.Abs(args[0])
		if err != nil {
			return "", fmt.Errorf("resolve %s: %w", args[0], err)
		}
		return abs, nil
	}
	dir, err := repo.Find()
	if err != nil {
		return "", fmt.Errorf("%w — run --update from inside your environment repo, or pass its path", err)
	}
	return dir, nil
}

// deriveAnswers reconstructs the scaffold inputs from the repo itself:
// identity from homie.toml, GitHub coordinates from the origin remote.
// Explicit flags win, which is also the escape hatch for a repo with no
// remote (a local-only or differently-hosted repo).
func deriveAnswers(cmd *cobra.Command, dir string) (scaffold.Answers, error) {
	cfg, err := config.Load(dir, "")
	if err != nil {
		return scaffold.Answers{}, fmt.Errorf("load %s: %w", filepath.Join(dir, "homie.toml"), err)
	}
	home, _ := os.UserHomeDir()
	a := scaffold.Answers{
		Name:         cfg.User.Name,
		Email:        cfg.User.Email,
		Profile:      cfg.Profile.Name,
		DefaultShell: cfg.Profile.DefaultShell,
		GitHubUser:   initGitHubUser,
		GitHubRepo:   initGitHubRepo,
		// The repo is sitting right here, so where a fresh machine
		// should clone it isn't a question either.
		RepoDir: scaffold.CloneTarget(dir, home),
	}
	if cmd.Flags().Changed("github-user") && cmd.Flags().Changed("github-repo") {
		return a, nil
	}

	user, name, err := githubFromRemote(dir)
	if err != nil {
		if !cmd.Flags().Changed("github-user") {
			return scaffold.Answers{}, fmt.Errorf("%w\npass --github-user and --github-repo to say where this repo lives", err)
		}
		return a, nil
	}
	if !cmd.Flags().Changed("github-user") {
		a.GitHubUser = user
	}
	if !cmd.Flags().Changed("github-repo") {
		a.GitHubRepo = name
	}
	return a, nil
}

// githubFromRemote reads owner and repo out of the origin URL, handling
// both forms git hands out: https://github.com/owner/repo.git and
// git@github.com:owner/repo.git.
func githubFromRemote(dir string) (user, name string, err error) {
	out, err := exec.Command("git", "-C", dir, "remote", "get-url", "origin").Output()
	if err != nil {
		return "", "", fmt.Errorf("no git remote 'origin' in %s", dir)
	}
	url := strings.TrimSpace(string(out))
	trimmed := strings.TrimSuffix(url, ".git")
	// Normalize the scp-style separator so one split handles both forms.
	trimmed = strings.ReplaceAll(trimmed, ":", "/")
	parts := strings.Split(strings.Trim(trimmed, "/"), "/")
	if len(parts) < 2 {
		return "", "", fmt.Errorf("can't read owner/repo out of origin URL %q", url)
	}
	return parts[len(parts)-2], parts[len(parts)-1], nil
}

// showDiff prints what --update would write, using git as the diff
// engine: Homie already depends on git, and this is the same output the
// user will read in `git diff` if they go ahead.
//
// Both sides are staged into a scratch tree as current/<name> and
// proposed/<name> and diffed with relative paths from there, so the
// headers read as those two words instead of a pair of absolute temp
// paths. The proposed side is written with the real mode so git doesn't
// report a mode change that isn't part of the update.
func showDiff(w io.Writer, current string, r scaffold.Result) error {
	tmp, err := os.MkdirTemp("", "hm-update-")
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(tmp) }()

	have, err := os.ReadFile(current)
	if err != nil {
		return err
	}
	name := filepath.Base(r.Path)
	sides := []struct {
		dir  string
		body []byte
	}{{"current", have}, {"proposed", r.Want}}
	for _, s := range sides {
		if err := os.Mkdir(filepath.Join(tmp, s.dir), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(tmp, s.dir, name), s.body, r.Mode); err != nil {
			return err
		}
	}

	c := exec.Command("git", "diff", "--no-index",
		"--src-prefix=", "--dst-prefix=", "--",
		filepath.Join("current", name), filepath.Join("proposed", name))
	c.Dir = tmp
	c.Stdout = w
	c.Stderr = w
	// git diff exits 1 when the files differ, which is the whole point.
	if err := c.Run(); err != nil {
		var exit *exec.ExitError
		if !errors.As(err, &exit) || exit.ExitCode() > 1 {
			return err
		}
	}
	return nil
}
