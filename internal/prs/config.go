package prs

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// configPath returns the path to ~/.jira_config.
func configPath() string {
	return filepath.Join(os.Getenv("HOME"), ".jira_config")
}

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

	file, err := os.Open(configPath())
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

// NormalizeRepo validates and canonicalizes an "owner/repo" string.
func NormalizeRepo(repo string) (string, error) {
	repo = strings.TrimSpace(repo)
	repo = strings.TrimPrefix(repo, "https://github.com/")
	repo = strings.TrimSuffix(repo, "/")
	repo = strings.TrimSuffix(repo, ".git")
	parts := strings.Split(repo, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", fmt.Errorf("expected owner/repo, got %q", repo)
	}
	return parts[0] + "/" + parts[1], nil
}

// ConfiguredRepos returns the repos from the [prs] section of the config file
// only (ignoring the env override), since that is what add/rm operate on.
func ConfiguredRepos() ([]string, error) {
	data, err := os.ReadFile(configPath())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	inSection := false
	for _, line := range strings.Split(string(data), "\n") {
		t := strings.TrimSpace(line)
		if strings.HasPrefix(t, "[") && strings.HasSuffix(t, "]") {
			inSection = strings.EqualFold(strings.Trim(t, "[]"), "prs")
			continue
		}
		if inSection && strings.HasPrefix(strings.ToLower(t), "github_repos") && strings.Contains(t, "=") {
			return splitRepos(strings.Trim(strings.TrimSpace(strings.SplitN(t, "=", 2)[1]), `"'`)), nil
		}
	}
	return nil, nil
}

// SetRepos writes the github_repos entry in the [prs] section, preserving the
// rest of the file. Creates the key or the whole section if absent.
func SetRepos(repos []string) error {
	value := "github_repos = " + strings.Join(repos, ",")

	data, err := os.ReadFile(configPath())
	if err != nil && !os.IsNotExist(err) {
		return err
	}

	var lines []string
	if len(data) > 0 {
		lines = strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	}

	// Locate the [prs] section and, within it, the github_repos line.
	prsStart, prsEnd, keyLine := -1, len(lines), -1
	inSection := false
	for i, line := range lines {
		t := strings.TrimSpace(line)
		if strings.HasPrefix(t, "[") && strings.HasSuffix(t, "]") {
			if inSection { // reached the next section → [prs] ended here
				prsEnd = i
				break
			}
			if strings.EqualFold(strings.Trim(t, "[]"), "prs") {
				inSection = true
				prsStart = i
			}
			continue
		}
		if inSection && strings.HasPrefix(strings.ToLower(t), "github_repos") && strings.Contains(t, "=") {
			keyLine = i
		}
	}

	switch {
	case keyLine >= 0:
		lines[keyLine] = value
	case prsStart >= 0:
		// Section exists but no github_repos key — insert at end of section.
		lines = append(lines[:prsEnd], append([]string{value}, lines[prsEnd:]...)...)
	default:
		// No [prs] section — append one.
		if len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) != "" {
			lines = append(lines, "")
		}
		lines = append(lines, "[prs]", value)
	}

	out := strings.Join(lines, "\n") + "\n"
	return os.WriteFile(configPath(), []byte(out), 0600)
}

// AddRepo appends a repo (deduped, case-insensitive) and returns the new list.
func AddRepo(repo string) ([]string, error) {
	norm, err := NormalizeRepo(repo)
	if err != nil {
		return nil, err
	}
	repos, err := ConfiguredRepos()
	if err != nil {
		return nil, err
	}
	for _, r := range repos {
		if strings.EqualFold(r, norm) {
			return repos, fmt.Errorf("%s is already configured", norm)
		}
	}
	repos = append(repos, norm)
	if err := SetRepos(repos); err != nil {
		return nil, err
	}
	return repos, nil
}

// RemoveRepo drops a repo (case-insensitive) and returns the new list.
func RemoveRepo(repo string) ([]string, error) {
	norm, err := NormalizeRepo(repo)
	if err != nil {
		return nil, err
	}
	repos, err := ConfiguredRepos()
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(repos))
	found := false
	for _, r := range repos {
		if strings.EqualFold(r, norm) {
			found = true
			continue
		}
		out = append(out, r)
	}
	if !found {
		return repos, fmt.Errorf("%s is not configured", norm)
	}
	if err := SetRepos(out); err != nil {
		return nil, err
	}
	return out, nil
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
