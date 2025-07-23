package cmd

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize JIRA configuration",
	Long: `Initialize JIRA configuration by setting up environment variables.
	
This command will prompt you for JIRA connection details and help you configure
the required environment variables. If values are already set, you can press
Enter to keep the existing values.`,
	RunE: runInit,
}

func runInit(cmd *cobra.Command, args []string) error {
	reader := bufio.NewReader(os.Stdin)

	fmt.Println("JIRA Jet Configuration")
	fmt.Println("======================")
	fmt.Println()

	// Get current values from environment
	currentURL := os.Getenv("JIRA_URL")
	currentEmail := os.Getenv("JIRA_EMAIL")
	currentUsername := os.Getenv("JIRA_USERNAME")
	currentToken := os.Getenv("JIRA_API_TOKEN")

	// Prompt for JIRA URL
	url := promptWithDefault(reader, "JIRA URL (e.g., https://yourcompany.atlassian.net)", currentURL)
	
	// Prompt for email or username
	email := promptWithDefault(reader, "JIRA Email (for cloud instances)", currentEmail)
	username := promptWithDefault(reader, "JIRA Username (for server instances, leave blank if using email)", currentUsername)
	
	// Prompt for API token
	token := promptWithDefault(reader, "JIRA API Token", currentToken)

	fmt.Println()
	fmt.Println("Configuration Summary:")
	fmt.Println("======================")
	fmt.Printf("JIRA_URL: %s\n", url)
	if email != "" {
		fmt.Printf("JIRA_EMAIL: %s\n", email)
	}
	if username != "" {
		fmt.Printf("JIRA_USERNAME: %s\n", username)
	}
	fmt.Printf("JIRA_API_TOKEN: %s\n", maskToken(token))
	fmt.Println()

	// Ask for confirmation
	confirm := promptWithDefault(reader, "Save configuration? (y/N)", "N")
	if !strings.EqualFold(strings.TrimSpace(confirm), "y") {
		fmt.Println("Configuration cancelled.")
		return nil
	}

	// Ask whether to save as environment variables or config file
	saveMethod := promptWithDefault(reader, "Save as (1) environment variables or (2) config file? (1/2)", "1")
	
	if strings.TrimSpace(saveMethod) == "2" {
		return saveToConfigFile(url, email, username, token)
	} else {
		return saveAsEnvVars(url, email, username, token)
	}
}

func promptWithDefault(reader *bufio.Reader, prompt, defaultValue string) string {
	if defaultValue != "" {
		fmt.Printf("%s [%s]: ", prompt, maskIfToken(prompt, defaultValue))
	} else {
		fmt.Printf("%s: ", prompt)
	}

	input, _ := reader.ReadString('\n')
	input = strings.TrimSpace(input)

	if input == "" {
		return defaultValue
	}
	return input
}

func maskIfToken(prompt, value string) string {
	if strings.Contains(strings.ToLower(prompt), "token") {
		return maskToken(value)
	}
	return value
}

func maskToken(token string) string {
	if len(token) <= 8 {
		return strings.Repeat("*", len(token))
	}
	return token[:4] + strings.Repeat("*", len(token)-8) + token[len(token)-4:]
}

func saveAsEnvVars(url, email, username, token string) error {
	fmt.Println()
	fmt.Println("Add these environment variables to your shell profile (.bashrc, .zshrc, etc.):")
	fmt.Println("===========================================================================")
	fmt.Printf("export JIRA_URL=\"%s\"\n", url)
	if email != "" {
		fmt.Printf("export JIRA_EMAIL=\"%s\"\n", email)
	}
	if username != "" {
		fmt.Printf("export JIRA_USERNAME=\"%s\"\n", username)
	}
	fmt.Printf("export JIRA_API_TOKEN=\"%s\"\n", token)
	fmt.Println()
	fmt.Println("After adding these to your shell profile, restart your terminal or run 'source ~/.bashrc' (or equivalent).")
	
	return nil
}

func saveToConfigFile(url, email, username, token string) error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get home directory: %w", err)
	}

	configPath := filepath.Join(homeDir, ".jira_config")
	
	// Check if file exists and read existing content
	var existingContent []string
	if _, err := os.Stat(configPath); err == nil {
		file, err := os.Open(configPath)
		if err != nil {
			return fmt.Errorf("failed to read existing config: %w", err)
		}
		defer file.Close()

		scanner := bufio.NewScanner(file)
		inJiraSection := false
		
		for scanner.Scan() {
			line := scanner.Text()
			
			// Skip existing JIRA section entries
			if strings.HasPrefix(strings.TrimSpace(line), "[jira]") {
				inJiraSection = true
				continue
			}
			if inJiraSection && strings.HasPrefix(strings.TrimSpace(line), "[") {
				inJiraSection = false
			}
			if inJiraSection && (strings.Contains(line, "url=") || 
				strings.Contains(line, "email=") || 
				strings.Contains(line, "username=") || 
				strings.Contains(line, "token=")) {
				continue
			}
			
			existingContent = append(existingContent, line)
		}
	}

	// Create new config content
	file, err := os.Create(configPath)
	if err != nil {
		return fmt.Errorf("failed to create config file: %w", err)
	}
	defer file.Close()

	// Write existing content first
	for _, line := range existingContent {
		fmt.Fprintln(file, line)
	}

	// Add JIRA section
	fmt.Fprintln(file, "[jira]")
	fmt.Fprintf(file, "url=%s\n", url)
	if email != "" {
		fmt.Fprintf(file, "email=%s\n", email)
	}
	if username != "" {
		fmt.Fprintf(file, "username=%s\n", username)
	}
	fmt.Fprintf(file, "token=%s\n", token)

	fmt.Printf("Configuration saved to %s\n", configPath)
	return nil
}

func init() {
	rootCmd.AddCommand(initCmd)
}