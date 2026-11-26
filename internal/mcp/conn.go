package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sync"
)

// maxLineBytes caps a single JSON-RPC line. Tool schemas can be large.
const maxLineBytes = 16 << 20

// ErrClosed is returned when the connection is gone.
var ErrClosed = errors.New("mcp: connection closed")

// Writer serialises concurrent writes to one stream.
type Writer struct {
	mu sync.Mutex
	w  io.Writer
}

// NewWriter wraps w.
func NewWriter(w io.Writer) *Writer { return &Writer{w: w} }

// Write marshals v as one newline-delimited JSON message.
func (w *Writer) Write(v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	b = append(b, '\n')
	w.mu.Lock()
	defer w.mu.Unlock()
	_, err = w.w.Write(b)
	return err
}

// Conn is a JSON-RPC client connection to one upstream MCP server.
type Conn struct {
	in  *bufio.Reader
	out *Writer

	mu      sync.Mutex
	pending map[string]chan *Message
	nextID  int64
	closed  chan struct{}
	once    sync.Once
	err     error

	notify  func(*Message)
	request func(*Message) *Message
}

// NewConn starts reading messages from in and writing to out.
func NewConn(in io.Reader, out io.Writer) *Conn {
	c := &Conn{
		in:      bufio.NewReaderSize(in, 64<<10),
		out:     NewWriter(out),
		pending: make(map[string]chan *Message),
		closed:  make(chan struct{}),
	}
	go c.readLoop()
	return c
}

// SetNotificationHandler sets the callback for server notifications.
func (c *Conn) SetNotificationHandler(fn func(*Message)) { c.notify = fn }

// SetRequestHandler sets the callback for server-initiated requests.
func (c *Conn) SetRequestHandler(fn func(*Message) *Message) { c.request = fn }

// Call sends a request and waits for its response.
func (c *Conn) Call(ctx context.Context, method string, params any) (*Message, error) {
	id, msg, ch, err := c.prepare(method, params)
	if err != nil {
		return nil, err
	}
	defer c.forget(id)
	if err := c.out.Write(msg); err != nil {
		return nil, err
	}
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-c.closed:
		return nil, c.closeErr()
	case resp := <-ch:
		return resp, nil
	}
}

// Notify sends a notification.
func (c *Conn) Notify(method string, params any) error {
	msg, err := NewNotification(method, params)
	if err != nil {
		return err
	}
	return c.out.Write(msg)
}

// Close shuts the connection down.
func (c *Conn) Close() {
	c.once.Do(func() {
		close(c.closed)
	})
}

func (c *Conn) prepare(method string, params any) (string, *Message, chan *Message, error) {
	c.mu.Lock()
	c.nextID++
	id := c.nextID
	ch := make(chan *Message, 1)
	key := fmt.Sprintf("%d", id)
	c.pending[key] = ch
	c.mu.Unlock()

	msg, err := NewRequest(id, method, params)
	if err != nil {
		c.mu.Lock()
		delete(c.pending, key)
		c.mu.Unlock()
		return "", nil, nil, err
	}
	return key, msg, ch, nil
}

func (c *Conn) forget(key string) {
	c.mu.Lock()
	delete(c.pending, key)
	c.mu.Unlock()
}

func (c *Conn) closeErr() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.err != nil {
		return c.err
	}
	return ErrClosed
}

func (c *Conn) readLoop() {
	defer c.Close()
	for {
		msg, err := readMessage(c.in)
		if err != nil {
			c.mu.Lock()
			if c.err == nil {
				c.err = err
			}
			c.mu.Unlock()
			return
		}
		switch {
		case msg.IsResponse():
			c.mu.Lock()
			ch := c.pending[string(msg.ID)]
			c.mu.Unlock()
			if ch != nil {
				select {
				case ch <- msg:
				default:
				}
			}
		case msg.IsNotification():
			if c.notify != nil {
				c.notify(msg)
			}
		case msg.IsRequest():
			resp := NewError(msg.ID, CodeMethodNotFound, "method not supported by mcpslim client")
			if c.request != nil {
				resp = c.request(msg)
			}
			_ = c.out.Write(resp)
		}
	}
}

// ReadMessage reads one newline-delimited JSON-RPC message.
func ReadMessage(r *bufio.Reader) (*Message, error) { return readMessage(r) }

func readMessage(r *bufio.Reader) (*Message, error) {
	for {
		line, err := r.ReadBytes('\n')
		if len(line) > 0 {
			if len(line) > maxLineBytes {
				return nil, fmt.Errorf("mcp: message exceeds %d bytes", maxLineBytes)
			}
			line = trimSpace(line)
			if len(line) == 0 {
				if err != nil {
					return nil, err
				}
				continue
			}
			var msg Message
			if err := json.Unmarshal(line, &msg); err != nil {
				return nil, fmt.Errorf("mcp: decode: %w", err)
			}
			if msg.JSONRPC != "2.0" {
				return nil, fmt.Errorf("mcp: unsupported jsonrpc version %q", msg.JSONRPC)
			}
			return &msg, nil
		}
		if err != nil {
			return nil, err
		}
	}
}

func trimSpace(b []byte) []byte {
	for len(b) > 0 && (b[0] == ' ' || b[0] == '\t' || b[0] == '\r' || b[0] == '\n') {
		b = b[1:]
	}
	for len(b) > 0 {
		last := b[len(b)-1]
		if last != ' ' && last != '\t' && last != '\r' && last != '\n' {
			break
		}
		b = b[:len(b)-1]
	}
	return b
}
