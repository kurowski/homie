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
)

var initCmd = &cobra.Command{
	Use:   "init [target-dir]",
	Short: "Scaffold a new user environment repo",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runInit,
}

func init() {
	initCmd.Flags().StringVar(&initName, "name", "", "your full name (required if non-interactive)")
	initCmd.Flags().StringVar(&initEmail, "email", "", "your primary email (required if non-interactive)")
	initCmd.Flags().StringVar(&initGitHubUser, "github-user", "", "GitHub username for the bootstrap URL")
	initCmd.Flags().StringVar(&initGitHubRepo, "github-repo", "dotfiles", "GitHub repo name for the env repo")
	initCmd.Flags().StringVar(&initProfile, "profile", "personal", "profile name (personal | work | devcontainer | ...)")
	initCmd.Flags().StringVar(&initShell, "shell", "zsh", "default shell")
	rootCmd.AddCommand(initCmd)
}

func runInit(cmd *cobra.Command, args []string) error {
	target := "."
	if len(args) == 1 {
		target = args[0]
	}
	abs, err := filepath.Abs(target)
	if err != nil {
		return fmt.Errorf("resolve %s: %w", target, err)
	}

	answers := scaffold.Answers{
		Name:         initName,
		Email:        initEmail,
		GitHubUser:   initGitHubUser,
		GitHubRepo:   initGitHubRepo,
		Profile:      initProfile,
		DefaultShell: initShell,
	}

	stdin := cmd.InOrStdin()
	stdout := cmd.OutOrStdout()
	interactive := isTerminal(stdin)
	if interactive {
		if err := promptMissing(stdin, stdout, &answers); err != nil {
			return err
		}
	}

	if err := scaffold.Run(abs, answers); err != nil {
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
