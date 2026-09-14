-- MR-004 runtime schema: what two agents working at the same time need.
--
-- A lease is one session holding one target — a task, or a repository-relative
-- file path — from an acquisition to a release. An operation is a mutation the
-- caller named with an operation_id, kept so that a second delivery of the same
-- request is answered from the first. A task gains a revision so that a move
-- decided on a stale reading of it is refused rather than applied.
--
-- 000001 and 000002 are not edited: the migrator checksums what it applied, and
-- an edit to either would report MIGRATION_CHECKSUM_MISMATCH to every database
-- that already holds it (decision D-53, migrations/README.md). Symbol leases,
-- Change rows and any revision outside tasks belong to MR-006 and MR-007.
--
-- STRICT is on for the same reason it is on in 000001 and 000002: every column
-- here is written by exactly one code path, and a type surprise in the runtime
-- store is a bug, not a value to coerce.

-- One row per tenure (decision D-64). A renewal moves renewed_at and
-- expires_at on the row; a release, an expiry noticed by the next acquirer, or a
-- terminal task move closes it by setting released_at and release_reason; a
-- takeover after expiry closes the old row and inserts a new one. The table is
-- therefore the lease history spec §59 keeps as events, and there is no second
-- table for them.
--
-- target_kind is 'task' or 'file' in MR-004 and target_key is the task id or
-- the cleaned repository-relative path. There is no foreign key on target_key
-- because the target is polymorphic; the holder is a session.
--
-- expires_at is TEXT like every timestamp in this schema and is never compared
-- in SQL: app.FormatTime trims trailing zeros, so the column is not
-- lexicographically ordered (decision D-59). Expiry is judged in Go against
-- the store's clock (decision D-65).
CREATE TABLE leases (
    lease_id       TEXT PRIMARY KEY,
    project_id     TEXT NOT NULL REFERENCES projects(project_id),
    target_kind    TEXT NOT NULL,
    target_key     TEXT NOT NULL,
    holder         TEXT NOT NULL REFERENCES sessions(session_id),
    acquired_at    TEXT NOT NULL,
    renewed_at     TEXT NOT NULL,
    expires_at     TEXT NOT NULL,
    released_at    TEXT,
    release_reason TEXT
) STRICT;

-- The guard itself: two unreleased rows cannot name one target in one project,
-- whatever the Go code does. The store reads the target's row inside its
-- transaction and refuses by name before it inserts, so a user meets
-- LEASE_CONFLICT with the holder and the expiry rather than a constraint
-- message — but the constraint is what holds even for a writer that never ran
-- the read. An expired row still counts as unreleased here; the acquirer that
-- takes over closes it in the same transaction that inserts its own.
CREATE UNIQUE INDEX idx_leases_active
    ON leases(project_id, target_kind, target_key)
    WHERE released_at IS NULL;

CREATE INDEX idx_leases_holder ON leases(holder);

-- An idempotent mutation's record (decision D-71): the caller-supplied id, the
-- command it named, a hash of the request as the store saw it, and the result
-- the command produced, as JSON. The row is inserted in the same transaction as
-- the write it records, after the write, so a refused operation records nothing
-- and a second delivery finds the first's row or nothing at all. There is no
-- retention in 0.1: one row per idempotent mutation.
CREATE TABLE operations (
    operation_id TEXT PRIMARY KEY,
    command      TEXT NOT NULL,
    request_hash TEXT NOT NULL,
    result       TEXT NOT NULL,
    recorded_at  TEXT NOT NULL
) STRICT;

-- Optimistic revision on the one mutable row MR-004 moves (decision D-72).
-- Every update of a task row adds one; a caller that read the task at revision
-- n and asks for a move expecting n is refused with STATE_REVISION_CONFLICT if
-- the row has moved on. ADD COLUMN rather than a recreate, because
-- checkpoints.task_id points a foreign key here and foreign_keys is on inside
-- this transaction; every row already written starts at 1 (decision D-73).
ALTER TABLE tasks ADD COLUMN revision INTEGER NOT NULL DEFAULT 1;
