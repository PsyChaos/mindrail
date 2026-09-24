# MR-016 — Design

- **Frozen at:** commit `c60d0af`, before any implementation commit.
- **Refines and is refined by:** [mr-016-requirements.md](mr-016-requirements.md)
  (D-205…D-214 record every place this design left a gap the implementation
  would otherwise fill silently).
- **Specification anchors:** spec-1.0 §49 (profile whitelist), §48 + AC-22
  (snapshot binding), §89 (gate checklist); AC-20 (agent independence);
  kernel-scope §3 (STRUCTURAL) and §4 (deferred breadth); tech-stack §7
  (layout).

## 1. What the milestone is, in one paragraph

The proving surface beside MR-015's doing surface: after MR-016, agents
run named validations to snapshot-bound evidence and ask completion —
and the server answers from its own discovery, freshness, bindings,
ambiguities and guard analysis, ALLOW or DENY with the gate's own codes.
Thirteen tools on the wire, every one smoke-answered over stdio.

## 2. What it is not

| Temptation | Ruling |
|---|---|
| Ad-hoc argv validation | named profiles only (D-205) |
| Caller-supplied verdicts | server composes (D-206) |
| Stored completion decisions | pure per call (gate pattern) |
| Budget/approval flows | version-error refusals (D-208) |
| Subprocess stdio test | in-process pipes (D-209) |
| CLI `mcp` command | still deferred (D-191) |

## 3. Package plan

| Package | Owns | Deliberately not there |
|---|---|---|
| `internal/mcp` (+file) | validate/complete handlers, composition (bindings/severity/guard mapping), version-error params | serving, CLI |
| `internal/validation` (+read) | `EvidenceForProfile` (newest-first list) | behavior change |
| `internal/index` (+read) | status-inclusive bindings per uid | traversal/severity |
| everything else | untouched (consumed as services) | — |

No migration, no codes.

## 4. Entity model

**Validate I/O.** `validate{profile, budget?, escalation?, approval?}` →
`{evidence[] (id, status, snapshot), pending?}`. Profile from loaded
config (`app.Config().Config.Validation`); unknown → usage refusal.
Version-error params refuse when present (D-208).

**Complete I/O.** `complete{task_id, required[]?, budget?, escalation?,
approval?}` → `{allow, denials[] (code, reason, key, provenance,
next_action)}` — the gate Decision shape verbatim (composition proves
parity by construction).

**Composition (complete):**
1. `EvaluateTask(task)` → attribution findings.
2. `EvidenceForProfile` per required → `Check(root, rows, required)` →
   coverage.
3. Task's change files (open change if any; none → empty file set) →
   test files (D-207 rule) → before (`git show HEAD:rel`, missing →
   empty) / after (disk) → mappings from bound-uid referrers in test
   files (name via UIDForKey+FactsForUID) with severity/active from
   knowledge bodies → `testguard.Evaluate` → guard findings.
4. Bindings with status per task symbol uid + `ListAmbiguitiesForUID` →
   invariant blocks (severity/active from knowledge) + ambiguities
   (anchored when a candidate/removed uid binds an active HIGH/CRITICAL
   invariant, else silent).
5. `gate.Evaluate` over all five → answer.

**Knowledge severity/active.** Parse invariant bodies
`{severity, status}`; active = status active. Unparseable body for a
bound invariant → fail-safe: treat as active CRITICAL? No — treat as
unknown → warn-scope (silent)? Fail-safe direction for a SAFETY gate is
to deny on uncertainty... but an unparseable body is a loader problem
(loader would have flagged it). Decision: unparseable → deny with
ORPHANED code? Overreach. Simplest honest: unparseable severity/status
→ treat as active CRITICAL (fail-safe deny, reason names the gap).
Hmm — that could block on loader-accepted-but-odd bodies... loader
validates schema, so bodies are well-formed; severity/status enums are
closed. JSON parse of known-good bytes cannot fail in practice; the
branch is defensive. Keep fail-safe CRITICAL with reason.

## 5. Runtime schema

None.

## 6. Tool flows

**validate:** config lookup → `validation.Service.RunProfile` (needs a
Runner + Store — server builds a validation service at construction
beside the changes service) → rows summary. Secret env for redaction:
`app.Config` secrets? Config has Secrets.Env names — pass them so
redaction watches configured names (MR-010 flow parity).

**complete:** compose (§4) → `gate.Evaluate` → verbatim decision. Empty
task (no change, no rows) composes empty → ALLOW iff required empty too
(vacuous clean, consistent with gate semantics).

## 7. Version-error params

`budget`, `escalation`, `approval` on both tools: present → refusal with
code + next_action naming the default flow. Absent → default.

## 8. Readiness, CLI and performance integration

None. No wall-clock assertions. Complete walks the task's rows +
test-file deltas — bounded by change size.

## 9. Outputs reserved for later milestones

- Thirteen-tool server — MR-017's CI drives it.
- Composition inputs — MR-017 reuses them for staged/CI verify.
- Version errors — MR-017 surfaces policy alternatives.

## 10. Test plan

Per tech-stack §90's categories, per task:

1. **Validate + reads** (TASK-01): named run to bound evidence;
   unknown-profile refusal; EvidenceForProfile order; bindings statuses.
2. **Complete + errors + stdio** (TASK-02): ALLOW clean; DENY per
   reachable family; stale-evidence parity with gate unit; version
   errors ×3 params ×2 tools; stdio 13-tool discovery + smokes.
3. **Proof** (TASK-03): verify + tidy, non-goal greps, mutation ledger,
   Durum.
4. **Mutation ledger**: every guard gets its mutation run before it is
   written down (unchanged rule).
