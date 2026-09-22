-- MR-008 runtime schema: explicit attribution records.
--
-- Attribution answers which Change owns a symbol when baselines overlap or
-- are absent. The evaluation itself persists nowhere (decision D-136): it
-- recomputes deterministically from baselines, change rows and these
-- records, so CI verify re-runs it instead of trusting it. What persists is
-- the human decision — one row per logical key, superseded by rewriting —
-- so an ambiguity or unregistered finding can clear exactly where it was
-- pointed.
--
-- 000001 through 000006 are not edited: the migrator checksums what it
-- applied, and an edit to any of them would report MIGRATION_CHECKSUM_MISMATCH
-- to every database that already holds it (migrations/README.md).
--
-- STRICT is on for the same reason it is on in 000001 through 000006: every
-- column here is written by exactly one code path, and a type surprise in the
-- runtime store is a bug, not a value to coerce.

CREATE TABLE scope_attributions (
    logical_key TEXT PRIMARY KEY,           -- what was assigned
    change_id   TEXT NOT NULL REFERENCES changes(change_id),
    decided_by  TEXT NOT NULL,              -- session/agent id, never empty
    reason      TEXT NOT NULL DEFAULT '',
    decided_at  TEXT NOT NULL               -- RFC 3339, UTC
) STRICT;
