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
	createProject     string
	createSummary     string
	createDescription string
	createDescFile    string
	createIssueType   string
	createEpic        string
)

var createCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a new JIRA ticket",
	Long: `Create a new JIRA ticket with the specified fields.
	
Required fields:
  --project: Project key (e.g., PROJ)
  --summary: Ticket summary/title
  
Optional fields:
  --description: Ticket description
  --description-file: Read description from file
  --type: Issue type (default: Story)
  --epic: Epic key to link this ticket to`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Validate required fields
		if createProject == "" {
			return fmt.Errorf("project key is required (use --project)")
		}
		if createSummary == "" {
			return fmt.Errorf("summary is required (use --summary)")
		}

		// Handle description from file
		description := createDescription
		if createDescFile != "" {
			var content []byte
			var err error
			if createDescFile == "-" {
				// Read from stdin
				content, err = io.ReadAll(os.Stdin)
				if err != nil {
					return fmt.Errorf("failed to read from stdin: %w", err)
				}
			} else {
				// Read from file
				content, err = os.ReadFile(createDescFile)
				if err != nil {
					return fmt.Errorf("failed to read description file: %w", err)
				}
			}
			description = strings.TrimSpace(string(content))
		}

		// Set default issue type if not specified
		issueType := createIssueType
		if issueType == "" {
			issueType = "Story"
		}

		// Load configuration
		cfg, err := config.Load()
		if err != nil {
			return fmt.Errorf("configuration error: %w", err)
		}

		// Create JIRA client
		client := jira.NewClient(cfg.URL, cfg.Email, cfg.Username, cfg.Token)

		// Create the ticket
		issue, err := client.CreateIssue(createProject, createSummary, description, issueType, createEpic)
		if err != nil {
			return err
		}

		fmt.Printf("Ticket created successfully: %s\n", issue.Key)
		fmt.Printf("Summary: %s\n", issue.Fields.Summary)
		if createEpic != "" {
			fmt.Printf("Linked to epic: %s\n", createEpic)
		}
		
		return nil
	},
}

func init() {
	rootCmd.AddCommand(createCmd)
	
	createCmd.Flags().StringVarP(&createProject, "project", "p", "", "Project key (required)")
	createCmd.Flags().StringVarP(&createSummary, "summary", "s", "", "Ticket summary/title (required)")
	createCmd.Flags().StringVarP(&createDescription, "description", "d", "", "Ticket description")
	createCmd.Flags().StringVar(&createDescFile, "description-file", "", "Read description from file (use '-' for stdin)")
	createCmd.Flags().StringVarP(&createIssueType, "type", "t", "Story", "Issue type")
	createCmd.Flags().StringVarP(&createEpic, "epic", "e", "", "Epic key to link this ticket to")
	
	// Mark required flags
	createCmd.MarkFlagRequired("project")
	createCmd.MarkFlagRequired("summary")
}