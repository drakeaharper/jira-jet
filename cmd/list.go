package cmd

import (
	"fmt"
	"strings"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
	"jet/internal/config"
	"jet/internal/jira"
)

var (
	listAssignee string
	listStatus   string
	listProject  string
	listMaxResults int
)

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List JIRA tickets",
	Long: `List JIRA tickets based on filters.

By default, lists tickets assigned to you that are open (not Done/Closed/Resolved).

Examples:
  jet list                                    # Your open tickets
  jet list --assignee=john.doe                # Tickets assigned to john.doe
  jet list --status="To Do,Done"              # Tickets with specific statuses
  jet list --project=PROJ                     # Tickets in specific project
  jet list --assignee=unassigned              # Unassigned tickets`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Load configuration
		cfg, err := config.Load()
		if err != nil {
			return fmt.Errorf("configuration error: %w", err)
		}

		// Create JIRA client
		client := jira.NewClient(cfg.URL, cfg.Email, cfg.Username, cfg.Token)

		// Build JQL query
		var jqlParts []string

		// Handle assignee
		if listAssignee == "me" {
			// Use currentUser() function in JQL instead of fetching user details
			jqlParts = append(jqlParts, "assignee = currentUser()")
		} else if listAssignee == "unassigned" {
			jqlParts = append(jqlParts, "assignee is EMPTY")
		} else if listAssignee != "" {
			jqlParts = append(jqlParts, fmt.Sprintf("assignee = \"%s\"", listAssignee))
		}

		// Handle status
		if listStatus != "" {
			statuses := strings.Split(listStatus, ",")
			if len(statuses) == 1 {
				jqlParts = append(jqlParts, fmt.Sprintf("status = \"%s\"", strings.TrimSpace(statuses[0])))
			} else {
				statusList := make([]string, len(statuses))
				for i, s := range statuses {
					statusList[i] = fmt.Sprintf("\"%s\"", strings.TrimSpace(s))
				}
				jqlParts = append(jqlParts, fmt.Sprintf("status IN (%s)", strings.Join(statusList, ",")))
			}
		}

		// Handle project
		if listProject != "" {
			jqlParts = append(jqlParts, fmt.Sprintf("project = \"%s\"", listProject))
		}

		// Build final JQL
		jql := strings.Join(jqlParts, " AND ")
		if jql == "" {
			jql = "order by updated DESC"
		} else {
			jql += " order by updated DESC"
		}


		// Search for issues
		searchResp, err := client.SearchIssues(jql, listMaxResults)
		if err != nil {
			return fmt.Errorf("search failed: %w", err)
		}

		// Display results
		if len(searchResp.Issues) == 0 {
			fmt.Println("No tickets found matching the criteria.")
			return nil
		}

		displayIssueList(searchResp.Issues, searchResp.Total)

		return nil
	},
}

func displayIssueList(issues []jira.Issue, total int) {
	// Define colors
	cyan := color.New(color.FgCyan, color.Bold)
	yellow := color.New(color.FgYellow)
	green := color.New(color.FgGreen)
	gray := color.New(color.FgHiBlack)

	// Header
	fmt.Printf("\n")
	cyan.Printf("📋 Found %d ticket(s)", total)
	if total > len(issues) {
		gray.Printf(" (showing %d)", len(issues))
	}
	fmt.Printf("\n")
	fmt.Println(gray.Sprint("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"))

	// List issues
	for _, issue := range issues {
		// Key and Summary
		fmt.Printf("%s  %s\n",
			cyan.Sprint(issue.Key),
			yellow.Sprint(issue.Fields.Summary))

		// Status, Type, Priority
		statusColor := getStatusColor(issue.Fields.Status.Name)
		details := []string{
			statusColor.Sprint(issue.Fields.Status.Name),
			issue.Fields.IssueType.Name,
		}

		if issue.Fields.Priority.Name != "" {
			priorityColor := getPriorityColor(issue.Fields.Priority.Name)
			details = append(details, priorityColor.Sprint(issue.Fields.Priority.Name))
		}

		// Assignee
		if issue.Fields.Assignee != nil {
			assigneeName := issue.Fields.Assignee.DisplayName
			if assigneeName == "" {
				assigneeName = issue.Fields.Assignee.Name
			}
			details = append(details, "👤 "+assigneeName)
		} else {
			details = append(details, gray.Sprint("👤 Unassigned"))
		}

		fmt.Printf("   %s\n", gray.Sprint(strings.Join(details, " • ")))

		// Project
		fmt.Printf("   %s %s\n",
			green.Sprint("📁"),
			gray.Sprintf("%s (%s)", issue.Fields.Project.Name, issue.Fields.Project.Key))

		fmt.Println()
	}
}

func init() {
	rootCmd.AddCommand(listCmd)

	listCmd.Flags().StringVar(&listAssignee, "assignee", "me", "Filter by assignee (use 'me' for yourself, 'unassigned' for unassigned tickets)")
	listCmd.Flags().StringVar(&listStatus, "status", "To Do,In Progress,Open,New,Backlog,In Review,In Development,In Validation", "Filter by status (comma-separated for multiple)")
	listCmd.Flags().StringVar(&listProject, "project", "", "Filter by project key")
	listCmd.Flags().IntVar(&listMaxResults, "max", 50, "Maximum number of results to return")
}
