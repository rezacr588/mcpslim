# Claims ledger

Numbers in a launch post must be reproducible from this repository. Until a
benchmark backs a claim, it does not ship.

| Claim | Status | Evidence |
| --- | --- | --- |
| 96.5% initial schema-token reduction (29 tools) | measured | `TestSchemaTokenReduction`: 12,641 B upstream → 439 B exposed |
| 80–92% reduction on real servers | unverified | run the 10-server benchmark; the meta-tool costs ~110 tokens, so small tool sets save less |
| <1 ms routing overhead | unverified | `bench/latency`, p50/p99 over 10k routed calls |
| <15 MB binary | measured | 3.3 MB darwin/arm64 `go build`; release artefacts are stripped |
| <15 MB RSS | unverified | `/usr/bin/time -l` on a release build |
| 86% of MCP servers run locally | external, uncited | cite the survey or drop it |
| 82% path traversal / 43% command injection / 79% env credentials | external, uncited | cite the audit or drop it |
| 500M monthly SDK downloads | external, uncited | cite the registry figures or drop it |
| 1,000 stars / 5,000 downloads in 90 days | target, not a claim | report actuals after launch |

Rule: a percentage without a fixture, a command and a commit hash is marketing,
and Hacker News will treat it as such.
