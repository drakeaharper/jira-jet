package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
	"jet/internal/prs"
)

var (
	prsSource string
	prsLimit  int
	prsJSON   bool
)

var prsCmd = &cobra.Command{
	Use:   "prs",
	Short: "Aggregate open pull requests across Gerrit and GitHub",
	Long: `Aggregate open pull requests / changes across Gerrit (via gerry's
credentials) and GitHub (via the gh CLI) into a single view.

Configure GitHub repos and an optional Gerrit team filter in the [prs]
section of ~/.jira_config:

  [prs]
  gerrit_filter = ownerin:learning-experience
  github_repos = instructure/canvas-lms,instructure/platform-ui`,
}

var prsMineCmd = &cobra.Command{
	Use:   "mine",
	Short: "Your open PRs across Gerrit and GitHub",
	RunE: func(cmd *cobra.Command, args []string) error {
		return runPRs(prs.Mine, "Your open PRs")
	},
}

var prsTeamCmd = &cobra.Command{
	Use:   "team",
	Short: "PRs awaiting your review across Gerrit and GitHub",
	RunE: func(cmd *cobra.Command, args []string) error {
		return runPRs(prs.Team, "PRs awaiting your review")
	},
}

func runPRs(fetch func(*prs.Config, prs.Options) ([]prs.PR, []error), heading string) error {
	cfg, err := prs.LoadConfig()
	if err != nil {
		return fmt.Errorf("failed to load [prs] config: %w", err)
	}

	list, errs := fetch(cfg, prs.Options{Source: prsSource, Limit: prsLimit})

	if prsJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(list)
	}

	// Surface per-source errors without aborting — partial results are useful.
	for _, e := range errs {
		fmt.Fprintln(os.Stderr, color.YellowString("! %v", e))
	}

	if len(list) == 0 {
		fmt.Println("No PRs found.")
		return nil
	}

	reviewable := 0
	for _, p := range list {
		if p.Reviewable {
			reviewable++
		}
	}
	color.New(color.Bold).Printf("%s — %d total, %d reviewable\n\n", heading, len(list), reviewable)

	for _, g := range prs.GroupBySourceRepo(list) {
		src := color.CyanString("gerrit")
		if g.Source == prs.SourceGitHub {
			src = color.MagentaString("github")
		}
		color.New(color.Bold).Printf("%s · %s (%d)\n", src, g.Repo, len(g.PRs))

		w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
		for _, p := range g.PRs {
			id := fmt.Sprintf("!%d", p.Number)
			if p.Source == prs.SourceGitHub {
				id = fmt.Sprintf("#%d", p.Number)
			}
			title := truncate(p.Title, 50)
			marker := ""
			if !p.Reviewable {
				// Dim the whole row and tag it with the reason.
				id = color.HiBlackString(id)
				title = color.HiBlackString(title)
				marker = color.HiBlackString(fmt.Sprintf("(blocked: %s)", p.BlockReason))
			}
			fmt.Fprintf(w, "    %s\t%s\t%s\t%s\t%s\n",
				id, title, statusColor(p.Status), p.Author, marker)
		}
		w.Flush()
		fmt.Println()
	}
	return nil
}

func statusColor(s string) string {
	switch s {
	case "approved", "CR+2":
		return color.GreenString(s)
	case "changes requested":
		return color.RedString(s)
	case "CR+1":
		return color.HiGreenString(s)
	default:
		return color.YellowString(s)
	}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	if n <= 1 {
		return s[:n]
	}
	return s[:n-1] + "…"
}

func init() {
	rootCmd.AddCommand(prsCmd)
	prsCmd.AddCommand(prsMineCmd, prsTeamCmd)

	for _, c := range []*cobra.Command{prsMineCmd, prsTeamCmd} {
		c.Flags().StringVar(&prsSource, "source", "all", "Which source to query (all, gerrit, github)")
		c.Flags().IntVarP(&prsLimit, "limit", "n", 25, "Max results per source")
		c.Flags().BoolVar(&prsJSON, "json", false, "Output raw JSON")
	}
}
