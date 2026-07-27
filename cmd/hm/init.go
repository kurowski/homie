package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/kurowski/homie/internal/scaffold"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

var (
	initName       string
	initEmail      string
	initGitHubUser string
	initGitHubRepo string
	initProfile    string
	initShell      string
	initRepoDir    string
	initUpdate     bool
	initForce      bool
)

var initCmd = &cobra.Command{
	Use:   "init [target-dir]",
	Short: "Scaffold a new user environment repo",
	Long: `Create a fresh user environment repo at target-dir (or the
current directory if none given). The scaffold includes:

  - homie.toml      — your identity, profile, packages, vars
  - bootstrap.sh    — the curl|bash entrypoint for fresh machines
  - home/.zshrc     — sample plain dotfile (symlinked on apply)
  - home/.gitconfig.tmpl — sample template (rendered on apply)
  - scripts/01-shell.sh  — sample setup script
  - README.md, .gitignore

Interactive by default — prompts for name, email, GitHub user, profile,
shell. Pass the flags below for a non-interactive run, useful in CI:

  hm init \
    --name "Scout Homes" --email scout@homie.sh \
    --github-user scouthomes --profile personal ~/dotfiles

Init refuses to overwrite an existing homie.toml — your work is safe.
Run ` + "`hm apply`" + ` against the scaffolded repo to materialize it.

REFRESHING AN EXISTING REPO

Most of the scaffold is a seed: homie.toml, home/, scripts/ become
yours the moment they're written, and Homie never touches them again.
bootstrap.sh is the exception — it encodes how the current hm wants to
be launched, so it goes stale when you upgrade hm. Refresh it in place:

  cd ~/dotfiles && hm init --update

Update needs no answers — it reads them off the repo: name and email
from homie.toml, GitHub user/repo from the origin remote, and the clone
destination bootstrap.sh should use on the next machine from where this
repo actually lives ($HOME-relative when it's under $HOME). Move the
repo, re-run --update, commit. Pass --github-user / --github-repo if
there's no remote to read.

Generated files carry an ` + "`hm:generated`" + ` stamp recording the hm version
and a digest of the file as written. Update rewrites a file only when
that digest still matches — if you've edited it (or it predates stamps,
like every repo scaffolded before v0.5.2), update prints the diff and
stops. Pass --force to take Homie's version anyway, or delete the stamp
line to opt the file out for good. Either way the change lands in your
working tree, so ` + "`git diff`" + ` is the final review.`,
	Args: cobra.MaximumNArgs(1),
	RunE: runInit,
}

func init() {
	initCmd.Flags().StringVar(&initName, "name", "", "your full name (required if non-interactive)")
	initCmd.Flags().StringVar(&initEmail, "email", "", "your primary email (required if non-interactive)")
	initCmd.Flags().StringVar(&initGitHubUser, "github-user", "", "GitHub username for the bootstrap URL")
	initCmd.Flags().StringVar(&initGitHubRepo, "github-repo", "dotfiles", "GitHub repo name for the env repo")
	initCmd.Flags().StringVar(&initProfile, "profile", "personal", "profile name (personal | work | devcontainer | ...)")
	initCmd.Flags().StringVar(&initShell, "shell", "zsh", "default shell")
	initCmd.Flags().StringVar(&initRepoDir, "repo-dir", "", "where bootstrap.sh clones this repo on a fresh machine (default: derived, e.g. $HOME/dotfiles)")
	initCmd.Flags().BoolVar(&initUpdate, "update", false, "refresh generated files in an existing repo instead of scaffolding a new one")
	initCmd.Flags().BoolVar(&initForce, "force", false, "with --update: overwrite files you've edited since Homie wrote them")
	rootCmd.AddCommand(initCmd)
}

func runInit(cmd *cobra.Command, args []string) error {
	if initUpdate {
		return runInitUpdate(cmd, args)
	}
	if initForce {
		return fmt.Errorf("--force applies to --update; a fresh init never overwrites")
	}
	target := "."
	if len(args) == 1 {
		target = args[0]
	}
	abs, err := filepath.Abs(target)
	if err != nil {
		return fmt.Errorf("resolve %s: %w", target, err)
	}

	// Where the repo is being scaffolded is where bootstrap.sh should
	// clone it on the next machine — no reason to ask. Scaffolding
	// somewhere outside $HOME (a build dir, a CI checkout) says nothing
	// about where it'll live, so that falls back to the default.
	home, _ := os.UserHomeDir()
	repoDir := initRepoDir
	if repoDir == "" {
		repoDir, _ = scaffold.CloneTarget(abs, home)
	}
	answers := scaffold.Answers{
		Name:         initName,
		Email:        initEmail,
		GitHubUser:   initGitHubUser,
		GitHubRepo:   initGitHubRepo,
		Profile:      initProfile,
		DefaultShell: initShell,
		RepoDir:      repoDir,
	}

	stdin := cmd.InOrStdin()
	stdout := cmd.OutOrStdout()
	interactive := isTerminal(stdin)
	if interactive {
		if err := promptMissing(stdin, stdout, &answers); err != nil {
			return err
		}
	}

	if err := scaffold.Run(abs, answers, version); err != nil {
		return err
	}

	fmt.Fprintf(stdout, "Scaffolded %s\n", abs)
	fmt.Fprintln(stdout, "\nNext steps:")
	fmt.Fprintf(stdout, "  cd %s\n", target)
	fmt.Fprintln(stdout, "  git init && git add . && git commit -m 'initial homie scaffold'")
	fmt.Fprintf(stdout, "  hm apply --home %s   # try it against a sandbox first\n", filepath.Join(abs, ".test-home"))
	fmt.Fprintln(stdout, "  hm apply              # apply against $HOME for real")
	return nil
}

func promptMissing(stdin io.Reader, stdout io.Writer, a *scaffold.Answers) error {
	r := bufio.NewReader(stdin)
	prompts := []struct {
		label   string
		field   *string
		def     string
		require bool
	}{
		{"Your full name", &a.Name, "", true},
		{"Your email", &a.Email, "", true},
		{"GitHub username", &a.GitHubUser, "", true},
		{"GitHub repo name", &a.GitHubRepo, "dotfiles", false},
		{"Profile", &a.Profile, "personal", false},
		{"Default shell", &a.DefaultShell, "zsh", false},
	}
	for _, p := range prompts {
		if *p.field != "" {
			continue
		}
		fmt.Fprintf(stdout, "%s", p.label)
		if p.def != "" {
			fmt.Fprintf(stdout, " [%s]", p.def)
		}
		fmt.Fprint(stdout, ": ")
		line, err := r.ReadString('\n')
		if err != nil && line == "" {
			return fmt.Errorf("read %s: %w", p.label, err)
		}
		val := strings.TrimSpace(line)
		if val == "" {
			val = p.def
		}
		if val == "" && p.require {
			return fmt.Errorf("%s is required", p.label)
		}
		*p.field = val
	}
	return nil
}

// isTerminal reports whether r is an *os.File backed by a tty. Tests
// pass a bytes.Reader / strings.Reader and get false back so the
// non-interactive path runs.
func isTerminal(r io.Reader) bool {
	f, ok := r.(*os.File)
	if !ok {
		return false
	}
	return term.IsTerminal(int(f.Fd()))
}
