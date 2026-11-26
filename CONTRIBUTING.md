# Contributing to mcpslim

Thanks for helping. This project is small on purpose: standard library only,
one binary, no build tags beyond the OS.

## Development setup

- Go 1.23 or newer
- macOS or Linux (Windows is not supported: no Seatbelt or Landlock)
- Optional: `bwrap` on Linux to exercise the sandbox path

```bash
git clone https://github.com/rezacr588/mcpslim
cd mcpslim
make test
```

## Before opening a pull request

1. `gofmt -l .` prints nothing.
2. `go vet ./...` is clean.
3. `go test -race ./...` passes.
4. New behaviour has a test. Sandbox changes need a test that proves the
   denial, not just that a profile string was produced.
5. Public behaviour changes update `README.md` and `CHANGELOG.md`.

## Commit messages

Conventional Commits, for example:

```
feat(router): hydrate tools on list_changed
fix(sandbox): resolve symlinks before writing Seatbelt rules
docs: describe the deny-list limitation
```

Keep the subject under 72 characters. Explain *why* in the body when the change
is not obvious.

## Sign-off

This project uses the Developer Certificate of Origin. Add a sign-off to every
commit:

```bash
git commit -s -m "feat: ..."
```

The sign-off certifies you wrote the change or have the right to submit it.

## Pull requests

- One logical change per pull request.
- Describe the user-visible effect and how you tested it.
- Link the issue it closes, if any.
- CI must be green. A maintainer reviews for correctness, not style.

## Reporting bugs

Use the issue templates. For a security problem, follow
[SECURITY.md](SECURITY.md) instead of opening a public issue.

## Scope

Good first contributions:

- More MCP client config formats in `mcpslim init`
- Benchmark fixtures from real servers
- Linux sandbox tests on distributions with restricted user namespaces
- Documentation of client-specific `list_changed` behaviour

Out of scope for now: Windows, a GUI, a hosted service, telemetry.
