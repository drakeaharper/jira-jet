package cmd

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"jet/internal/config"
	"jet/internal/jira"
)

var (
	updateDescription string
	updateDescFile    string
)

var updateCmd = &cobra.Command{
	Use:   "update TICKET-KEY",
	Short: "Update a JIRA ticket",
	Long: `Update fields of a JIRA ticket.
	
Currently supports updating the description field.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ticketKey := args[0]

		// Check if any update flags are provided
		if updateDescription == "" && updateDescFile == "" {
			return fmt.Errorf("no update fields specified. Use --description or --description-file")
		}

		// Load configuration
		cfg, err := config.Load()
		if err != nil {
			return fmt.Errorf("configuration error: %w", err)
		}

		// Create JIRA client
		client := jira.NewClient(cfg.URL, cfg.Email, cfg.Username, cfg.Token)

		// Prepare fields to update
		fields := make(map[string]interface{})

		// Handle description update
		if updateDescFile != "" {
			var content []byte
			if updateDescFile == "-" {
				// Read from stdin
				content, err = io.ReadAll(os.Stdin)
				if err != nil {
					return fmt.Errorf("failed to read from stdin: %w", err)
				}
			} else {
				// Read from file
				content, err = os.ReadFile(updateDescFile)
				if err != nil {
					return fmt.Errorf("failed to read description file: %w", err)
				}
			}
			fields["description"] = strings.TrimSpace(string(content))
		} else if updateDescription != "" {
			fields["description"] = updateDescription
		}

		// Update the ticket
		if err := client.UpdateIssue(ticketKey, fields); err != nil {
			return err
		}

		fmt.Printf("Ticket %s updated successfully\n", ticketKey)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(updateCmd)
	
	updateCmd.Flags().StringVar(&updateDescription, "description", "", "New description for the ticket")
	updateCmd.Flags().StringVar(&updateDescFile, "description-file", "", "Read new description from file (use '-' for stdin)")
}