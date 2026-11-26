package mcp

import (
	"context"
	"encoding/json"
	"fmt"
)

// ProtocolVersion is the MCP revision mcpslim speaks by default.
const ProtocolVersion = "2025-06-18"

// Initialize performs the MCP handshake.
func (c *Conn) Initialize(ctx context.Context, clientName, clientVersion string) (*InitializeResult, error) {
	params := InitializeParams{
		ProtocolVersion: ProtocolVersion,
		Capabilities:    map[string]any{},
		ClientInfo:      ServerInfo{Name: clientName, Version: clientVersion},
	}
	resp, err := c.Call(ctx, "initialize", params)
	if err != nil {
		return nil, err
	}
	if resp.Error != nil {
		return nil, resp.Error
	}
	var out InitializeResult
	if err := json.Unmarshal(resp.Result, &out); err != nil {
		return nil, fmt.Errorf("initialize: decode result: %w", err)
	}
	if err := c.Notify("notifications/initialized", nil); err != nil {
		return nil, err
	}
	return &out, nil
}

// ListTools returns every tool, following pagination cursors.
func (c *Conn) ListTools(ctx context.Context) ([]Tool, error) {
	var all []Tool
	cursor := ""
	for {
		params := map[string]any{}
		if cursor != "" {
			params["cursor"] = cursor
		}
		resp, err := c.Call(ctx, "tools/list", params)
		if err != nil {
			return nil, err
		}
		if resp.Error != nil {
			return nil, resp.Error
		}
		var page ListToolsResult
		if err := json.Unmarshal(resp.Result, &page); err != nil {
			return nil, fmt.Errorf("tools/list: decode result: %w", err)
		}
		all = append(all, page.Tools...)
		if page.NextCursor == "" {
			return all, nil
		}
		cursor = page.NextCursor
	}
}

// CallTool invokes a tool and returns its result.
func (c *Conn) CallTool(ctx context.Context, name string, args json.RawMessage) (*CallToolResult, error) {
	params := CallToolParams{Name: name, Arguments: args}
	resp, err := c.Call(ctx, "tools/call", params)
	if err != nil {
		return nil, err
	}
	if resp.Error != nil {
		return nil, resp.Error
	}
	var out CallToolResult
	if err := json.Unmarshal(resp.Result, &out); err != nil {
		return nil, fmt.Errorf("tools/call: decode result: %w", err)
	}
	return &out, nil
}
