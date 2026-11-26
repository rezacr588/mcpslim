# mcpslim

A zero-CGO, single-binary Go gateway that sits between AI coding clients
(Claude Code, OpenCode, Gemini CLI, Cursor) and local Model Context Protocol
servers. It attacks two problems at once:

- **Schema bloat** — index every upstream tool and expose one `search_tools`
  meta-tool, hydrating only the schemas the agent actually asks for.
- **Host privilege** — wrap `npx`/`uvx`/binary stdio servers in least-privilege
  OS sandboxes (macOS Seatbelt, Linux Landlock/bubblewrap) that deny reads of
  `~/.ssh`, `~/.aws`, `~/.gnupg` and `.env` by default.

It also pins each tool's name, description and JSON schema in `mcpslim.lock` so
a server cannot silently redefine a tool between runs.

Status: **scaffold** (v0.0.0). No gateway behaviour yet — see
[`docs/roadmap.md`](docs/roadmap.md).

## Why

An MCP-heavy session pays for every tool schema on every request, and a local
stdio server runs with the developer's full user permissions. mcpslim keeps the
schemas out of the prompt until they are needed, and keeps the process out of
anything it was not granted.

## Architecture

```
client (Claude Code / OpenCode / Gemini CLI / Cursor)
   │  stdio, MCP JSON-RPC
   ▼
mcpslim
   ├── router     BM25 index over tool name/description/schema → search_tools
   ├── lockfile   SHA-256 schema pinning + drift approval
   ├── sandbox    Seatbelt (.sb) / Landlock / bwrap profile generation
   └── config     client config discovery, backup and rewrite
        │
        ▼
   upstream stdio/HTTP MCP servers (sandboxed)
```

## Install

Not released yet. When v0.1.0 ships:

```bash
brew install rzeraat/tap/mcpslim
# or
go install github.com/rzeraat/mcpslim/cmd/mcpslim@latest
```

## Build from source

```bash
go build ./...
go test ./...
go run ./cmd/mcpslim version
```

## Documentation

- [`docs/mcpslim-growth-plan.pdf`](docs/mcpslim-growth-plan.pdf) — business and
  open-source growth plan (source of scope and KPIs)
- [`docs/roadmap.md`](docs/roadmap.md) — six-week build and launch sprint
- [`docs/claims.md`](docs/claims.md) — claims that must be backed by a
  reproducible benchmark before they appear in a launch post

## License

MIT — see [LICENSE](LICENSE).
