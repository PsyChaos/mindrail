# MR-009 — Design

- **Frozen at:** commit `a0b16e2`, before any implementation commit.
- **Refines and is refined by:** [mr-009-requirements.md](mr-009-requirements.md)
  (D-146…D-154 record every place this design left a gap the implementation
  would otherwise fill silently).
- **Specification anchors:** spec-1.0 §31 (impact outputs, never blocking),
  §32–33 (sources with provenance, edge vocabulary), §35 (hard rules
  outrank scores), §124 (breadth ladder), §126 (why/path/confidence/source);
  kernel-scope §3 (STRUCTURAL) and §4 (deferred breadth); tech-stack §7
  (layout). D-90 constraint: file-scoped resolution (mr-005-findings D-90).

## 1. What the milestone is, in one paragraph

The explanation layer on top of MR-005's facts and MR-008's ownership.
After MR-009, asking "what does changing this symbol touch" returns a
bounded, self-explaining answer: direct callers rode resolved edges,
cross-file name users are labelled weak matches, the file itself is the
floor, bound invariants are named, same-name collisions stay open — every
entry carrying its edge, confidence, depth and reason, breadth never above
MODULE without a written justification.

## 2. What it is not

| Temptation | Ruling |
|---|---|
| Cross-file resolution widening | file-scoped stays (D-146); name-match only |
| Scores, ranking, risk numbers | no scores — entries + breadth only (§31: score never blocks) |
| Persisted impact rows | pure computation (D-147) |
| Scores/policies (public API, path policy) | justification mechanism only (D-149); policies are later callers |
| Test/coverage edges | no provider in 0.1 (§39 area deferred); structural + invariant only |
| Semantic/call-graph precision | STRUCTURAL kernel scope; confidence tells the weakness |

## 3. Package plan

| Package | Owns | Deliberately not there |
|---|---|---|
| `internal/impact` (new) | `Breadth`, `Entry`, `Request`, `Result`, `Service.Analyze` (direct edges, fallbacks, ambiguity, invariant mapping, breadth) | persistence, scores |
| `internal/index` (+read methods) | `ReferrersOfUID`, `SymbolsNamed`, `BindingsForUID` — neutral row reads | traversal, breadth |
| `internal/app` | untouched (no codes — D-152) | — |
| `internal/changes` | untouched (pairs arrive as plain inputs — D-153) | — |

The `symbol` package is not involved: traversal reads stored reference and
fact rows, never live parse trees.

## 4. Entity model

**Request.** `{Symbols []Input (uid and/or key), Depth int, Justification string}`.
Depth ≤ 0 means 1 (D-150). Justification above MODULE is recorded verbatim
(D-149); empty in 0.1 owns the cap.

**Entry.** `{Target (uid, key, name, path), Via (referrer key/path, edge
kind), Confidence float64, Depth int, FallbackReason string (empty when
direct), Invariants []string, Breadth Breadth, Justification string}`.
Every entry explains itself (AC-01.1): no entry without all four of edge,
confidence, depth and reason-slot.

**Edge kinds.** `direct` (rode a resolved reference), `name-match`
(cross-file name use, ≤ 0.5), `file` (changed file floor), `ambiguous`
(same-name collision, names every candidate uid). Entry confidence is read
from the reference row, never invented: today the indexer writes 0.5 on
every row (resolved or not), so direct and name-match entries share the
number and differ in edge kind and label — the kind carries the strength,
not a heuristic bump.

**Breadth.** Per result: TARGETED when every entry is direct, MODULE when
any fallback engages; above MODULE only with non-empty Justification
(which 0.1 never supplies — the mechanism test passes one and records it).

**Ambiguity.** Same-name declarations (two symbols, one name, different
uids) reached by one target_text produce one `ambiguous` entry listing all
candidate uids. No choice, no merge (D-154).

**Invariants.** `invariant_symbol_bindings` (status bound) → invariant ids
per affected uid, listed on the entry; the id doubles as explicit scope
justification text (AC-02.4).

## 5. Runtime schema

None (D-147). No migration: this milestone adds no tables and touches no
ledger.

## 6. Analysis flows

**Analyze(request):** resolve each input to a uid (uid direct, or key via
the identities table — key→uid read on index store); depth-1: referrers of
the uid via resolved edges → `direct` entries; name users via symbols-by-name
excluding the uid itself → `name-match` entries (≤ 0.5, reason named);
changed file(s) of the uid → `file` entry (MODULE); bindings of the uid →
invariant ids on entries. Depth-2+: repeat from each depth-1 referrer's uid
(referrer rows carry their own uid? referrer_key→uid via identities… —
referrer_key is a logical key; resolve via key→uid read; unresolvable
referrer keys stop the climb, recorded by absence). Same-name collisions at
any step collapse to one `ambiguous` entry.

**Breadth:** TARGETED iff no fallback engaged; MODULE on any fallback;
Justification non-empty → recorded breadth uplift verbatim (test-only in
0.1).

## 7. Lease/scope inputs

None (D-153). Impact never reads baselines, leases or claims.

## 8. Readiness, CLI and performance integration

None (D-152). Performance: traversal touches only the changed symbols'
neighbourhood — rows read scale with referrer counts, never repository
size. No wall-clock assertions in 0.1.

## 9. Outputs reserved for later milestones

- Entries with edges + breadth — MR-010's runner input (what to validate)
  and MR-013's gate input (why it matters).
- Invariant lists — MR-011's invalidation input (what to expire).
- Justification slot — MR-013+ policy input (path policy, public API).

## 10. Test plan

Per tech-stack §90's categories, per task:

1. **Vocabulary/direct** (TASK-01): entry shape (all four explainers);
   resolved-edge traversal to same-file referrers; depth default 1 on
   zero-value + explicit 2 reaching referrers-of-referrers; breadth cap
   MODULE pinned.
2. **Fallbacks/ambiguity/invariants** (TASK-02): name-match ≤ 0.5 with
   reason, never direct; file fallback with MODULE and no invented edges;
   same-name collision naming all uids; bound invariant listed with
   justification text.
3. **Proof** (TASK-03): real indexed repo E2E (direct + fallback + file +
   invariant in one analysis); non-goal greps (43 codes, v1 knowledge, no
   tables); mutation ledger; Durum.
4. **Mutation ledger**: every guard gets its mutation run before it is
   written down (unchanged rule).
