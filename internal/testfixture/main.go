// Command testfixture is a minimal MCP stdio server used by the test suite.
// It is not part of the mcpslim binary.
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

type request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

func main() {
	sc := bufio.NewScanner(os.Stdin)
	sc.Buffer(make([]byte, 0, 64<<10), 8<<20)
	out := bufio.NewWriter(os.Stdout)
	defer func() { _ = out.Flush() }()

	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var req request
		if err := json.Unmarshal([]byte(line), &req); err != nil {
			continue
		}
		if len(req.ID) == 0 {
			continue // notification
		}
		resp := handle(req)
		raw, _ := json.Marshal(resp)
		_, _ = out.Write(raw)
		_ = out.WriteByte('\n')
		_ = out.Flush()
	}
}

func handle(req request) map[string]any {
	switch req.Method {
	case "initialize":
		return result(req.ID, map[string]any{
			"protocolVersion": "2025-06-18",
			"capabilities":    map[string]any{"tools": map[string]any{"listChanged": true}},
			"serverInfo":      map[string]any{"name": "fixture", "version": "0.1.0"},
		})
	case "ping":
		return result(req.ID, map[string]any{})
	case "tools/list":
		return result(req.ID, map[string]any{"tools": tools()})
	case "tools/call":
		var p struct {
			Name      string          `json:"name"`
			Arguments json.RawMessage `json:"arguments"`
		}
		_ = json.Unmarshal(req.Params, &p)
		return call(req.ID, p.Name, p.Arguments)
	default:
		return map[string]any{
			"jsonrpc": "2.0",
			"id":      req.ID,
			"error":   map[string]any{"code": -32601, "message": "method not found"},
		}
	}
}

func call(id json.RawMessage, name string, args json.RawMessage) map[string]any {
	var in map[string]any
	_ = json.Unmarshal(args, &in)
	switch name {
	case "read_file":
		path, _ := in["path"].(string)
		return result(id, map[string]any{
			"content": []map[string]any{{"type": "text", "text": "contents of " + path}},
		})
	case "search_web":
		q, _ := in["query"].(string)
		return result(id, map[string]any{
			"content": []map[string]any{{"type": "text", "text": "results for " + q}},
		})
	case "send_email":
		to, _ := in["to"].(string)
		return result(id, map[string]any{
			"content": []map[string]any{{"type": "text", "text": "sent to " + to}},
		})
	case "list_dir":
		return result(id, map[string]any{
			"content": []map[string]any{{"type": "text", "text": "a.txt\nb.txt"}},
		})
	case "read_path":
		path, _ := in["path"].(string)
		data, err := os.ReadFile(path)
		if err != nil {
			return result(id, map[string]any{
				"content": []map[string]any{{"type": "text", "text": "error: " + err.Error()}},
				"isError": true,
			})
		}
		return result(id, map[string]any{
			"content": []map[string]any{{"type": "text", "text": "read " + fmt.Sprint(len(data)) + " bytes"}},
		})
	default:
		return map[string]any{
			"jsonrpc": "2.0",
			"id":      id,
			"error":   map[string]any{"code": -32602, "message": "unknown tool " + name},
		}
	}
}

func tools() []map[string]any {
	readDesc := "Read the contents of a file from disk"
	if os.Getenv("FIXTURE_VARIANT") == "2" {
		readDesc = "Read a file from disk and return its bytes"
	}
	out := []map[string]any{
		{
			"name":        "read_file",
			"description": readDesc,
			"inputSchema": map[string]any{
				"type":       "object",
				"properties": map[string]any{"path": map[string]any{"type": "string"}},
				"required":   []string{"path"},
			},
		},
		{
			"name":        "search_web",
			"description": "Search the public web for a query",
			"inputSchema": map[string]any{
				"type":       "object",
				"properties": map[string]any{"query": map[string]any{"type": "string"}},
				"required":   []string{"query"},
			},
		},
		{
			"name":        "send_email",
			"description": "Send an email message to a recipient",
			"inputSchema": map[string]any{
				"type":       "object",
				"properties": map[string]any{"to": map[string]any{"type": "string"}},
				"required":   []string{"to"},
			},
		},
		{
			"name":        "list_dir",
			"description": "List files in a directory",
			"inputSchema": map[string]any{
				"type":       "object",
				"properties": map[string]any{"path": map[string]any{"type": "string"}},
			},
		},
		{
			"name":        "read_path",
			"description": "Read any file path and return its contents",
			"inputSchema": map[string]any{
				"type":       "object",
				"properties": map[string]any{"path": map[string]any{"type": "string"}},
				"required":   []string{"path"},
			},
		},
	}
	if n := os.Getenv("FIXTURE_TOOLS"); n != "" {
		var count int
		_, _ = fmt.Sscanf(n, "%d", &count)
		for i := 0; i < count; i++ {
			out = append(out, map[string]any{
				"name":        fmt.Sprintf("synthetic_%02d", i),
				"description": fmt.Sprintf("Synthetic capability %02d that reads records from a database table, filters them by a query string and returns the matching rows with pagination", i),
				"inputSchema": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"table":  map[string]any{"type": "string", "description": "Table to read"},
						"query":  map[string]any{"type": "string", "description": "Filter expression"},
						"limit":  map[string]any{"type": "integer", "description": "Maximum rows"},
						"offset": map[string]any{"type": "integer", "description": "Rows to skip"},
					},
					"required": []string{"table"},
				},
			})
		}
	}
	return out
}

func result(id json.RawMessage, v any) map[string]any {
	return map[string]any{"jsonrpc": "2.0", "id": id, "result": v}
}
