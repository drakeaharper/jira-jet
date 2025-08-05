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
	epicFormat string
	epicOutput string
	showAllTickets bool
)

var epicCmd = &cobra.Command{
	Use:   "epic EPIC-KEY",
	Short: "List child tickets of an epic (excludes closed by default)",
	Long: `List child tickets (subtasks) of the specified epic.
By default, closed tickets are excluded. Use --all to show all tickets.

Examples:
  jet epic PROJ-123                        # Show only non-closed tickets
  jet epic PROJ-123 --all                  # Show all tickets including closed
  jet epic PROJ-123 --format json
  jet epic PROJ-123 --output children.txt`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		epicKey := args[0]
		
		// Extract ticket key from URL if provided
		if strings.Contains(epicKey, "/browse/") {
			parts := strings.Split(epicKey, "/browse/")
			if len(parts) == 2 {
				epicKey = parts[1]
			}
		}

		cfg, err := config.Load()
		if err != nil {
			fmt.Printf("Error loading config: %v\n", err)
			os.Exit(1)
		}

		client := jira.NewClient(cfg.URL, cfg.Email, cfg.Username, cfg.Token)
		
		children, err := client.GetEpicChildren(epicKey)
		if err != nil {
			fmt.Printf("Error fetching epic children: %v\n", err)
			os.Exit(1)
		}

		// Filter out closed tickets unless --all flag is used
		if !showAllTickets {
			var filteredChildren []jira.Issue
			for _, child := range children {
				status := strings.ToLower(child.Fields.Status.Name)
				if status != "closed" && status != "done" && status != "resolved" {
					filteredChildren = append(filteredChildren, child)
				}
			}
			children = filteredChildren
		}

		if len(children) == 0 {
			if showAllTickets {
				fmt.Printf("No child tickets found for epic %s\n", epicKey)
			} else {
				fmt.Printf("No open child tickets found for epic %s (use --all to show closed tickets)\n", epicKey)
			}
			return
		}

		var output string
		if epicFormat == "json" {
			jsonData, err := json.MarshalIndent(children, "", "  ")
			if err != nil {
				fmt.Printf("Error formatting JSON: %v\n", err)
				os.Exit(1)
			}
			output = string(jsonData)
		} else {
			output = formatEpicChildren(epicKey, children)
		}

		if epicOutput != "" {
			err := os.WriteFile(epicOutput, []byte(output), 0644)
			if err != nil {
				fmt.Printf("Error writing to file: %v\n", err)
				os.Exit(1)
			}
			fmt.Printf("Epic children written to %s\n", epicOutput)
		} else {
			fmt.Print(output)
		}
	},
}

func formatEpicChildren(epicKey string, children []jira.Issue) string {
	var sb strings.Builder
	
	// Color definitions
	cyan := color.New(color.FgCyan, color.Bold)
	yellow := color.New(color.FgYellow, color.Bold)
	blue := color.New(color.FgBlue)
	white := color.New(color.FgWhite)
	gray := color.New(color.FgHiBlack)
	
	// Epic header
	cyan.Fprintf(&sb, "Epic: %s\n", epicKey)
	if showAllTickets {
		fmt.Fprintf(&sb, "Found %d child ticket(s):\n\n", len(children))
	} else {
		fmt.Fprintf(&sb, "Found %d open child ticket(s):\n\n", len(children))
	}

	w := tabwriter.NewWriter(&sb, 0, 0, 2, ' ', 0)
	
	// Table headers with color
	yellow.Fprintln(w, "KEY\tSTATUS\tASSIGNEE\tTYPE\tSUMMARY")
	gray.Fprintln(w, "---\t------\t--------\t----\t-------")

	for _, child := range children {
		assignee := "Unassigned"
		if child.Fields.Assignee != nil {
			assignee = child.Fields.Assignee.DisplayName
			if assignee == "" {
				assignee = child.Fields.Assignee.Name
			}
		}
		
		// Color the key
		keyColor := blue
		keyStr := keyColor.Sprint(child.Key)
		
		// Color the status based on its value
		statusColor := getEpicStatusColor(child.Fields.Status.Name)
		statusStr := statusColor.Sprint(child.Fields.Status.Name)
		
		// Color assignee (gray if unassigned)
		assigneeStr := assignee
		if assignee == "Unassigned" {
			assigneeStr = gray.Sprint(assignee)
		} else {
			assigneeStr = white.Sprint(truncateString(assignee, 20))
		}
		
		// Issue type in white
		typeStr := white.Sprint(child.Fields.IssueType.Name)
		
		// Summary in default color
		summaryStr := truncateString(child.Fields.Summary, 50)
		
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
			keyStr,
			statusStr,
			assigneeStr,
			typeStr,
			summaryStr)
	}
	
	w.Flush()
	return sb.String()
}

func getEpicStatusColor(status string) *color.Color {
	switch strings.ToLower(status) {
	case "done", "closed", "resolved":
		return color.New(color.FgGreen)
	case "in progress", "in review", "in development":
		return color.New(color.FgYellow)
	case "blocked":
		return color.New(color.FgRed)
	case "to do", "open", "new", "backlog":
		return color.New(color.FgCyan)
	default:
		return color.New(color.FgWhite)
	}
}

func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

func init() {
	rootCmd.AddCommand(epicCmd)
	
	epicCmd.Flags().StringVar(&epicFormat, "format", "readable", "Output format (readable or json)")
	epicCmd.Flags().StringVarP(&epicOutput, "output", "o", "", "Output file (default: stdout)")
	epicCmd.Flags().BoolVar(&showAllTickets, "all", false, "Show all tickets including closed ones")
}