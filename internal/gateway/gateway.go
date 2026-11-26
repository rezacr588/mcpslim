package gateway

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/rezacr588/mcpslim/internal/config"
	"github.com/rezacr588/mcpslim/internal/lockfile"
	"github.com/rezacr588/mcpslim/internal/mcp"
	"github.com/rezacr588/mcpslim/internal/router"
	"github.com/rezacr588/mcpslim/internal/sandbox"
)

const (
	searchToolName = "search_tools"
	callTimeout    = 120 * time.Second
	startTimeout   = 30 * time.Second
)

// Options configures a gateway session.
type Options struct {
	Config  *config.Config
	Version string
	In      io.Reader
	Out     io.Writer
	Log     io.Writer
}

// Gateway multiplexes MCP servers behind one client connection.
type Gateway struct {
	cfg     *config.Config
	version string
	out     *mcp.Writer
	log     io.Writer
	lock    *lockfile.Lock

	mu        sync.Mutex
	lockMu    sync.Mutex
	servers   []*upstream
	byExposed map[string]*upstream
	index     *router.Index
	hydrated  map[string]bool
}

type upstream struct {
	name  string
	conn  *mcp.Conn
	cmd   *exec.Cmd
	stdin io.Closer

	mu      sync.Mutex
	tools   []mcp.Tool
	exposed map[string]string // exposed name -> upstream name
	blocked map[string]string // upstream name -> reason
}

// Serve runs the gateway until the client disconnects or ctx is cancelled.
func Serve(ctx context.Context, opts Options) error {
	if opts.Log == nil {
		opts.Log = os.Stderr
	}
	g := &Gateway{
		cfg:       opts.Config,
		version:   opts.Version,
		out:       mcp.NewWriter(opts.Out),
		log:       opts.Log,
		byExposed: map[string]*upstream{},
		hydrated:  map[string]bool{},
	}
	lock, err := lockfile.Load(opts.Config.LockPath())
	if err != nil {
		return err
	}
	g.lock = lock

	if err := g.start(ctx); err != nil {
		g.close()
		return err
	}
	defer g.close()

	if err := g.saveLock(); err != nil {
		_, _ = fmt.Fprintf(g.log, "mcpslim: warning: could not write %s: %v\n", opts.Config.LockPath(), err)
	}
	return g.loop(ctx, opts.In)
}

func (g *Gateway) start(ctx context.Context) error {
	if len(g.cfg.Servers) == 0 {
		return errors.New("no upstream servers configured")
	}
	for _, name := range g.cfg.ServerNames() {
		if err := g.startServer(ctx, name, g.cfg.Servers[name]); err != nil {
			return fmt.Errorf("server %s: %w", name, err)
		}
	}
	g.rebuildIndex()
	return nil
}

func (g *Gateway) startServer(ctx context.Context, name string, sc config.Server) error {
	argv := append([]string{sc.Command}, sc.Args...)
	argv, note := sandbox.Wrap(argv, sandbox.Policy{
		Enabled:     !g.cfg.Sandbox.Disabled,
		Workspace:   workspace(sc.Dir),
		DenyNetwork: g.cfg.Sandbox.DenyNetwork,
	})
	if note != "" {
		_, _ = fmt.Fprintf(g.log, "mcpslim: %s: %s\n", name, note)
	}

	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
	cmd.Dir = sc.Dir
	cmd.Env = os.Environ()
	for k, v := range sc.Env {
		cmd.Env = append(cmd.Env, k+"="+v)
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	cmd.Stderr = &prefixWriter{w: g.log, prefix: "mcpslim[" + name + "]: "}
	if err := cmd.Start(); err != nil {
		return err
	}

	conn := mcp.NewConn(stdout, stdin)
	up := &upstream{name: name, conn: conn, cmd: cmd, stdin: stdin, exposed: map[string]string{}, blocked: map[string]string{}}
	conn.SetNotificationHandler(func(msg *mcp.Message) {
		if msg.Method == "notifications/tools/list_changed" {
			if err := g.refresh(ctx, up); err != nil {
				_, _ = fmt.Fprintf(g.log, "mcpslim: %s: refresh failed: %v\n", name, err)
			}
		}
	})

	initCtx, cancel := context.WithTimeout(ctx, startTimeout)
	defer cancel()
	if _, err := conn.Initialize(initCtx, "mcpslim", g.version); err != nil {
		_ = cmd.Process.Kill()
		return fmt.Errorf("initialize: %w", err)
	}
	tools, err := conn.ListTools(initCtx)
	if err != nil {
		_ = cmd.Process.Kill()
		return fmt.Errorf("tools/list: %w", err)
	}
	drift, changed, err := g.checkLock(name, tools)
	if err != nil {
		_ = cmd.Process.Kill()
		return err
	}
	if changed {
		if err := g.saveLock(); err != nil {
			_, _ = fmt.Fprintf(g.log, "mcpslim: warning: could not write lockfile: %v\n", err)
		}
	}
	up.setTools(tools)
	for _, d := range drift {
		up.block(d.Tool, "definition changed; run mcpslim approve "+d.Server+" "+d.Tool)
		_, _ = fmt.Fprintf(g.log, "mcpslim: %s/%s schema changed and is blocked until approved\n", d.Server, d.Tool)
	}
	g.servers = append(g.servers, up)
	return nil
}

func (g *Gateway) loop(ctx context.Context, in io.Reader) error {
	r := bufio.NewReaderSize(in, 64<<10)
	for {
		msg, err := mcp.ReadMessage(r)
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return err
		}
		if !msg.IsRequest() {
			continue
		}
		go g.handle(ctx, msg)
	}
}

func (g *Gateway) handle(ctx context.Context, req *mcp.Message) {
	var resp *mcp.Message
	switch req.Method {
	case "initialize":
		resp = g.initialize(req)
	case "ping":
		resp, _ = mcp.NewResult(req.ID, map[string]any{})
	case "tools/list":
		resp, _ = mcp.NewResult(req.ID, map[string]any{"tools": g.visibleTools()})
	case "tools/call":
		resp = g.callTool(ctx, req)
	case "resources/list":
		resp, _ = mcp.NewResult(req.ID, map[string]any{"resources": []any{}})
	case "prompts/list":
		resp, _ = mcp.NewResult(req.ID, map[string]any{"prompts": []any{}})
	case "shutdown":
		resp, _ = mcp.NewResult(req.ID, map[string]any{})
	default:
		resp = mcp.NewError(req.ID, mcp.CodeMethodNotFound, "method not found: "+req.Method)
	}
	if resp != nil {
		_ = g.out.Write(resp)
	}
}

func (g *Gateway) initialize(req *mcp.Message) *mcp.Message {
	var p mcp.InitializeParams
	_ = json.Unmarshal(req.Params, &p)
	version := p.ProtocolVersion
	if version == "" {
		version = mcp.ProtocolVersion
	}
	res := mcp.InitializeResult{
		ProtocolVersion: version,
		Capabilities:    mcp.Capabilities{Tools: &mcp.ListChanged{ListChanged: true}},
		ServerInfo:      mcp.ServerInfo{Name: "mcpslim", Version: g.version},
		Instructions:    "Tool schemas load on demand. Call search_tools with a capability description; matching tools appear in tools/list and become callable.",
	}
	resp, err := mcp.NewResult(req.ID, res)
	if err != nil {
		return mcp.NewError(req.ID, mcp.CodeInternalError, err.Error())
	}
	return resp
}

func (g *Gateway) callTool(ctx context.Context, req *mcp.Message) *mcp.Message {
	var p mcp.CallToolParams
	if err := json.Unmarshal(req.Params, &p); err != nil {
		return mcp.NewError(req.ID, mcp.CodeInvalidParams, "invalid tools/call params")
	}
	if p.Name == searchToolName {
		return g.search(req)
	}

	g.mu.Lock()
	up := g.byExposed[p.Name]
	g.mu.Unlock()
	if up == nil {
		return mcp.NewError(req.ID, mcp.CodeInvalidParams, "unknown tool: "+p.Name)
	}
	original := up.exposedName(p.Name)
	if original == "" {
		return mcp.NewError(req.ID, mcp.CodeInvalidParams, "unknown tool: "+p.Name)
	}
	if reason := up.blockedReason(original); reason != "" {
		return mcp.NewError(req.ID, mcp.CodeInvalidParams, fmt.Sprintf("tool %s is blocked: %s", p.Name, reason))
	}

	callCtx, cancel := context.WithTimeout(ctx, callTimeout)
	defer cancel()
	resp, err := up.conn.Call(callCtx, "tools/call", mcp.CallToolParams{Name: original, Arguments: p.Arguments})
	if err != nil {
		return mcp.NewError(req.ID, mcp.CodeInternalError, err.Error())
	}
	if resp.Error != nil {
		return mcp.NewError(req.ID, resp.Error.Code, resp.Error.Message)
	}
	out, err := mcp.NewResult(req.ID, json.RawMessage(resp.Result))
	if err != nil {
		return mcp.NewError(req.ID, mcp.CodeInternalError, err.Error())
	}
	return out
}

func (g *Gateway) search(req *mcp.Message) *mcp.Message {
	var args struct {
		Query string `json:"query"`
		Limit int    `json:"limit"`
	}
	_ = json.Unmarshal(req.Params, &args)
	// The client may nest arguments under "arguments" (MCP shape).
	var wrapper struct {
		Arguments json.RawMessage `json:"arguments"`
	}
	if len(args.Query) == 0 {
		if err := json.Unmarshal(req.Params, &wrapper); err == nil && len(wrapper.Arguments) > 0 {
			_ = json.Unmarshal(wrapper.Arguments, &args)
		}
	}
	if strings.TrimSpace(args.Query) == "" {
		return mcp.NewError(req.ID, mcp.CodeInvalidParams, "search_tools requires a non-empty query")
	}
	limit := args.Limit
	if limit <= 0 || limit > 20 {
		limit = g.cfg.Router.TopK
	}

	g.mu.Lock()
	hits := g.index.Search(args.Query, limit)
	type hitInfo struct {
		exposed string
		server  string
		desc    string
	}
	infos := make([]hitInfo, 0, len(hits))
	added := false
	for _, h := range hits {
		up := g.byExposed[h.ID]
		if up == nil {
			continue
		}
		original := up.exposedName(h.ID)
		if original == "" {
			continue
		}
		if g.cfg.Router.HydrateEnabled() && !g.hydrated[h.ID] {
			g.hydrated[h.ID] = true
			added = true
		}
		infos = append(infos, hitInfo{exposed: h.ID, server: up.name, desc: up.description(original)})
	}
	g.mu.Unlock()

	if added {
		if msg, err := mcp.NewNotification("notifications/tools/list_changed", nil); err == nil {
			_ = g.out.Write(msg)
		}
	}

	var b strings.Builder
	if len(infos) == 0 {
		_, _ = fmt.Fprintf(&b, "No tools matched %q. Try a broader capability description.", args.Query)
	} else {
		_, _ = fmt.Fprintf(&b, "Found %d tool(s) for %q:\n", len(infos), args.Query)
		for _, info := range infos {
			_, _ = fmt.Fprintf(&b, "- %s (%s): %s\n", info.exposed, info.server, truncate(info.desc, 180))
		}
		b.WriteString("These tools are now available in tools/list and can be called directly.")
	}
	resp, err := mcp.NewResult(req.ID, mcp.TextResult(b.String()))
	if err != nil {
		return mcp.NewError(req.ID, mcp.CodeInternalError, err.Error())
	}
	return resp
}

func (g *Gateway) visibleTools() []mcp.Tool {
	g.mu.Lock()
	defer g.mu.Unlock()
	tools := []mcp.Tool{searchTool(g.cfg.Router.TopK)}
	for _, up := range g.servers {
		for _, tool := range up.snapshot() {
			exposed := up.exposedFor(tool.Name)
			if exposed == "" || !g.hydrated[exposed] {
				continue
			}
			if up.blockedReason(tool.Name) != "" {
				continue
			}
			tools = append(tools, mcp.Tool{Name: exposed, Description: tool.Description, InputSchema: tool.InputSchema})
		}
	}
	sort.Slice(tools[1:], func(i, j int) bool { return tools[1+i].Name < tools[1+j].Name })
	return tools
}

func (g *Gateway) rebuildIndex() {
	g.mu.Lock()
	defer g.mu.Unlock()
	ix := router.New()
	g.byExposed = map[string]*upstream{}
	for _, up := range g.servers {
		for _, tool := range up.snapshot() {
			exposed := up.exposedFor(tool.Name)
			if exposed == "" {
				continue
			}
			g.byExposed[exposed] = up
			if up.blockedReason(tool.Name) != "" {
				continue
			}
			ix.Add(exposed, tool.Name+" "+tool.Description+" "+schemaProperties(tool.InputSchema))
		}
	}
	g.index = ix
}

func (g *Gateway) refresh(ctx context.Context, up *upstream) error {
	refreshCtx, cancel := context.WithTimeout(ctx, startTimeout)
	defer cancel()
	tools, err := up.conn.ListTools(refreshCtx)
	if err != nil {
		return err
	}
	drift, changed, err := g.checkLock(up.name, tools)
	if err != nil {
		return err
	}
	if changed {
		if err := g.saveLock(); err != nil {
			_, _ = fmt.Fprintf(g.log, "mcpslim: warning: could not write lockfile: %v\n", err)
		}
	}
	up.setTools(tools)
	for _, d := range drift {
		up.block(d.Tool, "definition changed; run mcpslim approve "+d.Server+" "+d.Tool)
		_, _ = fmt.Fprintf(g.log, "mcpslim: %s/%s schema changed and is blocked until approved\n", d.Server, d.Tool)
	}
	g.rebuildIndex()
	if msg, err := mcp.NewNotification("notifications/tools/list_changed", nil); err == nil {
		_ = g.out.Write(msg)
	}
	return nil
}

func (g *Gateway) checkLock(server string, tools []mcp.Tool) ([]lockfile.Drift, bool, error) {
	g.lockMu.Lock()
	defer g.lockMu.Unlock()
	return g.lock.Check(server, tools)
}

func (g *Gateway) saveLock() error {
	g.lockMu.Lock()
	defer g.lockMu.Unlock()
	return g.lock.Save()
}

func (g *Gateway) close() {
	for _, up := range g.servers {
		up.conn.Close()
		if up.stdin != nil {
			_ = up.stdin.Close()
		}
		if up.cmd != nil && up.cmd.Process != nil {
			_ = up.cmd.Process.Kill()
			_, _ = up.cmd.Process.Wait()
		}
	}
}

func searchTool(topK int) mcp.Tool {
	schema := fmt.Sprintf(`{"type":"object","properties":{"query":{"type":"string","description":"Capability to search for, for example 'read a file' or 'open a pull request'"},"limit":{"type":"integer","minimum":1,"maximum":20,"description":"Maximum results (default %d)"}},"required":["query"]}`, topK)
	return mcp.Tool{
		Name:        searchToolName,
		Description: "Search the MCP tools available in this session by capability. Matched tools become callable immediately.",
		InputSchema: json.RawMessage(schema),
	}
}

func schemaProperties(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var doc struct {
		Properties map[string]any `json:"properties"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		return ""
	}
	names := make([]string, 0, len(doc.Properties))
	for name := range doc.Properties {
		names = append(names, name)
	}
	sort.Strings(names)
	return strings.Join(names, " ")
}

func workspace(dir string) string {
	if dir != "" {
		return dir
	}
	wd, _ := os.Getwd()
	return wd
}

func truncate(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

type prefixWriter struct {
	w      io.Writer
	prefix string
	mu     sync.Mutex
}

func (p *prefixWriter) Write(b []byte) (int, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	text := strings.TrimRight(string(b), "\n")
	lines := strings.Split(text, "\n")
	for _, line := range lines {
		if _, err := fmt.Fprintln(p.w, p.prefix+line); err != nil {
			return 0, err
		}
	}
	return len(b), nil
}

func (u *upstream) setTools(tools []mcp.Tool) {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.tools = tools
	counts := map[string]int{}
	for _, t := range tools {
		counts[t.Name]++
	}
	u.exposed = map[string]string{}
	u.blocked = map[string]string{}
	for _, t := range tools {
		if counts[t.Name] > 1 {
			u.exposed[u.name+"__"+t.Name] = t.Name
		} else {
			u.exposed[t.Name] = t.Name
		}
	}
}

func (u *upstream) snapshot() []mcp.Tool {
	u.mu.Lock()
	defer u.mu.Unlock()
	out := make([]mcp.Tool, len(u.tools))
	copy(out, u.tools)
	return out
}

func (u *upstream) exposedName(exposed string) string {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.exposed[exposed]
}

func (u *upstream) exposedFor(original string) string {
	u.mu.Lock()
	defer u.mu.Unlock()
	for exposed, name := range u.exposed {
		if name == original {
			return exposed
		}
	}
	return ""
}

func (u *upstream) block(tool, reason string) {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.blocked[tool] = reason
}

func (u *upstream) blockedReason(tool string) string {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.blocked[tool]
}

func (u *upstream) description(original string) string {
	u.mu.Lock()
	defer u.mu.Unlock()
	for _, t := range u.tools {
		if t.Name == original {
			return t.Description
		}
	}
	return ""
}
