# MR-012 — Design

- **Frozen at:** commit `36c2af5`, before any implementation commit.
- **Refines and is refined by:** [mr-012-requirements.md](mr-012-requirements.md)
  (D-170…D-178 record every place this design left a gap the implementation
  would otherwise fill silently).
- **Specification anchors:** spec-1.0 §103 area (trigger scope, minimum
  heuristics, finding shape, policy ladder); AC-23-adjacent (reconcile
  ambiguity is MR-008's; this is the weakening guard); kernel-scope §3
  (STRUCTURAL) and §4 (deferred breadth); tech-stack §7 (layout).

## 1. What the milestone is, in one paragraph

The cheap anti-cheat layer on test files: after MR-012, deleting asserts,
skipping tests or removing the test that watches a CRITICAL invariant
produces a named, explained, confidence-tagged finding — blocking exactly
where the spec demands hardness, warning everywhere else, suppressible
only by a marker that announces itself. One service answers all four
trigger paths with the same analysis.

## 2. What it is not

| Temptation | Ruling |
|---|---|
| Assertion-quality / mutation testing | explicitly not (spec: "bu ... değildir"); counts + markers only |
| Text search (`grep assert`) | tree-sitter nodes only; strings/comments never count (D-174) |
| Near-cousin markers | spec list only; rest recorded as non-goals (D-171) |
| Computed invariant↔test mapping | caller input in 0.1 (D-172) |
| Warning enforcement | MR-013's; warnings are values |
| Auto-fix / revert | never; findings only |

## 3. Package plan

| Package | Owns | Deliberately not there |
|---|---|---|
| `internal/testguard` (new) | `Finding`, `TestMapping`, `FileDelta`, `Service.Evaluate` (parse, diff per test, markers, mapping, blocking, suppressions, policy) + tree-sitter queries | store, CLI |
| `internal/app` (+1 code) | `TEST_GUARD_WEAKENED` with remedy | — |
| everything else | untouched | — |

No migration. Query files ride beside the package (own `queries/`
directory, compiled once — the parser Registry pattern, not reused
because kinds differ).

## 4. Entity model

**FileDelta.** `{Path, Language python|typescript|javascript|tsx, Before, After []byte, Trigger string}`.
Trigger ∈ {after_change, reconcile, staged, ci} — provenance only.

**TestMapping.** `{TestKey string (path + test name), ProductionUID string, InvariantID string, Severity string, Active bool}`.
Unknown test keys (mapping names a test neither version defines) refuse
before analysis (D-172: loud garbage).

**Finding.** `{Code TEST_GUARD_WEAKENED, Signal enum (spec heuristics), TestKey, ProductionUID, InvariantID, BeforeSummary, AfterSummary, Reason, Confidence 0.5, Blocking bool, Trigger}`.
Summaries carry counts (`asserts 3→1`, `markers []→[skip]`, `present→removed`).

**Suppression.** `{TestKey, Reason "marked allow-test-weakening"}` listed
in the result beside findings.

**Result.** `{Findings []Finding, Suppressions []Suppression}`.

**Signals.** `ASSERTION_REMOVED, ASSERTION_COUNT_DECREASED, TEST_SKIPPED, TEST_XFAILED, TEST_DISABLED, EXPECTATION_REMOVED, ASSERT_TO_NOOP, CRITICAL_TEST_REMOVED`.
ASSERT_TO_NOOP: asserts gone AND body trivialized (pass/ellipsis/return
only in Python; empty body in TS/JS) — still a removal shape, reason notes
the trivialization.

**Policy.** block iff `Active && Severity==CRITICAL && signal ∈ {CRITICAL_TEST_REMOVED, TEST_DISABLED}`; else warn.
Documented as a table in findings + tested matrix.

## 5. Runtime schema

None (D-178). No migration.

## 6. Analysis flows

**Evaluate(deltas, mappings, trigger):** per file — parse before/after
(parse failure → fail-closed error, never quiet); index test functions per
language rule; per test present in either version — diff asserts/markers/
presence; map test key → mappings; emit findings with blocking per
policy; collect suppressions for marked tests (marker read from comments
preceding the function in AFTER bytes; a marker in BEFORE that is removed
does not suppress — removal of the marker is itself un-weakening... rule:
suppression applies when the AFTER version carries the marker).

Per-test assert counting: Python `assert_statement` nodes inside the
function body; TS/JS `expect(` call nodes inside the test callback.
Markers: Python decorator dotted-names in the skip set; TS/JS
`x.skip`-style member calls on the test name (`it.skip`, `test.skip`,
`describe.skip` for suite-level — suite skip marks every test inside:
implement by attributing suite markers to contained tests).

Test identity across versions: same (path, name). Renamed tests read as
remove+add (add is quiet) — recorded limitation, not guessed through.

## 7. Secret inputs

None. Test files are source, not secrets; findings never echo file
contents beyond counts and names.

## 8. Readiness, CLI and performance integration

None (D-178). Performance: two parses per file + one query pass each —
bounded by file size, never repository size. No wall-clock assertions.

## 9. Outputs reserved for later milestones

- Findings + blocking flags — MR-013's gate input (the hard-block source).
- Suppressions — MR-013's audit input (who allowed what).
- Policy table — MR-013's configuration input.

## 10. Test plan

Per tech-stack §90's categories, per task:

1. **Python** (TASK-01): count decrease/removal, skip/xfail markers +
   negative near-cousins, function removal, added-quiet, string/comment
   non-count, finding shape (all fields), code registry + exit class.
2. **TS/JS + mapping + hatch** (TASK-02): expect decrease/removal, skip
   variants + removal, CRITICAL remove/disable blocks, CRITICAL skip
   warns, unmapped never blocks, unknown-key refusal, 4-provenance
   identity, marker suppression + listing, marker-without-weakening
   quiet, policy matrix.
3. **Proof** (TASK-03): §1 E2E across provenances; non-goal greps (44
   codes, tree-sitter only); mutation ledger; Durum.
4. **Mutation ledger**: every guard gets its mutation run before it is
   written down (unchanged rule).
