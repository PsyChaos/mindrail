# MR-014 — Design

- **Frozen at:** commit `273090a`, before any implementation commit.
- **Refines and is refined by:** [mr-014-requirements.md](mr-014-requirements.md)
  (D-185…D-194 record every place this design left a gap the implementation
  would otherwise fill silently).
- **Specification anchors:** spec-1.0 §36–37 (aggregation boundaries,
  `detail_level`, drill-down pagination), §55 area (decision/invariant
  candidates), §8 + §95 step 6 (record layout, filename==id); AC-20
  (agent independence); kernel-scope §3 (STRUCTURAL) and §4 (deferred
  breadth); tech-stack §7 (layout).

## 1. What the milestone is, in one paragraph

The agent-facing surface of everything built so far: after MR-014, any
MCP client lists six tools with identical schemas, reads status and
context through the CLI's own services, searches records and symbols
within loud bounds, records decisions and active invariants the loader
reads back — and hears `NOT_IMPLEMENTED_IN_THIS_VERSION` with a next
action for everything 0.1 defers, never silence.

## 2. What it is not

| Temptation | Ruling |
|---|---|
| Reimplemented status/search logic | bind, never fork (D-186) |
| Silent clamping/truncation | refuse over-limit (D-188) |
| `full`/drill-down/pagination | refused parameters (D-188) |
| `candidate` invariants | refused mode (D-188) |
| Ranking/relevance engine | source order + exact preference (D-190) |
| CLI `mcp` command | deferred wiring (D-191) |
| Coordination/change tools | MR-015's milestone |

## 3. Package plan

| Package | Owns | Deliberately not there |
|---|---|---|
| `internal/mcp` (new) | `Server` (6 tools, typed handlers), record persistence (allocate + write canonical JSON), search binding, deferred refusals | transport serving, CLI |
| `internal/app` (+1 code) | `NOT_IMPLEMENTED_IN_THIS_VERSION` (usage class) | — |
| `internal/knowledge/...` | untouched (loader reads back; record validates) | writes live in mcp (single writer, D-187) |
| everything else | untouched (consumed as services) | — |

go.mod gains the SDK (D-185).

## 4. Entity model

**Tool I/O.** Typed structs per tool; SDK derives JSON Schema. Deferred
fields are TYPED OPTIONAL (pointer/omitted): absence = default, presence
of an unsupported value = refusal. The present-vs-absent distinction is
the mechanism (D-188).

- `bootstrap`: `{}` → `{runtime paths, readiness, schema versions}`.
- `status`: `{}` → report JSON (the `status.Report` struct itself).
- `search`: `{query, limit?}` → `[{source knowledge|symbol, id, snippet}]`.
- `context`: `{detail_level?, target_type?, target_id?, cursor?, page_size?}` → summary/focused JSON.
- `decide`: `{title, decision, context?, rationale?...}` → `{id, path}`.
- `invariant`: `{mode, statement, ...}` → `{id, path}` or refusal.

**Refusal shape.** `{code: NOT_IMPLEMENTED_IN_THIS_VERSION, message,
next_action[]}` — same envelope vocabulary as findings (code + remedy).

**Record files.** Canonical `record.Decision`/`Invariant` JSON under
`knowledge/decisions|invariants/DEC-*.json|INV-*.json`. Allocation scans
sibling ids (max+1, `%04d` floor); write is create-exclusive (fail if the
allocated name exists — best-effort race guard, D-187).

## 5. Runtime schema

None. Knowledge files are repo content, not runtime schema.

## 6. Tool flows

**bootstrap/status:** `bootstrap.New(ModeReadOnly)` → `Run` →
`status.Build(a.Subject(), elapsed)` → JSON. The CLI's `runStatus` keeps
its cobra shape; the shared core is bootstrap+Build, and equivalence is
pinned by comparing tool output against `status.Build` on the same
subject (not by refactoring the CLI).

**search:** loader `Load` → substring over decision titles/decisions and
invariant statements (+ ids); index `SymbolsNamed` exact per query token?
— exact-name lookup on the whole query; snippet = title/statement/name +
path. Cap 50, refuse above.

**context:** summary = {readiness, counts(decisions, invariants, units?),
paths}; focused adds ≤5 record excerpts (newest? loader order — document
whichever, deterministic). Everything else refused when present.

**decide/invariant:** validate inputs (non-empty title/decision/statement;
mode active-only) → `record.NewDecision/NewInvariant` → allocate id →
marshal canonical → create-exclusive write → return {id, path}. Loader
read-back pinned in tests.

## 7. Client identities

The two-client fixture uses distinct `Implementation{Name, Version}` over
separate in-memory transports against one server: same ListTools schemas,
same Call results. Client identity never changes behavior (AC-20).

## 8. Readiness, CLI and performance integration

None beyond D-191. No wall-clock assertions. Search walks loaded records
+ one name lookup — bounded by repo knowledge size, capped at 50 results.

## 9. Outputs reserved for later milestones

- Tool surface — MR-015's coordination/change tools extend the same server.
- Record files — MR-017's CI reads them; symbol-refresh (MR-006 service)
  binds new invariants on later runs.
- Refusal codes — MR-017 surfaces them to agents.

## 10. Test plan

Per tech-stack §90's categories, per task:

1. **Server + read tools** (TASK-01): 6 tools registered with schemas;
   bootstrap/status equivalence with CLI path; search cap + sources;
   summary/focused shapes; present-but-deferred refusals.
2. **Write tools + matrix + clients** (TASK-02): decide/invariant
   round-trip via loader; id allocation max+1; candidate refusal; full
   deferred matrix; two-client identity; diagnostics value-free.
3. **Proof** (TASK-03): verify + tidy, non-goal greps, mutation ledger,
   Durum.
4. **Mutation ledger**: every guard gets its mutation run before it is
   written down (unchanged rule).
