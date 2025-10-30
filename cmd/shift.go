package cmd

import (
	"fmt"
	"strings"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
	"jet/internal/config"
	"jet/internal/jira"
)

var shiftCmd = &cobra.Command{
	Use:   "shift TICKET-KEY STATUS",
	Short: "Transition a JIRA ticket to a new status",
	Long: `Transition a JIRA ticket to a new status.

The command will find the appropriate transition based on the target status name.
Status names are case-insensitive and can be partial matches.

Examples:
  jet shift LX-123 "In Progress"
  jet shift LX-123 done
  jet shift LX-123 closed`,
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		ticketKey := args[0]
		targetStatus := args[1]

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

		// Find matching transition
		var matchedTransition *jira.Transition
		targetStatusLower := strings.ToLower(targetStatus)

		for i := range transitions {
			transitionStatusLower := strings.ToLower(transitions[i].To.Name)
			if transitionStatusLower == targetStatusLower || strings.Contains(transitionStatusLower, targetStatusLower) {
				matchedTransition = &transitions[i]
				break
			}
		}

		if matchedTransition == nil {
			// Show available transitions
			cyan := color.New(color.FgCyan, color.Bold)
			gray := color.New(color.FgHiBlack)

			fmt.Printf("No transition found for status '%s'\n\n", targetStatus)
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
		fmt.Printf("%s Transitioned %s to %s\n", green.Sprint("✓"), ticketKey, matchedTransition.To.Name)

		return nil
	},
}

func init() {
	rootCmd.AddCommand(shiftCmd)
}
