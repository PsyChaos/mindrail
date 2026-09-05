# Mindrail — Technology Stack & Implementation Guide

> **Document type:** Engineering / Implementation Specification  
> **Product:** Mindrail  
> **Applies to:** Mindrail, aligned with Technical Specification 1.0  
> **Status:** Initial implementation baseline  
> **Date:** 2026-09-01  
> **Primary language:** Go  
> **Purpose:** Define how Mindrail is implemented without changing the behavioral and architectural contracts defined by the Mindrail Technical Specification 1.0.

---

# 1. Purpose and Authority

This document defines the concrete engineering stack used to implement Mindrail.

The Mindrail Technical Specification 1.0 defines:

```text
WHAT Mindrail must do
WHY the system behaves that way
WHICH behavioral guarantees exist
WHICH state transitions and safety rules are required
```

This document defines:

```text
HOW those requirements are implemented
WHICH technologies and libraries are used
HOW source code is organized
HOW processes, storage, parsing and integration are built
HOW Mindrail is compiled, tested and distributed
```

If an implementation choice in this document conflicts with the Technical Specification 1.0:

> **The Technical Specification 1.0 wins.**

This document must not redefine task semantics, lease semantics, invariant behavior, impact rules, evidence rules or completion gates.

---

# 2. Implementation Principles

Mindrail follows these implementation principles.

## 2.1 Local-first

Mindrail must run locally without requiring:

- a cloud account,
- Redis,
- PostgreSQL,
- Kubernetes,
- a central server,
- an external AI API.

Normal installation target:

```text
one local `mindrail` executable
+
project-specific external semantic resolvers when required
```

## 2.2 Deterministic core

Core decisions are implemented deterministically.

Examples:

```text
symbol changed?
lease valid?
snapshot stale?
resolver healthy?
validation passed?
knowledge schema valid?
impact edge exists?
completion allowed?
```

These decisions are not delegated to another LLM.

## 2.3 Standard library first

Prefer the Go standard library unless an external package clearly improves:

- protocol correctness,
- interoperability,
- parser capability,
- portability,
- maintenance burden.

## 2.4 Small dependency surface

A runtime dependency should be accepted only when at least one is true:

1. Reimplementing it would be technically risky.
2. It implements an external standard/protocol.
3. It substantially reduces cross-platform complexity.
4. It is mature and actively maintained.
5. It removes a large amount of error-prone infrastructure code.

## 2.5 No premature distributed architecture

The target architecture does not introduce:

```text
Redis
message broker
remote graph DB
distributed locks
microservices
workflow engine
```

Default architecture remains:

```text
Mindrail process
      │
      ├── SQLite
      ├── filesystem
      ├── Git CLI
      ├── Tree-sitter
      └── supervised resolver processes
```

---

# 3. Primary Language

Mindrail core is implemented in:

```text
Go
```

Initial development baseline:

```text
Go 1.27.x
```

Initial `go.mod` directive:

```go
go 1.27
```

Patch releases are consumed through normal toolchain maintenance.

---

# 4. Why Go

Mindrail is primarily a:

```text
local developer tool
CLI
MCP server
scheduler
process supervisor
SQLite application
filesystem/indexing worker
cross-platform executable
```

Go provides a strong balance of:

- fast development,
- simple concurrency,
- good subprocess management,
- strong standard library,
- low runtime overhead,
- practical distribution,
- good cross-platform support.

Mindrail's main complexity is domain and coordination logic rather than low-level memory management.

Rust remains technically valid but adds implementation complexity that is not justified for the target architecture.

Python remains useful for fixtures and resolver ecosystems but is not the Mindrail core implementation language.

---

# 5. Runtime Architecture

```text
                    ┌──────────────────────┐
                    │    AI Coding Agent   │
                    │ Claude / Codex / ... │
                    └──────────┬───────────┘
                               │
                         MCP / CLI
                               │
                    ┌──────────▼───────────┐
                    │       Mindrail       │
                    │      Go Process      │
                    └──────────┬───────────┘
                               │
        ┌──────────────────────┼─────────────────────────┐
        │                      │                         │
        ▼                      ▼                         ▼
   Runtime Core          Code Intelligence         Integration
        │                      │                         │
   SQLite/WAL              Tree-sitter                 Git
   Tasks                   Semantic                  MCP
   Leases                  Resolvers                 CLI
   Changes                 Coverage                 CI
   Evidence                Impact                   Hooks
```

---

# 6. Executable Model

Primary executable:

```text
mindrail
```

The same executable serves:

```text
CLI
MCP stdio server
local index worker
validation runner
doctor
Git verification
CI verification
knowledge tools
```

Do not split these into separate executables unless a concrete technical requirement appears.

Example commands:

```bash
mindrail init
mindrail status
mindrail doctor
mindrail search
mindrail context
mindrail index
mindrail mcp
mindrail verify
mindrail knowledge validate
```

---

# 7. Source Repository Layout

Recommended structure:

```text
mindrail/
├── cmd/
│   └── mindrail/
│       └── main.go
│
├── internal/
│   ├── app/
│   ├── bootstrap/
│   ├── project/
│   ├── workspace/
│   ├── session/
│   ├── task/
│   ├── change/
│   ├── lease/
│   ├── checkpoint/
│   │
│   ├── knowledge/
│   │   ├── decision/
│   │   ├── invariant/
│   │   ├── schema/
│   │   └── loader/
│   │
│   ├── index/
│   │   ├── inventory/
│   │   ├── scheduler/
│   │   ├── snapshot/
│   │   ├── parser/
│   │   ├── symbol/
│   │   └── graph/
│   │
│   ├── semantic/
│   │   ├── resolver/
│   │   ├── manager/
│   │   ├── projectunit/
│   │   ├── pyright/
│   │   └── typescript/
│   │
│   ├── coverage/
│   │   ├── provider/
│   │   ├── python/
│   │   └── javascript/
│   │
│   ├── impact/
│   │   ├── graph/
│   │   ├── traversal/
│   │   ├── scoring/
│   │   └── grouping/
│   │
│   ├── validation/
│   │   ├── planner/
│   │   ├── runner/
│   │   └── profile/
│   │
│   ├── evidence/
│   ├── approval/
│   ├── reconcile/
│   ├── git/
│   ├── filesystem/
│   ├── config/
│   ├── storage/
│   ├── migration/
│   ├── doctor/
│   ├── telemetry/
│   ├── mcp/
│   └── cli/
│
├── schemas/
│   └── knowledge/
├── migrations/
├── queries/
│   ├── python/
│   ├── javascript/
│   └── typescript/
├── testdata/
│   ├── repositories/
│   ├── languages/
│   └── knowledge/
├── docs/
│   ├── specification/
│   ├── engineering/
│   └── adr/
├── scripts/
├── go.mod
├── go.sum
├── Makefile
└── README.md
```

---

# 8. Package Design Rules

Prefer domain packages over generic technical buckets.

Avoid:

```text
models/
services/
repositories/
utils/
helpers/
```

Prefer:

```text
task/
change/
knowledge/
impact/
validation/
semantic/
```

Most code belongs under `internal/` because Mindrail is an executable product, not a public Go framework.

Do not create a public `pkg/` API until a real external consumer exists.

---

# 9. Dependency Injection

Mindrail does not use a DI framework.

Dependencies are wired explicitly through constructors.

```go
type Service struct {
    store Store
    clock Clock
}

func NewService(store Store, clock Clock) *Service {
    return &Service{store: store, clock: clock}
}
```

Reasons:

- explicit dependencies,
- easy navigation,
- simple tests,
- no runtime magic.

---

# 10. Interface Policy

Do not create interfaces preemptively.

Interfaces are justified when:

- multiple implementations exist,
- a provider boundary exists,
- a test boundary is genuinely useful,
- architecture explicitly requires pluggability.

Good candidates:

```text
SemanticResolver
CoverageProvider
ApprovalProvider
Clock
CommandRunner
GitAdapter
```

Do not create an interface for every service merely for style.

---

# 11. CLI Framework

Selected:

```text
github.com/spf13/cobra
```

Reason:

Mindrail has a hierarchical command surface:

```text
mindrail knowledge validate
mindrail knowledge migrate
mindrail doctor --resolver
mindrail index --foreground
mindrail verify --ci
```

Cobra provides mature subcommands, flag handling, help and completion support.

---

# 12. CLI Output

Every significant CLI command supports:

```text
human-readable output
JSON output
```

Example:

```bash
mindrail status --json
```

Agents and CI must consume structured output rather than parsing colored prose.

Rules:

- JSON never contains ANSI escape codes.
- `NO_COLOR` is respected.
- human output remains concise and actionable.
- JSON schemas should be stable once 1.0 is released.

---

# 13. CLI Exit Codes

Recommended small convention:

```text
0 success
1 operation failed
2 invalid usage/config
3 verification denied
4 temporary/runtime unavailable
```

Fine-grained error information belongs in structured output, not dozens of OS exit codes.

---

# 14. MCP SDK

Selected:

```text
github.com/modelcontextprotocol/go-sdk/mcp
```

Use the official MCP Go SDK.

Primary transport:

```text
stdio
```

Command:

```bash
mindrail mcp
```

Mindrail must not implement MCP manually unless an isolated compatibility workaround becomes necessary.

---

# 15. MCP Layering

MCP handlers stay thin.

```text
MCP request
    ↓
input schema validation
    ↓
application service
    ↓
domain result
    ↓
MCP response mapper
```

Business logic never lives primarily in MCP handlers.

CLI and MCP call the same application services.

---

# 16. MCP Tool Surface

Technical Specification 1.0 tool surface remains approximately:

```text
mindrail_bootstrap
mindrail_status
mindrail_search
mindrail_context
mindrail_claim
mindrail_before_change
mindrail_after_change
mindrail_reconcile
mindrail_decide
mindrail_invariant
mindrail_checkpoint
mindrail_validate
mindrail_complete
```

Public MCP tools use the `mindrail_*` prefix. Technical Specification 1.0 defines no legacy tool aliases, because no public compatibility surface has shipped yet.

---

# 17. MCP Serialization

Use:

```text
encoding/json
```

Inputs and outputs use explicit structs.

Avoid `map[string]any` for stable contracts except genuinely dynamic fields.

---

# 18. Runtime Database

Canonical runtime database:

```text
SQLite
```

Mindrail accesses SQLite through the standard `database/sql` boundary.

The concrete driver is selected by the benchmark policy defined in the next section; no driver is considered architecturally fixed before ADR-0002 is accepted.

---

# 19. ORM Policy

Do not use:

```text
GORM
Ent
SQLBoiler
```

for the Mindrail runtime DB.

Use:

```text
database/sql
+
explicit SQL
```

Mindrail depends on precise:

- transaction boundaries,
- SQLite WAL behavior,
- revision checks,
- idempotency,
- migrations.

An ORM does not improve these core concerns.

---

# 20. SQLite Driver Policy

Canonical storage API:

```text
database/sql
```

The concrete SQLite driver is intentionally **provisional until Mindrail-specific benchmark results are available**.

Candidates:

```text
modernc.org/sqlite
mattn/go-sqlite3
```

Reason for reopening the choice:

Mindrail already requires a native C toolchain for the official Tree-sitter Go binding. Therefore "avoid all CGO" is not by itself a sufficient architectural reason to choose the SQLite driver.

The driver decision MUST be based on the workload Mindrail actually performs.

Required benchmark matrix:

```text
WAL concurrent read/write
100 / 500 / 1000 row symbol+edge batch inserts
FTS5 search
short interactive writes during cold index
SQLITE_BUSY contention
open/reopen recovery
supported OS build matrix
```

Acceptance criteria:

- correct `database/sql` behavior,
- WAL support,
- FTS5 support required by Mindrail,
- acceptable interactive p95 under index load,
- supported release platforms,
- acceptable maintenance/security profile.

Benchmark harness requirement:

FTS5 must be explicitly enabled in every candidate build before measurement. `mattn/go-sqlite3` only compiles FTS5 with the `sqlite_fts5` build tag; a benchmark run without it silently measures a driver that cannot satisfy Mindrail's search requirement and invalidates the comparison.

```bash
go test -tags sqlite_fts5 -bench . ./internal/storage/...
```

Any driver-specific build tag required for FTS5 must also be recorded in the Makefile, the release CI workflow and ADR-0002.

Until ADR-0002 records benchmark evidence, storage code MUST remain behind `database/sql` and MUST NOT depend on driver-specific APIs outside a tiny adapter/bootstrap boundary.

---

# 21. SQLite Transaction Policy

Long work never runs inside a DB transaction.

Forbidden:

```text
BEGIN
Tree-sitter parse
Pyright request
run tests
COMMIT
```

Required pattern:

```text
read state
release transaction
perform expensive work
open short write transaction
compare revision
persist result
commit
```

---

# 22. Optimistic Concurrency

Mutable state that can receive stale writes uses revisions.

Example:

```sql
UPDATE changes
SET status = ?, revision = revision + 1
WHERE id = ? AND revision = ?;
```

Result:

```text
1 affected row → success
0 affected rows → REVISION_CONFLICT
```

No silent last-write-wins.

---

# 23. Operation Idempotency

Expensive repeatable operations receive deterministic operation IDs.

Examples:

```text
after_change
validation execution
coverage import
knowledge import
```

The same request must safely survive:

- duplicate MCP delivery,
- client retry,
- SQLite busy retry,
- process interruption.

---

# 24. SQLite Migrations

Do not add a migration framework initially.

Use:

```text
embedded numbered SQL files
+
schema_migrations table
```

Example:

```text
migrations/
├── 000001_initial.sql
├── 000002_content_cache.sql
└── 000003_evidence.sql
```

Embed with Go `embed`.

Migration rules:

- ordered,
- forward-only by default,
- transactional where possible,
- fail-fast,
- no implicit downgrade.

---

# 25. Tree-sitter

Selected Go binding:

```text
github.com/tree-sitter/go-tree-sitter
```

Initial grammars:

```text
Python
JavaScript
TypeScript / TSX
```

Use official Tree-sitter grammar modules where available.

---

# 26. Tree-sitter Build Constraint

The official Go binding uses CGO.

Therefore release engineering must not assume:

```text
CGO_ENABLED=0
one-host cross compilation for every platform
```

Instead use:

> **platform-native CI builds**

Initial build matrix:

```text
Linux amd64
Linux arm64
macOS amd64
macOS arm64
Windows amd64
```

Each advertised target must be compiled and smoke-tested in an appropriate runner/toolchain environment.

---

# 27. Single Executable Goal

Distribution goal:

```text
one `mindrail` executable per OS/architecture
```

Tree-sitter parser/grammar code should be linked into the Mindrail executable for supported built-in languages where practical.

Users should not normally need to install Tree-sitter shared libraries.

Dynamic grammar loading may later exist as an extension mechanism, but it is not the default model for the target architecture.

---

# 28. Tree-sitter Resource Lifecycle

Tree-sitter native resources must be explicitly closed.

Examples include:

```text
Parser
Tree
TreeCursor
Query
QueryCursor
LookaheadIterator
```

Do not rely on Go finalizers for correctness.

Adapter tests must include repeated parse/dispose cycles to catch leaks.

---

# 29. Tree-sitter Adapter Layer

Core code does not depend directly on grammar-specific node names.

Conceptual contract:

```go
type SyntaxAdapter interface {
    Parse(ctx context.Context, src SourceFile) (*SyntaxSnapshot, error)
    Symbols(snapshot *SyntaxSnapshot) ([]Symbol, error)
    References(snapshot *SyntaxSnapshot) ([]Reference, error)
}
```

Each language adapter outputs normalized Mindrail structures.

---

# 30. Tree-sitter Query Files

Prefer query files where useful:

```text
queries/python/symbols.scm
queries/python/imports.scm
queries/python/tests.scm
queries/typescript/symbols.scm
queries/typescript/imports.scm
queries/typescript/tests.scm
```

`tests.scm` captures the test-shaped constructs the false-green guard needs — test declarations, assertion/expectation calls and skip/xfail markers — so the guard heuristics defined by Technical Specification 1.0 stay grammar-local instead of leaking framework knowledge into core packages.

Benefits:

- grammar-specific logic stays localized,
- fixture testing becomes easier,
- parser adapters remain smaller.

Semantic binding remains the resolver's responsibility.

---

# 31. Content-Addressed Parse Cache

Parse cache identity includes at least:

```text
language
grammar version
parser schema version
content hash
```

The same file content in multiple worktrees should reuse the same parse snapshot.

Default content hash:

```text
SHA-256
```

using Go `crypto/sha256`.

---


# 32. Agent Workspace Modes

Mindrail supports:

```text
same workspace + sequential agents
separate worktrees + concurrent agents (recommended isolation)
same workspace + concurrent agents (reduced isolation)
```

A Git worktree is not a prerequisite for Mindrail adoption.

The implementation must not assume `agent_session_id == worktree`.

For same-workspace sequential use, session/checkpoint/reconcile boundaries provide continuity.

For same-workspace concurrency, attribution is more conservative and reconcile may return ambiguity instead of guessing.

---


# 33. Durable Symbol Identity Implementation

Storage separates:

```text
symbol_uid
logical_key
```

`symbol_uid` is the durable relation target; `logical_key` is mutable current identity used for lookup/display.

Rename migration evaluates Git move/rename signal, semantic declaration identity, owner identity and structural fingerprints.

Ambiguous migration is represented explicitly; HIGH/CRITICAL protected relations cannot be silently orphaned.

## Allocation

`symbol_uid` allocation is concurrency-critical: multiple processes and worktrees can observe the same new symbol at the same time. Two uids for one symbol would split its invariant, evidence and lease relations.

Storage layout:

```text
symbols          (symbol_uid PRIMARY KEY, ...)
symbol_identity  (allocation_key UNIQUE, symbol_uid, ...)
```

Allocation MUST use insert-then-read-back against the unique constraint:

```sql
INSERT INTO symbol_identity (allocation_key, symbol_uid)
VALUES (?, ?)
ON CONFLICT (allocation_key) DO NOTHING;

SELECT symbol_uid FROM symbol_identity WHERE allocation_key = ?;
```

Rules:

- Never `SELECT` first and `INSERT` when missing; that sequence races between processes.
- The locally generated candidate uid is discarded whenever the conflict path returns an existing uid.
- Rename migration `UPDATE`s the `symbol_identity` row's `allocation_key`; it never inserts a new `symbols` row.
- Allocation runs only after identity migration has been attempted and failed, otherwise every rename silently creates a new symbol.
- Sharing a content-addressed parse snapshot does not imply sharing a `symbol_uid`; allocation is a separate atomic step.

Tests must cover:

```text
method rename
file move + method rename
overload rename ambiguity
nested function
class rename with child methods
protected invariant migration
concurrent first observation from two processes yields one uid
```

---

# 34. Git Integration

Use the system:

```text
git
```

through `os/exec`.

Do not use `go-git` as the canonical implementation.

Mindrail relies on Git's exact behavior for:

- worktrees,
- common-dir,
- staged index,
- merge-base,
- repository state,
- rename/diff semantics,
- hooks.

The installed Git executable is the repository truth.

---

# 35. Safe Git Execution

Never construct shell strings from agent/user input.

Bad:

```go
exec.Command("sh", "-c", "git diff " + userInput)
```

Good:

```go
exec.CommandContext(ctx, "git", "diff", "--", path)
```

Use stable machine output where possible:

```text
--porcelain=v2
-z
--name-status
explicit --format
```

---

# 36. File Watching

Selected:

```text
github.com/fsnotify/fsnotify
```

File watcher events are optimization hints, not correctness evidence.

Correctness remains based on:

```text
content hashes
Git diff
snapshot comparison
```

Watcher flow:

```text
filesystem event
      ↓
dirty hint
      ↓
debounce
      ↓
hash actual file
      ↓
real change?
      ↓
refresh relevant index
```

---

# 37. Project Configuration

Primary format:

```text
TOML
```

Selected library:

```text
github.com/pelletier/go-toml/v2
```

Use typed configuration structs.

Use strict decoding where practical so unknown safety-critical fields are not silently ignored.

Precedence, highest wins:

```text
1. explicit CLI option/flag
2. process environment (MINDRAIL_* variables)
3. repository `.mindrail/config.toml`
4. user config <os.UserConfigDir()>/mindrail/config.toml
5. Mindrail defaults
```

Layer rules:

- Repository config outranks user config for anything that affects analysis or gate behavior, so project behavior stays repository-reproducible.
- The user layer may only carry machine-local preferences: resolver resource ceilings, managed resolver cache location, color/output preferences, local telemetry retention.
- The user layer MUST NOT weaken a repository-defined safety setting — `on_resolver_unavailable`, risk minimums, approval policy, validation profiles, evidence policy. Such keys are rejected with an explicit diagnostic instead of being silently applied.
- Environment variables use the `MINDRAIL_` prefix and map to typed config fields. An unknown `MINDRAIL_*` variable produces a warning rather than a silent ignore.
- `mindrail doctor` reports the effective value and its source layer for every safety-critical setting, so "why is this setting active?" is answerable without reading five files.

---

# 38. Knowledge Records

Decisions and Invariants remain repository-owned JSON files according to Technical Specification 1.0.

Serialization:

```text
encoding/json
```

Schemas are bundled under:

```text
schemas/knowledge/
```

---

# 39. JSON Schema Validation

Selected library:

```text
github.com/santhosh-tekuri/jsonschema/v6
```

Target schema dialect:

```text
JSON Schema Draft 2020-12
```

Supported schemas are embedded in the executable.

Normal knowledge validation must not fetch schemas from the network.

---

# 40. Knowledge Schema Compatibility

Implementation supports:

```text
readable_schema_versions
write_schema_version
```

Rule:

> **Read old, write current.**

Mixed readable schema generations are allowed.

Unknown newer blocking schema fails closed.

Bulk migration remains explicit.

---

# 41. Stable Identifiers

Persistent entities use opaque identifiers with readable prefixes.

Examples:

```text
TASK-...
CHG-...
INV-...
DEC-...
EV-...
```

Database integer IDs are never external identity.

If creation-time ordering is useful, use a time-sortable opaque ID implementation.

---

# 42. Time Handling

All persisted timestamps:

```text
UTC
```

Use Go `time.Time`.

Serialize with RFC 3339 / RFC 3339 Nano as appropriate.

Inject a `Clock` interface only into areas where deterministic time matters, especially:

- leases,
- stale session detection,
- approval expiration,
- tests.

---


# 43. Interactive Latency Engineering

Technical Specification 1.0 defines warm-path latency SLOs for `mindrail_status`, `mindrail_context`, `mindrail_before_change`, `mindrail_after_change` and `mindrail_reconcile`.

Those SLOs are defined per capability mode: STRUCTURAL and FULL semantic have separate targets.

Implementation requirements:

- instrument operation duration from the first implementation,
- separate interactive work from deep/cold enrichment,
- timeout semantic resolver queries,
- avoid repository-wide work in synchronous request paths,
- benchmark with representative fixture repositories,
- report p50/p95 separately for STRUCTURAL and FULL runs,
- publish p50/p95 in performance test output.

Compliance mechanism:

An operation that cannot finish its closure within budget returns a partial result with an explicit pending state and schedules the remainder at high priority. It does not block past the budget, and it does not skip analysis and report a clean result. Deferral is the only accepted way to meet an SLO.

A feature that is correct but repeatedly violates inner-loop SLOs is not considered complete.

---

# 44. Concurrency Model

Use:

```text
goroutines
context.Context
bounded worker pools
channels where ownership transfer is clear
small mutex-protected in-memory state
SQLite transactions for persistent atomicity
```

Do not use channels as a global event bus.

---

# 45. Goroutine Ownership

Every goroutine must have:

- a clear owner,
- cancellation,
- shutdown behavior.

Avoid fire-and-forget goroutines.

Preferred lifecycle:

```text
Manager.Start(ctx)
→ worker goroutines
→ root cancellation
→ WaitGroup
→ Stop
```

---

# 46. Context Cancellation

Potentially long operations must accept `context.Context`.

Examples:

- inventory,
- parsing,
- resolver queries,
- index jobs,
- Git commands,
- validation commands,
- MCP calls.

Cancellation propagates to child processes where possible.

---

# 47. Scheduler

Implement a small internal priority scheduler.

Do not add Redis or a generic job framework.

Priority classes:

```text
P0 current before_change target
P1 relevant dependency/test closure
P2 active task ProjectUnit
P3 active workspace
P4 cold repository remainder
```

Recommended internals:

```text
container/heap
bounded worker pools
deduplication map
persistent job state
priority aging
```

---

# 48. Scheduler Worker Classes

Separate resource classes when useful:

```text
parse workers
semantic workers
coverage workers
validation workers
```

Validation workers require independent concurrency control because tests/builds can be expensive.

Already-running parse work should generally not be preempted.

---

# 49. Backpressure

Queues must be bounded.

If capacity is reached:

- coalesce duplicate index jobs,
- persist work state,
- delay/reject low-priority work,
- do not allocate unbounded memory queues.

---

# 50. Semantic Resolver Architecture

Semantic resolvers run out-of-process.

```text
Mindrail Go
    │
Resolver Manager
    │
    ├── Pyright process
    └── TypeScript semantic process
```

Future adapters:

```text
Roslyn
gopls
JDT LS
PHP language server/static analyzer
```

Mindrail does not embed Pyright or TypeScript compiler runtime inside Go.

---

# 51. Resolver Interface

Conceptual normalized boundary:

```go
type Resolver interface {
    Start(ctx context.Context, unit ProjectUnit) error
    Health(ctx context.Context) Health
    ResolveSymbol(ctx context.Context, loc Location) (SymbolRef, error)
    References(ctx context.Context, symbol SymbolRef) ([]Reference, error)
    Implementations(ctx context.Context, symbol SymbolRef) ([]SymbolRef, error)
    TypeAt(ctx context.Context, loc Location) (TypeInfo, error)
    Stop(ctx context.Context) error
}
```

Actual implementation may split lifecycle and query interfaces.

---

# 52. Python Resolver

Python FULL semantic mode uses:

```text
Pyright
```

Resolver Manager owns:

- executable discovery,
- project configuration discovery,
- interpreter/environment context,
- health probes,
- bounded restart,
- diagnostics normalization.

Mindrail must not alter the repository's dependency manifest merely to install Pyright.

---

# 53. TypeScript / JavaScript Resolver

FULL semantic mode uses a project-aware TypeScript semantic service.

Project-local TypeScript is preferred when compatible because repository semantics may depend on its compiler version.

Mindrail must support multiple:

```text
tsconfig.json
jsconfig.json
workspace/package roots
```

through ProjectUnit discovery.

---

# 54. Managed Resolver Cache

If managed fallback is enabled, tools live outside the repository.

Use Go:

```go
os.UserCacheDir()
```

Logical location:

```text
<user-cache>/mindrail/resolvers/
```

Do not mutate:

```text
repo/node_modules
repo/.venv
project lock files
```

for managed fallback installation.

---

# 55. Resolver Version Policy

Do not run `latest` on every start.

Each Mindrail release defines:

```text
tested resolver compatibility range
managed default resolver version
```

Resolver upgrades are tested through semantic fixtures.

Process startup alone does not prove compatibility.

---

# 56. Resolver Process Supervision

Use standard Go process tools:

```text
os/exec
context
bounded restart policy
sanitized stdout/stderr capture
```

No general-purpose supervisor framework.

## Pool and eviction

The Resolver Manager owns a pool keyed by workspace/snapshot identity, ProjectUnit, config hash and resolver version, as required by Technical Specification 1.0.

Because that key does not merge across worktrees, process count grows roughly with `workspaces × project units`. A fixed small ceiling therefore causes evict/restart thrash on monorepos and multi-worktree setups, and resolver cold start costs seconds.

Implementation rules:

- `max_processes` defaults to `auto`, derived from discovered ProjectUnit count and host CPU/memory caps; explicit config always overrides.
- Eviction candidates exclude the active target unit's resolver, any instance serving an in-flight request, and any instance younger than `min_residency_seconds`.
- `idle_ttl` applies only to instances outside those protections.
- When no evictable candidate exists, return `RESOLVER_RESOURCE_LIMIT` and let policy degrade to STRUCTURAL. Never evict a hot resolver to serve a new request.
- Track `start count`, `eviction count`, `evict→restart within min_residency`, `cold start duration` and `RESOLVER_RESOURCE_LIMIT count`; rising thrash is reported as a performance defect.

Tests must cover pool key separation across worktrees, eviction protection for the active unit, and thrash counting under an over-subscribed budget.

---

# 57. Resolver Diagnostics

`mindrail doctor` must expose enough information to answer:

> Why is this ProjectUnit not FULL?

Minimum fields:

```text
ProjectUnit
config source
resolver source
resolver version
selected executable/package
health
generation
normalized diagnostics
effective capability
why FULL is unavailable
suggested next actions
```

Resolver diagnostics are a product feature, not debug-only logs.

---

# 58. Coverage Provider Boundary

Coverage integration is language-neutral at core level.

Conceptual interface:

```go
type CoverageProvider interface {
    Health(ctx context.Context) Health
    Import(ctx context.Context, artifact Artifact) (CoverageRun, error)
    Tests(ctx context.Context, run CoverageRun) ([]TestID, error)
    Ranges(ctx context.Context, run CoverageRun, test TestID) ([]CoveredRange, error)
}
```

Range-to-symbol mapping belongs to Mindrail's normalized index layer.

---

# 59. Coverage Capability

Providers declare actual capability:

```text
NONE
STATIC
RUNTIME_FILE
RUNTIME_RANGE
RUNTIME_SYMBOL
```

Semantic FULL support does not imply runtime test-impact FULL support.

Initial targets:

```text
Python
TypeScript / JavaScript
```

Other languages remain capability-gated until equivalent providers exist.

---

# 60. Coverage Format Isolation

Provider-specific formats are normalized immediately.

Do not spread knowledge of formats such as:

```text
coverage.py
lcov
Cobertura
JaCoCo
```

through core packages.

The provider adapter owns external format interpretation.

---


# 61. Reconcile Implementation Rule

`mindrail_reconcile` is not implemented as an exceptional recovery utility.

It is the canonical code path that derives actual change state from:

```text
Git/worktree diff
content hashes
syntax delta
semantic delta when available
```

`mindrail_before_change` supplies better baseline/lease context, but `after_change`, completion and verify MUST be able to call the same reconcile engine when the agent skipped the protocol step.

There must be one attribution engine, not separate "registered" and "unregistered" implementations.

---

# 62. Impact Graph Storage

Do not use Neo4j.

Graph state is stored in SQLite tables such as:

```text
symbols
edges
impact_runs
impact_findings
impact_groups
```

Traversal is implemented with bounded Go graph logic and targeted DB reads.

---

# 63. Impact Traversal

Prefer loading the relevant graph neighborhood into memory rather than the whole repository graph.

Use:

```text
adjacency maps
priority queue when needed
visited sets
depth budget
fan-out budget
architecture boundaries
```

---

# 64. Explainability Storage

Each impact finding stores structured provenance:

```text
origin
path
edge types
confidence
resolver generation
analysis generation
policy reasons
```

Do not reconstruct important causality later from free-text logs.

---

# 65. Validation Runner

Run approved validation commands as child processes via `os/exec`.

Preferred configuration:

```toml
command = ["pytest", "-q", "tests/auth"]
```

Default shell mode:

```text
disabled
```

Agent-provided free-form shell commands never become trusted evidence.

---

# 66. Validation Timeout

Every validation profile has a timeout.

Use:

```go
context.WithTimeout
```

Timeout result:

```text
VALIDATION_TIMEOUT
```

A timeout is never a pass.

---

# 67. Process Tree Cleanup

Cross-platform process cleanup must be explicit.

Create OS-specific helpers:

```text
process_unix.go
process_windows.go
```

Tests must verify that timed-out validation/resolver child processes do not remain alive.

---

# 68. Validation Output

Capture:

```text
stdout
stderr
exit code
duration
```

Output must be bounded while the process is running.

Do not buffer unlimited process output in RAM.

Use bounded head/tail/ring buffers where appropriate.

---

# 69. Secret Redaction

Pipeline:

```text
process output
    ↓
known exact-secret masking
    ↓
pattern masking
    ↓
structured sanitization
    ↓
bounded excerpt
    ↓
evidence storage
```

Do not persist an unredacted hidden copy by default.

---

# 70. Logging

Selected:

```text
log/slog
```

No Zap/Logrus dependency for the target architecture.

Log levels:

```text
DEBUG
INFO
WARN
ERROR
```

Useful structured fields:

```text
project_id
workspace_id
session_id
task_id
change_id
project_unit_id
operation_id
component
duration
error_code
```

Do not log source code or environment secrets by default.

---

# 71. Human Output vs Diagnostic Logs

CLI output:

```text
concise and actionable
```

Logs:

```text
structured diagnostics
```

Users should not need DEBUG logs to understand normal resolver/config failures.

---

# 72. Error Model

Stable domain errors carry:

```text
machine code
human message
wrapped cause
metadata
next actions
```

Use standard Go error wrapping:

```text
errors.Is
errors.As
fmt.Errorf("...: %w", err)
```

Never use string matching as primary control flow.

---

# 73. Panic Policy

Panics are reserved for impossible programmer-state violations.

User/project input must produce ordinary errors, never intentional panics.

Boundary recovery may emit fatal diagnostics, but Mindrail must not silently continue after suspected state corruption.

---

# 74. Path Representation

Stable stored paths use repository-relative normalized `/` separators.

OS-native conversion happens at filesystem boundaries.

Absolute machine paths are not part of stable symbol identity.

---

# 75. Filesystem Safety

Before file operations:

- clean paths,
- verify root containment,
- enforce symlink policy,
- reject traversal outside allowed roots.

Use Go standard `path/filepath` and `os` APIs.

---

# 76. Ignore Rules

Inventory combines:

```text
Mindrail explicit ignore config
vendor/generated defaults
Git ignore semantics where useful
```

For exact Git ignore behavior, prefer Git queries instead of reimplementing every ignore edge case.

---

# 77. HTTP Stack

Mindrail does not require an HTTP server.

If future remote transport/provider support needs HTTP, use:

```text
net/http
```

before considering a web framework.

No Gin/Echo/Fiber requirement.

---

# 78. CI Integration

CI uses the same executable:

```bash
mindrail verify --ci
```

No separate Mindrail CI service.

Typical CI flow:

```text
checkout
install Mindrail
validate knowledge
restore safe cache
determine base/head
perform impact analysis
run required validation
emit structured result
```

---

# 79. CI Cache

Safe cache candidates:

```text
content-addressed parse snapshots
managed resolver downloads
portable semantic cache where validated
```

Never cache live coordination state such as:

```text
task leases
agent sessions
active local change ownership
```

---

# 80. Git Hooks

Hooks are thin shims.

Example:

```text
pre-commit
   ↓
mindrail verify --staged
```

Mindrail must not silently overwrite existing hooks.

Existing hook integration must be explicit and diagnosable.

---

# 81. Approval Provider Architecture

Approval remains provider-based.

Implementation priority:

```text
CI_VERIFICATION
LOCAL_INTERACTIVE
MANAGED_IDENTITY abstraction
SIGNED_LOCAL optional
```

Do not require every developer to configure SSH/GPG for ordinary use.

Critical policy can require stronger independent identity according to Technical Specification 1.0.

---

# 82. Cryptography

Prefer Go standard cryptographic packages.

Do not invent custom signature formats or cryptographic protocols.

If signed-local approval is implemented, use an established SSH/GPG/hardware-backed mechanism or vetted compatible library/tooling.

---

# 83. `mindrail doctor`

Doctor is implemented as composable component checks.

Conceptual interface:

```go
type Check interface {
    Name() string
    Run(context.Context) Result
}
```

Checks include:

```text
GitCheck
SQLiteCheck
KnowledgeCheck
ParserCheck
ResolverCheck
CoverageCheck
HookCheck
CIConfigCheck
IndexCheck
```

---

# 84. Doctor Output Modes

Examples:

```bash
mindrail doctor
mindrail doctor --verbose
mindrail doctor --resolver
mindrail doctor --knowledge
mindrail doctor --index
mindrail doctor --json
```

Standard health states:

```text
OK
DEGRADED
ERROR
UNAVAILABLE
NOT_APPLICABLE
```

Optional missing capabilities must not all be shown as fatal errors.

---

# 85. Application Wiring

`cmd/mindrail/main.go` must stay minimal.

Responsibilities:

```text
root process context
application bootstrap
CLI execution
exit code
```

No domain logic in `main.go`.

---

# 86. Top-Level Application Lifecycle

A top-level application may own:

```text
ConfigLoader
Database
GitAdapter
KnowledgeStore
IndexManager
ResolverManager
ValidationRunner
MCPServerFactory
Doctor
```

Lifecycle:

```text
Start
Run
Shutdown
```

Expensive services are initialized lazily.

---

# 87. Startup Sequence

Recommended:

```text
1. resolve repository
2. load config
3. resolve runtime paths
4. open SQLite
5. migrate DB
6. validate knowledge
7. register workspace
8. load index state
9. lazily initialize managers
10. execute requested command
```

`mindrail status` must not start every language server.

---

# 88. Graceful Shutdown

Handle:

```text
SIGINT
SIGTERM
```

Sequence:

1. cancel root context,
2. stop accepting new work,
3. cancel child processes,
4. persist resumable job state,
5. close SQLite,
6. exit within bounded time.

---

# 89. Testing Stack

Primary test framework:

```text
Go standard `testing`
```

Primary command:

```bash
go test ./...
```

No mandatory third-party assertion framework.

---

# 90. Test Categories

Mindrail test categories:

```text
unit
domain
adapter fixture
integration
concurrency
process supervision
CLI contract
MCP contract
Git fixture
end-to-end
```

Test count is not a quality metric.

Behavior/rule coverage matters more than thousands of repetitive tests.

---

# 91. Language Adapter Fixtures

Use real source fixtures:

```text
testdata/languages/python/
testdata/languages/typescript/
testdata/languages/javascript/
```

Examples:

```text
simple_call
inheritance
imports
dynamic_getattr
pytest
interfaces
overloads
monorepo_reference
```

Expected normalized symbol/edge output is compared against fixtures.

---

# 92. Git Integration Tests

Create temporary real Git repositories.

Scenarios:

```text
worktrees
dirty workspace
staged-only change
merge-base
rebase divergence
rename
deletion
hooks
```

Do not mock Git everywhere.

---

# 93. SQLite Concurrency Tests

Run concurrent goroutines/processes against one DB.

Validate:

```text
single lease winner
bounded busy retry
operation idempotency
revision conflict
no duplicate evidence
WAL reopen/recovery
single symbol_uid per concurrent first observation
cold-index chunking does not starve interactive writes
```

---

# 94. Process Supervision Tests

Use tiny fixture processes that:

- start successfully,
- crash immediately,
- hang,
- spawn child processes,
- produce large output,
- ignore graceful stop.

Verify cleanup and restart policy.

---

# 95. MCP Contract Tests

Where practical, run the real stdio MCP server.

Validate:

```text
tool discovery
input schemas
output schemas
structured errors
cancellation
duplicate requests
```

---

# 96. Race Detector

CI should regularly run:

```bash
go test -race ./...
```

Concurrency is central to Mindrail.

---

# 97. Fuzzing

Use native Go fuzzing selectively.

Good targets:

```text
knowledge JSON loader
TOML config loader
symbol key parser
Git path parsing
resolver message normalization
```

Fuzzing should improve robustness, not inflate test count.

---

# 98. Benchmarks and Profiling

Use standard Go tooling:

```text
go test -bench
pprof
go tool trace
```

Initial benchmark targets:

```text
content hashing
Tree-sitter parse
symbol normalization
graph traversal
SQLite batch insert
FTS search
cold inventory
```

Do not optimize solely from synthetic microbenchmarks.

---

# 99. Dependency Management

Use Go modules:

```text
go.mod
go.sum
```

Exact module versions are pinned in `go.mod`.

This document intentionally identifies technology choices rather than every patch version.

Rule:

```text
tech-stack.md → technology choice
go.mod        → exact dependency version
ADR           → architectural reason
```

---

# 100. Dependency Update Policy

Critical dependencies include:

```text
MCP SDK
Tree-sitter binding
grammar modules
SQLite driver
JSON Schema validator
```

Updates require targeted integration tests.

Do not blindly run runtime `latest` dependency resolution.

---

# 101. Linting and Formatting

Mandatory baseline:

```text
gofmt
go vet
```

Recommended:

```text
staticcheck
```

Optional:

```text
goimports
```

Avoid giant linter bundles with dozens of subjective rules.

---

# 102. Supply-Chain Checks

Release/security CI should include:

```text
govulncheck ./...
```

As the project matures, add:

```text
dependency review
license inventory
release provenance
checksum/signature publication
```

---

# 103. Build

Developer build:

```bash
go build ./cmd/mindrail
```

Release builds inject version metadata through `-ldflags`.

Recommended fields:

```text
version
commit
buildDate
dirty
```

---

# 104. Version Command

Expose:

```bash
mindrail version
```

Human/JSON output should include:

```text
Mindrail version
Git commit
Go version
knowledge writer schema
readable knowledge schemas
MCP compatibility
build platform
```

Where useful, Tree-sitter/parser build metadata may also be reported.

---

# 105. Native Build Toolchain

Because the chosen Tree-sitter Go binding uses CGO, contributors/release runners need a compatible C toolchain.

This requirement must be explicit in developer setup docs.

Mindrail should not present itself as a universally `CGO_ENABLED=0` application.

---

# 106. Release Matrix

Initial intended targets:

```text
linux/amd64
linux/arm64
darwin/amd64
darwin/arm64
windows/amd64
```

Only advertise a target after CI build and smoke tests exist.

---

# 107. Release Artifacts

Example names:

```text
mindrail_1.0.0_linux_amd64.tar.gz
mindrail_1.0.0_linux_arm64.tar.gz
mindrail_1.0.0_darwin_arm64.tar.gz
mindrail_1.0.0_windows_amd64.zip
```

Publish checksums.

---

# 108. GoReleaser

GoReleaser may be used for packaging/checksum/release orchestration if it fits the platform-native CGO strategy cleanly.

It is not an architectural dependency.

If it complicates Tree-sitter builds, use explicit CI workflows instead.

---

# 109. CI Provider

GitHub Actions can be the initial CI system, but Mindrail build/test behavior must not be fundamentally tied to GitHub Actions.

Important logic remains ordinary commands and scripts.

---

# 110. Makefile

Use a small Makefile for contributor convenience.

Suggested targets:

```text
make build
make test
make test-race
make lint
make integration
make fixtures
make generate
```

Do not hide essential build behavior exclusively inside Make.

---

# 111. Embedded Assets

Use Go `embed` for:

```text
SQLite migrations
JSON schemas
Tree-sitter query files
default config templates
default policy templates
protocol templates
```

This minimizes runtime installation complexity.

---

# 112. Runtime Paths

Project-shared runtime state:

```text
<GIT_COMMON_DIR>/mindrail/mindrail.db
<GIT_COMMON_DIR>/mindrail/cache/
```

Global managed resolver cache:

```text
<os.UserCacheDir()>/mindrail/resolvers/
```

All runtime directories, config paths, MCP tool names and error codes use the `mindrail` product name consistently. No alternative product prefix is defined.

---

# 113. Temporary Files

Use:

```text
os.CreateTemp
os.MkdirTemp
```

Temporary files:

- are cleaned after use,
- are not durable state,
- should normally live outside repository source,
- receive restrictive permissions where applicable.

---

# 114. Search

Search uses:

```text
SQLite FTS5
exact lookup
path lookup
symbol lookup
```

No vector DB.

No embeddings.

Do not index entire source files into FTS without a demonstrated requirement.

---

# 115. Telemetry

Remote telemetry is not required.

Local operational metrics may include:

```text
SQLite busy retries
revision conflicts
resolver restart count
index queue depth
cache hit rate
impact prediction metrics
```

Local telemetry must have bounded retention.

Remote collection, if ever added, is explicit opt-in.

---

# 116. Network Policy

Normal core operations should not perform arbitrary network calls.

Explicit network-capable features may include:

```text
managed resolver download
optional update check
external approval provider
future remote MCP
```

These must be visible and configurable.

---

# 117. Offline Operation

Core Mindrail should work offline when:

- required resolvers are already available,
- project dependencies are present,
- external approval is not required by policy.

`mindrail doctor` should explain which capabilities need network access.

---

# 118. Security Boundaries

Treat these as distinct trust boundaries:

```text
AI agent input
repository source
repository config
validation process
semantic resolver
Git
human approval provider
CI identity
```

Local does not automatically mean trusted.

---

# 119. Agent Input Safety

MCP inputs are strongly validated.

Agent-controlled strings must never become uncontrolled:

```text
shell commands
filesystem paths outside allowed roots
SQL
executable paths
```

without a dedicated validation layer.

---

# 120. Validation Command Trust

Trusted validation commands come from project policy/configuration.

The agent may request approved profiles.

The agent cannot provide an arbitrary command and mark its result as trusted evidence.

---

# 121. Large File Policy

Config supports a maximum parser file size.

Very large files may degrade to:

```text
FILE-level analysis
```

rather than exhausting memory.

Binary files are detected before parser selection.

---

# 122. Generated Code

Generated/vendor paths are ignored by default.

If explicitly included, generated provenance should be retained when practical.

Source-of-truth generated files should be configured deliberately.

---

# 123. Memory Usage

Do not load the entire repository graph into memory.

Use:

```text
bounded graph neighborhoods
pagination
content-addressed cache
incremental indexes
```

Large-repository testing must include resident-memory measurements.

---

# 124. Cache Garbage Collection

Content-addressed snapshots need conservative GC.

A maintenance command such as:

```bash
mindrail gc
```

may remove:

```text
unreferenced parse snapshots
expired diagnostics
bounded telemetry
orphaned cache artifacts
```

Never delete:

- active change baselines,
- current workspace snapshots,
- evidence-linked snapshots,
- unfinished task state.

---

# 125. Compatibility Surfaces

Mindrail has several independently important compatibility surfaces:

```text
CLI
MCP tools
knowledge schema
SQLite schema
config schema
resolver adapters
coverage providers
```

Do not use one global version assumption for all of them.

---

# 126. MCP Compatibility

MCP tool schemas are external contracts.

Breaking changes require an explicit compatibility strategy.

Do not casually rename fields or tools after integrations depend on them.

---

# 127. Runtime DB Compatibility

SQLite state is local operational state.

Migrations are forward-only by default.

Downgrades are not guaranteed unless explicitly implemented.

---

# 128. Knowledge Compatibility

Repository-owned knowledge has stronger compatibility guarantees.

The loader implements the Technical Specification 1.0 mixed-version reader window and fail-closed behavior for unsupported newer blocking schemas.

---

# 129. Developer Requirements

Minimum contributor environment:

```text
Go 1.27.x
Git
C compiler/toolchain for Tree-sitter CGO
```

Semantic integration tests additionally require the relevant ecosystems, such as:

```text
Pyright / Python fixtures
Node.js / TypeScript fixtures
```

---

# 130. Developer Commands

Baseline:

```bash
go mod download
go test ./...
go test -race ./...
go vet ./...
go build ./cmd/mindrail
```

Integration:

```bash
make integration
```

---

# 131. Test Isolation

Tests must not write to a developer's real:

```text
.git
Mindrail DB
Mindrail user cache
resolver cache
```

except explicitly controlled end-to-end tests.

Use temporary directories and injectable runtime/cache roots.

---

# 132. Fixture Repository Strategy

Maintain tiny real fixture repositories rather than mocking every integration.

Examples:

```text
fixture-python-basic
fixture-python-monorepo
fixture-ts-project-reference
fixture-dynamic-dispatch
fixture-git-worktree
fixture-conflicting-agents
```

---

# 133. Core E2E Workflow

Two mandatory end-to-end scenarios. Both must pass; the second is not optional, because §61 makes reconcile the canonical correctness path and a suite that only exercises the protocol-compliant path would never catch a registered-only attribution engine.

Protocol-compliant path:

```text
init
→ bootstrap
→ claim
→ before_change
→ edit
→ after_change
→ validate
→ complete
```

Protocol-skipped path (agent never called `before_change`):

```text
init
→ bootstrap
→ claim
→ edit
→ reconcile
→ validate
→ complete
```

Both scenarios must reach the same change attribution and the same completion decision for the same edit. A divergence between them is a defect in the attribution engine, not an expected difference between two modes.

Verify the same core behavior through both CLI and MCP paths.

---

# 134. Multi-Agent E2E

Run two independent Mindrail processes against:

```text
same Git common-dir
different worktrees
same SQLite DB
```

Verify:

```text
task lease conflict
symbol lease conflict
different-symbol concurrency
stale lease recovery
revision conflict
evidence isolation
```

---

# 135. Crash Recovery E2E

Terminate Mindrail during:

```text
cold index
semantic query
validation
SQLite retry
active task
```

Restart and verify deterministic recovery/resume behavior.

---

# 136. Release Quality Gate

A release should not ship unless:

```text
unit tests pass
integration tests pass
race tests pass
knowledge schema tests pass
MCP contract tests pass
Git worktree tests pass
SQLite concurrency tests pass
supported binaries build
binary smoke tests pass
```

---

# 137. Concrete Dependency Matrix

| Area | Choice |
|---|---|
| Core language | Go 1.27.x |
| CLI | `github.com/spf13/cobra` |
| MCP | `github.com/modelcontextprotocol/go-sdk/mcp` |
| JSON | stdlib `encoding/json` |
| TOML | `github.com/pelletier/go-toml/v2` |
| SQLite | `database/sql`; driver selected by benchmark (`modernc.org/sqlite` vs `mattn/go-sqlite3`) |
| SQL API | stdlib `database/sql` |
| Tree-sitter | `github.com/tree-sitter/go-tree-sitter` |
| File watching | `github.com/fsnotify/fsnotify` |
| Logging | stdlib `log/slog` |
| JSON Schema | `github.com/santhosh-tekuri/jsonschema/v6` |
| Git | system `git` via `os/exec` |
| Scheduler | internal + stdlib `container/heap` |
| Process management | stdlib `os/exec`, `context` + OS helpers |
| Hashing | stdlib `crypto/sha256` |
| HTTP | stdlib `net/http` if needed |
| Testing | stdlib `testing` |
| Profiling | stdlib Go tooling / `pprof` |

---

# 138. Explicit Non-Choices

Mindrail intentionally does not use:

```text
Gin
Echo
Fiber
GORM
Ent
Redis
PostgreSQL
Kafka
RabbitMQ
Temporal
Neo4j
Elasticsearch
Zap
Logrus
Viper
Wire
Fx
DI container
distributed queue
workflow framework
```

These tools are not inherently bad; they are unnecessary for Mindrail.

---

# 139. Why Not Viper

Mindrail config is controlled and typed.

Use:

```text
go-toml/v2
+
typed structs
+
explicit precedence
```

instead of a global configuration framework.

---

# 140. Why Not GORM

Mindrail needs explicit:

```text
SQLite transactions
CAS revision updates
WAL behavior
content-addressed inserts
idempotency
```

ORM abstraction would hide important behavior rather than simplify it.

---

# 141. Why Not `go-git`

Git itself is part of Mindrail's coordination model.

System Git already defines the exact behavior of:

```text
index
worktrees
merge-base
rename detection
porcelain state
```

Using the Git executable is the more faithful implementation.

---

# 142. Why Not Rust Core

Rust is valid but the target architecture bottlenecks are expected to be mostly:

```text
filesystem IO
SQLite
Tree-sitter native parser
resolver subprocesses
validation subprocesses
```

not Go application CPU throughput.

Its additional complexity is not justified for the initial product line.

---

# 143. Why Not Python Core

Python would accelerate a prototype but creates additional product-distribution concerns:

- interpreter management,
- packaging,
- environment collision,
- concurrency/process complexity,
- binary distribution.

Mindrail should not require Python just to run the core executable.

---

# 144. Dependency Review Rule

Before adding a new runtime dependency, answer:

```text
What problem does it solve?
Why is stdlib insufficient?
Is it maintained?
What is the security surface?
What does it add to binary/runtime complexity?
Does it affect cross-platform builds?
Can it stay behind an adapter?
```

---

# 145. Recommended ADRs

Create these ADRs during implementation:

```text
ADR-0001 Use Go for Mindrail Core
ADR-0002 Use SQLite for Runtime State; select driver by benchmark
ADR-0003 Use Tree-sitter for Syntax Analysis
ADR-0004 Run Semantic Resolvers Out-of-Process
ADR-0005 Use System Git as Canonical Git Adapter
ADR-0006 Use Official MCP Go SDK
ADR-0007 Use Explicit SQL Instead of ORM
ADR-0008 Use Version-Controlled Knowledge Records
ADR-0009 Use Platform-Native Release Builds for Tree-sitter CGO
ADR-0010 No Mandatory Remote Telemetry
ADR-0011 Durable Symbol Identity, Allocation and Rename Migration
ADR-0012 Resolver Pooling, Resource Budget and Eviction Policy
ADR-0013 Reconcile as the Canonical Change-Discovery Path
```

---

# 146. Go Upgrade Policy

A Go baseline upgrade requires:

1. update CI toolchain,
2. run all unit/integration tests,
3. run race tests,
4. build supported release targets,
5. run parser/resolver fixtures,
6. run MCP contract tests,
7. update `go.mod`,
8. update this document only if the baseline changes.

---

# 147. MCP SDK Upgrade Policy

MCP SDK upgrades require:

- protocol compatibility review,
- stdio integration tests,
- tool schema tests,
- supported client smoke tests where practical.

Internal domain types should not directly depend on MCP SDK types.

---

# 148. Tree-sitter Upgrade Policy

Tree-sitter binding or grammar upgrades require:

```text
parser fixtures
changed-range tests
resource lifecycle tests
symbol extraction tests
release build matrix
```

Grammar versions can alter AST structure and must be treated as meaningful parser changes.

---

# 149. SQLite Driver Upgrade Policy

SQLite driver upgrades require:

```text
migration tests
WAL concurrency tests
busy retry tests
crash/reopen tests
FTS tests
platform build tests
```

---

# 150. Resolver Upgrade Policy

Pyright/TypeScript upgrades require semantic fixture validation:

```text
cross-file references
inheritance
interfaces/overloads
monorepo project references
dynamic unresolved behavior
broken-config diagnostics
```

Mindrail must not claim semantic compatibility merely because the process starts.

---

# 151. Code Quality Rules

Core rules:

- no unnecessary global mutable state,
- every goroutine has lifecycle ownership,
- external processes are timeout-bound,
- errors carry context,
- SQL transactions are short,
- I/O is bounded where practical,
- domain state transitions are explicit,
- no silent degradation of safety-critical capability.

---

# 152. Mutex Rules

Mutexes protect small in-memory structures.

Do not hold a mutex while:

```text
calling a resolver
executing Git
running validation
reading large files
waiting on SQLite
```

---

# 153. Canonical Hashing

Snapshot hashes must be deterministic across platforms.

Rules:

- normalize repository paths,
- define ordering,
- never depend on Go map order,
- define separators and encoding,
- hash structured data canonically.

For a file set:

```text
sort by normalized repository path
hash path + separator + content hash
```

---

# 154. Cold Index Worker Model

Cold index is resumable and target-first according to Technical Specification 1.0.

Implementation uses:

```text
persistent job rows
bounded workers
priority heap
content hash deduplication
ProjectUnit-level semantic jobs
```

`mindrail init` does not wait for full repository semantic indexing before becoming usable.

---

# 155. Background Work Clarification

Mindrail may run local worker processes as part of the local tool lifecycle.

This is not a remote/cloud asynchronous service.

The user can explicitly run:

```bash
mindrail index --foreground
```

for visible foreground progress.

Interrupted work is resumable through persisted job state.

---

# 156. Validation Budget Implementation

Validation profiles may expose coarse cost metadata:

```text
FAST
NORMAL
EXPENSIVE
VERY_EXPENSIVE
```

Mindrail does not need exact duration prediction.

Planner chooses the smallest sufficient proof permitted by policy.

If a critical uncertainty requires a profile beyond the interactive budget, the operation reports an explicit blocked state rather than silently weakening proof.

---

# 157. Candidate Invariant Implementation

Candidate detection stays deterministic and bounded.

Persist candidate state:

```text
NEW
SHOWN
PROMOTED
DISMISSED
SUPPRESSED
STALE
```

Use normalized fingerprints to prevent repeated spam.

Do not use an LLM to automatically convert every assertion into an active invariant.

---

# 158. Human Approval Implementation Priority

Implementation order:

```text
1. CI_VERIFICATION evidence
2. LOCAL_INTERACTIVE evidence
3. provider interface for managed identity
4. SIGNED_LOCAL if demanded by real policy needs
```

This keeps early user onboarding simple without weakening the Technical Specification 1.0 trust model.

---

# 159. First Implementation Milestone

## Relationship to the specification milestones

Technical Specification 1.0 §128 defines ten capability milestones. The seven milestones below are **build order for this codebase**, not a competing plan. Where the two differ, §128 wins on scope and this document wins on sequencing.

| This document | Specification §128 |
|---|---|
| First — usable binary | Milestone 1 (runtime core half) |
| Second — coordination kernel | Milestone 1 (knowledge/session/lease half) |
| Third — Tree-sitter syntax index | Milestone 2 |
| Fourth — semantic layer | Milestone 3 |
| Fifth — change engine + impact + gates | Milestones 4 and 5 |
| Sixth — MCP, Git and CI surfaces | Milestones 8 and 9 |
| Seventh — coverage, candidates, calibration | Milestones 6, 7 and 10 |

Delivery scope for the first release is narrower still and is fixed by `mindrail-0.1-kernel-scope.md`.

The first usable binary should support:

```bash
mindrail init
mindrail status
mindrail doctor
```

and prove:

```text
Git repository discovery
runtime path discovery
SQLite DB creation
embedded migrations
knowledge loading
workspace registration
structured CLI output
```

Do not begin implementation with Pyright or impact analysis.

---

# 160. Second Milestone

Implement the persistent coordination kernel:

```text
session
task
claim
lease
checkpoint
change lifecycle skeleton
```

This should work before AST complexity is introduced.

---

# 161. Third Milestone

Implement Tree-sitter syntax indexing:

```text
Python
JavaScript
TypeScript
```

Prove:

```text
content-addressed cache
normalized symbols
incremental changed-file reindex
language fixtures
```

---

# 162. Fourth Milestone

Implement semantic layer:

```text
ProjectUnit discovery
Resolver Manager
Pyright adapter
TypeScript adapter
FULL vs STRUCTURAL capability
resolver diagnostics
```

---

# 163. Fifth Milestone

Implement:

```text
reconcile (canonical attribution engine)
before_change
after_change
impact graph
fan-out groups
validation planner
evidence gate
completion gate
```

`reconcile` lands **with** the change engine, not after it. §61 requires one attribution engine rather than separate "registered" and "unregistered" implementations; shipping `before_change`/`after_change` first and adding reconcile in a later milestone guarantees the second engine this document forbids.

Only after core behavior is stable should MCP become the primary agent interface.

---

# 164. Sixth Milestone

Add:

```text
MCP stdio server
13 tool contracts
Git hook verification
CI verification
```

The reconcile engine already exists from the fifth milestone; this milestone only exposes it through the MCP and verify surfaces.

CLI and MCP must exercise the same application services.

---

# 165. Seventh Milestone

Add runtime test-impact and advanced trust features:

```text
coverage provider(s)
candidate invariants
approval providers
calibration/feedback
```

These must not destabilize the core coordination kernel.

---

# 166. Technology Baseline Summary

```text
CORE
Go 1.27.x

CLI
Cobra

MCP
Official MCP Go SDK
stdio first

STATE
SQLite via `database/sql`; concrete driver chosen by ADR benchmark
Explicit database/sql queries
Embedded SQL migrations

SYNTAX
Official Tree-sitter Go binding
Python + JavaScript + TypeScript grammars
CGO build requirement

SEMANTICS
Out-of-process resolver adapters
Pyright
TypeScript semantic service

FILESYSTEM
Go stdlib + fsnotify hints

GIT
System Git CLI via os/exec

CONFIG
TOML via go-toml/v2
Typed Go structs

KNOWLEDGE
JSON + embedded JSON Schema
santhosh-tekuri/jsonschema/v6

LOGGING
log/slog

CONCURRENCY
context + goroutines + bounded workers
internal priority scheduler

VALIDATION
os/exec argv-only by default
timeouts
bounded output
redaction

TESTING
stdlib testing
real repository fixtures
race detector
integration/E2E

BUILD
platform-native CGO CI matrix

DISTRIBUTION
one `mindrail` executable per supported platform
```

---

# 167. Final Engineering Rule

Mindrail's technology stack should remain intentionally boring.

The product's value comes from:

```text
engineering memory
coordination
semantic impact
invariant protection
evidence
```

—not infrastructure novelty.

When choosing between:

```text
a small explicit Go implementation
```

and:

```text
another framework/service/abstraction
```

the default is the small explicit Go implementation unless concrete evidence proves otherwise.

---

# 168. Relationship to Technical Specification 1.0

This document is subordinate to the Mindrail Technical Specification 1.0.

It MUST NOT alter:

- task semantics,
- lease semantics,
- invariant semantics,
- impact semantics,
- evidence semantics,
- knowledge compatibility,
- completion gates,
- agent independence.

Its sole role is to define a clear and maintainable Go implementation strategy for those requirements.

---

# 169. Current External Technology Notes

As of 2026-09-01:

- Go 1.27 is the current Go major release.
- MCP provides an official Tier-1 Go SDK.
- The official MCP Go SDK supports stdio-based server usage suitable for local agent integration.
- Tree-sitter provides official Go bindings; the binding uses CGO.
- SQLite driver choice is benchmark-gated; exact pinned driver/version is recorded in `go.mod` and ADR-0002.
- `github.com/pelletier/go-toml/v2` is the selected typed TOML implementation.
- `github.com/santhosh-tekuri/jsonschema/v6` is the selected JSON Schema validator.

Exact dependency versions remain pinned in `go.mod`.

---

# 170. One-Sentence Stack Definition

> **Mindrail is a local-first Go 1.27 application distributed as a platform-native executable, using SQLite for runtime state, Tree-sitter for syntax intelligence, supervised external semantic resolvers for language-aware code intelligence, the official MCP Go SDK for agent interoperability, system Git for repository truth, and deterministic Go services for scheduling, impact analysis, validation and evidence enforcement.**
