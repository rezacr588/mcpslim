# mcpslim

[![CI](https://github.com/rezacr588/mcpslim/actions/workflows/ci.yml/badge.svg)](https://github.com/rezacr588/mcpslim/actions/workflows/ci.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/rezacr588/mcpslim.svg)](https://pkg.go.dev/github.com/rezacr588/mcpslim)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)
[![Status: alpha](https://img.shields.io/badge/status-alpha-orange.svg)](#status)

A single Go binary that sits between your AI coding client and your local MCP
servers. It loads tool schemas on demand and sandboxes the servers it spawns.

- **Less context, same tools.** One `search_tools` meta-tool replaces every
  upstream schema; matched tools are hydrated mid-session.
- **No host-wide trust.** Upstream `npx`/`uvx`/binary servers run under macOS
  Seatbelt or Linux bubblewrap, with `~/.ssh`, `~/.aws`, `~/.gnupg` and
  workspace `.env` files denied.
- **No silent tool changes.** `mcpslim.lock` pins each tool's name,
  description and JSON schema by hash; a changed definition is blocked until
  you run `mcpslim approve`.

No CGO, no runtime dependencies, one binary.

## Status

Alpha (`v0.1.x`). The gateway, BM25 routing, schema pinning and sandbox are
implemented and tested. Not yet implemented: resources, prompts, sampling,
OAuth, and a default-deny sandbox profile (see [Limitations](#limitations)).

## Install

```bash
go install github.com/rezacr588/mcpslim/cmd/mcpslim@latest
```

Homebrew and prebuilt binaries arrive with the first tagged release; the
Homebrew tap follows once `rezacr588/homebrew-tap` exists.

## Quick start

Wrap one server without touching any config:

```bash
mcpslim wrap -- npx -y @modelcontextprotocol/server-filesystem .
```

Use that whole command wherever your client expects an MCP server command.
For several servers, write a config and run `mcpslim serve`:

```json
{
  "servers": {
    "filesystem": {
      "command": "npx",
      "args": ["-y", "@modelcontextprotocol/server-filesystem", "."]
    },
    "github": {
      "command": "npx",
      "args": ["-y", "@modelcontextprotocol/server-github"],
      "env": { "GITHUB_TOKEN": "..." }
    }
  },
  "router": { "topK": 5 },
  "sandbox": { "denyNetwork": false }
}
```

```bash
mcpslim serve --config ~/.config/mcpslim/config.json
```

`mcpslim init` finds existing Claude, Cursor and Gemini configs, backs them up,
and rewrites them to route through mcpslim:

```bash
mcpslim init            # dry run
mcpslim init --write
```

## How it works

```
client (Claude Code, OpenCode, Gemini CLI, Cursor)
   │  MCP over stdio
   ▼
mcpslim
   ├── router    BM25 over tool name, description and schema → search_tools
   ├── lockfile  canonical SHA-256 pinning, drift blocking, approve
   ├── sandbox   Seatbelt profile / bubblewrap wrapper
   └── gateway   one session, N upstream servers, collision-safe names
        │
        ▼
   sandboxed stdio servers
```

The client starts with one tool:

```
search_tools({"query": "read a file"})
→ Found 1 tool(s) for "read a file":
  - read_file (filesystem): Read the contents of a file from disk
```

mcpslim then sends `notifications/tools/list_changed`; the next `tools/list`
contains `read_file` with its real schema, and calls route to the owning
server.

### Measured effect

`TestSchemaTokenReduction` runs 29 synthetic tools with realistic schemas:
12,641 bytes of upstream tool JSON become 439 bytes initially — a 96.5%
reduction. The saving depends on how many tools you have; a handful of tiny
tools saves little, because `search_tools` itself costs about 110 tokens.

### Commands

| Command | Purpose |
| --- | --- |
| `mcpslim wrap -- <cmd>` | Proxy one server, no config changes |
| `mcpslim serve` | Serve every configured upstream |
| `mcpslim init [--write]` | Discover and rewrite client configs |
| `mcpslim approve` | Accept a pinned schema change |
| `mcpslim version` | Build information |

### Flags

`serve` and `wrap` accept `--no-sandbox`, `--deny-network`, `--top-k`,
`--lock`. `serve` also accepts repeatable `--server name=command`.

## Limitations

- **Deny-list sandbox, not default-deny.** Credential paths and `.env` are
  denied; everything else is allowed. A default-deny profile is on the roadmap.
- **Network is allowed by default** because `npx`/`uvx` must fetch packages on
  first run. `--deny-network` closes it.
- **Linux sandboxing needs `bwrap`** and unprivileged user namespaces. Without
  them mcpslim logs a warning and runs the server unsandboxed.
- **`list_changed` support varies by client.** Clients that ignore it will not
  see hydrated tools; keep `--top-k` small in that case.
- **Tools-only.** Resources, prompts and sampling are not proxied.

## Development

```bash
make test        # unit + integration tests
make race        # race detector
make build       # ./mcpslim
make lint        # go vet (golangci-lint in CI)
```

The test suite includes a real Seatbelt enforcement test on macOS and a
schema-drift test that runs the gateway twice against a mutated server.

See [CONTRIBUTING.md](CONTRIBUTING.md), [SECURITY.md](SECURITY.md) and
[docs/roadmap.md](docs/roadmap.md).

## License

MIT — see [LICENSE](LICENSE).
