// Package lockfile pins MCP tool definitions by canonical hash so an upstream
// server cannot silently change a tool's description or schema.
package lockfile

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/rezacr588/mcpslim/internal/mcp"
)

// Version is the lockfile format version.
const Version = 1

// Lock is the on-disk mcpslim.lock document.
type Lock struct {
	Version int                `json:"version"`
	Servers map[string]*Server `json:"servers"`
	path    string
}

// Server pins the tools of one upstream server.
type Server struct {
	Tools map[string]*Pin `json:"tools"`
}

// Pin holds the approved hash and, when drift was seen, the pending hash.
type Pin struct {
	SHA256     string `json:"sha256"`
	Pending    string `json:"pending,omitempty"`
	ApprovedAt string `json:"approved_at,omitempty"`
}

// Drift describes a tool whose definition no longer matches its pin.
type Drift struct {
	Server string
	Tool   string
}

// Load reads a lockfile, returning an empty lock when the file is absent.
func Load(path string) (*Lock, error) {
	l := &Lock{Version: Version, Servers: map[string]*Server{}, path: path}
	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return l, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(raw, l); err != nil {
		return nil, fmt.Errorf("lockfile %s: %w", path, err)
	}
	if l.Servers == nil {
		l.Servers = map[string]*Server{}
	}
	l.path = path
	return l, nil
}

// Save writes the lockfile atomically.
func (l *Lock) Save() error {
	if l.path == "" {
		return errors.New("lockfile: no path")
	}
	if err := os.MkdirAll(filepath.Dir(l.path), 0o755); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(l, "", "  ")
	if err != nil {
		return err
	}
	raw = append(raw, '\n')
	tmp := l.path + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, l.path)
}

// Check compares the upstream tools of server with the pins and records drift.
// Unknown tools are pinned on first sight. It returns the drifted tools and
// whether the lock changed.
func (l *Lock) Check(server string, tools []mcp.Tool) ([]Drift, bool, error) {
	s := l.Servers[server]
	if s == nil {
		s = &Server{Tools: map[string]*Pin{}}
		l.Servers[server] = s
	}
	if s.Tools == nil {
		s.Tools = map[string]*Pin{}
	}
	var drift []Drift
	changed := false
	now := time.Now().UTC().Format(time.RFC3339)
	for _, tool := range tools {
		hash, err := Hash(tool)
		if err != nil {
			return nil, false, fmt.Errorf("%s/%s: %w", server, tool.Name, err)
		}
		pin := s.Tools[tool.Name]
		switch {
		case pin == nil:
			s.Tools[tool.Name] = &Pin{SHA256: hash, ApprovedAt: now}
			changed = true
		case pin.SHA256 == hash:
			if pin.Pending != "" {
				pin.Pending = ""
				changed = true
			}
		default:
			if pin.Pending != hash {
				pin.Pending = hash
				changed = true
			}
			drift = append(drift, Drift{Server: server, Tool: tool.Name})
		}
	}
	return drift, changed, nil
}

// Approve promotes pending hashes to approved. With empty server/tool it
// approves every pending change.
func (l *Lock) Approve(server, tool string) int {
	now := time.Now().UTC().Format(time.RFC3339)
	n := 0
	for name, s := range l.Servers {
		if server != "" && name != server {
			continue
		}
		for toolName, pin := range s.Tools {
			if tool != "" && toolName != tool {
				continue
			}
			if pin.Pending == "" {
				continue
			}
			pin.SHA256 = pin.Pending
			pin.Pending = ""
			pin.ApprovedAt = now
			n++
		}
	}
	return n
}

// Pending lists every unapproved change.
func (l *Lock) Pending() []Drift {
	var out []Drift
	for name, s := range l.Servers {
		for toolName, pin := range s.Tools {
			if pin.Pending != "" {
				out = append(out, Drift{Server: name, Tool: toolName})
			}
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Server == out[j].Server {
			return out[i].Tool < out[j].Tool
		}
		return out[i].Server < out[j].Server
	})
	return out
}

// Hash returns the SHA-256 of a tool's canonical definition.
func Hash(tool mcp.Tool) (string, error) {
	canonical, err := Canonical(tool)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(canonical)
	return hex.EncodeToString(sum[:]), nil
}

// Canonical serialises a tool with sorted keys, normalised numbers and no
// insignificant whitespace, so formatting changes never look like drift.
func Canonical(tool mcp.Tool) ([]byte, error) {
	var schema any
	if len(tool.InputSchema) > 0 {
		dec := json.NewDecoder(bytes.NewReader(tool.InputSchema))
		dec.UseNumber()
		if err := dec.Decode(&schema); err != nil {
			return nil, fmt.Errorf("inputSchema: %w", err)
		}
	}
	doc := map[string]any{
		"name":        tool.Name,
		"description": normaliseText(tool.Description),
		"inputSchema": schema,
	}
	return json.Marshal(doc)
}

func normaliseText(s string) string {
	out := make([]rune, 0, len(s))
	space := false
	for _, r := range s {
		if r == ' ' || r == '\t' || r == '\n' || r == '\r' {
			space = true
			continue
		}
		if space && len(out) > 0 {
			out = append(out, ' ')
		}
		space = false
		out = append(out, r)
	}
	return string(out)
}
