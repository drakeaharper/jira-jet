package cmd

import (
	"fmt"
	"time"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
	"jet/internal/config"
	"jet/internal/jira"
)

var (
	standupDays    int
	standupProject string
)

var standupCmd = &cobra.Command{
	Use:   "standup",
	Short: "Show daily standup report",
	Long: `Show a daily standup report with recently completed tickets and current work in progress.

All tickets are scoped to the current user (assignee = currentUser()).

Examples:
  jet standup                    # Default: last 2 days of completed + in progress
  jet standup --days 5           # Look back 5 days for completed tickets
  jet standup --project PROJ     # Scope to a specific project`,
	RunE: runStandup,
}

func runStandup(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("configuration error: %w", err)
	}

	client := jira.NewClient(cfg.URL, cfg.Email, cfg.Username, cfg.Token)

	projectClause := ""
	if standupProject != "" {
		projectClause = fmt.Sprintf(" AND project = \"%s\"", jira.EscapeString(standupProject))
	}

	// Query 1: Recently completed tickets
	completedJQL := fmt.Sprintf(
		"assignee = currentUser()%s AND statusCategory = \"Done\" AND resolved >= -%dd ORDER BY resolved DESC",
		projectClause, standupDays,
	)

	completedResp, err := client.SearchIssues(completedJQL, 50)
	if err != nil {
		return fmt.Errorf("failed to fetch completed tickets: %w", err)
	}

	// Query 2: Work in progress
	wipJQL := fmt.Sprintf(
		"assignee = currentUser()%s AND statusCategory = \"In Progress\" ORDER BY updated DESC",
		projectClause,
	)

	wipResp, err := client.SearchIssues(wipJQL, 50)
	if err != nil {
		return fmt.Errorf("failed to fetch in-progress tickets: %w", err)
	}

	displayStandupReport(completedResp.Issues, wipResp.Issues)

	return nil
}

func displayStandupReport(completed []jira.Issue, wip []jira.Issue) {
	cyan := color.New(color.FgCyan, color.Bold)
	yellow := color.New(color.FgYellow)
	green := color.New(color.FgGreen, color.Bold)
	gray := color.New(color.FgHiBlack)
	white := color.New(color.FgWhite, color.Bold)

	// Header
	fmt.Println()
	cyan.Println("🧍 Standup Report")
	fmt.Println(gray.Sprint("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"))

	// Completed section
	fmt.Println()
	green.Printf("✅ Completed (%d)\n", len(completed))

	grouped := groupByResolutionDate(completed)

	// Build continuous date sequence so gaps are visible
	dates := completedDateRange(standupDays)

	for _, dateStr := range dates {
		fmt.Println()
		white.Printf("  %s\n", formatDateHeading(dateStr))

		if issues, ok := grouped[dateStr]; ok {
			for _, issue := range issues {
				fmt.Printf("    %s  %s\n",
					cyan.Sprint(issue.Key),
					yellow.Sprint(issue.Fields.Summary))
			}
		} else {
			fmt.Printf("    %s\n", gray.Sprint("No tickets closed"))
		}
	}

	// In Progress section
	fmt.Println()
	if len(wip) > 0 {
		wipColor := color.New(color.FgYellow, color.Bold)
		wipColor.Printf("🔄 In Progress (%d)\n", len(wip))

		for _, issue := range wip {
			statusColor := getStatusColor(issue.Fields.Status.Name)
			fmt.Printf("    %s  %-50s %s\n",
				cyan.Sprint(issue.Key),
				yellow.Sprint(truncateString(issue.Fields.Summary, 50)),
				statusColor.Sprint(issue.Fields.Status.Name))
		}
	} else {
		gray.Println("🔄 In Progress: None")
	}

	fmt.Println()
}

// completedDateRange returns a slice of date strings from today back through
// the lookback period, most recent first (e.g. ["2026-04-15", "2026-04-14", "2026-04-13"]).
func completedDateRange(days int) []string {
	today := time.Now()
	dates := make([]string, 0, days+1)
	for i := 0; i <= days; i++ {
		dates = append(dates, today.AddDate(0, 0, -i).Format("2006-01-02"))
	}
	return dates
}

func groupByResolutionDate(issues []jira.Issue) map[string][]jira.Issue {
	grouped := make(map[string][]jira.Issue)
	for _, issue := range issues {
		dateKey := "Unknown"
		if len(issue.Fields.ResolutionDate) >= 10 {
			dateKey = issue.Fields.ResolutionDate[:10]
		}
		grouped[dateKey] = append(grouped[dateKey], issue)
	}
	return grouped
}

func formatDateHeading(dateStr string) string {
	t, err := time.Parse("2006-01-02", dateStr)
	if err != nil {
		return dateStr
	}
	return t.Format("January 2, 2006")
}

func init() {
	rootCmd.AddCommand(standupCmd)
	standupCmd.Flags().IntVar(&standupDays, "days", 2, "Number of days to look back for completed tickets")
	standupCmd.Flags().StringVar(&standupProject, "project", "", "Filter by project key")
}
