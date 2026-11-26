# Security Policy

## Supported versions

| Version | Supported |
| --- | --- |
| 0.1.x | yes |
| < 0.1 | no |

## Reporting a vulnerability

Do not open a public issue. Use GitHub's private reporting:

<https://github.com/rezacr588/mcpslim/security/advisories/new>

Include:

- affected version (`mcpslim version`) and OS
- a minimal reproduction: config, upstream server, client request sequence
- what you expected versus what happened
- whether the issue is a sandbox escape, a lockfile bypass, a crash, or
  something else

You will get an acknowledgement within 72 hours and an assessment within seven
days. Fixes are released as a patch version with an advisory that credits you
unless you ask otherwise.

## Scope

In scope:

- **Sandbox escape.** A sandboxed upstream server reading or writing a denied
  path (`~/.ssh`, `~/.aws`, `~/.gnupg`, `~/.netrc`, `~/.config/gcloud`,
  workspace `.env`) or using the network when `--deny-network` is set.
- **Lockfile bypass.** A changed tool definition reaching the client without
  `mcpslim approve`.
- **Gateway vulnerabilities.** Memory exhaustion or panics from malformed
  JSON-RPC, request smuggling between upstreams, or a tool call routed to the
  wrong server.
- **Config rewrite damage.** `mcpslim init` losing or corrupting a client
  config; it is supposed to keep a timestamped backup.

Out of scope:

- An upstream server doing something harmful that the sandbox permits. The
  current sandbox is a deny-list, not default-deny; see the README.
- Vulnerabilities in MCP servers you choose to run.
- Clients ignoring `notifications/tools/list_changed`.
- Running mcpslim itself with elevated privileges.

## Design assumptions

mcpslim assumes the client is trusted and the upstream server is not. It does
not protect against a malicious local user with the same account, who can edit
the config or the lockfile directly.

The lockfile is a change detector, not a signature: it stops silent drift, not
a determined attacker with write access to `mcpslim.lock`.
