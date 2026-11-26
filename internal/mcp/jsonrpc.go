// Package mcp implements the JSON-RPC 2.0 framing used by the Model Context
// Protocol over stdio. MCP stdio messages are newline-delimited JSON.
package mcp

import (
	"encoding/json"
	"fmt"
)

// JSON-RPC error codes.
const (
	CodeParseError     = -32700
	CodeInvalidRequest = -32600
	CodeMethodNotFound = -32601
	CodeInvalidParams  = -32602
	CodeInternalError  = -32603
)

// Message is a JSON-RPC request, response or notification.
type Message struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *Error          `json:"error,omitempty"`
}

// IsRequest reports whether the message is a request (method plus id).
func (m *Message) IsRequest() bool { return m.Method != "" && len(m.ID) > 0 }

// IsNotification reports whether the message is a notification (method, no id).
func (m *Message) IsNotification() bool { return m.Method != "" && len(m.ID) == 0 }

// IsResponse reports whether the message is a response (id, no method).
func (m *Message) IsResponse() bool { return m.Method == "" && len(m.ID) > 0 }

// Error is a JSON-RPC error object.
type Error struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

func (e *Error) Error() string { return fmt.Sprintf("jsonrpc error %d: %s", e.Code, e.Message) }

// NewRequest builds a request with a numeric id.
func NewRequest(id int64, method string, params any) (*Message, error) {
	p, err := marshalParams(params)
	if err != nil {
		return nil, err
	}
	rawID, err := json.Marshal(id)
	if err != nil {
		return nil, err
	}
	return &Message{JSONRPC: "2.0", ID: rawID, Method: method, Params: p}, nil
}

// NewNotification builds a notification.
func NewNotification(method string, params any) (*Message, error) {
	p, err := marshalParams(params)
	if err != nil {
		return nil, err
	}
	return &Message{JSONRPC: "2.0", Method: method, Params: p}, nil
}

// NewResult builds a successful response.
func NewResult(id json.RawMessage, result any) (*Message, error) {
	raw, err := json.Marshal(result)
	if err != nil {
		return nil, err
	}
	return &Message{JSONRPC: "2.0", ID: id, Result: raw}, nil
}

// NewError builds an error response.
func NewError(id json.RawMessage, code int, message string) *Message {
	if len(id) == 0 {
		id = json.RawMessage("null")
	}
	return &Message{JSONRPC: "2.0", ID: id, Error: &Error{Code: code, Message: message}}
}

func marshalParams(params any) (json.RawMessage, error) {
	if params == nil {
		return nil, nil
	}
	if raw, ok := params.(json.RawMessage); ok {
		return raw, nil
	}
	raw, err := json.Marshal(params)
	if err != nil {
		return nil, err
	}
	return raw, nil
}
