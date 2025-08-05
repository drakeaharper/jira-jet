package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"jet/internal/config"
	"jet/internal/jira"
)

var assignCmd = &cobra.Command{
	Use:   "assign TICKET-KEY",
	Short: "Assign a JIRA ticket to yourself",
	Long: `Assign a JIRA ticket to yourself.
	
This is a convenient shortcut for assigning tickets to the currently authenticated user.`,
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

		// Get current user
		currentUser, err := client.GetCurrentUser()
		if err != nil {
			return fmt.Errorf("failed to get current user: %w", err)
		}

		// Prepare fields to update
		fields := map[string]interface{}{
			"assignee": map[string]string{"name": currentUser.Name},
		}

		// Update the ticket
		if err := client.UpdateIssue(ticketKey, fields); err != nil {
			return err
		}

		fmt.Printf("Ticket %s assigned to %s\n", ticketKey, currentUser.DisplayName)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(assignCmd)
}