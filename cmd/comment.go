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
	commentFile string
)

var commentCmd = &cobra.Command{
	Use:   "comment TICKET-KEY COMMENT",
	Short: "Add a comment to a JIRA ticket",
	Long: `Add a comment to a JIRA ticket.
	
You can provide the comment text directly as an argument or read from a file using --file.`,
	Args: cobra.RangeArgs(1, 2),
	RunE: func(cmd *cobra.Command, args []string) error {
		ticketKey := args[0]
		var commentText string

		// Get comment text from file or argument
		if commentFile != "" {
			if commentFile == "-" {
				// Read from stdin
				content, err := io.ReadAll(os.Stdin)
				if err != nil {
					return fmt.Errorf("failed to read from stdin: %w", err)
				}
				commentText = strings.TrimSpace(string(content))
			} else {
				// Read from file
				content, err := os.ReadFile(commentFile)
				if err != nil {
					return fmt.Errorf("failed to read comment file: %w", err)
				}
				commentText = strings.TrimSpace(string(content))
			}
		} else if len(args) == 2 {
			commentText = args[1]
		} else {
			return fmt.Errorf("comment text required: provide as argument or use --file")
		}

		if commentText == "" {
			return fmt.Errorf("comment text cannot be empty")
		}

		// Load configuration
		cfg, err := config.Load()
		if err != nil {
			return fmt.Errorf("configuration error: %w", err)
		}

		// Create JIRA client
		client := jira.NewClient(cfg.URL, cfg.Email, cfg.Username, cfg.Token)

		// Add the comment
		if err := client.AddComment(ticketKey, commentText); err != nil {
			return err
		}

		fmt.Printf("Comment added to %s successfully\n", ticketKey)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(commentCmd)
	
	commentCmd.Flags().StringVarP(&commentFile, "file", "f", "", "Read comment from file (use '-' for stdin)")
}