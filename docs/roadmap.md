# Roadmap — six-week build and launch sprint

Source: `docs/mcpslim-growth-plan.pdf` §5. Each week is done when its
acceptance criteria pass on a clean macOS and Linux runner.

## Week 1 — Core multiplexer and SDK integration

- Initialise the repo on `github.com/modelcontextprotocol/go-sdk`.
- stdio and HTTP upstream pooling; concurrent JSON-RPC routing.
- `mcpslim wrap -- <command>` passthrough mode.

Done when: `mcpslim wrap -- npx -y @modelcontextprotocol/server-filesystem .`
answers `initialize`, `tools/list` and `tools/call` unchanged, with N upstreams
multiplexed behind one session.

## Week 2 — BM25 router and `mcpslim.lock`

- Zero-dependency BM25 scorer over tool name, description and schema.
- `search_tools` meta-tool; hydrate top-k schemas via
  `notifications/tools/list_changed`.
- SHA-256 canonical schema hashing and drift detection.

Done when: a 5-server fixture drops initial tool-schema tokens by the measured
figure (see `docs/claims.md`), and a mutated description is blocked until
`mcpslim approve`.

## Week 3 — macOS Seatbelt and Linux sandbox

- Generate `.sb` profiles for `sandbox-exec`; deny `~/.ssh`, `~/.aws`,
  `~/.gnupg`, `.env` and unapproved egress by default.
- Landlock with bubblewrap fallback.
- Bootstrap policy for `npx`/`uvx` first-run downloads (package fetch must be
  allowed before the server starts).

Done when: a fixture server attempting `~/.ssh/id_rsa` is denied on macOS and
Linux, and a legitimate workspace write succeeds.

## Week 4 — `mcpslim init` and release automation

- Detect and back up configs for Claude Code, OpenCode, Gemini CLI, Cursor;
  rewrite entries to route through mcpslim.
- GoReleaser, Homebrew tap, unit and integration tests.

Done when: `mcpslim init` is reversible (backup restore is one command) and a
tagged release produces darwin/linux, amd64/arm64 artefacts.

## Week 5 — Benchmarks and launch assets

- Reproducible token and latency benchmarks across 10 popular MCP servers.
- 15-second split-screen `vhs` demo.
- README with measured numbers only.

## Week 6 — Launch

- Tag v0.1.0.
- Show HN, X, LinkedIn.
- PRs to `punkpeye/awesome-mcp-servers`, `avelino/awesome-go`, and AI CLI
  directories.

## Open design risks

1. **Prompt-cache invalidation.** Hydrating tools mid-session changes the tool
   block, which can invalidate provider prompt caches and cost more than it
   saves. Measure cache-adjusted cost, not raw token count.
2. **`list_changed` support is uneven.** Some clients re-list, some do not.
   Need a fallback (fixed top-k at `initialize`) and a capability probe.
3. **Sandbox bootstrapping.** `npx`/`uvx` need network on first run; a deny-all
   egress policy breaks cold start. Ship a two-phase policy.
4. **Landlock availability.** Kernel ≥5.13 and unprivileged user namespaces are
   not universal (Ubuntu 24.04 restricts them). Detect and degrade explicitly.
5. **Schema canonicalisation.** JSON key order and whitespace must be
   normalised or every upstream update looks like drift.
