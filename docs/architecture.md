# Architecture

## Request lifecycle

1. **Start.** `mcpslim` reads its config, loads `mcpslim.lock`, and spawns each
   upstream with its sandbox wrapper. It sends `initialize` and `tools/list` to
   every server, then pins any tool it has not seen before.
2. **Client handshake.** The client's `initialize` is answered locally. The
   gateway advertises only `tools.listChanged`.
3. **Cold tool list.** `tools/list` returns exactly one tool, `search_tools`.
   No upstream schema is in the prompt.
4. **Search.** `search_tools({"query": ...})` scores every allowed tool with
   BM25 over its name, description and property names. The top-k become
   *hydrated*; the gateway emits `notifications/tools/list_changed`.
5. **Warm tool list.** The next `tools/list` includes the hydrated tools with
   their original schemas. `tools/call` routes to the owning server with the
   original tool name.
6. **Drift.** A definition that no longer matches its pin is removed from the
   index, blocked at call time, and its pending hash is stored. `mcpslim
   approve` promotes the pending hash.

## Packages

| Package | Responsibility |
| --- | --- |
| `internal/mcp` | JSON-RPC framing, stdio connection, MCP client methods |
| `internal/gateway` | Session state, routing, hydration, drift handling |
| `internal/router` | BM25 index over tool text |
| `internal/lockfile` | Canonical hashing, pin storage, approve |
| `internal/sandbox` | Seatbelt profile generation, bubblewrap wrapping |
| `internal/config` | Config load/save, client config discovery and rewrite |
| `cmd/mcpslim` | CLI |

## Invariants

- **stdout is protocol only.** Every diagnostic goes to stderr.
- **One checksum per tool.** The canonical form sorts keys, normalises number
  encoding and collapses whitespace, so formatting never looks like drift.
- **Blocked tools are not advertised.** They are absent from `tools/list` and
  from the BM25 index, and calling one returns an error naming `mcpslim
  approve`.
- **Sandbox paths are resolved.** Seatbelt matches resolved paths; `/tmp` and
  `/var` are symlinks on macOS, so an unresolved deny is a silent no-op.

## Threat model

Trusted: the client, the local user, the mcpslim binary.

Untrusted: upstream MCP servers, including `npx`/`uvx` packages.

The sandbox is a deny-list: credential directories, `.env` files in the
workspace, and optionally the network. It is not a default-deny jail. A server
can still read the rest of the workspace and any file outside the deny list.
That is a deliberate MVP trade-off: a default-deny profile breaks `npx` cold
starts and most servers that read shared caches.

The lockfile is a drift detector, not a signature. A local attacker who can
write `mcpslim.lock` can approve anything.

## Why BM25

Tool choice is a retrieval problem over short, keyword-dense documents. BM25
needs no model, no network and no dependency, runs in microseconds, and is
predictable: the same query always returns the same ranking. Embeddings would
rank better on paraphrases but add a model download, latency and
nondeterminism to a security tool.

## Failure modes

| Symptom | Cause | Handling |
| --- | --- | --- |
| Hydrated tool missing in client | client ignores `list_changed` | keep `topK` small; tools are still callable by name |
| `npx` fails at startup | `--deny-network` on a cold cache | pre-install the package or allow network |
| No sandbox on Linux | `bwrap` missing or userns disabled | warning on stderr, server runs unsandboxed |
| Everything blocked after upgrade | legitimate schema change | `mcpslim approve` |
| Prompt cache misses | tool block changes mid-session | measure cache-adjusted cost before enabling hydration |
