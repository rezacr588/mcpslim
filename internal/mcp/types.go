package mcp

import "encoding/json"

// Tool is an MCP tool definition as returned by tools/list.
type Tool struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	InputSchema json.RawMessage `json:"inputSchema,omitempty"`
}

// ServerInfo identifies an MCP implementation.
type ServerInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// Capabilities is the subset of MCP capabilities mcpslim understands.
type Capabilities struct {
	Tools     *ListChanged `json:"tools,omitempty"`
	Resources *ListChanged `json:"resources,omitempty"`
	Prompts   *ListChanged `json:"prompts,omitempty"`
}

// ListChanged advertises list_changed notification support.
type ListChanged struct {
	ListChanged bool `json:"listChanged"`
}

// InitializeParams is the client side of initialize.
type InitializeParams struct {
	ProtocolVersion string         `json:"protocolVersion"`
	Capabilities    map[string]any `json:"capabilities"`
	ClientInfo      ServerInfo     `json:"clientInfo"`
}

// InitializeResult is the server side of initialize.
type InitializeResult struct {
	ProtocolVersion string       `json:"protocolVersion"`
	Capabilities    Capabilities `json:"capabilities"`
	ServerInfo      ServerInfo   `json:"serverInfo"`
	Instructions    string       `json:"instructions,omitempty"`
}

// ListToolsResult is the tools/list result.
type ListToolsResult struct {
	Tools      []Tool `json:"tools"`
	NextCursor string `json:"nextCursor,omitempty"`
}

// CallToolParams is the tools/call request.
type CallToolParams struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments,omitempty"`
}

// Content is one content block in a tool result.
type Content struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

// CallToolResult is the tools/call result.
type CallToolResult struct {
	Content []Content `json:"content"`
	IsError bool      `json:"isError,omitempty"`
}

// TextResult builds a single text content result.
func TextResult(text string) CallToolResult {
	return CallToolResult{Content: []Content{{Type: "text", Text: text}}}
}
