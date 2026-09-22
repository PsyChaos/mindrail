-- MR-007 runtime schema: actual change discovery and attribution.
--
-- A Change is one task's durable answer to "what actually changed": files
-- with kinds and hashes, symbols with kinds, flags and uids. Rows accumulate
-- by natural key and never duplicate — the same discovery through the
-- baseline path and the reconcile path converges on the same rows, and a
-- retried operation answers from its log. Baselines are the declared truth
-- one before_change call replaces wholesale; the operation log is AC-15's
-- Change half, replaying same-id same-content requests.
--
-- 000001 through 000005 are not edited: the migrator checksums what it
-- applied, and an edit to any of them would report MIGRATION_CHECKSUM_MISMATCH
-- to every database that already holds it (migrations/README.md).
--
-- STRICT is on for the same reason it is on in 000001 through 000005: every
-- column here is written by exactly one code path, and a type surprise in the
-- runtime store is a bug, not a value to coerce.

CREATE TABLE changes (
    change_id    TEXT PRIMARY KEY,           -- "CHG-<26>", identity.NewID
    task_id      TEXT REFERENCES tasks(task_id),
    operation_id TEXT,
    created_at   TEXT NOT NULL,              -- RFC 3339, UTC
    updated_at   TEXT NOT NULL
) STRICT;

CREATE UNIQUE INDEX idx_changes_task_unique ON changes(task_id) WHERE task_id IS NOT NULL;
CREATE INDEX idx_changes_operation ON changes(operation_id) WHERE operation_id IS NOT NULL;

CREATE TABLE change_files (
    change_id      TEXT NOT NULL REFERENCES changes(change_id),
    path           TEXT NOT NULL,              -- absolute
    kind           TEXT NOT NULL CHECK (kind IN ('added', 'modified', 'deleted', 'renamed')),
    old_path       TEXT NOT NULL DEFAULT '',
    content_hash   TEXT NOT NULL DEFAULT '',
    discovered_via TEXT NOT NULL CHECK (discovered_via IN ('baseline', 'reconcile')),
    PRIMARY KEY (change_id, path)
) STRICT;

CREATE TABLE change_symbols (
    change_id         TEXT NOT NULL REFERENCES changes(change_id),
    logical_key       TEXT NOT NULL,
    symbol_uid        TEXT REFERENCES symbol_identities(symbol_uid),
    kind              TEXT NOT NULL CHECK (kind IN ('added', 'removed', 'modified')),
    body_changed      INTEGER NOT NULL DEFAULT 0,
    signature_changed INTEGER NOT NULL DEFAULT 0,
    structure_changed INTEGER NOT NULL DEFAULT 0,
    signature_hash    TEXT NOT NULL DEFAULT '',
    body_hash         TEXT NOT NULL DEFAULT '',
    structure_hash    TEXT NOT NULL DEFAULT '',
    discovered_via    TEXT NOT NULL CHECK (discovered_via IN ('baseline', 'reconcile')),
    PRIMARY KEY (change_id, logical_key)
) STRICT;

CREATE TABLE change_baselines (
    task_id      TEXT NOT NULL,
    path         TEXT NOT NULL,              -- absolute, caller scope
    content_hash TEXT NOT NULL DEFAULT '',
    captured_at  TEXT NOT NULL,              -- RFC 3339, UTC
    PRIMARY KEY (task_id, path)
) STRICT;

CREATE TABLE change_operations (
    operation_id TEXT PRIMARY KEY,
    task_id      TEXT,
    request_hash TEXT NOT NULL,
    result       TEXT NOT NULL,              -- JSON: change id + counts
    recorded_at  TEXT NOT NULL               -- RFC 3339, UTC
) STRICT;
