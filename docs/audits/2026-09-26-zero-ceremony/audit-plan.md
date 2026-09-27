# Audit plan — zero-ceremony UX

Mode: parallel independent subagents
Task baseline: dirty remediation worktree at HEAD `2814acb`; the task boundary is the
frozen requirements file, worker ownership and listed files below. Previous remediation
files outside these clusters are covered by `docs/audits/2026-09-26-remediation/` and
are not re-audited merely because they were already dirty.

| Cluster | Files | Depth | Breaker moves | Targeted command | Status |
| --- | --- | --- | --- | --- | --- |
| TASK-001/002 automatic lifecycle + MCP contract | `internal/workflow/**`, `internal/changes/{automatic,automatic_restore,attribution,reconcile,service}.go`, automatic/lifecycle/discovery/server/complete/proving MCP files and tests, `cmd/mindrail/mcp_subprocess_test.go`, relevant coordination completion profile fields | A | 1–6 + cost, compatibility, class sweep | `go test -race ./internal/workflow ./internal/changes ./internal/coordination ./internal/completion ./internal/mcp ./cmd/mindrail` | pending |
| TASK-003 setup and human CLI | `internal/setup/**`, `internal/cli/{root,init,verify,simple_usage_test}.go` and narrowly changed verify/init fixtures, generated `AGENTS.md`, `.mindrail/**` | A | 1–6; threat model and upgrade path mandatory | `go test -race ./internal/setup ./internal/cli` | pending |
| FIX-012 schema/index generation and restoration | `internal/index/{store_cas,store,index,errors}.go`, focused index tests, `migrations/000010_file_index_generations.sql`, migration/CLI schema-10 tests and goldens | A | 1–6; old schema-9 DB upgrade; ABA/stale token; dual readers | `go test -race ./internal/index/... ./internal/migration ./migrations ./internal/workflow` | pending |
| TASK-004 documentation | `README.md`, `docs/usage-tr.md`, frozen requirements and generated managed instructions | C | Reader doc↔code comparison | runnable quickstart + help/schema probes | pending |
| Integration seams | setup→AGENTS/hook, MCP connection→workflow→changes→index→completion, CLI→MCP stdio, v9→v10 migration | A | 1, 4, 5, 6 + backward compatibility | `make verify && make gate`; disposable stdio/upgrade probes | pending |

Downgrades: none.
Sampling: mutation may target representative guards per semantic class because the
integrated diff includes prior already-audited remediation and generated graph files;
every task-owned protective class must still have at least one mutation, and every
survivor must be discriminated manually.
Excluded from this audit: pre-existing `.claude/**`, `.wrongstack/**`, `.zcode/**`,
the earlier validation/completion remediation cluster already audited in the report
above, and generated `graphify-out/**` content except freshness/existence.
