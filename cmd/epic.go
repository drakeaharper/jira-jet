package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"
	"jet/internal/config"
	"jet/internal/jira"
)

var (
	epicFormat string
	epicOutput string
)

var epicCmd = &cobra.Command{
	Use:   "epic EPIC-KEY",
	Short: "List child tickets of an epic",
	Long: `List all child tickets (subtasks) of the specified epic.

Examples:
  jet epic PROJ-123
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

		if len(children) == 0 {
			fmt.Printf("No child tickets found for epic %s\n", epicKey)
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
	
	sb.WriteString(fmt.Sprintf("Epic: %s\n", epicKey))
	sb.WriteString(fmt.Sprintf("Found %d child ticket(s):\n\n", len(children)))

	w := tabwriter.NewWriter(&sb, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "KEY\tSTATUS\tTYPE\tSUMMARY")
	fmt.Fprintln(w, "---\t------\t----\t-------")

	for _, child := range children {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n",
			child.Key,
			child.Fields.Status.Name,
			child.Fields.IssueType.Name,
			truncateString(child.Fields.Summary, 60))
	}
	
	w.Flush()
	return sb.String()
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
}