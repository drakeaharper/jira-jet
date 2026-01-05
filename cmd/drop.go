package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"jet/internal/config"
	"jet/internal/jira"
)

var dropCmd = &cobra.Command{
	Use:   "drop TICKET-KEY",
	Short: "Drop (unassign) a JIRA ticket",
	Long: `Drop (unassign) a JIRA ticket.

This is a convenient shortcut for unassigning tickets that are currently assigned to someone.`,
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ticketKey := args[0]

		// Load configuration
		cfg, err := config.Load()
		if err != nil {
			return fmt.Errorf("configuration error: %w", err)
		}

		// Create JIRA client
		client := jira.NewClient(cfg.URL, cfg.Email, cfg.Username, cfg.Token)

		// Prepare fields to update - set assignee to null to unassign
		fields := map[string]interface{}{
			"assignee": nil,
		}

		// Update the ticket
		if err := client.UpdateIssue(ticketKey, fields); err != nil {
			return err
		}

		fmt.Printf("Ticket %s unassigned\n", ticketKey)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(dropCmd)
}
