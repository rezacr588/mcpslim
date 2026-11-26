package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadDefaults(t *testing.T) {
	cfg, err := Load(filepath.Join(t.TempDir(), "missing.json"))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Router.TopK != 5 {
		t.Errorf("topK = %d, want 5", cfg.Router.TopK)
	}
	if !cfg.Router.HydrateEnabled() {
		t.Error("hydration should default to on")
	}
	if cfg.Lockfile != "mcpslim.lock" {
		t.Errorf("lockfile = %q", cfg.Lockfile)
	}
}

func TestLoadExplicitHydrateFalse(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(`{"router":{"topK":3,"hydrate":false}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Router.TopK != 3 || cfg.Router.HydrateEnabled() {
		t.Fatalf("router = %+v", cfg.Router)
	}
}

func TestDiscoverAndApply(t *testing.T) {
	home := t.TempDir()
	cwd := t.TempDir()
	cursor := filepath.Join(home, ".cursor")
	if err := os.MkdirAll(cursor, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(cursor, "mcp.json"), `{
	  "mcpServers": {
	    "filesystem": {"command": "npx", "args": ["-y", "@modelcontextprotocol/server-filesystem", "."]},
	    "remote": {"url": "https://example.com/mcp"}
	  }
	}`)
	writeFile(t, filepath.Join(cwd, ".mcp.json"), `{
	  "mcpServers": {
	    "github": {"command": "npx", "args": ["-y", "@modelcontextprotocol/server-github"], "env": {"TOKEN": "x"}}
	  }
	}`)

	cands, err := Discover(home, cwd)
	if err != nil {
		t.Fatal(err)
	}
	if len(cands) != 2 {
		t.Fatalf("candidates = %d, want 2", len(cands))
	}
	var cursorCand Candidate
	for _, c := range cands {
		if c.Kind == "cursor" {
			cursorCand = c
		}
	}
	if _, ok := cursorCand.Servers["filesystem"]; !ok {
		t.Fatalf("filesystem server missing: %+v", cursorCand.Servers)
	}
	if len(cursorCand.Skipped) != 1 || cursorCand.Skipped[0] != "remote" {
		t.Fatalf("skipped = %v", cursorCand.Skipped)
	}

	cfgPath := filepath.Join(home, ".config", "mcpslim", "config.json")
	cfg := New()
	if n := Merge(cfg, cands); n != 2 {
		t.Fatalf("merged %d, want 2", n)
	}
	if err := cfg.Save(cfgPath); err != nil {
		t.Fatal(err)
	}
	backup, err := Apply(cursorCand, "/usr/local/bin/mcpslim", cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(backup); err != nil {
		t.Fatalf("backup missing: %v", err)
	}
	raw, err := os.ReadFile(cursorCand.Path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "/usr/local/bin/mcpslim") || !strings.Contains(string(raw), "mcpslim") {
		t.Fatalf("rewritten config: %s", raw)
	}
	if strings.Contains(string(raw), "server-filesystem") {
		t.Fatal("original server entries survived the rewrite")
	}
}
