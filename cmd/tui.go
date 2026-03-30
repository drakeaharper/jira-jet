package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"jet/internal/config"
	"jet/internal/jira"
	"jet/internal/tui"
)

var (
	tuiProject string
	tuiJQL     string
)

var tuiCmd = &cobra.Command{
	Use:   "tui",
	Short: "Launch interactive TUI dashboard",
	Long: `Launch a full-screen interactive terminal UI for browsing,
viewing, creating, editing, and transitioning JIRA tickets.

Examples:
  jet tui                          # Dashboard with your open tickets
  jet tui --project=PROJ           # Filter to a specific project
  jet tui --jql="assignee = me"    # Custom JQL query`,
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load()
		if err != nil {
			return fmt.Errorf("configuration error: %w", err)
		}

		client := jira.NewClient(cfg.URL, cfg.Email, cfg.Username, cfg.Token)

		// Build initial JQL
		jql := tuiJQL
		if jql == "" {
			jql = `assignee = currentUser() AND status IN ("To Do","In Progress","Open","New","Backlog","In Review","In Development","In Validation") ORDER BY updated DESC`
			if tuiProject != "" {
				jql = fmt.Sprintf(`project = "%s" AND %s`, tuiProject, jql)
			}
		}

		return tui.Run(client, jql)
	},
}

func init() {
	rootCmd.AddCommand(tuiCmd)

	tuiCmd.Flags().StringVar(&tuiProject, "project", "", "Filter by project key")
	tuiCmd.Flags().StringVar(&tuiJQL, "jql", "", "Custom JQL query (overrides other filters)")
}
