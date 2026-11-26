// Package config loads mcpslim's server configuration and rewrites the MCP
// client configs it discovers.
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

// Config is the mcpslim configuration document.
type Config struct {
	Servers  map[string]Server `json:"servers"`
	Router   Router            `json:"router"`
	Sandbox  Sandbox           `json:"sandbox"`
	Lockfile string            `json:"lockfile,omitempty"`
	dir      string
}

// Server is one upstream MCP server.
type Server struct {
	Command string            `json:"command"`
	Args    []string          `json:"args,omitempty"`
	Env     map[string]string `json:"env,omitempty"`
	Dir     string            `json:"dir,omitempty"`
}

// Router tunes dynamic tool loading.
type Router struct {
	TopK    int   `json:"topK"`
	Hydrate *bool `json:"hydrate,omitempty"`
}

// HydrateEnabled reports whether matched tools are exposed after a search.
func (r Router) HydrateEnabled() bool { return r.Hydrate == nil || *r.Hydrate }

// Sandbox tunes process isolation.
type Sandbox struct {
	Disabled    bool `json:"disabled"`
	DenyNetwork bool `json:"denyNetwork"`
}

// DefaultPath is ~/.config/mcpslim/config.json (or the OS equivalent).
func DefaultPath() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		home, _ := os.UserHomeDir()
		dir = filepath.Join(home, ".config")
	}
	return filepath.Join(dir, "mcpslim", "config.json")
}

// New returns a config with defaults applied, rooted at the working directory.
func New() *Config {
	c := &Config{}
	c.applyDefaults(".")
	return c
}

// Load reads path, applying defaults. A missing file yields an empty config.
func Load(path string) (*Config, error) {
	cfg := &Config{}
	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		cfg.applyDefaults(filepath.Dir(path))
		return cfg, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(raw, cfg); err != nil {
		return nil, fmt.Errorf("config %s: %w", path, err)
	}
	cfg.applyDefaults(filepath.Dir(path))
	return cfg, nil
}

// Save writes the config atomically.
func (c *Config) Save(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	raw = append(raw, '\n')
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// LockPath resolves the lockfile relative to the config directory.
func (c *Config) LockPath() string {
	if filepath.IsAbs(c.Lockfile) {
		return c.Lockfile
	}
	return filepath.Join(c.dir, c.Lockfile)
}

// Dir is the directory the config was loaded from.
func (c *Config) Dir() string { return c.dir }

// ServerNames returns the configured server names, sorted.
func (c *Config) ServerNames() []string {
	names := make([]string, 0, len(c.Servers))
	for name := range c.Servers {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func (c *Config) applyDefaults(dir string) {
	if c.Servers == nil {
		c.Servers = map[string]Server{}
	}
	if c.Router.TopK <= 0 {
		c.Router.TopK = 5
	}
	if c.Lockfile == "" {
		c.Lockfile = "mcpslim.lock"
	}
	if dir == "" {
		dir = "."
	}
	c.dir = dir
}
