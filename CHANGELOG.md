# Changelog

All notable changes are documented here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/) and this project
adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [0.1.0] - 2026-11-26

First working alpha.

### Added

- MCP stdio gateway: one client session, many upstream stdio servers,
  collision-safe tool names.
- BM25 tool router with a `search_tools` meta-tool and mid-session hydration
  through `notifications/tools/list_changed`.
- `mcpslim.lock` schema pinning: canonical SHA-256 hashes, drift blocking and
  `mcpslim approve`.
- OS sandbox: generated macOS Seatbelt profiles (symlink-resolved) and Linux
  bubblewrap wrappers; denies `~/.ssh`, `~/.aws`, `~/.gnupg`, `~/.netrc`,
  `~/.config/gcloud` and workspace `.env`.
- CLI: `wrap`, `serve`, `init`, `approve`, `version`.
- `mcpslim init` discovery and rewrite for Claude project, Cursor and Gemini
  configs, with timestamped backups.
- Test suite: gateway round trip, hydration, schema drift, Seatbelt
  enforcement, config rewrite, and a token-reduction test (96.5% on 29 tools).

### Known limitations

- Deny-list sandbox rather than default-deny.
- Network allowed unless `--deny-network`.
- Tools only: resources, prompts and sampling are not proxied.
- Linux sandboxing requires `bwrap` and unprivileged user namespaces.
