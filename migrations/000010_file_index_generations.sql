-- Index-owned generation tombstones prevent an old CAS observation from
-- becoming current again after a file is removed and recreated with the same
-- bytes. Live state/facts are still physically removed; only the monotonic
-- per-path generation survives. Shipped migrations remain immutable.
CREATE TABLE file_index_generations (
    path       TEXT PRIMARY KEY,
    generation INTEGER NOT NULL CHECK (generation >= 0)
) STRICT;
