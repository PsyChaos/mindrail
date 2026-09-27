# MR-014 — Frozen requirements and acceptance criteria

- **Frozen at:** commit `273090a`, branch `mr-002-knowledge-lifecycle`
- **Contract this refines:** [mr-014-design.md](mr-014-design.md). Where the two
  disagree, the design wins on intent and this document wins on the detail the
  design left open — every such gap is recorded below as a decision D-185…D-194.
- **Task:** [mindrail-0.1-task-list.md](mindrail-0.1-task-list.md) MR-014
- **Tier:** 3 (Program). It adds the official MCP Go SDK dependency, a new
  package (`internal/mcp`), six tools on the wire vocabulary, one `app.Code`,
  and knowledge-record persistence (decisions/invariants files agents can
  create through tools).

This file is frozen **before** implementation so that an audit has something
to grade against that the implementation did not shape. The milestone runs
**one task at a time** as MR-004 through MR-013 did: each task in §4 is
implemented, gated by an independent Reader/Breaker pair and recorded before
the next is begun.

---

## 0. Baseline evidence

Captured on the tree this work starts from, so a red suite later is unambiguous.

| Measurement | Value | Command |
|---|---|---|
| Commit | `273090a` | `git rev-parse --short HEAD` |
| Go toolchain | `go1.27.0 linux/amd64` | `go version` |
| Packages | 33 | `go list ./... \| wc -l` |
| Top-level test functions | 1215 | `go test -list '.*' ./... \| grep -c '^Test'` |
| `make verify` | green at MR-013 close | `make verify` |
| `make tidy-check` | green at MR-013 close | `make tidy-check` |
| Migrations | `000001_initial.sql` … `000008_evidence.sql`; runtime `schema_version` reports 8 | `ls migrations/*.sql` |
| Registered `app.Code` values | 45 | `grep -c 'Code = "' internal/app/code.go` |
| MCP tools | do not exist (no package, no SDK) | go.mod + code |
| Writable knowledge records | do not exist (records read-only) | code |

---

## 1. The scenario, in one paragraph

Two different MCP clients open the same repository: both list the same
six tools with identical schemas, both read the same status JSON the CLI
prints, both search the same records and symbols, both get
`NOT_IMPLEMENTED_IN_THIS_VERSION` with a next action when asking for
`full` detail or a `candidate` invariant — and both record a decision and
an active invariant that the loader reads back on the next run. No
behavior exists twice: every tool calls the application services the CLI
already uses.

---

## 2. Decisions

MR-013 ended at D-184. These continue the sequence.

### D-185 — official SDK v1.8.0, pinned in go.mod

`github.com/modelcontextprotocol/go-sdk@v1.8.0` over in-memory transports
in tests and stdio in production wiring. Typed handlers
(`ToolHandlerFor[In, Out]`) so schemas derive from structs, never from
hand-written JSON.

### D-186 — tools bind application services, never reimplement

bootstrap/status via `bootstrap.App` + `status.Build` (the CLI's own
path); search over `loader.Store` records + `index` name reads; context
aggregates the same report; decide/invariant validate via
`record.NewDecision/NewInvariant` and persist canonical JSON. A behavior
fork is a bug, pinned by CLI/MCP equivalence tests where a CLI path
exists.

### D-187 — persistence mirrors the loader layout, ids allocated

Records persist as flat `.json` under `knowledge/decisions` and
`knowledge/invariants` with filename==id (spec §95 step 6). Ids allocate
max+1 (`DEC-%04d`/`INV-%04d`) inside the tool call: agents never guess
ids, and concurrent allocation is serialized per call (single writer per
tool invocation; races report, never duplicate — best effort in 0.1,
recorded).

### D-188 — deferred values refuse loudly, never silently drop

`full`, `impact_group`, cursor/page size, `candidate`, over-limit search
and unknown detail levels/modes return `NOT_IMPLEMENTED_IN_THIS_VERSION`
with `next_action`. Absent optional fields behave by default; PRESENT but
unsupported values refuse. That distinction is the whole rule, pinned per
parameter.

### D-189 — NOT_IMPLEMENTED_IN_THIS_VERSION is an app.Code (usage class)

Wire vocabulary +1 (45→46, pinned). Usage class: the request is
well-formed and retryable with supported values — actionable like usage,
not a failure of the run.

### D-190 — search is bounded text match over two sources

`{query, limit}`: substring match over knowledge record titles/statements
and exact-name index declarations. Limit capped at 50; above refuses
(D-188: no silent truncation). No ranking beyond source order + exact
preference — ranking is deferred, recorded.

### D-191 — no CLI command, no MCP surface beyond the six tools

Server constructor + handlers only, driven by tests now and by process
wiring later (the D-91 pattern). No `mindrail mcp` command in 0.1.

### D-192 — secret values never cross tool boundaries differently

Tool inputs/outputs carry no secrets by construction (profiles, paths,
records); the redaction boundary stays MR-010's. Diagnostics echo names
and keys, never values (spec §19, MR-010 precedent).

### D-193 — serial tasks, Reader/Breaker gates, findings rows

The MR-004…MR-013 process repeats: one task at a time, each gated before
the next, each recorded in `mr-014-findings.md` (new file — MR-013's record
stays closed). Post-gate hygiene (`git status`/`git diff` on tracked
paths) applies to every subagent.

### D-194 — contract is structs + snapshots, not prose

Tool In/Out structs ARE the schema (SDK derives JSON Schema from them);
contract tests pin marshalled shapes plus the deferred matrix. Prose
describes; structs decide.

---

## 3. Requirements and acceptance criteria

REQ-01…REQ-07. Each task's criteria are dotted under it, in the form the
gates grade. A criterion not met at the end of its task is recorded as not
met, with the reason, in `mr-014-findings.md`.

**REQ-01 — SDK + server + six tools** (dependency, registration, handlers).
**REQ-02 — service binding** (no fork: bootstrap/status/search/context).
**REQ-03 — decide/invariant persistence** (validate + allocate + write + read-back).
**REQ-04 — deferred matrix** (loud refusal per parameter).
**REQ-05 — non-goals, guarded**.
**REQ-06 — evidence discipline** (every new guard mutation-red).
**REQ-07 — surface discipline** (one code; six tools; no CLI command/tables/timings).

### TASK-01 — SDK, server, bootstrap/status/search/context

Owns REQ-01, REQ-02.

- **AC-01.1** SDK pinned in go.mod; server constructor registers exactly
  the six tools with derived schemas; handlers are typed In/Out structs.
- **AC-01.2** bootstrap and status tools return the same report the CLI
  builds (same `bootstrap.App` + `status.Build` path — equivalence
  pinned, not asserted by prose).
- **AC-01.3** search binds loader records + index names with limit cap 50
  (over-limit refuses); results carry source, id/key and snippet.
- **AC-01.4** context serves summary (readiness + counts + paths) and
  focused (+ bounded excerpts); every other detail level refuses.
- **AC-01.5** deferred context params (full, impact_group, cursor,
  page_size) refuse with code + next_action when PRESENT; absent means
  default, never refusal.

### TASK-02 — decide/invariant, deferred matrix, two-client fixture

Owns REQ-03, REQ-04.

- **AC-02.1** decide validates + persists a decision readable back by the
  loader (filename==id, canonical JSON); ids allocate max+1.
- **AC-02.2** invariant mode=active validates + persists an active
  invariant readable back; mode=candidate (and any other) refuses with
  code + next_action.
- **AC-02.3** the full deferred matrix refuses loudly per parameter
  (full/impact_group/cursor/page_size/candidate/over-limit/unknown
  enum), each with code + next_action, none silent.
- **AC-02.4** two-client fixture: two SDK clients with distinct
  identities over in-memory transports assert identical tool lists,
  schemas and call results against one server.
- **AC-02.5** diagnostics across tools echo names/keys only, never values
  (spec §19 pin).

### TASK-03 — proof, non-goals and the record

Owns REQ-05, REQ-06, REQ-07.

- **AC-03.1** full-suite green evidence: `make verify`, `make tidy-check`,
  test count by the single command.
- **AC-03.2** the non-goals hold under grep: one new code (registry 46),
  six tools exactly, no CLI command, no tables, no value echo.
- **AC-03.3** every new guard added by TASK-01…02 has its mutation run and
  recorded red (REQ-06), and the task list's MR-014 entry carries a
  `#### Durum` block written the way MR-013's is.

---

## 4. Work breakdown and dependency order

Serialised: each task is gated before the next is written.

| Task | Owns | REQ | Depends on |
|---|---|---|---|
| TASK-01 | SDK + server + 4 read tools (bootstrap/status/search/context) | REQ-01, REQ-02 | — |
| TASK-02 | decide/invariant tools + deferred matrix + 2-client fixture | REQ-03, REQ-04 | TASK-01 |
| TASK-03 | proof, non-goals, record, Durum block | REQ-05, REQ-06, REQ-07 | TASK-02 |

---

## 5. Definition of done

1. Every acceptance criterion above is met, or is recorded as not met with
   the reason in `mr-014-findings.md`.
2. `make verify` green, `make tidy-check` green, and the test count reported
   with `go test -list '.*' ./... | grep -c '^Test'` — that command and no
   other.
3. Every new guard has a recorded mutation that turns it red, recorded after
   it was run.
4. Every task in §4 has passed its Reader/Breaker gate before the next was
   begun, and the gate's findings — confirmed, refuted, unconfirmed — are in
   `mr-014-findings.md` per task, with what was done about each.
5. The task list's MR-014 entry carries a `#### Durum` block written the way
   MR-013's is.
6. The knowledge graph is refreshed in its own commit at the end.

---

## 6. Traceability to the task list's five acceptance criteria

| Task list AC | Where it is owned |
|---|---|
| CLI ve MCP aynı application servislerini kullanır; davranış fork'u oluşmaz | AC-01.2 (REQ-02) |
| Tool input/output şemaları contract testleriyle sabitlenir | AC-01.1, AC-02.4 (REQ-01, REQ-04; D-194) |
| Ertelenmiş değerler sessizce düşürülmez | AC-01.5, AC-02.3 (REQ-02, REQ-04; D-188) |
| Desteklenmeyen değer NOT_IMPLEMENTED + next_action döndürür | AC-01.5, AC-02.2, AC-02.3 (REQ-02, REQ-04) |
| En az iki farklı MCP client uyumunu simüle eden fixture bulunur | AC-02.4 (REQ-04) |
