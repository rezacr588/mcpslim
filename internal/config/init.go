package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// Candidate is a client config file that contains stdio MCP servers.
type Candidate struct {
	Path    string
	Kind    string
	Servers map[string]Server
	Skipped []string
}

// Discover finds MCP client configs under home and cwd.
func Discover(home, cwd string) ([]Candidate, error) {
	type target struct{ path, kind string }
	targets := []target{
		{filepath.Join(cwd, ".mcp.json"), "claude-project"},
		{filepath.Join(home, ".cursor", "mcp.json"), "cursor"},
		{filepath.Join(home, ".gemini", "settings.json"), "gemini"},
	}
	var out []Candidate
	for _, t := range targets {
		cand, err := readCandidate(t.path, t.kind)
		if err != nil {
			return nil, err
		}
		if cand != nil && len(cand.Servers) > 0 {
			out = append(out, *cand)
		}
	}
	return out, nil
}

func readCandidate(path, kind string) (*Candidate, error) {
	raw, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var doc struct {
		MCPServers map[string]struct {
			Command string            `json:"command"`
			Args    []string          `json:"args"`
			Env     map[string]string `json:"env"`
			URL     string            `json:"url"`
			Type    string            `json:"type"`
		} `json:"mcpServers"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	cand := &Candidate{Path: path, Kind: kind, Servers: map[string]Server{}}
	for name, s := range doc.MCPServers {
		if s.Command == "" {
			cand.Skipped = append(cand.Skipped, name)
			continue
		}
		cand.Servers[name] = Server{Command: s.Command, Args: s.Args, Env: s.Env}
	}
	sort.Strings(cand.Skipped)
	return cand, nil
}

// Apply rewrites one client config to route through mcpslim, keeping a backup.
// The command is normally the absolute path to the mcpslim binary.
func Apply(c Candidate, command, cfgPath string) (string, error) {
	raw, err := os.ReadFile(c.Path)
	if err != nil {
		return "", err
	}
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		return "", fmt.Errorf("%s: %w", c.Path, err)
	}
	backup := fmt.Sprintf("%s.mcpslim-backup-%s", c.Path, time.Now().UTC().Format("20060102T150405Z"))
	if err := os.WriteFile(backup, raw, 0o644); err != nil {
		return "", err
	}
	doc["mcpServers"] = map[string]any{
		"mcpslim": map[string]any{
			"command": command,
			"args":    []string{"serve", "--config", cfgPath},
		},
	}
	out, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return "", err
	}
	out = append(out, '\n')
	if err := os.WriteFile(c.Path, out, 0o644); err != nil {
		return "", err
	}
	return backup, nil
}

// Merge folds discovered servers into cfg, returning the number added.
func Merge(cfg *Config, candidates []Candidate) int {
	n := 0
	for _, c := range candidates {
		for name, server := range c.Servers {
			cfg.Servers[name] = server
			n++
		}
	}
	return n
}
