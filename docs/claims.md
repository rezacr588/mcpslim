# Claims ledger

Numbers in a launch post must be reproducible from this repository. Until a
benchmark backs a claim, it does not ship.

| Claim (from the plan) | Status | How to prove |
| --- | --- | --- |
| 80–92% schema-token reduction | unverified | `bench/tokens` over the 10-server fixture set; report per-server and mean |
| <1 ms routing overhead | unverified | `bench/latency`, p50/p99 over 10k routed calls |
| 86% of MCP servers run locally | external, uncited | cite the survey or drop it |
| 82% path traversal / 43% command injection / 79% env credentials | external, uncited | cite the audit or drop it |
| 500M monthly SDK downloads | external, uncited | cite the registry figures or drop it |
| <15 MB binary, <15 MB RSS | unverified | `ls -lh` on release artefacts + `/usr/bin/time -l` |
| 1,000 stars / 5,000 downloads in 90 days | target, not a claim | report actuals after launch |

Rule: a percentage without a fixture, a command and a commit hash is marketing,
and Hacker News will treat it as such.
