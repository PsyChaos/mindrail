-- MR-006 runtime schema: durable symbol identity, rename/move lineage and
-- invariant bindings.
--
-- symbols.logical_key is an index, not an identity (decision D-83): it names
-- a symbol until a rename or an overload makes two meanings share one key.
-- symbol_identities is the durable side — one row per identity lineage, keyed
-- by the first-observation allocation key (decision D-94) and rewritten, never
-- re-minted, by identity migration. invariant_symbol_bindings joins
-- repository-owned knowledge (INV-xxx record ids, schema v1, untouched) to
-- those identities, so a rename carries the relation without touching the
-- record. symbol_identity_ambiguities is the audit trail of every blocked
-- identity decision: candidates that later resolve do not delete the row.
--
-- 000001 through 000004 are not edited: the migrator checksums what it
-- applied, and an edit to any of them would report MIGRATION_CHECKSUM_MISMATCH
-- to every database that already holds it (migrations/README.md).
--
-- STRICT is on for the same reason it is on in 000001 through 000004: every
-- column here is written by exactly one code path, and a type surprise in the
-- runtime store is a bug, not a value to coerce.
--
-- symbols.symbol_uid arrives as an ADD COLUMN and stays nullable (decision
-- D-103): every row MR-005 wrote predates it, and backfill (decision D-100)
-- stamps those rows without reparse. The REFERENCES posture follows the
-- D-89 discussion: facts that lose their identity row are damage, not a
-- state to cascade into, so no ON DELETE clause quietly unlinks them.

CREATE TABLE symbol_identities (
    symbol_uid    TEXT PRIMARY KEY,           -- "SYM-<26>", identity.NewID
    project_id    TEXT NOT NULL,              -- workspace project scope (D-94)
    unit_id       TEXT NOT NULL REFERENCES project_units(id),
    language      TEXT NOT NULL,              -- a registry entry name
    logical_key   TEXT NOT NULL,              -- mutable current key; an index, not the identity (D-94)
    container_uid TEXT REFERENCES symbol_identities(symbol_uid),
    previous_keys TEXT NOT NULL DEFAULT '[]', -- abandoned keys, JSON array (D-96)
    created_at    TEXT NOT NULL               -- RFC 3339, UTC
) STRICT;

CREATE UNIQUE INDEX idx_identity_alloc
    ON symbol_identities(project_id, unit_id, language, logical_key);
CREATE INDEX idx_identity_unit_key ON symbol_identities(unit_id, logical_key);

CREATE TABLE invariant_symbol_bindings (
    invariant_id TEXT NOT NULL,               -- INV-xxxx, repo-owned record id (schema v1, never written here)
    symbol_uid   TEXT NOT NULL REFERENCES symbol_identities(symbol_uid),
    status       TEXT NOT NULL CHECK (status IN ('bound', 'ambiguous', 'orphaned')),
    reason       TEXT NOT NULL DEFAULT '',
    updated_at   TEXT NOT NULL                -- RFC 3339, UTC
) STRICT;

CREATE UNIQUE INDEX idx_binding_pair
    ON invariant_symbol_bindings(invariant_id, symbol_uid);

CREATE TABLE symbol_identity_ambiguities (
    id             INTEGER PRIMARY KEY,
    unit_id        TEXT NOT NULL REFERENCES project_units(id),
    removed_uid    TEXT NOT NULL REFERENCES symbol_identities(symbol_uid),
    removed_key    TEXT NOT NULL,
    candidate_keys TEXT NOT NULL,             -- JSON array of logical_key
    created_at     TEXT NOT NULL              -- RFC 3339, UTC
) STRICT;

ALTER TABLE symbols ADD COLUMN symbol_uid TEXT
    REFERENCES symbol_identities(symbol_uid);
