-- MR-010 runtime schema: append-only validation evidence.
--
-- Evidence is machine-observed runtime fact: one row per command run, bound
-- to the source snapshot it ran against, with output already redacted. The
-- runner writes; MR-011's invalidation and MR-013's gate read. Rows are
-- never updated or deleted by any code path — correction is a new row.
--
-- 000001 through 000007 are not edited: the migrator checksums what it
-- applied, and an edit to any of them would report MIGRATION_CHECKSUM_MISMATCH
-- to every database that already holds it (migrations/README.md).
--
-- STRICT is on for the same reason it is on in 000001 through 000007: every
-- column here is written by exactly one code path, and a type surprise in the
-- runtime store is a bug, not a value to coerce.
--
-- operation_id is UNIQUE but nullable: a retry with the same id replays the
-- stored row (decision D-160), while an empty id runs without idempotency —
-- the retroactive pattern EnsureOpenChange uses for NULL-task changes.

CREATE TABLE evidence (
    evidence_id   TEXT PRIMARY KEY,           -- "EVD-<26>", identity.NewID
    profile       TEXT NOT NULL,
    type          TEXT NOT NULL,
    command_argv  TEXT NOT NULL,              -- JSON argv, redacted
    status        TEXT NOT NULL CHECK (status IN ('pass', 'fail', 'timeout', 'error')),
    exit_code     INTEGER NOT NULL,
    output        TEXT NOT NULL,              -- redacted excerpt, canonical (spec §79)
    snapshot_hash TEXT NOT NULL,              -- hex sha256 over the profile scope
    provenance    TEXT NOT NULL,              -- JSON: profile, command index, scope
    created_at    TEXT NOT NULL,              -- RFC 3339, UTC
    operation_id  TEXT UNIQUE,
    request_hash  TEXT NOT NULL               -- profile + type + argv + snapshot
) STRICT;
