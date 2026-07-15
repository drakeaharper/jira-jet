// Package gerry loads authentication config from the gerry (gerrit-cli) tool
// so jet can talk to Gerrit's REST API without duplicating credential setup.
package gerry

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// Config mirrors the subset of ~/.gerry/config.json that jet needs: REST auth
// plus the reviewability rules, so jet's PR split matches `gerry team`.
type Config struct {
	Server        string        `json:"server"`
	Port          int           `json:"port"`
	HTTPPort      int           `json:"http_port"`
	User          string        `json:"user"`
	HTTPPassword  string        `json:"http_password"`
	Reviewability Reviewability `json:"reviewability,omitempty"`
}

// Reviewability controls which conditions mark a change as not reviewable,
// mirroring gerry's own config shape.
type Reviewability struct {
	// BlockMergeConflict, when set, controls whether a merge conflict makes a
	// change not reviewable. Nil falls back to the default (true).
	BlockMergeConflict *bool `json:"block_merge_conflict,omitempty"`
	// BlockingLabels maps a label to the vote threshold at or below which the
	// change is not reviewable. Nil falls back to DefaultBlockingLabels.
	BlockingLabels map[string]int `json:"blocking_labels,omitempty"`
}

// DefaultBlockingLabels reproduces gerry's built-in rules: Code-Review and
// QA-Review block at -1 or below, Lint-Review only at -2.
var DefaultBlockingLabels = map[string]int{
	"Code-Review": -1,
	"QA-Review":   -1,
	"Lint-Review": -2,
}

// ReviewabilityRules returns the effective settings with defaults applied.
func (c *Config) ReviewabilityRules() (blockMergeConflict bool, blockingLabels map[string]int) {
	blockMergeConflict = true
	if c.Reviewability.BlockMergeConflict != nil {
		blockMergeConflict = *c.Reviewability.BlockMergeConflict
	}
	blockingLabels = c.Reviewability.BlockingLabels
	if blockingLabels == nil {
		blockingLabels = DefaultBlockingLabels
	}
	return blockMergeConflict, blockingLabels
}

// Load reads ~/.gerry/config.json.
func Load() (*Config, error) {
	path := filepath.Join(os.Getenv("HOME"), ".gerry", "config.json")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("gerry config not found at %s — run `gerry init` first", path)
		}
		return nil, fmt.Errorf("failed to read gerry config: %w", err)
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse gerry config: %w", err)
	}
	if cfg.Server == "" || cfg.User == "" || cfg.HTTPPassword == "" {
		return nil, fmt.Errorf("gerry config incomplete (need server, user, http_password)")
	}
	return &cfg, nil
}

// RESTURL builds an authenticated-endpoint URL for the given path.
// Gerrit exposes authenticated REST under /a/.
func (c *Config) RESTURL(path string) string {
	protocol := "https"
	port := c.HTTPPort
	if port == 0 {
		// gerry default: SSH on 29418 → HTTPS on 443.
		if c.Port == 29418 || c.Port == 0 {
			port = 443
		} else {
			port = c.Port
		}
	}
	switch port {
	case 80, 8080:
		protocol = "http"
	}
	if port == 443 {
		return fmt.Sprintf("%s://%s/a/%s", protocol, c.Server, path)
	}
	return fmt.Sprintf("%s://%s:%d/a/%s", protocol, c.Server, port, path)
}
