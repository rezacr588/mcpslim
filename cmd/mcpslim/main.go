// Command mcpslim is a single-binary MCP gateway that reduces tool-schema
// context bloat and sandboxes local stdio MCP servers.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"

	"github.com/rezacr588/mcpslim/internal/config"
	"github.com/rezacr588/mcpslim/internal/gateway"
	"github.com/rezacr588/mcpslim/internal/lockfile"
)

// Set by GoReleaser via -ldflags.
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

const usage = `mcpslim - MCP gateway with dynamic tool loading and OS sandboxing

Usage:
  mcpslim <command> [arguments]

Commands:
  wrap      Run one MCP server behind the gateway: mcpslim wrap -- npx -y pkg
  serve     Serve every configured upstream on stdio
  init      Detect MCP client configs and route them through mcpslim
  approve   Accept a tool-schema change recorded in mcpslim.lock
  version   Print version, commit and build date
  help      Print this help

Run "mcpslim <command> -h" for command flags.
`

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr *os.File) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, usage)
		return 2
	}
	switch args[0] {
	case "version", "--version", "-v":
		_, _ = fmt.Fprintf(stdout, "mcpslim %s (commit %s, built %s, %s/%s, %s)\n",
			version, commit, date, runtime.GOOS, runtime.GOARCH, runtime.Version())
		return 0
	case "help", "--help", "-h":
		_, _ = fmt.Fprint(stdout, usage)
		return 0
	case "serve":
		return cmdServe(args[1:], stderr)
	case "wrap":
		return cmdWrap(args[1:], stderr)
	case "init":
		return cmdInit(args[1:], stdout, stderr)
	case "approve":
		return cmdApprove(args[1:], stdout, stderr)
	default:
		_, _ = fmt.Fprintf(stderr, "mcpslim: unknown command %q\n\n%s", args[0], usage)
		return 2
	}
}

type serverFlags map[string]config.Server

func (s *serverFlags) String() string { return "" }

func (s *serverFlags) Set(v string) error {
	name, cmd, ok := strings.Cut(v, "=")
	if !ok || name == "" || cmd == "" {
		return errors.New("expected name=command")
	}
	if *s == nil {
		*s = serverFlags{}
	}
	(*s)[name] = config.Server{Command: cmd}
	return nil
}

func cmdServe(args []string, stderr *os.File) int {
	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	fs.SetOutput(stderr)
	cfgPath := fs.String("config", config.DefaultPath(), "config file")
	lockPath := fs.String("lock", "", "lockfile path (default: beside the config)")
	noSandbox := fs.Bool("no-sandbox", false, "do not sandbox upstream servers")
	denyNetwork := fs.Bool("deny-network", false, "block network access for upstream servers")
	topK := fs.Int("top-k", 0, "number of tools hydrated per search")
	var servers serverFlags
	fs.Var(&servers, "server", "upstream as name=command (repeatable)")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	cfg, err := config.Load(*cfgPath)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "mcpslim: %v\n", err)
		return 1
	}
	if len(servers) > 0 {
		cfg.Servers = servers
	}
	if *lockPath != "" {
		cfg.Lockfile = *lockPath
	}
	if *noSandbox {
		cfg.Sandbox.Disabled = true
	}
	if *denyNetwork {
		cfg.Sandbox.DenyNetwork = true
	}
	if *topK > 0 {
		cfg.Router.TopK = *topK
	}
	if len(cfg.Servers) == 0 {
		_, _ = fmt.Fprintf(stderr, "mcpslim: no servers configured; pass --server name=command or edit %s\n", *cfgPath)
		return 1
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := gateway.Serve(ctx, gateway.Options{
		Config:  cfg,
		Version: version,
		In:      os.Stdin,
		Out:     os.Stdout,
		Log:     stderr,
	}); err != nil {
		_, _ = fmt.Fprintf(stderr, "mcpslim: %v\n", err)
		return 1
	}
	return 0
}

func cmdWrap(args []string, stderr *os.File) int {
	fs := flag.NewFlagSet("wrap", flag.ContinueOnError)
	fs.SetOutput(stderr)
	lockPath := fs.String("lock", "mcpslim.lock", "lockfile path")
	noSandbox := fs.Bool("no-sandbox", false, "do not sandbox the upstream server")
	denyNetwork := fs.Bool("deny-network", false, "block network access for the upstream server")
	topK := fs.Int("top-k", 5, "number of tools hydrated per search")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	argv := fs.Args()
	if len(argv) == 0 {
		_, _ = fmt.Fprintln(stderr, "usage: mcpslim wrap [flags] -- <command> [args...]")
		return 2
	}

	cfg := config.New()
	cfg.Servers = map[string]config.Server{
		"default": {Command: argv[0], Args: argv[1:]},
	}
	cfg.Lockfile = *lockPath
	cfg.Sandbox.Disabled = *noSandbox
	cfg.Sandbox.DenyNetwork = *denyNetwork
	if *topK > 0 {
		cfg.Router.TopK = *topK
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := gateway.Serve(ctx, gateway.Options{
		Config:  cfg,
		Version: version,
		In:      os.Stdin,
		Out:     os.Stdout,
		Log:     stderr,
	}); err != nil {
		_, _ = fmt.Fprintf(stderr, "mcpslim: %v\n", err)
		return 1
	}
	return 0
}

func cmdInit(args []string, stdout, stderr *os.File) int {
	fs := flag.NewFlagSet("init", flag.ContinueOnError)
	fs.SetOutput(stderr)
	write := fs.Bool("write", false, "rewrite the discovered client configs")
	home := fs.String("home", "", "home directory to scan (default: current user)")
	cwd := fs.String("cwd", "", "project directory to scan (default: working directory)")
	cfgPath := fs.String("config", config.DefaultPath(), "mcpslim config to write")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *home == "" {
		*home, _ = os.UserHomeDir()
	}
	if *cwd == "" {
		*cwd, _ = os.Getwd()
	}

	cands, err := config.Discover(*home, *cwd)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "mcpslim: %v\n", err)
		return 1
	}
	if len(cands) == 0 {
		_, _ = fmt.Fprintf(stdout, "No MCP client configs found under %s or %s.\n", *home, *cwd)
		return 0
	}
	total := 0
	for _, c := range cands {
		_, _ = fmt.Fprintf(stdout, "%s (%s): %d server(s)", c.Path, c.Kind, len(c.Servers))
		if len(c.Skipped) > 0 {
			_, _ = fmt.Fprintf(stdout, ", skipped non-stdio: %s", strings.Join(c.Skipped, ", "))
		}
		_, _ = fmt.Fprintln(stdout)
		total += len(c.Servers)
	}
	if !*write {
		_, _ = fmt.Fprintln(stdout, "\nDry run. Re-run with --write to back up and rewrite these files.")
		return 0
	}

	cfg, err := config.Load(*cfgPath)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "mcpslim: %v\n", err)
		return 1
	}
	config.Merge(cfg, cands)
	if err := cfg.Save(*cfgPath); err != nil {
		_, _ = fmt.Fprintf(stderr, "mcpslim: %v\n", err)
		return 1
	}
	command, err := os.Executable()
	if err != nil {
		command = "mcpslim"
	}
	command, _ = filepath.Abs(command)
	for _, c := range cands {
		backup, err := config.Apply(c, command, *cfgPath)
		if err != nil {
			_, _ = fmt.Fprintf(stderr, "mcpslim: %v\n", err)
			return 1
		}
		_, _ = fmt.Fprintf(stdout, "Rewrote %s (backup: %s)\n", c.Path, backup)
	}
	_, _ = fmt.Fprintf(stdout, "Wrote %s with %d server(s). Restart your MCP client.\n", *cfgPath, total)
	return 0
}

func cmdApprove(args []string, stdout, stderr *os.File) int {
	fs := flag.NewFlagSet("approve", flag.ContinueOnError)
	fs.SetOutput(stderr)
	lockPath := fs.String("lock", "", "lockfile path (default: beside the config)")
	server := fs.String("server", "", "limit approval to one server")
	tool := fs.String("tool", "", "limit approval to one tool")
	cfgPath := fs.String("config", config.DefaultPath(), "config file used to locate the lockfile")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	path := *lockPath
	if path == "" {
		cfg, err := config.Load(*cfgPath)
		if err != nil {
			_, _ = fmt.Fprintf(stderr, "mcpslim: %v\n", err)
			return 1
		}
		path = cfg.LockPath()
	}
	lock, err := lockfile.Load(path)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "mcpslim: %v\n", err)
		return 1
	}
	n := lock.Approve(*server, *tool)
	if n == 0 {
		_, _ = fmt.Fprintln(stdout, "No pending schema changes.")
		return 0
	}
	if err := lock.Save(); err != nil {
		_, _ = fmt.Fprintf(stderr, "mcpslim: %v\n", err)
		return 1
	}
	_, _ = fmt.Fprintf(stdout, "Approved %d schema change(s) in %s\n", n, path)
	return 0
}
