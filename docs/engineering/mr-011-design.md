# MR-011 — Design

- **Frozen at:** commit `5aaaae3`, before any implementation commit.
- **Refines and is refined by:** [mr-011-requirements.md](mr-011-requirements.md)
  (D-165…D-169 record every place this design left a gap the implementation
  would otherwise fill silently).
- **Specification anchors:** spec-1.0 §48 (snapshot binding, STALE rule for
  every evidence type); AC-22 (evidence stale); kernel-scope §3
  (STRUCTURAL) and §4 (deferred breadth); tech-stack §7 (layout).

## 1. What the milestone is, in one paragraph

The expiry layer on MR-010's evidence: after MR-011, asking "does this
evidence still prove anything" re-hashes the scope the row stapled and
answers current or stale with the reason, maps stale rows and coverage gaps
to a re-run list, and never writes a byte — the old row stays beside the
new one, and the verdict recomputes every time.

## 2. What it is not

| Temptation | Ruling |
|---|---|
| Stale flags/columns | computed only (D-165); append-only absolute |
| Required-policy ownership | caller input; MR-013 owns the policy (D-167) |
| Gate enforcement (ALLOW/DENY) | MR-013's; coverage is a value, never a verdict |
| Auto re-run | drivers re-run; this milestone names what to re-run |
| Fuzzy relevance | own-scope hash equality only (D-166) |

## 3. Package plan

| Package | Owns | Deliberately not there |
|---|---|---|
| `internal/validation` (+file) | `Verdict`, `Coverage`, `Service.Check` (freshness + required coverage + re-run union) | writes, codes |
| everything else | untouched | — |

One file (`freshness.go`) plus tests. No new reads on any store: the check
takes evidence rows as input (MR-010 already returns them) and hashes the
live tree — no DB access at all, which makes read-only structural.

## 4. Entity model

**Verdict.** `{EvidenceID, Profile, Status current|stale, Reason string,
SnapshotHash (stored), CurrentHash (recomputed, empty when unhashable)}`.
Reason names the gap: content change, added/removed file, unreadable
scope, or the satisfied scope for current.

**Coverage.** `{Required []string, Satisfied map[string]bool, ReRun []ReRunItem}`.
`ReRunItem{Profile, Reason}` — one per profile, first reason wins,
deterministic order (sorted).

**Check inputs.** `(rows []Evidence, required []string)`. Rows arrive as
plain values (D-153 pattern from MR-008/009: milestones trade structs, not
store handles). Scope comes from each row's provenance JSON (`scope`
array) — the same bytes Record stored.

**Re-hash rule.** Scope paths are absolute (Record stored them absolute).
Re-hash = sha256 over sorted `rel + NUL + content` with rel against the
common root of... — scope files may span directories; the hash must be
root-independent to compare with the stored hash. SnapshotScope hashes rel
paths against the profile ROOT, but Check sees only absolute scope paths.
Resolution: recompute with rel paths against the longest common directory
prefix of the scope? Fragile. Better: store the root IN provenance at
Record time? That changes MR-010's provenance shape (committed, gated).

Alternative honest rule: re-hash each file's content keyed by ABSOLUTE
path (`abs + NUL + content`), and change SnapshotScope/Record to use the
same rule? That changes MR-010's committed hash (its tests pin
determinism, not the exact algorithm — "re-runs agree", AC-02.3; the E2E
reproduces via independent SnapshotScope call, which would still agree if
both change together). Hmm — but changing MR-010's gated hash function in
MR-011 is behavior change to gated work. Allowed only with re-gating and
honest record... OR: Check takes the ROOT as a parameter! `Check(ctx, root, rows, required)`: re-hash scope paths as rel-against-root, exactly SnapshotScope's rule. Root is caller knowledge (the repo root the profile ran against). No MR-010 change, exact algorithm reuse: factor the hash core into `hashFiles(root, files)` used by both SnapshotScope and Check. Clean. Decision: Check receives root explicitly; scope paths must resolve under it (escape → stale, fail-safe).

**Current rule.** Recomputed hash == stored hash → current. Anything else
(including unhashable) → stale.

## 5. Runtime schema

None (D-165). No migration.

## 6. Evaluation flows

**Check(root, rows, required):** per row — parse scope from provenance
(malformed/unparseable → stale, reason names it); re-hash under root
(escape/missing/unreadable → stale); equal → current. Then coverage: per
required profile, satisfied iff ≥1 current row names it; re-run = stale
rows' profiles ∪ unsatisfied required, sorted, first reason each.

**Provenance scope parsing.** `{"profile","command_index","scope":[...]}`.
Unknown extra keys ignored; `scope` must be a string array (empty array =
hash of nothing = deterministic; matches SnapshotScope's empty-dir case).

## 7. Secret inputs

None. Hashing reads file bytes; redaction already happened at Record.

## 8. Readiness, CLI and performance integration

None (D-168). Performance: one walk per distinct scope — rows sharing a
scope could share a hash, but 0.1 hashes per row (simplicity over cache;
recorded). No wall-clock assertions.

## 9. Outputs reserved for later milestones

- Verdicts + coverage — MR-013's gate input (required evidence check).
- Re-run lists — MR-015's tool input (what to re-run).
- Current hashes — MR-013's audit input (what the tree looked like).

## 10. Test plan

Per tech-stack §90's categories, per task:

1. **Freshness/coverage** (TASK-01): current on untouched scope; stale on
   content change / added file / removed file / unreadable scope, each
   reason named; out-of-scope edit leaves current; required satisfied /
   stale-only / missing with reasons; re-run union sorted; read-only
   (row-count + bytes before/after).
2. **Proof** (TASK-02): real-tree E2E (run → current; relevant edit →
   stale + re-run + unsatisfied; unrelated edit → current; second run →
   current with two rows); non-goal greps; mutation ledger; Durum.
3. **Mutation ledger**: every guard gets its mutation run before it is
   written down (unchanged rule).
