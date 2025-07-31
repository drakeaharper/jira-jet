package config

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type Config struct {
	URL      string
	Email    string
	Username string
	Token    string
}

func Load() (*Config, error) {
	config := &Config{}

	// Load from environment variables first
	config.URL = os.Getenv("JIRA_URL")
	config.Email = os.Getenv("JIRA_EMAIL")
	config.Username = os.Getenv("JIRA_USERNAME")
	config.Token = os.Getenv("JIRA_API_TOKEN")

	// Load from config file if env vars are missing
	configFile := filepath.Join(os.Getenv("HOME"), ".jira_config")
	if _, err := os.Stat(configFile); err == nil {
		fileConfig, err := loadFromFile(configFile)
		if err != nil {
			return nil, fmt.Errorf("failed to load config file: %w", err)
		}

		// Use file values as fallback
		if config.URL == "" {
			config.URL = fileConfig.URL
		}
		if config.Email == "" {
			config.Email = fileConfig.Email
		}
		if config.Username == "" {
			config.Username = fileConfig.Username
		}
		if config.Token == "" {
			config.Token = fileConfig.Token
		}
	}

	// Validate required fields
	if config.URL == "" {
		return nil, fmt.Errorf("JIRA URL not configured. Set JIRA_URL environment variable or add 'url' to ~/.jira_config")
	}
	if config.Token == "" {
		return nil, fmt.Errorf("JIRA API token not configured. Set JIRA_API_TOKEN environment variable or add 'token' to ~/.jira_config")
	}
	if config.Email == "" && config.Username == "" {
		return nil, fmt.Errorf("JIRA email or username not configured. Set JIRA_EMAIL/JIRA_USERNAME environment variable or add 'email'/'username' to ~/.jira_config")
	}

	return config, nil
}

func loadFromFile(filename string) (*Config, error) {
	// Check file permissions
	fileInfo, err := os.Stat(filename)
	if err != nil {
		return nil, err
	}
	
	// Warn if file permissions are too permissive
	mode := fileInfo.Mode()
	if mode.Perm()&0077 != 0 {
		// Try to fix permissions automatically
		if err := os.Chmod(filename, 0600); err != nil {
			return nil, fmt.Errorf("config file has insecure permissions and could not be fixed: %w", err)
		}
		fmt.Fprintf(os.Stderr, "Warning: Fixed insecure permissions on %s (now 0600)\n", filename)
	}
	
	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	config := &Config{}
	scanner := bufio.NewScanner(file)
	inJiraSection := false

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		
		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Check for section headers
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			sectionName := strings.Trim(line, "[]")
			inJiraSection = strings.ToLower(sectionName) == "jira"
			continue
		}

		// Parse key=value pairs only in [jira] section
		if inJiraSection && strings.Contains(line, "=") {
			parts := strings.SplitN(line, "=", 2)
			if len(parts) == 2 {
				key := strings.TrimSpace(parts[0])
				value := strings.TrimSpace(parts[1])
				
				// Remove quotes if present
				if (strings.HasPrefix(value, "\"") && strings.HasSuffix(value, "\"")) ||
				   (strings.HasPrefix(value, "'") && strings.HasSuffix(value, "'")) {
					value = value[1 : len(value)-1]
				}

				switch strings.ToLower(key) {
				case "url":
					config.URL = value
				case "email":
					config.Email = value
				case "username":
					config.Username = value
				case "token":
					config.Token = value
				}
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return config, nil
}