# MR-010 — Design

- **Frozen at:** commit `2444309`, before any implementation commit.
- **Refines and is refined by:** [mr-010-requirements.md](mr-010-requirements.md)
  (D-155…D-164 record every place this design left a gap the implementation
  would otherwise fill silently).
- **Specification anchors:** spec-1.0 §45 (evidence fields), §48 (snapshot
  binding), §49 (profile whitelist), §§77–79 (redaction pipeline, known
  secrets, patterns, no separate log), §80 area (log retention deferred);
  AC-24 (secret redaction); kernel-scope §3 (STRUCTURAL) and §4 (deferred
  breadth); tech-stack §7 (layout).

## 1. What the milestone is, in one paragraph

The trustworthy-execution layer: after MR-010, a named profile runs as a
real process with no shell in the path, bounded in time and output, and its
result lands in an append-only row stapled to the exact source bytes it ran
against — secrets scrubbed before storage, retries deduplicated by
operation id. Every step is fail-closed: unknown profiles, empty argv,
missing scope and unknown types refuse before anything runs or stores.

## 2. What it is not

| Temptation | Ruling |
|---|---|
| Shell convenience (`sh -c`) | never — argv only, even in tests (D-157) |
| Configurable redaction regex | fixed default set; configurability deferred (D-161) |
| Structured sanitizer step | deferred per D-159; order otherwise follows §77 |
| Stale/invalidation logic | MR-011 reads these rows; this milestone writes them |
| Gate enforcement (ALLOW/DENY) | MR-013's; results are values, never blockers |
| CLI/MCP surface | drivers belong to later milestones (D-163) |

## 3. Package plan

| Package | Owns | Deliberately not there |
|---|---|---|
| `internal/validation` (new) | `Runner` (argv exec, timeout, bounds, normalization), `Redactor` (exact + patterns), `Snapshot` (scope hash), `Store` (evidence rows, op-id replay) | shell, policies |
| `internal/config` (+fields) | `[validation.<profile>]`, `[secrets]` + `Validate` rules | pattern config |
| `internal/app` | untouched (no codes — D-163) | — |

No other packages change. The `changes` op-id machinery is mirrored, not
reused — evidence rows have their own grain (one row per command run, not
per task).

## 4. Entity model

**Profile.** `{Name, Type, Paths []string (repo-relative), Commands [][]string (argv)}`.
Type ∈ spec §45 list; commands non-empty, every argv non-empty with a
blank-free executable.

**Result.** `{Status pass|fail|timeout|error, ExitCode int, Stdout, Stderr string (bounded, raw), TimedOut bool, ErrText string}`.
Redaction boundary: the runner returns raw bounded output; the store
applies redaction before insert (AC-02.1 owns storage-time). Tests pin both
halves — runner output raw-but-bounded, stored output redacted — and the
raw never persists beyond the calling frame.

**Snapshot.** `{Scope []string (abs paths), Hash string (hex sha256)}`.
Hash over sorted `rel + NUL + content` of regular files; dirs walked;
symlinks skipped (not followed); missing/unreadable → error before exec.

**Evidence.** `{ID EVD-<26>, Profile, Type, CommandIndex, Argv, Status, ExitCode, Output (redacted excerpt), SnapshotHash, Provenance (profile + command + snapshot scope), CreatedAt, OperationID}`.
One row per command run (profiles with N commands yield N rows, sharing
the snapshot). `operation_id` UNIQUE; `(operation_id, request_hash)` replay
per D-160.

## 5. Runtime schema — `migrations/000008_evidence.sql`

```sql
CREATE TABLE evidence (
  evidence_id  TEXT PRIMARY KEY,
  profile      TEXT NOT NULL,
  type         TEXT NOT NULL,
  command_argv TEXT NOT NULL,   -- JSON argv
  status       TEXT NOT NULL CHECK (status IN ('pass','fail','timeout','error')),
  exit_code    INTEGER NOT NULL,
  output       TEXT NOT NULL,   -- redacted excerpt, canonical
  snapshot_hash TEXT NOT NULL,
  provenance   TEXT NOT NULL,   -- JSON: profile, command index, scope
  created_at   TEXT NOT NULL,   -- RFC 3339, UTC
  operation_id TEXT UNIQUE,     -- NULL allowed: retroactive rows skip idempotency
  request_hash TEXT NOT NULL
) STRICT;
```

`validation.TableSchemaVersion = 8`. Request hash covers profile + type +
argv + snapshot hash (the identity of "the same run").

## 6. Execution flows

**Run(profile, root):** validate profile → snapshot scope (fail before
exec) → per command: spawn argv with timeout, inherit env, capture bounded
streams → normalize status. Raw output lives only in the result struct.

**Record(result, snapshot, opID):** redact exact values (config-named envs
read now) → patterns → excerpt bound → conflict-check op id → insert.
Refusals (unknown profile/type, empty argv, missing scope, op conflict)
happen before any spawn/store side effect they guard.

**Replay:** same op + same hash → stored row, no new write; same op +
different hash → conflict error naming the operation.

## 7. Secret inputs

`[secrets] env = ["API_KEY", ...]` names env vars whose live values are
exact-redacted. Values are read at record time from the process
environment, replaced with `[REDACTED]`, and never stored — not in rows,
not in errors, not in logs. The environment itself is never persisted.

## 8. Readiness, CLI and performance integration

None (D-163). Performance: snapshot walks profile scope only; output
bounded by construction. No wall-clock assertions; timeout tests use
generous margins (seconds-scale timeout vs millisecond sleeps inverted —
short sleeps with short timeouts, asserting status not duration).

## 9. Outputs reserved for later milestones

- Evidence rows — MR-011's invalidation input and MR-013's gate input.
- Snapshot hashes — MR-011's staleness comparator.
- Profiles — MR-015's tool input for named runs.

## 10. Test plan

Per tech-stack §90's categories, per task:

1. **Config/runner** (TASK-01): TOML profiles + secrets parse; refusal
   table (unknown type, empty argv, blank exe, unknown profile at run);
   real-process pass/fail/timeout/bound-excerpt tests with direct binaries;
   no-shell construction pin.
2. **Redaction/snapshot/store** (TASK-02): exact-value redaction (live env,
   never stored); pattern scrubbing per category; no-original anywhere
   (error paths included); snapshot determinism + missing-scope refusal;
   migration + ledger pins; op-id replay/conflict; full flow with leaked
   secret.
3. **Proof** (TASK-03): verify + tidy, non-goal greps, mutation ledger,
   Durum.
4. **Mutation ledger**: every guard gets its mutation run before it is
   written down (unchanged rule).
