package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
	"jet/internal/config"
	"jet/internal/jira"
)

var (
	epicsFormat     string
	epicsOutput     string
	epicsShowAll    bool
	epicsMaxResults int
)

var epicsCmd = &cobra.Command{
	Use:   "epics PROJECT-KEY",
	Short: "List epics in a project",
	Long: `List epics in a JIRA project.

By default, closed/done/resolved epics are excluded. Use --all to show all epics.

Examples:
  jet epics PROJ                          # Show open epics
  jet epics PROJ --all                    # Show all epics including closed
  jet epics PROJ --max 100                # Return up to 100 epics
  jet epics PROJ --format json            # JSON output
  jet epics PROJ --output epics.txt       # Write to file`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		projectKey := args[0]

		cfg, err := config.Load()
		if err != nil {
			return fmt.Errorf("configuration error: %w", err)
		}

		client := jira.NewClient(cfg.URL, cfg.Email, cfg.Username, cfg.Token)

		// Build JQL
		jql := fmt.Sprintf("project = \"%s\" AND issuetype = Epic", jira.EscapeString(projectKey))
		if !epicsShowAll {
			jql += " AND status NOT IN (\"Done\", \"Closed\", \"Resolved\")"
		}
		jql += " ORDER BY updated DESC"

		searchResp, err := client.SearchIssues(jql, epicsMaxResults)
		if err != nil {
			return fmt.Errorf("search failed: %w", err)
		}

		if len(searchResp.Issues) == 0 {
			if epicsShowAll {
				fmt.Printf("No epics found in project %s\n", projectKey)
			} else {
				fmt.Printf("No open epics found in project %s (use --all to include closed)\n", projectKey)
			}
			return nil
		}

		var output string
		if epicsFormat == "json" {
			jsonData, err := json.MarshalIndent(searchResp.Issues, "", "  ")
			if err != nil {
				return fmt.Errorf("error formatting JSON: %w", err)
			}
			output = string(jsonData)
		} else {
			output = formatEpics(projectKey, searchResp.Issues, searchResp.Total)
		}

		if epicsOutput != "" {
			err := os.WriteFile(epicsOutput, []byte(output), 0644)
			if err != nil {
				return fmt.Errorf("error writing to file: %w", err)
			}
			fmt.Printf("Epics written to %s\n", epicsOutput)
		} else {
			fmt.Print(output)
		}

		return nil
	},
}

func formatEpics(projectKey string, epics []jira.Issue, total int) string {
	var sb strings.Builder

	cyan := color.New(color.FgCyan, color.Bold)
	yellow := color.New(color.FgYellow, color.Bold)
	blue := color.New(color.FgBlue)
	white := color.New(color.FgWhite)
	gray := color.New(color.FgHiBlack)

	cyan.Fprintf(&sb, "Project: %s\n", projectKey)
	if epicsShowAll {
		fmt.Fprintf(&sb, "Found %d epic(s)", total)
	} else {
		fmt.Fprintf(&sb, "Found %d open epic(s)", total)
	}
	if total > len(epics) {
		gray.Fprintf(&sb, " (showing %d)", len(epics))
	}
	fmt.Fprintln(&sb)
	fmt.Fprintln(&sb)

	w := tabwriter.NewWriter(&sb, 0, 0, 2, ' ', 0)

	yellow.Fprintln(w, "KEY\tSTATUS\tPRIORITY\tASSIGNEE\tSUMMARY")
	gray.Fprintln(w, "---\t------\t--------\t--------\tSUMMARY")

	for _, epic := range epics {
		assignee := "Unassigned"
		if epic.Fields.Assignee != nil {
			assignee = epic.Fields.Assignee.DisplayName
			if assignee == "" {
				assignee = epic.Fields.Assignee.Name
			}
		}

		keyStr := blue.Sprint(epic.Key)

		statusColor := getEpicStatusColor(epic.Fields.Status.Name)
		statusStr := statusColor.Sprint(epic.Fields.Status.Name)

		priorityStr := ""
		if epic.Fields.Priority.Name != "" {
			priorityColor := getPriorityColor(epic.Fields.Priority.Name)
			priorityStr = priorityColor.Sprint(epic.Fields.Priority.Name)
		}

		assigneeStr := assignee
		if assignee == "Unassigned" {
			assigneeStr = gray.Sprint(assignee)
		} else {
			assigneeStr = white.Sprint(truncateString(assignee, 20))
		}

		summaryStr := truncateString(epic.Fields.Summary, 50)

		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
			keyStr,
			statusStr,
			priorityStr,
			assigneeStr,
			summaryStr)
	}

	w.Flush()
	return sb.String()
}

func init() {
	rootCmd.AddCommand(epicsCmd)

	epicsCmd.Flags().StringVar(&epicsFormat, "format", "readable", "Output format (readable or json)")
	epicsCmd.Flags().StringVarP(&epicsOutput, "output", "o", "", "Output file (default: stdout)")
	epicsCmd.Flags().BoolVar(&epicsShowAll, "all", false, "Show all epics including closed ones")
	epicsCmd.Flags().IntVar(&epicsMaxResults, "max", 50, "Maximum number of results")
}
