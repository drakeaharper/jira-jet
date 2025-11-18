package cmd

import (
	"fmt"
	"strings"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
	"jet/internal/config"
	"jet/internal/jira"
)

var linkCmd = &cobra.Command{
	Use:   "link TICKET-KEY RELATIONSHIP TICKET-KEY",
	Short: "Link two JIRA tickets with a relationship",
	Long: `Link two JIRA tickets with a specific relationship type.

Common relationship types:
  - blocks / is-blocked-by
  - relates-to
  - duplicates / is-duplicated-by
  - clones / is-cloned-by
  - causes / is-caused-by

Examples:
  jet link PROJ-123 blocks PROJ-456
  jet link PROJ-123 relates-to PROJ-789
  jet link PROJ-123 duplicates PROJ-999`,
	Args: cobra.ExactArgs(3),
	RunE: func(cmd *cobra.Command, args []string) error {
		// Parse the relationship
		// Format: "TICKET-KEY RELATIONSHIP TICKET-KEY"
		inwardIssue := args[0]
		relationship := strings.ToLower(args[1])
		outwardIssue := args[2]

		// Map common relationship names to Jira link type names
		linkTypeMap := map[string]string{
			"blocks":          "Blocks",
			"is-blocked-by":   "Blocks",
			"relates-to":      "Relates",
			"relates":         "Relates",
			"duplicates":      "Duplicate",
			"is-duplicated-by": "Duplicate",
			"clones":          "Cloners",
			"is-cloned-by":    "Cloners",
			"causes":          "Causes",
			"is-caused-by":    "Causes",
		}

		linkTypeName, ok := linkTypeMap[relationship]
		if !ok {
			return fmt.Errorf("unknown relationship type '%s'. Common types: blocks, relates-to, duplicates, clones, causes", relationship)
		}

		// Determine if this is an inward or outward link
		isInward := strings.HasPrefix(relationship, "is-")

		// Load configuration
		cfg, err := config.Load()
		if err != nil {
			return fmt.Errorf("configuration error: %w", err)
		}

		// Create JIRA client
		client := jira.NewClient(cfg.URL, cfg.Email, cfg.Username, cfg.Token)

		// Create the link
		if err := client.LinkIssues(inwardIssue, outwardIssue, linkTypeName, isInward); err != nil {
			return err
		}

		// Success message
		green := color.New(color.FgGreen, color.Bold)
		fmt.Printf("%s Successfully linked %s %s %s\n",
			green.Sprint("✓"),
			color.CyanString(inwardIssue),
			relationship,
			color.CyanString(outwardIssue))

		return nil
	},
}

func init() {
	rootCmd.AddCommand(linkCmd)
}
