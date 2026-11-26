// Command mcpslim is a single-binary MCP gateway that reduces tool-schema
// context bloat and sandboxes local stdio MCP servers.
//
// This is the v0.0.0 scaffold: it wires the CLI surface only. Gateway,
// router, lockfile and sandbox behaviour land per docs/roadmap.md.
package main

import (
	"fmt"
	"os"
	"runtime"
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
  version     Print version, commit and build date
  help        Print this help

Planned (see docs/roadmap.md):
  wrap        Run any command behind the gateway without touching global config
  init        Detect existing MCP client configs and route them through mcpslim
  serve       Run the gateway for an existing client configuration
  approve     Accept an upstream tool-schema change recorded in mcpslim.lock
`

func main() {
	if len(os.Args) < 2 {
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}

	switch os.Args[1] {
	case "version", "--version", "-v":
		fmt.Printf("mcpslim %s (commit %s, built %s, %s/%s, %s)\n",
			version, commit, date, runtime.GOOS, runtime.GOARCH, runtime.Version())
	case "help", "--help", "-h":
		fmt.Print(usage)
	default:
		fmt.Fprintf(os.Stderr, "mcpslim: unknown command %q\n\n%s", os.Args[1], usage)
		os.Exit(2)
	}
}
