# MR-010 — Frozen requirements and acceptance criteria

- **Frozen at:** commit `2444309`, branch `mr-002-knowledge-lifecycle`
- **Contract this refines:** [mr-010-design.md](mr-010-design.md). Where the two
  disagree, the design wins on intent and this document wins on the detail the
  design left open — every such gap is recorded below as a decision D-155…D-164.
- **Task:** [mindrail-0.1-task-list.md](mindrail-0.1-task-list.md) MR-010
- **Tier:** 3 (Program). It adds a runtime migration (one table), a new
  package (`internal/validation`), the execution core MR-011/MR-013 will
  consume, and config surface (validation profiles, secret env names) every
  repository will read. No wire vocabulary.

This file is frozen **before** implementation so that an audit has something
to grade against that the implementation did not shape. The milestone runs
**one task at a time** as MR-004 through MR-009 did: each task in §4 is
implemented, gated by an independent Reader/Breaker pair and recorded before
the next is begun.

---

## 0. Baseline evidence

Captured on the tree this work starts from, so a red suite later is unambiguous.

| Measurement | Value | Command |
|---|---|---|
| Commit | `2444309` | `git rev-parse --short HEAD` |
| Go toolchain | `go1.27.0 linux/amd64` | `go version` |
| Packages | 30 | `go list ./... \| wc -l` |
| Top-level test functions | 1154 | `go test -list '.*' ./... \| grep -c '^Test'` |
| `make verify` | green at MR-009 close | `make verify` |
| `make tidy-check` | green at MR-009 close | `make tidy-check` |
| Migrations | `000001_initial.sql` … `000007_scope_attribution.sql`; runtime `schema_version` reports 7 | `ls migrations/*.sql` |
| Registered `app.Code` values | 43 | `grep -c 'Code = "' internal/app/code.go` |
| Evidence records | do not exist (no table, no type) | schema |

---

## 1. The scenario, in one paragraph

A repository declares a `test` profile (`pytest -q`, over `tests/`) and
names `API_KEY` a secret-bearing env. The runner executes the profile's
argv directly — no shell — kills it on timeout, keeps the first 64KB per
stream, and normalizes pass/fail/timeout into a structured result. Before
storage, the exact leaked key becomes `[REDACTED]` and token-shaped output
is pattern-scrubbed; the original is kept nowhere. The evidence row binds
profile, command, redacted excerpt, snapshot hash, provenance and timestamp
together; retrying with the same operation id returns the stored row
instead of a duplicate.

---

## 2. Decisions

MR-009 ended at D-154. These continue the sequence.

### D-155 — evidence lives in the runtime database, not in knowledge

Knowledge record kinds stay `decision`/`invariant` (repo-owned). Validation
evidence is machine-observed runtime fact: one append-only `evidence`
table, written only by the runner, read by MR-011's invalidation and
MR-013's gate.

### D-156 — migration 000008 and version 8

`migrations/000008_evidence.sql` creates `evidence`;
`validation.TableSchemaVersion = 8`. The ledger's expected-columns list is
amended (D-73 forward, fifth time).

### D-157 — execution is argv-only with fixed bounds

`exec.Command(argv[0], argv[1:]...)` — a shell string never exists, not
even in tests. Context timeout kills the process group; 65536 bytes per
stream are kept with a truncation marker; empty argv and blank executable
are refused before spawn. Environment is inherited (the process must run)
but never stored (spec §78).

### D-158 — snapshot hashes profile scope, fail-closed

Snapshot = sha256 over sorted `relpath + NUL + content` for every regular
file under the profile's paths (dirs walked, symlinks not followed, newest
wins nothing — content addressing only). A missing path or an unreadable
file fails the run before execution: evidence never binds a partial scope.

### D-159 — redaction order follows spec §77 minus the sanitizer

Size limit → exact known-secret values → default patterns → bounded
excerpt → storage. The structured sanitizer step does not exist in 0.1
(deferred, recorded in findings); the original output is kept nowhere
(spec §79: no separate log even on false positives).

### D-160 — operation-id replay mirrors D-121

Same operation id + same request hash returns the stored row; same id +
different hash is a conflict error, never a second row. Empty operation id
runs without idempotency (like EnsureOpenChange's retroactive rows).

### D-161 — profiles and secret names live in config, patterns do not

`[validation.<profile>]` declares type, paths and argv commands;
`[secrets] env = [...]` names secret-bearing env vars. The pattern set is
a fixed default (bearer/private-key/token/db-url/password-kv); configurable
regex is deferred and recorded.

### D-162 — serial tasks, Reader/Breaker gates, findings rows

The MR-004…MR-009 process repeats: one task at a time, each gated before
the next, each recorded in `mr-010-findings.md` (new file — MR-009's record
stays closed).

### D-163 — no CLI/MCP surface, no new codes, no timings

Service, store and config only, driven by tests now and by milestone
drivers later (the D-91 pattern). Run outcomes are result values
(pass/fail/timeout/error), never blockers — the registry stays 43, pinned.
No wall-clock assertions; timeouts are behaviorally pinned with generous
margins, never exact durations.

### D-164 — evidence type is declared and validated, never defaulted

The profile declares its type from the spec §45 list; unknown types are
refused at config validation. No default classification is invented.

---

## 3. Requirements and acceptance criteria

REQ-01…REQ-07. Each task's criteria are dotted under it, in the form the
gates grade. A criterion not met at the end of its task is recorded as not
met, with the reason, in `mr-010-findings.md`.

**REQ-01 — config surface** (profiles, secret names, validation).
**REQ-02 — safe execution** (argv-only, timeout, bounds, normalization).
**REQ-03 — redaction** (exact values, default patterns, no originals).
**REQ-04 — snapshot + evidence store** (hash, append-only, op-id replay).
**REQ-05 — non-goals, guarded**.
**REQ-06 — evidence discipline** (every new guard mutation-red).
**REQ-07 — surface discipline** (no codes/commands/keys/timings beyond REQ-01).

### TASK-01 — config, runner, result normalization

Owns REQ-01, REQ-02.

- **AC-01.1** `[validation.<profile>]` parses type/paths/commands from
  TOML; unknown evidence types, empty profiles, empty argv and blank
  executables are refused at validation with named reasons.
- **AC-01.2** execution is argv-only: no shell exists in the spawn path
  (pinned by construction + test), working directory is the repo root,
  environment is inherited.
- **AC-01.3** timeout kills the run and normalizes to a structured
  `timeout` result; exit 0 → `pass`, non-zero → `fail`, spawn failure →
  `error` — each carrying exit code, durations are reported but never
  asserted exactly.
- **AC-01.4** stdout/stderr are bounded at 65536 bytes each with a
  truncation marker; the bound is pinned, not emergent.
- **AC-01.5** runner unit tests plus real-process integration tests
  (pass, fail, timeout, oversized output) run real binaries, never a
  shell.

### TASK-02 — redaction, snapshot, evidence store

Owns REQ-03, REQ-04.

- **AC-02.1** exact known-secret values (config-named envs, read at
  runtime) become `[REDACTED]` in stored output; values never reach any
  log, error or row besides the redaction itself.
- **AC-02.2** default patterns scrub token-shaped output (bearer, private
  key, api token, credentialed db url, password kv); the original output
  is kept nowhere — redacted text is canonical.
- **AC-02.3** snapshot hashes the profile scope deterministically
  (re-runs agree); missing/unreadable scope fails before execution.
- **AC-02.4** migration 000008 creates `evidence` and nothing else;
  `validation.TableSchemaVersion = 8`; the evidence row binds profile,
  type, command, result, redacted excerpt, snapshot hash, provenance,
  timestamp and operation id; same op + same hash replays the row, same
  op + different hash conflicts.
- **AC-02.5** end-to-end flow: profile → run → redact → snapshot → store,
  with a leaked secret redacted in the stored row and the snapshot hash
  reproducible from the tree.

### TASK-03 — proof, non-goals and the record

Owns REQ-05, REQ-06, REQ-07.

- **AC-03.1** full-suite green evidence: `make verify`, `make tidy-check`,
  test count by the single command.
- **AC-03.2** the non-goals hold under grep: no shell in the spawn path,
  no new codes (registry still 43), no env stored, knowledge kinds still
  decision/invariant only.
- **AC-03.3** every new guard added by TASK-01…02 has its mutation run and
  recorded red (REQ-06), and the task list's MR-010 entry carries a
  `#### Durum` block written the way MR-009's is.

---

## 4. Work breakdown and dependency order

Serialised: each task is gated before the next is written.

| Task | Owns | REQ | Depends on |
|---|---|---|---|
| TASK-01 | config profiles/secrets, argv runner, timeout/bounds/normalization | REQ-01, REQ-02 | — |
| TASK-02 | redaction, snapshot, migration 000008, evidence store, flow | REQ-03, REQ-04 | TASK-01 |
| TASK-03 | proof, non-goals, record, Durum block | REQ-05, REQ-06, REQ-07 | TASK-02 |

---

## 5. Definition of done

1. Every acceptance criterion above is met, or is recorded as not met with
   the reason in `mr-010-findings.md`.
2. `make verify` green, `make tidy-check` green, and the test count reported
   with `go test -list '.*' ./... | grep -c '^Test'` — that command and no
   other.
3. Every new guard has a recorded mutation that turns it red, recorded after
   it was run.
4. Every task in §4 has passed its Reader/Breaker gate before the next was
   begun, and the gate's findings — confirmed, refuted, unconfirmed — are in
   `mr-010-findings.md` per task, with what was done about each.
5. The task list's MR-010 entry carries a `#### Durum` block written the way
   MR-009's is.
6. The knowledge graph is refreshed in its own commit at the end.

---

## 6. Traceability to the task list's five acceptance criteria

| Task list AC | Where it is owned |
|---|---|
| Shell string yerine yalnızca argv tabanlı komut çalıştırılır | AC-01.2 (REQ-02) |
| Timeout ve çıktı sınırı deterministic, yapılandırılmış sonuç üretir | AC-01.3, AC-01.4 (REQ-02) |
| Evidence; profile, command sonucu, snapshot hash, provenance ve timestamp taşır | AC-02.4 (REQ-04) |
| Configured known secret plaintext olarak Evidence içine girmez | AC-02.1 (REQ-03) |
| Runner unit testleri ile gerçek process integration testleri bulunur | AC-01.5 (REQ-02) |
