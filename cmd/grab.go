package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"jet/internal/config"
	"jet/internal/jira"
)

var grabCmd = &cobra.Command{
	Use:   "grab TICKET-KEY",
	Short: "Grab (assign) a JIRA ticket to yourself",
	Long: `Grab (assign) a JIRA ticket to yourself.
	
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
		var fields map[string]interface{}
		// Use Account ID for GDPR compliance (newer JIRA versions)
		if currentUser.AccountID != "" {
			fields = map[string]interface{}{
				"assignee": map[string]string{"id": currentUser.AccountID},
			}
		} else {
			// Fallback to name for older JIRA instances
			fields = map[string]interface{}{
				"assignee": map[string]string{"name": currentUser.Name},
			}
		}

		// Update the ticket
		if err := client.UpdateIssue(ticketKey, fields); err != nil {
			return err
		}

		fmt.Printf("Ticket %s grabbed by %s\n", ticketKey, currentUser.DisplayName)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(grabCmd)
}