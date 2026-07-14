package prs

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
)

// Config holds the [prs] section of ~/.jira_config.
type Config struct {
	GerritFilter string   // extra Gerrit query filter for `team`, e.g. "ownerin:learning-experience"
	GitHubRepos  []string // owner/repo entries to scan for GitHub PRs
}

// LoadConfig reads the [prs] section from ~/.jira_config. A missing file or
// section yields an empty (but usable) config.
func LoadConfig() (*Config, error) {
	cfg := &Config{}

	// Environment overrides.
	if v := os.Getenv("JET_PRS_GERRIT_FILTER"); v != "" {
		cfg.GerritFilter = v
	}
	if v := os.Getenv("JET_PRS_GITHUB_REPOS"); v != "" {
		cfg.GitHubRepos = splitRepos(v)
	}

	path := filepath.Join(os.Getenv("HOME"), ".jira_config")
	file, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return cfg, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	inSection := false
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			inSection = strings.EqualFold(strings.Trim(line, "[]"), "prs")
			continue
		}
		if !inSection || !strings.Contains(line, "=") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		key := strings.TrimSpace(parts[0])
		val := strings.Trim(strings.TrimSpace(parts[1]), `"'`)
		switch strings.ToLower(key) {
		case "gerrit_filter":
			if cfg.GerritFilter == "" {
				cfg.GerritFilter = val
			}
		case "github_repos":
			if len(cfg.GitHubRepos) == 0 {
				cfg.GitHubRepos = splitRepos(val)
			}
		}
	}
	return cfg, scanner.Err()
}

func splitRepos(v string) []string {
	var repos []string
	for _, r := range strings.Split(v, ",") {
		if r = strings.TrimSpace(r); r != "" {
			repos = append(repos, r)
		}
	}
	return repos
}
