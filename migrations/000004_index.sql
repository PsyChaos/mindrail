-- MR-005 runtime schema: what structural indexing needs.
--
-- A project unit is one analysis root — a Python package, a TypeScript or
-- JavaScript package — that discovery found under the repository. The file
-- index state is one row per file under a unit, and its state column is the
-- queue's durable state: `pending` is work nobody finished, `indexed` is a
-- hash this binary has already parsed and need never parse again, `failed`
-- is a parse or extraction that produced something and complained, and
-- `unsupported` is a language no adapter in this binary carries, which is a
-- terminal answer and not work (decision D-81). Symbols, imports and
-- references are the structural facts one parse produced; an unchanged file
-- keeps them, a changed file has them replaced in one transaction.
--
-- 000001, 000002 and 000003 are not edited: the migrator checksums what it
-- applied, and an edit to any of them would report MIGRATION_CHECKSUM_MISMATCH
-- to every database that already holds it (migrations/README.md).
--
-- STRICT is on for the same reason it is on in 000001 through 000003: every
-- column here is written by exactly one code path, and a type surprise in the
-- runtime store is a bug, not a value to coerce.
--
-- symbols.logical_key is indexed and deliberately NOT unique (decision D-83):
-- it identifies a symbol until a rename or an overload makes two meanings
-- share one key, and durable identity (`symbol_uid`) is MR-006's allocation,
-- which will arrive as an ADD COLUMN. symbol_references.resolved_symbol_id is
-- deliberately nullable (decision D-87): an extraction pass without a resolver
-- can name a target and sometimes find exactly one same-key row to point at,
-- and pointing at nothing is a fact about the edge, not an error.

CREATE TABLE project_units (
    id            TEXT PRIMARY KEY,          -- "UNT-<26>", identity.NewID
    path          TEXT NOT NULL UNIQUE,      -- absolute, below the repo root
    kind          TEXT NOT NULL,             -- python | typescript | javascript
    discovered_at TEXT NOT NULL              -- RFC 3339, UTC
) STRICT;

CREATE TABLE file_index_state (
    path         TEXT PRIMARY KEY,           -- absolute, below the repo root
    unit_id      TEXT NOT NULL REFERENCES project_units(id),
    language     TEXT NOT NULL,              -- a registry entry name
    content_hash TEXT,                       -- NULL until first hashed
    state        TEXT NOT NULL CHECK (state IN ('pending', 'indexed', 'failed', 'unsupported')),
    attempts     INTEGER NOT NULL DEFAULT 0,
    last_error   TEXT,
    indexed_at   TEXT                        -- RFC 3339, UTC; NULL unless indexed or failed
) STRICT;

CREATE INDEX idx_file_state_unit ON file_index_state(unit_id, state);

CREATE TABLE symbols (
    id             INTEGER PRIMARY KEY,
    unit_id        TEXT NOT NULL REFERENCES project_units(id),
    path           TEXT NOT NULL,
    logical_key    TEXT NOT NULL,            -- path + kind + name + container; an index, not an identity (D-83)
    kind           TEXT NOT NULL,            -- function | method | class | variable
    name           TEXT NOT NULL,
    container      TEXT NOT NULL DEFAULT '',
    start_line     INTEGER NOT NULL,
    start_col      INTEGER NOT NULL,
    end_line       INTEGER NOT NULL,
    end_col        INTEGER NOT NULL,
    signature_hash TEXT NOT NULL,
    body_hash      TEXT NOT NULL,
    structure_hash TEXT NOT NULL
) STRICT;

CREATE INDEX idx_symbols_key ON symbols(unit_id, logical_key);
CREATE INDEX idx_symbols_path ON symbols(unit_id, path);

CREATE TABLE symbol_imports (
    id           INTEGER PRIMARY KEY,
    unit_id      TEXT NOT NULL REFERENCES project_units(id),
    path         TEXT NOT NULL,
    importer_key TEXT NOT NULL DEFAULT '',   -- the importing symbol's logical_key; '' is module level
    module       TEXT NOT NULL,
    names        TEXT NOT NULL,              -- JSON array of strings
    alias        TEXT NOT NULL DEFAULT '',
    is_relative  INTEGER NOT NULL            -- 0 or 1
) STRICT;

CREATE INDEX idx_imports_path ON symbol_imports(unit_id, path);

CREATE TABLE symbol_references (
    id                 INTEGER PRIMARY KEY,
    unit_id            TEXT NOT NULL REFERENCES project_units(id),
    path               TEXT NOT NULL,
    referrer_key       TEXT NOT NULL DEFAULT '',
    target_text        TEXT NOT NULL,        -- the identifier as written
    scope_text         TEXT NOT NULL DEFAULT '',
    label              TEXT NOT NULL DEFAULT 'STRUCTURAL_NAME_MATCH',
    confidence         REAL NOT NULL CHECK (confidence <= 0.5),
    resolved_symbol_id INTEGER REFERENCES symbols(id) ON DELETE SET NULL  -- nullable on purpose (D-87); a replaced symbol clears stale pointers (D-89)
) STRICT;

CREATE INDEX idx_refs_path ON symbol_references(unit_id, path);
CREATE INDEX idx_refs_resolved ON symbol_references(resolved_symbol_id)
    WHERE resolved_symbol_id IS NOT NULL;
