# MR-020 — Design

- **Frozen at:** commit `9f9fbb6`, before any demo run.
- **Refines and is refined by:** [mr-020-requirements.md](mr-020-requirements.md)
  (D-237…D-244 record every place this design left a gap the
  demonstrations would otherwise fill silently).
- **Specification anchors:** kernel-scope §8 (exit condition — the
  sentence this milestone proves); kernel-scope §7 AC-05 (reconcile),
  AC-07/AC-18 (rename/`symbol_uid`), AC-08/AC-09 (evidence/stale),
  AC-10/AC-20 (test weakening), AC-11 (completion gate), AC-12/AC-19
  (staged/CI verify); spec-1.0 §118 (soft protocol, hard gates —
  reconcile/lease/completion-gate/pre-commit/`verify --ci`); tech-stack
  §106 (advertised set linux/amd64), §109 (Makefile-first).

## 1. What the milestone is, in one paragraph

A scripted but honest adversarial review of the finished 0.1 kernel:
six bypass attempts staged on scratch repositories, each judged by the
real gate binaries, plus one honest run — all witnessed by the product
owner, who then issues the release verdict. Nothing is built; the only
durable outputs are `mr-020-findings.md` (transcripts + verdicts), the
task-list Durum block, and the owner's named decision.

## 2. What it is not

| Temptation | Ruling |
|---|---|
| Fixing a bug a demo finds | record it, let the verdict reflect it (D-241) |
| Polishing demo scripts into committed tooling | scratch lives in `/tmp`, uncommitted (D-241) |
| One worktree playing both agents | two genuinely separate copies (D-239) |
| Narrated verdicts ("this blocks") | gate output pasted verbatim (D-238, D-242) |
| Implied approval | owner's explicit ship/not-yet line (D-244) |
| Re-demonstrating bench/gate/release numbers | MR-019 evidence is input, not rework |

## 3. Demo topology

One binary, built once from the frozen tree (`go build ./...` or the
`Makefile` build target), drives all demos. Each demo gets a fresh
sample repository under `/tmp/mr020-demo-N/`:

- `git init`, a small Python module (a few functions — the STRUCTURAL
  path needs real symbols), a validation profile (e.g. a fast test
  command), and `mindrail init` for knowledge/coordination state.
- Two working copies: either `git worktree add` pair or a second
  clone; the findings record names which agent (A/B) acts in which
  copy at each step.
- Protected symbols come from `mindrail_invariant` (or knowledge
  records) on sample functions; CRITICAL verification tests map to
  the sample's test files.

Per-demo shapes:

| Demo | Setup | Bypass attempt | Judging gate | Expected output |
|---|---|---|---|---|
| ALLOW (AC-01.1) | agent A edits with full protocol, runs validation | none — honest run | completion gate | ALLOW |
| forgotten call (AC-01.2) | agent B edits, skips `before_change` | undeclared diff presented as complete | `reconcile` + completion | real diff surfaced; DENY until reconciled |
| `--no-verify` (AC-02.1) | agent A commits with `--no-verify` past a failing hook | hook-skipped commit | `verify --ci` on fresh clone | DENY, same policy as staged |
| rename (AC-02.2) | agent renames protected symbol | rename breaking invariant link | identity/ambiguity path + completion | same `symbol_uid` carried, or AMBIGUOUS block; no silent orphan |
| test weakening (AC-03.1) | agent weakens critical test only | assertion removal / skip / deletion | test guard (+ staged/CI verify) | `TEST_GUARD_WEAKENED` blocking |
| stale evidence (AC-03.2) | validate, then edit source | old result claimed as proof | completion/freshness | stale verdict; re-run list named |

## 4. Entity model

There is no new entity and no schema. The record reuses existing
vocabulary: reason codes (`TEST_GUARD_WEAKENED`,
`SYMBOL_IDENTITY_AMBIGUOUS`, `ORPHANED_PROTECTED_SYMBOL`, stale
verdicts), `next_action` strings, ALLOW/DENY decisions. Findings rows
follow the established shape: per-task rounds, gate readings,
mutation-style pins where a demo's strength is asserted (e.g. "this
reconcile output would also catch X" needs the X run, not the claim).

## 5. Runtime schema

None. No migration (stays v8), no new `app.Code` (stays 46).

## 6. Readiness, CLI and performance integration

Demos use the CLI surface the gates already expose
(`verify --staged`, `verify --ci`, completion/validate flows, MCP
tools only where the honest path uses them). No new commands. Bench
and gate-matrix numbers are cited from MR-019's evidence as demo
context (the system under demo is the measured system), not re-run
as acceptance proof — TASK-04's proof re-runs `make verify` +
`make tidy-check` + count on the frozen tree only.

## 7. Secrets

None new. Demo repos contain fixture code only; transcripts must not
carry environment secrets (sample secrets for redaction demos are
fixture strings, consistent with MR-010 practice).

## 8. Outputs reserved for later milestones

- Any product fix a demo uncovers — new milestone, not this one.
- FULL-semantic or coverage-driven extensions — post-0.1 backlog.
- Demo automation as committed tooling — explicitly not wanted.

## 9. Test plan

Per demo task:

1. **Run** the scripted steps on scratch repos; paste commands and
   gate outputs into `mr-020-findings.md` verbatim (D-242).
2. **Strengthen** any demo that passes for the wrong reason (the
   standing lesson: fixture demos that green without biting are the
   norm) — e.g. assert the DENY reason code, not just non-ALLOW;
   assert the reconciled diff contains the undeclared file.
3. **Gate** each task with an independent Reader/Breaker pair grading
   record-honesty, two-agent reality and deviation handling; fix
   findings, re-gate, then proceed.
4. **Close** (TASK-04): owner verdict on record, Durum block, verify +
   tidy + count, non-goal greps (46 codes, no `000009`, no MCP
   surface change), graph refresh in its own commit.
