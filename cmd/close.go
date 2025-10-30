package cmd

import (
	"fmt"
	"strings"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
	"jet/internal/config"
	"jet/internal/jira"
)

var closeCmd = &cobra.Command{
	Use:   "close TICKET-KEY",
	Short: "Close a ticket (transition to Done/Closed)",
	Long: `Close a JIRA ticket by transitioning it to "Done" or "Closed" status.

This is a shortcut for: jet shift TICKET-KEY "Done"

Example:
  jet close LX-123`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ticketKey := args[0]

		// Load configuration
		cfg, err := config.Load()
		if err != nil {
			return fmt.Errorf("configuration error: %w", err)
		}

		// Create JIRA client
		client := jira.NewClient(cfg.URL, cfg.Email, cfg.Username, cfg.Token)

		// Get available transitions
		transitions, err := client.GetTransitions(ticketKey)
		if err != nil {
			return fmt.Errorf("failed to get transitions: %w", err)
		}

		if len(transitions) == 0 {
			return fmt.Errorf("no transitions available for %s", ticketKey)
		}

		// Find "Done", "Closed", or similar transition
		var matchedTransition *jira.Transition
		closeStatuses := []string{"done", "closed", "resolved", "complete"}

		for i := range transitions {
			transitionStatusLower := strings.ToLower(transitions[i].To.Name)
			for _, status := range closeStatuses {
				if transitionStatusLower == status || strings.Contains(transitionStatusLower, status) {
					matchedTransition = &transitions[i]
					break
				}
			}
			if matchedTransition != nil {
				break
			}
		}

		if matchedTransition == nil {
			// Show available transitions
			cyan := color.New(color.FgCyan, color.Bold)
			gray := color.New(color.FgHiBlack)

			fmt.Printf("No 'Done/Closed' transition found for %s\n\n", ticketKey)
			cyan.Println("Available transitions:")
			for _, t := range transitions {
				fmt.Printf("  %s %s\n", gray.Sprint("→"), t.To.Name)
			}
			return fmt.Errorf("invalid transition")
		}

		// Perform the transition
		if err := client.TransitionIssue(ticketKey, matchedTransition.ID); err != nil {
			return fmt.Errorf("failed to transition issue: %w", err)
		}

		green := color.New(color.FgGreen, color.Bold)
		fmt.Printf("%s Closed %s → %s\n", green.Sprint("✓"), ticketKey, matchedTransition.To.Name)

		return nil
	},
}

func init() {
	rootCmd.AddCommand(closeCmd)
}
