package cmd

import (
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "jet",
	Short: "A command-line tool for interacting with JIRA",
	Long: `Jet is a fast and simple CLI tool for JIRA operations.
	
It allows you to view tickets, add comments, update descriptions, and create new tickets
directly from the command line.

Configuration:
Set environment variables or create ~/.jira_config:
  JIRA_URL - Your JIRA instance URL (e.g., https://yourcompany.atlassian.net)
  JIRA_EMAIL - Your email address (for cloud instances)
  JIRA_API_TOKEN - Your API token
  JIRA_USERNAME - Your username (for server instances)`,
}

func Execute() error {
	return rootCmd.Execute()
}

func init() {
	rootCmd.CompletionOptions.DisableDefaultCmd = true
}