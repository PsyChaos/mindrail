# MR-009 — Frozen requirements and acceptance criteria

- **Frozen at:** commit `a0b16e2`, branch `mr-002-knowledge-lifecycle`
- **Contract this refines:** [mr-009-design.md](mr-009-design.md). Where the two
  disagree, the design wins on intent and this document wins on the detail the
  design left open — every such gap is recorded below as a decision D-146…D-154.
- **Task:** [mindrail-0.1-task-list.md](mindrail-0.1-task-list.md) MR-009
- **Tier:** 3 (Program). It adds a new package (`internal/impact`), the
  traversal core MR-010/MR-013 will consume, and the breadth vocabulary
  completion/verify will read. No database schema, no wire vocabulary.

This file is frozen **before** implementation so that an audit has something
to grade against that the implementation did not shape. The milestone runs
**one task at a time** as MR-004 through MR-008 did: each task in §4 is
implemented, gated by an independent Reader/Breaker pair and recorded before
the next is begun.

---

## 0. Baseline evidence

Captured on the tree this work starts from, so a red suite later is unambiguous.

| Measurement | Value | Command |
|---|---|---|
| Commit | `a0b16e2` | `git rev-parse --short HEAD` |
| Go toolchain | `go1.27.0 linux/amd64` | `go version` |
| Packages | 29 | `go list ./... \| wc -l` |
| Top-level test functions | 1138 | `go test -list '.*' ./... \| grep -c '^Test'` |
| `make verify` | green at MR-008 close | `make verify` |
| `make tidy-check` | green at MR-008 close | `make tidy-check` |
| Migrations | `000001_initial.sql` … `000007_scope_attribution.sql`; runtime `schema_version` reports 7 | `ls migrations/*.sql` |
| Registered `app.Code` values | 43 | `grep -c 'Code = "' internal/app/code.go` |
| Impact records | do not exist (no table, no type) | schema |

---

## 1. The scenario, in one paragraph

A task changes `a.py`'s `helper`; `a.py`'s `handler` calls it, `c.py`'s
`worker` uses the same name, and an explicit invariant watches `helper`'s
uid. Impact answers in layers without guessing: the same-file caller
arrives as a direct structural edge at full confidence; the cross-file
name-user arrives as a `STRUCTURAL_NAME_MATCH` fallback capped at 0.5 with
the reason named; the changed file itself arrives as file fallback; the
bound invariant is listed beside its symbol; and two declarations sharing
one name ambiguate instead of collapsing. Depth stays 1 unless asked, and
nothing structural alone ever recommends above MODULE.

---

## 2. Decisions

MR-008 ended at D-145. These continue the sequence.

### D-146 — the graph is designed with the D-90 constraint, not around it

Resolution stays file-scoped: MR-009 does not widen the key and does not
re-resolve. Cross-file edges come only from the name-match fallback
(symbols.name against reference target_text), always labelled
`STRUCTURAL_NAME_MATCH` at confidence ≤ 0.5. A cross-file caller is a weak
signal by construction, never a quiet promotion.

### D-147 — impact is pure computation, stored nowhere

`Analyze` is a deterministic function of index rows and its request. No
migration, no tables: MR-010's runner and MR-013's gate re-run it instead
of trusting stored verdicts (the D-136 pattern). What persists is inputs
(index facts, baselines, overrides), never outputs.

### D-148 — new package, neutral index reads

Traversal lives in `internal/impact`. The index store gains only neutral
read methods (references by target, symbols by name, bindings by uid) —
no attribution or breadth logic leaks into the index package.

### D-149 — breadth is capped, uplift is justified text

Breadth enum is `TARGETED < MODULE < PACKAGE < SERVICE < REPOSITORY`
(spec §124). Structural evidence alone caps at MODULE: direct edges start
TARGETED, any fallback engages MODULE. Above MODULE requires an explicit
caller-supplied justification, recorded verbatim in the output; in 0.1 no
caller supplies one, so the cap holds everywhere and the mechanism is
tested, not the policy.

### D-150 — depth defaults to 1, deeper is explicit

A zero-value request analyzes depth 1. Depth above 1 runs only when the
caller passes it explicitly. Depth counts reverse edges from each changed
symbol: depth-1 entries name direct referrers; depth-2 names referrers of
referrers, each entry carrying its own depth.

### D-151 — serial tasks, Reader/Breaker gates, findings rows

The MR-004…MR-008 process repeats: one task at a time, each gated before
the next, each recorded in `mr-009-findings.md` (new file — MR-008's record
stays closed).

### D-152 — no codes, no CLI/MCP surface, no timings

Impact results block nothing (spec §31: completion is never blocked on a
score), so no `app.Code` is added — the registry stays 43, pinned. No
commands, no status/doctor keys, no wall-clock assertions. Service and
structs only, driven by tests now and by milestone drivers later (the D-91
pattern).

### D-153 — scope, leases and claims are not inputs

Impact is a structural fact about the repository, not about who declared
what: baselines, leases and claims never enter the traversal. MR-008's
attributed (change, symbol) pairs arrive as plain uid/key inputs — the only
bridge between the milestones.

### D-154 — same-name ambiguity never collapses

Two or more declarations sharing one name produce an ambiguous entry naming
every candidate uid: no choice is taken, nothing is merged, and the entry
explains itself like every other (source, confidence, depth, reason).

---

## 3. Requirements and acceptance criteria

REQ-01…REQ-07. Each task's criteria are dotted under it, in the form the
gates grade. A criterion not met at the end of its task is recorded as not
met, with the reason, in `mr-009-findings.md`.

**REQ-01 — types, breadth and depth rules** (vocabulary, cap, default).
**REQ-02 — direct reference edges** (resolved traversal + explainability).
**REQ-03 — fallbacks and ambiguity** (name-match, file/module, same-name).
**REQ-04 — invariant scope mapping** (bound invariants listed, justification).
**REQ-05 — non-goals, guarded**.
**REQ-06 — evidence discipline** (every new guard mutation-red).
**REQ-07 — surface discipline** (no codes/commands/keys/timings/tables).

### TASK-01 — vocabulary, direct edges, depth and breadth rules

Owns REQ-01, REQ-02.

- **AC-01.1** every impact entry explains itself: source edge (referrer,
  target, edge kind), confidence, traversal depth, and fallback reason
  (empty when direct) — no entry without all four.
- **AC-01.2** reverse traversal from a changed symbol follows resolved
  reference edges (`resolved_symbol_id`) to direct referrers; each entry
  names the edge it rode.
- **AC-01.3** depth defaults to 1 on a zero-value request; depth above 1
  runs only when passed explicitly; entries carry their own depth and
  depth-2 reaches referrers of referrers.
- **AC-01.4** breadth starts TARGETED for direct-only results and never
  exceeds MODULE on structural evidence; the cap is pinned, not emergent.

### TASK-02 — fallbacks, ambiguity and invariant mapping

Owns REQ-03, REQ-04.

- **AC-02.1** cross-file name users arrive as `STRUCTURAL_NAME_MATCH`
  fallback at confidence ≤ 0.5 with the reason named; they never promote
  to direct edges.
- **AC-02.2** file/module fallback: the changed file (and its unit) is
  reported with its reason, engaging MODULE breadth; fallback never
  invents symbol edges.
- **AC-02.3** same-name declarations ambiguate: one entry names every
  candidate uid with its source and reason, takes no choice, and blocks
  nothing (impact never blocks).
- **AC-02.4** bound invariants are listed beside their symbols from the
  bindings table; entries naming an invariant carry its id as explicit
  scope justification text.

### TASK-03 — proof, non-goals and the record

Owns REQ-05, REQ-06, REQ-07.

- **AC-03.1** end-to-end: one real indexed repository — a changed symbol
  with a same-file caller, a cross-file name user, a bound invariant —
  yields direct + fallback + file + invariant entries in one analysis,
  every entry self-explaining, breadth ≤ MODULE, depth 1 default.
- **AC-03.2** the non-goals hold under grep: no persistence, no new
  codes/commands/keys (registry still 43), knowledge schema still v1.
- **AC-03.3** every new guard added by TASK-01…02 has its mutation run and
  recorded red (REQ-06), and the task list's MR-009 entry carries a
  `#### Durum` block written the way MR-008's is.

---

## 4. Work breakdown and dependency order

Serialised: each task is gated before the next is written.

| Task | Owns | REQ | Depends on |
|---|---|---|---|
| TASK-01 | types, direct edges, depth default, breadth cap | REQ-01, REQ-02 | — |
| TASK-02 | name-match + file/module fallback, same-name ambiguity, invariant mapping | REQ-03, REQ-04 | TASK-01 |
| TASK-03 | E2E proof, non-goals, record, Durum block | REQ-05, REQ-06, REQ-07 | TASK-02 |

---

## 5. Definition of done

1. Every acceptance criterion above is met, or is recorded as not met with
   the reason in `mr-009-findings.md`.
2. `make verify` green, `make tidy-check` green, and the test count reported
   with `go test -list '.*' ./... | grep -c '^Test'` — that command and no
   other.
3. Every new guard has a recorded mutation that turns it red, recorded after
   it was run.
4. Every task in §4 has passed its Reader/Breaker gate before the next was
   begun, and the gate's findings — confirmed, refuted, unconfirmed — are in
   `mr-009-findings.md` per task, with what was done about each.
5. The task list's MR-009 entry carries a `#### Durum` block written the way
   MR-008's is.
6. The knowledge graph is refreshed in its own commit at the end.

---

## 6. Traceability to the task list's five acceptance criteria

| Task list AC | Where it is owned |
|---|---|
| Her impact sonucu kaynak edge, confidence, traversal depth ve fallback nedenini açıklar | AC-01.1 (REQ-01) |
| `depth > 1` yalnızca explicit istekte çalışır | AC-01.3 (REQ-01) |
| Salt structural edge validation breadth'i MODULE üstüne çıkaramaz | AC-01.4 (REQ-01) |
| Path policy, public API kuralı veya explicit invariant scope daha geniş breadth'i gerekçelendirebilir | AC-02.4 + D-149 (REQ-04; mechanism tested, policy deferred) |
| Aynı isimli declaration'lar için belirsizlik kaybolmadan raporlanır | AC-02.3 (REQ-03) |
