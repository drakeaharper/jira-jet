// Package gerry loads authentication config from the gerry (gerrit-cli) tool
// so jet can talk to Gerrit's REST API without duplicating credential setup.
package gerry

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// Config mirrors the subset of ~/.gerry/config.json that jet needs for REST auth.
type Config struct {
	Server       string `json:"server"`
	Port         int    `json:"port"`
	HTTPPort     int    `json:"http_port"`
	User         string `json:"user"`
	HTTPPassword string `json:"http_password"`
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
