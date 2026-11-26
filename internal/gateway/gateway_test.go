package gateway

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/rezacr588/mcpslim/internal/config"
	"github.com/rezacr588/mcpslim/internal/lockfile"
	"github.com/rezacr588/mcpslim/internal/mcp"
)

var fixtureBin string

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "mcpslim-fixture")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fixtureBin = filepath.Join(dir, "fixture")
	cmd := exec.Command("go", "build", "-o", fixtureBin, "./internal/testfixture")
	cmd.Dir = "../.."
	if out, err := cmd.CombinedOutput(); err != nil {
		fmt.Fprintf(os.Stderr, "build fixture: %v\n%s", err, out)
		os.Exit(1)
	}
	code := m.Run()
	_ = os.RemoveAll(dir)
	os.Exit(code)
}

type testClient struct {
	t       *testing.T
	w       io.Writer
	wmu     sync.Mutex
	mu      sync.Mutex
	next    int64
	pending map[string]chan *mcp.Message
	notes   chan string
}

func newTestClient(t *testing.T, w io.Writer, r io.Reader) *testClient {
	c := &testClient{t: t, w: w, pending: map[string]chan *mcp.Message{}, notes: make(chan string, 16)}
	go func() {
		br := bufio.NewReaderSize(r, 64<<10)
		for {
			msg, err := mcp.ReadMessage(br)
			if err != nil {
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
				select {
				case c.notes <- msg.Method:
				default:
				}
			}
		}
	}()
	return c
}

func (c *testClient) request(method string, params any) (*mcp.Message, error) {
	c.mu.Lock()
	c.next++
	id := c.next
	key := fmt.Sprintf("%d", id)
	ch := make(chan *mcp.Message, 1)
	c.pending[key] = ch
	c.mu.Unlock()

	msg, err := mcp.NewRequest(id, method, params)
	if err != nil {
		return nil, err
	}
	raw, err := json.Marshal(msg)
	if err != nil {
		return nil, err
	}
	c.wmu.Lock()
	_, err = c.w.Write(append(raw, '\n'))
	c.wmu.Unlock()
	if err != nil {
		return nil, err
	}
	select {
	case resp := <-ch:
		return resp, nil
	case <-time.After(5 * time.Second):
		return nil, fmt.Errorf("timeout waiting for %s", method)
	}
}

func (c *testClient) waitNote(method string) bool {
	select {
	case got := <-c.notes:
		return got == method
	case <-time.After(2 * time.Second):
		return false
	}
}

func startGateway(t *testing.T, cfg *config.Config) (*testClient, func()) {
	t.Helper()
	inR, inW := io.Pipe()
	outR, outW := io.Pipe()
	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		errCh <- Serve(ctx, Options{Config: cfg, Version: "test", In: inR, Out: outW, Log: io.Discard})
		_ = outW.Close()
	}()
	tc := newTestClient(t, inW, outR)
	stop := func() {
		cancel()
		_ = inW.Close()
		select {
		case err := <-errCh:
			_ = err
		case <-time.After(5 * time.Second):
			t.Error("gateway did not stop")
		}
	}
	return tc, stop
}

func initialize(t *testing.T, tc *testClient) mcp.InitializeResult {
	t.Helper()
	resp, err := tc.request("initialize", mcp.InitializeParams{
		ProtocolVersion: "2025-06-18",
		Capabilities:    map[string]any{},
		ClientInfo:      mcp.ServerInfo{Name: "test", Version: "1"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Error != nil {
		t.Fatalf("initialize error: %v", resp.Error)
	}
	var out mcp.InitializeResult
	if err := json.Unmarshal(resp.Result, &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func listTools(t *testing.T, tc *testClient) []mcp.Tool {
	t.Helper()
	resp, err := tc.request("tools/list", map[string]any{})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Error != nil {
		t.Fatalf("tools/list error: %v", resp.Error)
	}
	var out mcp.ListToolsResult
	if err := json.Unmarshal(resp.Result, &out); err != nil {
		t.Fatal(err)
	}
	return out.Tools
}

func search(t *testing.T, tc *testClient, query string) string {
	t.Helper()
	resp, err := tc.request("tools/call", map[string]any{
		"name":      "search_tools",
		"arguments": map[string]any{"query": query},
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Error != nil {
		t.Fatalf("search error: %v", resp.Error)
	}
	var out mcp.CallToolResult
	if err := json.Unmarshal(resp.Result, &out); err != nil {
		t.Fatal(err)
	}
	if len(out.Content) == 0 {
		t.Fatal("empty search result")
	}
	return out.Content[0].Text
}

func call(t *testing.T, tc *testClient, name string, args map[string]any) (*mcp.Message, *mcp.CallToolResult) {
	t.Helper()
	resp, err := tc.request("tools/call", map[string]any{"name": name, "arguments": args})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Error != nil {
		return resp, nil
	}
	var out mcp.CallToolResult
	if err := json.Unmarshal(resp.Result, &out); err != nil {
		t.Fatal(err)
	}
	return resp, &out
}

func TestGatewayHydratesAndRoutes(t *testing.T) {
	cfg := config.New()
	cfg.Servers = map[string]config.Server{"fixture": {Command: fixtureBin}}
	cfg.Sandbox.Disabled = true
	cfg.Lockfile = filepath.Join(t.TempDir(), "mcpslim.lock")

	tc, stop := startGateway(t, cfg)
	defer stop()

	init := initialize(t, tc)
	if init.ServerInfo.Name != "mcpslim" {
		t.Fatalf("serverInfo = %+v", init.ServerInfo)
	}
	if init.Capabilities.Tools == nil || !init.Capabilities.Tools.ListChanged {
		t.Fatal("list_changed not advertised")
	}

	tools := listTools(t, tc)
	if len(tools) != 1 || tools[0].Name != "search_tools" {
		t.Fatalf("initial tools = %+v", tools)
	}

	text := search(t, tc, "read a file")
	if !strings.Contains(text, "read_file") {
		t.Fatalf("search text = %q", text)
	}
	if !tc.waitNote("notifications/tools/list_changed") {
		t.Fatal("missing list_changed notification")
	}

	tools = listTools(t, tc)
	if !hasTool(tools, "read_file") {
		t.Fatalf("read_file not hydrated: %+v", tools)
	}

	resp, out := call(t, tc, "read_file", map[string]any{"path": "/tmp/demo"})
	if resp.Error != nil {
		t.Fatalf("read_file error: %v", resp.Error)
	}
	if len(out.Content) == 0 || !strings.Contains(out.Content[0].Text, "/tmp/demo") {
		t.Fatalf("read_file result = %+v", out)
	}

	resp, _ = call(t, tc, "does_not_exist", map[string]any{})
	if resp.Error == nil {
		t.Fatal("unknown tool did not error")
	}
}

func TestSchemaDriftBlocksUntilApproved(t *testing.T) {
	lock := filepath.Join(t.TempDir(), "mcpslim.lock")
	run := func(variant string, body func(tc *testClient)) {
		cfg := config.New()
		cfg.Servers = map[string]config.Server{"fixture": {
			Command: fixtureBin,
			Env:     map[string]string{"FIXTURE_VARIANT": variant},
		}}
		cfg.Sandbox.Disabled = true
		cfg.Lockfile = lock
		tc, stop := startGateway(t, cfg)
		defer stop()
		initialize(t, tc)
		body(tc)
	}

	run("1", func(tc *testClient) {
		if text := search(t, tc, "read a file"); !strings.Contains(text, "read_file") {
			t.Fatalf("v1 search = %q", text)
		}
		if resp, _ := call(t, tc, "read_file", map[string]any{"path": "/tmp/x"}); resp.Error != nil {
			t.Fatalf("v1 call error: %v", resp.Error)
		}
	})

	run("2", func(tc *testClient) {
		if text := search(t, tc, "read a file"); strings.Contains(text, "read_file") {
			t.Fatalf("drifted tool still indexed: %q", text)
		}
		resp, _ := call(t, tc, "read_file", map[string]any{"path": "/tmp/x"})
		if resp.Error == nil || !strings.Contains(resp.Error.Message, "blocked") {
			t.Fatalf("drifted call = %+v, want blocked", resp)
		}
	})

	l, err := lockfile.Load(lock)
	if err != nil {
		t.Fatal(err)
	}
	if n := l.Approve("fixture", "read_file"); n != 1 {
		t.Fatalf("approved %d, want 1", n)
	}
	if err := l.Save(); err != nil {
		t.Fatal(err)
	}

	run("2", func(tc *testClient) {
		if text := search(t, tc, "read a file"); !strings.Contains(text, "read_file") {
			t.Fatalf("approved search = %q", text)
		}
		if resp, _ := call(t, tc, "read_file", map[string]any{"path": "/tmp/x"}); resp.Error != nil {
			t.Fatalf("approved call error: %v", resp.Error)
		}
	})
}

func TestSchemaTokenReduction(t *testing.T) {
	// Baseline: every upstream schema, as a client would load it directly.
	cmd := exec.Command(fixtureBin)
	cmd.Env = append(os.Environ(), "FIXTURE_TOOLS=25")
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	conn := mcp.NewConn(stdout, stdin)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if _, err := conn.Initialize(ctx, "test", "1"); err != nil {
		t.Fatal(err)
	}
	upstreamTools, err := conn.ListTools(ctx)
	if err != nil {
		t.Fatal(err)
	}
	conn.Close()
	_ = stdin.Close()
	_ = cmd.Wait()
	baseline, _ := json.Marshal(map[string]any{"tools": upstreamTools})

	cfg := config.New()
	cfg.Servers = map[string]config.Server{"fixture": {
		Command: fixtureBin,
		Env:     map[string]string{"FIXTURE_TOOLS": "25"},
	}}
	cfg.Sandbox.Disabled = true
	cfg.Lockfile = filepath.Join(t.TempDir(), "mcpslim.lock")
	tc, stop := startGateway(t, cfg)
	defer stop()
	initialize(t, tc)
	exposed, _ := json.Marshal(map[string]any{"tools": listTools(t, tc)})

	reduction := 100 * (1 - float64(len(exposed))/float64(len(baseline)))
	t.Logf("upstream=%d bytes exposed=%d bytes reduction=%.1f%% (%d tools)", len(baseline), len(exposed), reduction, len(upstreamTools))
	if reduction < 60 {
		t.Fatalf("reduction %.1f%% below the 60%% floor", reduction)
	}
}

func TestSandboxDeniesEnvEndToEnd(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("Seatbelt is macOS only")
	}
	if _, err := os.Stat("/usr/bin/sandbox-exec"); err != nil {
		t.Skip("sandbox-exec not available")
	}
	work := t.TempDir()
	if err := os.WriteFile(filepath.Join(work, ".env"), []byte("SECRET=1"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(work, "ok.txt"), []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := config.New()
	cfg.Servers = map[string]config.Server{"fixture": {Command: fixtureBin, Dir: work}}
	cfg.Lockfile = filepath.Join(t.TempDir(), "mcpslim.lock")
	tc, stop := startGateway(t, cfg)
	defer stop()
	initialize(t, tc)
	search(t, tc, "read any file path")

	if resp, out := call(t, tc, "read_path", map[string]any{"path": "ok.txt"}); resp.Error != nil {
		t.Fatalf("allowed read failed: %v", resp.Error)
	} else if len(out.Content) == 0 || !strings.Contains(out.Content[0].Text, "read 5 bytes") {
		t.Fatalf("allowed read = %+v", out)
	}

	resp, out := call(t, tc, "read_path", map[string]any{"path": ".env"})
	if resp.Error != nil {
		t.Fatalf("call error: %v", resp.Error)
	}
	if len(out.Content) == 0 || !strings.Contains(out.Content[0].Text, "operation not permitted") {
		t.Fatalf("sandbox did not deny .env: %+v", out)
	}
}

func hasTool(tools []mcp.Tool, name string) bool {
	for _, t := range tools {
		if t.Name == name {
			return true
		}
	}
	return false
}
