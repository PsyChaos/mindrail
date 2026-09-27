# Remediation round 2 verification

Working tree: current uncommitted remediation delta over base
`2814acb22d8aa94d48ade1242b58e1fdb14239fc`. Commands ran in the shared
repository after all round-1 audit fixes were integrated. No result below is a
CI assumption; every row was executed locally.

| Command | Result |
|---|---|
| targeted integration: validation, changes, completion, MCP, coordination, CLI, cmd, migration, migrations | PASS; CLI 61.320 s, MCP 14.331 s, remaining packages PASS |
| `graphify update .` | PASS; no topology change; SQL parser dependency still unavailable and reported as a warning |
| `make tidy-check` | PASS |
| `make verify` | PASS; vet, normal `go test ./...`, full `go test -race ./...`, and cmd smoke all green |
| full race notable timings | CLI 496.379 s, MCP 85.143 s, coordination 92.835 s, changes 45.399 s, validation 18.861 s |
| `make gate` | PASS; unit 1341, domain 417, integration 10, race 9, knowledge-schema 216, MCP-contract 34, worktree 77 + named 7, SQLite concurrency 17, E2E 10, smoke 5 |
| `make bench` | PASS; MCP after-change p95 11.061 ms / 1.5 s target; MCP reconcile p95 21.348 ms / 2 s; changes after-change p95 2.100 ms / 1.5 s; changes reconcile p95 14.337 ms / 2 s |
| `make release` | PASS; linux/amd64 built, checksums generated, cmd smoke PASS 6.918 s |
| cross targets | `not-built` for linux/arm64, darwin/amd64, darwin/arm64, windows/amd64 because the existing Tree-sitter Go bindings exclude those cross-builds; Makefile reports these as best-effort |
| `git diff --check` | PASS |

The release build correctly reports `dirty` because the remediation has not been
committed. The audit's destructive and mutation probes run in separate `/tmp`
copies and are not part of this validation record.
