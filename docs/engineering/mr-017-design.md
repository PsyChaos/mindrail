# MR-017 — Design

- **Frozen at:** commit `2339a1b`, before any implementation commit.
- **Refines and is refined by:** [mr-017-requirements.md](mr-017-requirements.md)
  (D-215…D-220 record every place this design left a gap the implementation
  would otherwise fill silently).
- **Specification anchors:** spec-1.0 §64 (independent discovery), §72
  (local pre-commit gate), §89 (checklist), §95 (record validation
  steps); AC-22/AC-23 area; kernel-scope §3 (STRUCTURAL) and §4 (deferred
  breadth); tech-stack §7 (layout).

## 1. What the milestone is, in one paragraph

The commit-time teeth of everything built so far: after MR-017, staging
a bad change and asking for verification gets a deterministic, explained
refusal from the same services the agents use — and installing the
pre-commit guard never disturbs the hook it found.

## 2. What it is not

| Temptation | Ruling |
|---|---|
| Worktree evaluation in staged mode | index only (D-216) |
| Quiet knowledge problems | fail-closed first (D-217) |
| Hook rewriting/normalizing | append-only, owned lines only (D-218) |
| `--ci` or other flags | MR-018's, not stubbed (D-220) |
| Auto-fix / amend | never; verdicts only |

## 3. Package plan

| Package | Owns | Deliberately not there |
|---|---|---|
| `internal/cli` (+files) | `verify` (staged), `knowledge validate`, `hook install` commands following the invocation/envelope pattern | other commands |
| `internal/git` (+read) | staged enumeration + index/HEAD byte reads (neutral git plumbing) | evaluation |
| everything else | untouched (consumed as services) | — |

No migration, no codes. Staged reconciliation reuses the changes
service through a staged file-source adapter: worktree discovery reads
porcelain, staged discovery reads the index — same rows downstream.

## 4. Entity model

**Verify input.** `--staged` flag (required in 0.1; bare `verify`
without a mode refuses with usage + next action — no silent default).

**Verify flow.**
1. Knowledge validate first (fail-closed gate, D-217).
2. Enumerate staged entries (`git diff --cached --name-status -z`).
3. Read index bytes (`git show :path`) + HEAD bytes (missing → empty).
4. Reconcile-staged into change rows (D-215): file rows + symbol rows
   through the changes service with the staged file source.
5. Attribute (`EvaluateTask` per task with changes? — which tasks?
   tasks owning the staged files via baselines + a synthetic
   verify-task? Decision: evaluate every task with an open change after
   step 4 (bounded by change, never repo-wide).
6. Guard over staged test files (HEAD vs index bytes, mappings from
   bindings as in MR-016).
7. Gate over composed inputs (bindings/ambiguities from index,
   coverage required = ...? — required profiles for verify: which?
   Decision: required = profiles covering the staged scope? No profile→
   path mapping exists. Use: NO required profiles in 0.1 verify
   (evidence family silent); MR-010/011 prove evidence separately.
   Recorded, not guessed.)
8. Verdict → exit code + JSON/human report with denials.

**Knowledge flow.** `knowledge validate`: loader Load + validator
report; any fatal (corrupt/unreadable/newer) → non-zero with codes;
warnings listed, non-fatal.

**Hook flow.** `hook install`: read `.git/hooks/pre-commit` (missing →
create `#!/bin/sh` + block); block delimited by exact markers
(`# mindrail:begin` / `# mindrail:end`); present → byte-identical
no-op; absent → append (with preceding newline discipline). Block body:
`mindrail verify --staged` + exit propagation. Executable bit ensured.

## 5. Runtime schema

None.

## 6. Readiness, CLI and performance integration

CLI-native: cobra commands, invocation/envelope pattern, JSON + human
goldens where the suite demands. Performance: staged scope only. No
wall-clock assertions.

## 7. Secrets

None new. Diffs are source, not secrets; findings never echo contents
beyond names and counts (established pattern).

## 8. Outputs reserved for later milestones

- `verify --staged` — the pre-commit block calls it; MR-018's `--ci`
  extends the command.
- Hook installer — MR-018 documents CI-side equivalents.
- Staged file source — MR-018 reuses it for worktree-vs-HEAD ranges.

## 9. Test plan

Per tech-stack §90's categories, per task:

1. **Verify + knowledge** (TASK-01): staged ambiguity/denial, staged
   guard blocking, corrupt/newer fail-closed ordering, clean green,
   envelope exit codes, goldens.
2. **Hook + fixtures** (TASK-02): create/append/idempotent/foreign-lines
   preserved; block executes verify; clean/allow/denial git fixtures
   through real commands.
3. **Proof** (TASK-03): verify + tidy, non-goal greps, mutation ledger,
   Durum.
4. **Mutation ledger**: every guard gets its mutation run before it is
   written down (unchanged rule).
