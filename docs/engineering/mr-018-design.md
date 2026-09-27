# MR-018 — Design

- **Frozen at:** commit `c4e4295`, before any implementation commit.
- **Refines and is refined by:** [mr-018-requirements.md](mr-018-requirements.md)
  (D-221…D-228 record every place this design left a gap the implementation
  would otherwise fill silently).
- **Specification anchors:** kernel-scope §Git enforcement (CI pipeline
  order, no lease/manifest/cache); spec-1.0 §73 (CI rebuilds knowledge, no
  runtime task lease), §74 (base→HEAD diff, knowledge in diff), §76
  (manifest is optimization-only when CI recomputes), §95 (knowledge first),
  §112 (merge-base awareness); AC-16/AC-17/AC-34 (corrupt/schema/guard on
  the CI path); tech-stack §92 (real-git fixtures).

## 1. What the milestone is, in one paragraph

The bypass-proof half of commit-time enforcement: after MR-018, a commit
that dodged the local hook with `--no-verify` meets the same judgment in a
fresh clone — same services, same codes, same provenances — over the
merge-base range, with knowledge failing closed before a single source byte
is read.

## 2. What it is not

| Temptation | Ruling |
|---|---|
| Worktree bytes on the CI path | never read; both sides are committed revs (D-222) |
| Executing validation profiles in CI | empty required set in 0.1 (D-226) |
| Portable manifest / cross-run cache / task lease | not built (D-227) |
| New error codes for range failures | existing codes reused (D-227) |
| Auto-fix / amend / rebase help | never; verdicts only |
| `--staged` behaviour change | untouched; parity is one-directional (CI matches staged) |

## 3. Package plan

| Package | Owns | Deliberately not there |
|---|---|---|
| `internal/cli` (+files) | `--ci`, `--base`, `--head` flags; range verdict emission through the invocation/envelope pattern | staged logic (untouched) |
| `internal/git` (+range) | base/head resolution, `merge-base` computation, range enumeration (`diff --name-status -z base...head` shape), rev byte reads (`git show rev:path`) | evaluation |
| `internal/changes` (+range) | `ReconcileRange` over the range file source: same rows downstream as staged/worktree (D-215's family), NULL-task change via fixed `"verify-ci"` op id | new classifications |
| `internal/verify` (+ci) | `VerifyCI`: knowledge-first, reconcile-range, global attribution, ranged guard (base vs head bytes), bindings/ambiguities/drift, gate — mirroring `VerifyStaged` step for step (D-225) | profile execution |
| everything else | untouched (consumed as services) | — |

No migration, no codes. The range file source is the third member of the
family worktree → staged → range: discovery reads the range, before bytes
read the merge-base SHA, after bytes read head.

## 4. Entity model

**Verify input.** Mode flags exclusive (`--staged` xor `--ci`, D-221);
`--base`/`--head` accepted only with `--ci` (otherwise usage refusal).
`--base` default: first resolvable of `origin/main`, `main`, `master`
(D-223); `--head` default `HEAD`.

**Verify flow.**
1. Knowledge validate first (fail-closed gate, D-224) — before even
   resolving revs, so a corrupt clone never spends git answers on lies.
2. Resolve head rev → SHA; resolve base rev (flag or default chain) →
   SHA; compute `merge-base(baseSHA, headSHA)`; any failure → structured
   error with the tried candidates named (D-223).
3. Enumerate range entries (merge-base..head name-status).
4. Read base bytes (missing → empty) + head bytes (missing → empty).
5. Reconcile-range into change rows (NULL-task, `"verify-ci"` op id).
6. Attribute globally (same D-133 rule as staged — which tasks? same
   answer: every open change's baselines + NULL rows, never repo-wide).
7. Guard over ranged test files (before = base bytes, after = head
   bytes, mappings from bindings as in staged).
8. Gate over composed inputs (required profiles: none in 0.1 — which?
   Decision: empty set, evidence family silent, D-226. Recorded, not
   guessed.)
9. Verdict → exit code + JSON/human report with denials (same report
   shape as staged; same envelope table, no new codes).

**Range flow (git).** Resolve → merge-base → enumerate → per-path
`show`. A git that cannot answer is an error at every step, never an
empty diff (staged rule extended: an empty diff must never certify a
broken range clean).

**Fresh-clone flow (fixtures).** Real repos via the git fixture helper:
`init` → commit base → branch/edit → commit head (with `--no-verify`
spelling where the scenario demands) → fresh `clone` → `init` in the
clone → `verify --ci` → verdict asserted. Clean/denial/guard/knowledge
scenarios each get one.

## 5. Runtime schema

None.

## 6. Readiness, CLI and performance integration

CLI-native: cobra flags, invocation/envelope pattern, JSON + human
goldens where the suite demands. Performance: range scope only (changed
paths, never repo-wide). No wall-clock assertions.

## 7. Secrets

None new. Range diffs are source, not secrets; findings never echo
contents beyond names and counts (established pattern).

## 8. Outputs reserved for later milestones

- `verify --ci` — MR-019 measures it; MR-020 demos the bypass story.
- Default-base chain — revisited if CI ergonomics demand config.
- Profile execution in CI — needs the profile→scope mapping that does
  not exist in 0.1 (D-226).
- Portable manifest / cross-run cache — optimization only (spec-1.0 §76).

## 9. Test plan

Per tech-stack §90's categories, per task:

1. **Range + core** (TASK-01): clean green, ranged denial, default-base
   chain, bad-rev/merge-base structured refusals, mode-exclusion
   (`--staged --ci`, bare, stray args), knowledge-first ordering,
   staged↔CI parity probe, envelope exit codes, goldens.
2. **Fixtures** (TASK-02): fresh-clone clean/allow/denial through real
   commands; `--no-verify` reproduction; ranged guard blocking;
   corrupt/newer-schema fail-closed in clone.
3. **Proof** (TASK-03): verify + tidy, non-goal greps, mutation ledger,
   Durum.
4. **Mutation ledger**: every guard gets its mutation run before it is
   written down (unchanged rule).
