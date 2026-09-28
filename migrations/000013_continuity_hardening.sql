ALTER TABLE agent_runtime_observations
ADD COLUMN producer_sequence INTEGER NOT NULL DEFAULT 0 CHECK (producer_sequence >= 0);

ALTER TABLE continuity_intents
ADD COLUMN activated_at TEXT;

CREATE TRIGGER continuity_phase_insert_guard
BEFORE INSERT ON continuity_intents
WHEN
    (NEW.state IN ('CHECKPOINTED','SPAWN_REQUESTED','SPAWN_READY','HANDED_OFF','CLAIMED','RESUMED','COMPLETED') AND NEW.checkpoint_id IS NULL) OR
	(NEW.state IN ('SPAWN_REQUESTED','SPAWN_READY','HANDED_OFF','CLAIMED','RESUMED','COMPLETED') AND NEW.takeover_token_hash IS NULL) OR
	(NEW.state IN ('SPAWN_READY','HANDED_OFF','CLAIMED','RESUMED','COMPLETED') AND NEW.host_operation_id IS NULL) OR
    (NEW.state IN ('CLAIMED','RESUMED','COMPLETED') AND NEW.successor_run_hash IS NULL) OR
    (NEW.state IN ('RESUMED','COMPLETED') AND NEW.successor_session_id IS NULL)
BEGIN
    SELECT RAISE(ABORT, 'invalid continuity phase fields');
END;

CREATE TRIGGER continuity_phase_update_guard
BEFORE UPDATE ON continuity_intents
WHEN
    (NEW.state IN ('CHECKPOINTED','SPAWN_REQUESTED','SPAWN_READY','HANDED_OFF','CLAIMED','RESUMED','COMPLETED') AND NEW.checkpoint_id IS NULL) OR
	(NEW.state IN ('SPAWN_REQUESTED','SPAWN_READY','HANDED_OFF','CLAIMED','RESUMED','COMPLETED') AND NEW.takeover_token_hash IS NULL) OR
	(NEW.state IN ('SPAWN_READY','HANDED_OFF','CLAIMED','RESUMED','COMPLETED') AND NEW.host_operation_id IS NULL) OR
    (NEW.state IN ('CLAIMED','RESUMED','COMPLETED') AND NEW.successor_run_hash IS NULL) OR
    (NEW.state IN ('RESUMED','COMPLETED') AND NEW.successor_session_id IS NULL)
BEGIN
    SELECT RAISE(ABORT, 'invalid continuity phase fields');
END;

-- Force every schema-12 row through the update guard. This is a no-op for a
-- valid row and aborts the migration transaction for an advanced-phase row
-- whose optional columns were legal in schema 12 but cannot satisfy schema 13.
UPDATE continuity_intents SET revision = revision;
