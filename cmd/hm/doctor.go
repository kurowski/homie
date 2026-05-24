package main

import (
	"fmt"
	"io"
	"os"
	"sort"

	"github.com/charmbracelet/lipgloss"
	"github.com/kurowski/homie/internal/config"
	"github.com/kurowski/homie/internal/detect"
	"github.com/kurowski/homie/internal/doctor"
	"github.com/kurowski/homie/internal/packages"
	"github.com/kurowski/homie/internal/repo"
	"github.com/muesli/termenv"
	"github.com/spf13/cobra"
)

var doctorHome string

var doctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "Check for broken symlinks, missing deps, common problems",
	Long: `Doctor walks the user environment repo and host without making
changes, reporting issues hm apply would care about: broken or stale
dotfile symlinks, unrendered or out-of-date templates, missing packages,
scripts that aren't executable, and unknown distros.

Exit code is 1 when any error-severity finding is reported, 0 otherwise
— useful in CI to gate merges against a Homie-managed environment.`,
	RunE:          runDoctor,
	SilenceUsage:  true,
	SilenceErrors: true,
}

func init() {
	doctorCmd.Flags().StringVar(&doctorHome, "home", "", "override target home directory (default $HOME)")
	rootCmd.AddCommand(doctorCmd)
}

func runDoctor(cmd *cobra.Command, args []string) error {
	repoDir, err := repo.Find()
	if err != nil {
		return err
	}
	cfg, err := config.Load(repoDir)
	if err != nil {
		return err
	}
	home := doctorHome
	if home == "" {
		home, err = os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("resolve home: %w", err)
		}
	}
	env := detect.Detect()
	mgr := packages.For(env)

	noTTY, _ := cmd.Root().PersistentFlags().GetBool("no-tty")
	report := doctor.Run(repoDir, home, cfg, env, mgr)
	writeReport(cmd.OutOrStdout(), report, noTTY)
	if report.HasErrors() {
		return errSilentExit
	}
	return nil
}

// writeReport renders r to w with lipgloss styling. When noTTY is set
// (or w isn't a terminal), lipgloss degrades to ASCII automatically via
// the renderer's color profile detection.
func writeReport(w io.Writer, r doctor.Report, noTTY bool) {
	s := newDoctorStyles(w, noTTY)

	fmt.Fprintln(w, s.header.Render("hm doctor"))
	fmt.Fprintln(w)

	if len(r.Findings) == 0 {
		fmt.Fprintln(w, s.ok.Render("  ✓ All checks passed."))
		return
	}

	for _, area := range groupAreas(r.Findings) {
		fmt.Fprintln(w, "  "+s.area.Render(area.name))
		for _, f := range area.findings {
			glyph, style := s.glyphFor(f.Severity)
			fmt.Fprintf(w, "    %s %s\n", style.Render(glyph), f.Message)
		}
		fmt.Fprintln(w)
	}

	errs, warns := r.Counts()
	summary := fmt.Sprintf("%s, %s", pluralize(errs, "error"), pluralize(warns, "warning"))
	switch {
	case errs > 0:
		fmt.Fprintln(w, s.summaryErr.Render(summary))
	case warns > 0:
		fmt.Fprintln(w, s.summaryWarn.Render(summary))
	default:
		fmt.Fprintln(w, s.summaryOK.Render(summary))
	}
}

type doctorStyles struct {
	header      lipgloss.Style
	area        lipgloss.Style
	errGlyph    lipgloss.Style
	warnGlyph   lipgloss.Style
	ok          lipgloss.Style
	summaryErr  lipgloss.Style
	summaryWarn lipgloss.Style
	summaryOK   lipgloss.Style
}

func (s doctorStyles) glyphFor(sev doctor.Severity) (string, lipgloss.Style) {
	switch sev {
	case doctor.SeverityError:
		return "✘", s.errGlyph
	case doctor.SeverityWarn:
		return "⚠", s.warnGlyph
	default:
		return "•", s.area
	}
}

func newDoctorStyles(w io.Writer, noTTY bool) doctorStyles {
	r := lipgloss.NewRenderer(w)
	if noTTY {
		r.SetColorProfile(termenv.Ascii)
	}
	return doctorStyles{
		header: r.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("205")).
			Border(lipgloss.NormalBorder(), false, false, true, false).
			BorderForeground(lipgloss.Color("245")).
			Padding(0, 1),
		area:       r.NewStyle().Bold(true).Foreground(lipgloss.Color("117")),
		errGlyph:   r.NewStyle().Bold(true).Foreground(lipgloss.Color("203")),
		warnGlyph:  r.NewStyle().Bold(true).Foreground(lipgloss.Color("214")),
		ok:         r.NewStyle().Bold(true).Foreground(lipgloss.Color("42")),
		summaryErr: r.NewStyle().Bold(true).Foreground(lipgloss.Color("203")),
		summaryWarn: r.NewStyle().Bold(true).Foreground(lipgloss.Color("214")),
		summaryOK:  r.NewStyle().Bold(true).Foreground(lipgloss.Color("42")),
	}
}

// groupedArea preserves the first-seen order of areas in a report.
type groupedArea struct {
	name     string
	findings []doctor.Finding
}

func groupAreas(findings []doctor.Finding) []groupedArea {
	byArea := map[string][]doctor.Finding{}
	var order []string
	for _, f := range findings {
		if _, ok := byArea[f.Area]; !ok {
			order = append(order, f.Area)
		}
		byArea[f.Area] = append(byArea[f.Area], f)
	}
	// Stable area ordering: errors first by area, then warnings —
	// but inside each area, errors before warnings.
	for area, list := range byArea {
		sort.SliceStable(list, func(i, j int) bool {
			return list[i].Severity == doctor.SeverityError &&
				list[j].Severity != doctor.SeverityError
		})
		byArea[area] = list
	}
	out := make([]groupedArea, 0, len(order))
	for _, a := range order {
		out = append(out, groupedArea{name: a, findings: byArea[a]})
	}
	return out
}

func pluralize(n int, word string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, word)
	}
	return fmt.Sprintf("%d %ss", n, word)
}

