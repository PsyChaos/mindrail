# MR-018 — Frozen requirements and acceptance criteria

- **Frozen at:** commit `c4e4295`, branch `mr-002-knowledge-lifecycle`
- **Contract this refines:** [mr-018-design.md](mr-018-design.md). Where the two
  disagree, the design wins on intent and this document wins on the detail the
  design left open — every such gap is recorded below as a decision D-221…D-228.
- **Task:** [mindrail-0.1-task-list.md](mindrail-0.1-task-list.md) MR-018
- **Tier:** 3 (Program). It adds the second verify mode (`--ci`), range-diff
  evaluation through the shared services, and the fresh-clone fixture harness
  the 0.1 exit condition relies on. No database schema, no wire codes, no
  portable manifest, no cross-run cache, no runtime task lease.
- **Blocked by:** MR-017 (reuses `verify --staged` composition, staged file
  source, hook installation as the bypassed control).

This file is frozen **before** implementation so that an audit has something
to grade against that the implementation did not shape. The milestone runs
**one task at a time** as MR-004 through MR-017 did: each task in §4 is
implemented, gated by an independent Reader/Breaker pair and recorded before
the next is begun.

---

## 0. Baseline evidence

Captured on the tree this work starts from, so a red suite later is unambiguous.

| Measurement | Value | Command |
|---|---|---|
| Commit | `c4e4295` | `git rev-parse --short HEAD` |
| Go toolchain | `go1.27.0 linux/amd64` | `go version` |
| Packages | 35 | `go list ./... \| wc -l` |
| Top-level test functions | 1260 | `go test -list '.*' ./... \| grep -c '^Test'` |
| `make verify` | green at MR-017 close | `make verify` |
| `make tidy-check` | green at MR-017 close | `make tidy-check` |
| Migrations | `000001_initial.sql` … `000008_evidence.sql`; runtime `schema_version` reports 8 | `ls migrations/*.sql` |
| Registered `app.Code` values | 46 | `grep -c 'Code = "' internal/app/code.go` |
| `verify --ci` | does not exist (`--staged` only) | CLI surface |

---

## 1. The scenario, in one paragraph

A developer's hook fires `verify --staged` and denies; the developer commits
anyway with `git commit --no-verify`. CI checks out the head commit fresh,
runs `mindrail verify --ci`, and reproduces the same denial from the same
services — knowledge validated first and fail-closed, the merge-base range
reconciled into change rows, the guard firing blocking on the ranged test
file, every reason named with code and remedy, non-zero exit. A clean range
verifies green. A corrupt knowledge record or an unresolvable base refuses
before any source check runs, with the failure named structurally.

---

## 2. Decisions

MR-017 ended at D-220. These continue the sequence.

### D-221 — `--ci` is a second mode of the same command, never both flags

`verify` accepts `--staged` or `--ci`, never both and never neither. Bare
`verify` keeps refusing with usage + next action (MR-017 rule, unchanged);
`--staged --ci` together refuses the same way. No silent default: the command
cannot know whether the index or a committed range is judged.

### D-222 — CI judges the merge-base range, head bytes against base bytes

The judged set is `merge-base(base, head)..head`. Before bytes come from the
merge-base SHA, after bytes from head — the committed truth on both sides,
never the worktree desk (D-216's principle extended to ranges). Knowledge
files are inside the range like any other file (§112: knowledge files are
part of the diff and invariant changes are reviewable changes).

### D-223 — base/head selection is explicit flags with a documented default

`--base` and `--head` are optional revs. `--head` defaults to `HEAD`.
`--base` defaults to the first resolvable of `origin/main`, `main`,
`master`; none resolving is a structured usage error naming the candidates
tried. `merge-base` resolution failure and unknown revs are structured
errors (existing codes only — D-227), never an empty diff certifying a
broken range clean.

### D-224 — knowledge validates first and fails closed, same as staged

Corrupt records and newer-than-binary schemas refuse the whole run with the
loader's codes before merge-base resolution or any source evaluation runs
(spec-1.0 §95 CI order; AC-16/AC-17). The malformed-JSON non-fatal semantics
from MR-017 AC-01.3 carry over unchanged — only fatal problems gate.

### D-225 — CI reuses the staged composition and the same gate input shape

Attribution (global rule, D-133), drift, guard (before = base bytes, after =
head bytes), bindings with knowledge severity, ambiguities — all composed
exactly as `VerifyStaged` does, over the same `changes`/`index`/`testguard`/
`gate` services. A denial reproducible locally must reproduce byte-identical
in code and provenance under CI; divergence between the two paths is a bug.

### D-226 — no required validation profiles are executed in 0.1 CI

The kernel-scope pipeline names a "required validation profiles" step, but
no profile→scope mapping exists in 0.1 (MR-017 design §4 recorded the same
gap for staged). The step therefore resolves to the empty required set and
the evidence family stays silent — recorded, not guessed. CI *may* re-run
validations per spec-1.0 §74, but 0.1 does not; executing profiles without a
mapping would invent policy. MR-010/011 prove evidence separately.

### D-227 — no new `app.Code` values, no migration, no manifest/cache/lease

Registry stays 46; range/base failures reuse existing command-line/git
codes. No migration (schema stays v8). No portable manifest, no cross-run
impact cache, no runtime task lease (kernel-scope §Git enforcement,
spec-1.0 §73/§76). CI recomputes; the NULL-task change converges repeat
runs onto one row via the fixed `"verify-ci"` operation id (D-215's
principle).

### D-228 — serial tasks, Reader/Breaker gates, findings rows

The MR-004…MR-017 process repeats: one task at a time, each gated before
the next, each recorded in `mr-018-findings.md` (new file — MR-017's record
stays closed). Post-gate hygiene (`git status`/`git diff` on tracked
paths, unique mutant-backup names, md5-verified restore + full-suite
re-run) applies to every subagent.

---

## 3. Requirements and acceptance criteria

REQ-01…REQ-07. Each task's criteria are dotted under it, in the form the
gates grade. A criterion not met at the end of its task is recorded as not
met, with the reason, in `mr-018-findings.md`.

**REQ-01 — verify --ci range evaluation** (merge-base diff through shared services).
**REQ-02 — base/head + merge-base errors** (structured, never empty-green).
**REQ-03 — service parity with staged** (same policy/domain path, reproducible denials).
**REQ-04 — knowledge-first fail-closed** (fatal before source).
**REQ-05 — fresh-clone fixtures** (clean/denial/guard/knowledge scenarios).
**REQ-06 — non-goals, guarded**.
**REQ-07 — evidence discipline** (every new guard mutation-red).

### TASK-01 — range plumbing + `verify --ci` core

Owns REQ-01, REQ-02, REQ-03, REQ-04.

- **AC-01.1** `verify --ci --base <rev> --head <rev>` reconciles the
  merge-base range into change rows and denies blocking findings
  (attribution/guard/binding/ambiguity/drift) through the shared gate;
  a clean range exits 0 with empty denials.
- **AC-01.2** base/head selection follows D-223: defaults resolve
  deterministically; unresolvable base, unknown rev, and merge-base
  failure refuse with structured code + remedy and never evaluate source.
- **AC-01.3** `--staged` + `--ci` together and bare `verify` refuse with
  usage + next action (D-221); stray positionals refuse (MR-017 rule).
- **AC-01.4** fatal knowledge (corrupt/newer-schema) fails closed with
  loader codes before merge-base resolution or any source check (D-224).
- **AC-01.5** a denial produced by `verify --staged` on the same content
  reproduces under `verify --ci` with identical code + provenance
  (parity probe, D-225).

### TASK-02 — fresh-clone fixtures + `--no-verify` reproduction

Owns REQ-05 (REQ-03/REQ-04 exercised through fixtures).

- **AC-02.1** fixtures build real repos (no git mocking, tech-stack §92):
  clean range greens; a `--no-verify` commit carrying staged-denied
  content denies under `verify --ci` in a fresh clone.
- **AC-02.2** ranged test weakening (base vs head bytes) denies blocking
  through the shared guard (AC-34 on the CI path).
- **AC-02.3** corrupt/newer-schema knowledge in the fresh clone fails
  closed before source checks (AC-16/AC-17 on the CI path).
- **AC-02.4** every denial in the fixtures carries code + provenance +
  remedy; exit codes follow the envelope table (no new codes, D-227).

### TASK-03 — proof, non-goals and the record

Owns REQ-06, REQ-07.

- **AC-03.1** full-suite green evidence: `make verify`, `make tidy-check`,
  test count by the single command.
- **AC-03.2** the non-goals hold under grep: registry still 46, no
  migration beyond `000008`, no manifest/cache/lease/task-lease symbols
  in the new path, no profile execution in CI, worktree bytes unread on
  the CI path.
- **AC-03.3** every new guard added by TASK-01…02 has its mutation run and
  recorded red (REQ-07), and the task list's MR-018 entry carries a
  `#### Durum` block written the way MR-017's is.

---

## 4. Work breakdown and dependency order

Serialised: each task is gated before the next is written.

| Task | Owns | REQ | Depends on |
|---|---|---|---|
| TASK-01 | range plumbing, `verify --ci` core, parity probe | REQ-01, REQ-02, REQ-03, REQ-04 | — |
| TASK-02 | fresh-clone fixtures, `--no-verify` reproduction | REQ-05 (+ REQ-03/04 via fixtures) | TASK-01 |
| TASK-03 | proof, non-goals, record, Durum block | REQ-06, REQ-07 | TASK-02 |

---

## 5. Definition of done

1. Every acceptance criterion above is met, or is recorded as not met with
   the reason in `mr-018-findings.md`.
2. `make verify` green, `make tidy-check` green, and the test count reported
   with `go test -list '.*' ./... | grep -c '^Test'` — that command and no
   other.
3. Every new guard has a recorded mutation that turns it red, recorded after
   it was run.
4. Every task in §4 has passed its Reader/Breaker gate before the next was
   begun, and the gate's findings — confirmed, refuted, unconfirmed — are in
   `mr-018-findings.md` per task, with what was done about each.
5. The task list's MR-018 entry carries a `#### Durum` block written the way
   MR-017's is.
6. The knowledge graph is refreshed in its own commit at the end.

---

## 6. Traceability to the task list's five acceptance criteria

| Task list AC | Where it is owned |
|---|---|
| Base/head seçimi ve merge-base hataları yapılandırılmış raporlanır | AC-01.2 (REQ-02) |
| `--no-verify` ile atlanan denial CI'da yeniden üretilir | AC-01.5, AC-02.1 (REQ-03, REQ-05) |
| CI staged/local yol ile aynı servisleri kullanır | AC-01.5 (REQ-03) |
| Bozuk/incompatible knowledge en erken aşamada fail-closed | AC-01.4, AC-02.3 (REQ-04, REQ-05) |
| Fresh-clone fixture tüm akışı doğrular | AC-02.1…02.4 (REQ-05) |
