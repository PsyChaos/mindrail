package index

import (
	"context"
	"database/sql"
	"encoding/hex"
	"errors"
	"strings"

	"github.com/PsyChaos/mindrail/internal/app"
	"github.com/PsyChaos/mindrail/internal/storage"
)

// FileStateObservation carries a readable state and an opaque copy of every
// persisted cell used by the CAS. Capture it before reading source bytes.
// Callers may inspect State, but changing it cannot forge the private token.
type FileStateObservation struct {
	State  FileIndexState
	Exists bool
	token  fileStateToken
}

type fileStateToken struct {
	exists      bool
	path        string
	unitID      string
	language    string
	contentHash sql.NullString
	state       FileState
	attempts    int
	lastError   sql.NullString
	indexedAt   sql.NullString
}

type stateRowQuerier interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

// deepestUnitOwnsFile excludes a stale parent registration after inventory has
// discovered a nested unit. The check runs in the same transaction as the
// state CAS, so a competing unit registration cannot interleave with it.
func deepestUnitOwnsFile(ctx context.Context, tx *sql.Tx, unitID, path string) error {
	if err := fileBelongsToUnit(ctx, tx, unitID, path); err != nil {
		return err
	}
	rows, err := tx.QueryContext(ctx, `SELECT id, path FROM project_units`)
	if err != nil {
		return err
	}
	defer rows.Close()
	ownerID, ownerRoot := "", ""
	for rows.Next() {
		var id, root string
		if err := rows.Scan(&id, &root); err != nil {
			return err
		}
		if root != path && pathInRoot(root, path) && len(root) > len(ownerRoot) {
			ownerID, ownerRoot = id, root
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if ownerID != unitID {
		return invalidInput("file belongs to a more specific project unit")
	}
	return nil
}

func readFileStateToken(ctx context.Context, db stateRowQuerier, path string) (fileStateToken, error) {
	token := fileStateToken{path: path}
	err := db.QueryRowContext(ctx, `SELECT unit_id, language, content_hash, state, attempts, last_error, indexed_at
		FROM file_index_state WHERE path = ?`, path).
		Scan(&token.unitID, &token.language, &token.contentHash, &token.state, &token.attempts, &token.lastError, &token.indexedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return token, nil
	}
	if err != nil {
		return fileStateToken{}, err
	}
	token.exists = true
	return token, nil
}

func (token fileStateToken) observation() (FileStateObservation, error) {
	state := FileIndexState{Path: token.path}
	if token.exists {
		state.UnitID = token.unitID
		state.Language = token.language
		state.ContentHash = token.contentHash.String
		state.State = token.state
		state.Attempts = token.attempts
		state.LastError = token.lastError.String
		if token.indexedAt.Valid {
			stamped, err := app.ParseTime(token.indexedAt.String)
			if err != nil {
				return FileStateObservation{}, err
			}
			state.IndexedAt = &stamped
		}
	}
	return FileStateObservation{State: state, Exists: token.exists, token: token}, nil
}

// ReadFileState reads the durable state without changing it. Missing rows are
// represented by Exists=false and still carry a path-bound CAS token.
func (s *Store) ReadFileState(ctx context.Context, path string) (FileStateObservation, error) {
	if !isCleanAbsolutePath(path) {
		return FileStateObservation{}, invalidInput("file path must be clean and absolute")
	}
	if err := s.requireSchema(ctx); err != nil {
		return FileStateObservation{}, err
	}
	token, err := readFileStateToken(ctx, s.db, path)
	if err != nil {
		return FileStateObservation{}, corruptState(err)
	}
	observed, err := token.observation()
	if err != nil {
		return FileStateObservation{}, corruptState(err)
	}
	return observed, nil
}

func validContentHash(hash string) bool {
	if len(hash) != 64 || strings.ToLower(hash) != hash {
		return false
	}
	_, err := hex.DecodeString(hash)
	return err == nil
}

// RegisterFileCAS records the hash of the bytes this attempt intends to
// parse. A pending row therefore names desired bytes; indexed/failed rows
// name the bytes actually parsed. attempts is the monotonic per-file
// registration generation, also used by completion CAS to reject ABA races.
// An unchanged indexed hash is never re-registered. A failed same-hash row
// is re-registered for retry with a new generation.
func (s *Store) RegisterFileCAS(ctx context.Context, expected FileStateObservation, unitID, path, language, targetHash string) (FileStateObservation, bool, error) {
	if unitID == "" || !isCleanAbsolutePath(path) || language == "" || !validContentHash(targetHash) || expected.token.path != path || expected.Exists != expected.token.exists {
		return FileStateObservation{}, false, invalidInput("registration needs a unit, path, language, SHA-256 hash and path-bound observation")
	}
	if err := s.requireSchemaVersion(ctx, generationSchemaVersion); err != nil {
		return FileStateObservation{}, false, err
	}
	var observed FileStateObservation
	applied := false
	err := storage.InTx(ctx, s.db, func(ctx context.Context, tx *sql.Tx) error {
		if err := deepestUnitOwnsFile(ctx, tx, unitID, path); err != nil {
			return err
		}
		current, err := readFileStateToken(ctx, tx, path)
		if err != nil {
			return err
		}
		if current != expected.token || current.exists && current.unitID == unitID && current.language == language && current.state == StateIndexed && current.contentHash.String == targetHash {
			observed, err = current.observation()
			return err
		}
		if current.exists && current.unitID != unitID {
			// Ownership changed to a nested unit. Old-unit facts cannot remain.
			for _, table := range []string{"symbol_references", "symbol_imports", "symbols"} {
				if _, err := tx.ExecContext(ctx, "DELETE FROM "+table+" WHERE path = ? AND unit_id = ?", path, current.unitID); err != nil {
					return err
				}
			}
		}
		if current.exists {
			_, err = tx.ExecContext(ctx, `UPDATE file_index_state SET
				unit_id = ?, language = ?, content_hash = ?, state = 'pending', attempts = attempts + 1,
				last_error = NULL, indexed_at = NULL WHERE path = ?`, unitID, language, targetHash, path)
		} else {
			_, err = tx.ExecContext(ctx, `INSERT INTO file_index_state
				(path, unit_id, language, content_hash, state, attempts)
				VALUES (?, ?, ?, ?, 'pending', COALESCE((SELECT generation + 1 FROM file_index_generations WHERE path = ?), 1))`, path, unitID, language, targetHash, path)
		}
		if err != nil {
			return err
		}
		registered, err := readFileStateToken(ctx, tx, path)
		if err != nil {
			return err
		}
		observed, err = registered.observation()
		if err == nil {
			applied = true
		}
		return err
	})
	if err != nil {
		return FileStateObservation{}, false, writeFailure(ctx, s.db, "file registration", err)
	}
	return observed, applied, nil
}

// ReplaceFileFactsCAS verifies the exact registered generation and target
// hash inside the write transaction, before deleting any old facts. A stale
// completion is a no-op; callers may retry after observing newer state.
func (s *Store) ReplaceFileFactsCAS(ctx context.Context, registered FileStateObservation, facts FileFacts) (FileStateObservation, storage.TxStats, bool, error) {
	if registered.token.path != facts.Path || !registered.token.exists || registered.token.unitID != facts.UnitID || registered.token.language != facts.Language || registered.token.contentHash.String != facts.ContentHash || registered.token.state != StatePending {
		return FileStateObservation{}, storage.TxStats{}, false, invalidInput("completion does not match a registered pending file")
	}
	if err := validReplacement(facts); err != nil {
		return FileStateObservation{}, storage.TxStats{}, false, err
	}
	if err := s.requireSchema(ctx); err != nil {
		return FileStateObservation{}, storage.TxStats{}, false, err
	}
	var observed FileStateObservation
	applied := false
	stats, err := storage.InTxMeasured(ctx, s.db, func(ctx context.Context, tx *sql.Tx) error {
		if err := deepestUnitOwnsFile(ctx, tx, facts.UnitID, facts.Path); err != nil {
			return err
		}
		current, err := readFileStateToken(ctx, tx, facts.Path)
		if err != nil {
			return err
		}
		if current != registered.token {
			observed, err = current.observation()
			return err
		}
		if err := s.replaceFileFactsTx(ctx, tx, facts, 0); err != nil {
			return err
		}
		completed, err := readFileStateToken(ctx, tx, facts.Path)
		if err != nil {
			return err
		}
		observed, err = completed.observation()
		if err == nil {
			applied = true
		}
		return err
	})
	if err != nil {
		return FileStateObservation{}, storage.TxStats{}, false, writeFailure(ctx, s.db, "structural facts", err)
	}
	return observed, stats, applied, nil
}

// InvalidateFileCAS removes an indexed/failed completion and its facts only if
// its exact token is still current. It is used when a post-commit filesystem
// check can no longer identify readable target bytes. A newer registration is
// returned untouched, including its already-retained previous facts.
func (s *Store) InvalidateFileCAS(ctx context.Context, completed FileStateObservation) (FileStateObservation, bool, error) {
	if !completed.token.exists || !isCleanAbsolutePath(completed.token.path) || completed.token.state != StateIndexed && completed.token.state != StateFailed {
		return FileStateObservation{}, false, invalidInput("invalidation needs an indexed or failed completion token")
	}
	return s.RemoveFileCAS(ctx, completed)
}

// RemoveFileCAS retires an exact observed registration and all its live facts.
// Unlike completion invalidation, explicit removal also supports a canceled
// pending registration when the file has returned to its absent baseline.
// A stale observation never removes a newer generation; identities stay durable.
// The caller must establish that the filesystem path should no longer be indexed.
func (s *Store) RemoveFileCAS(ctx context.Context, completed FileStateObservation) (FileStateObservation, bool, error) {
	if !completed.token.exists || !completed.Exists || !isCleanAbsolutePath(completed.token.path) {
		return FileStateObservation{}, false, invalidInput("removal needs an existing path-bound observation")
	}
	if err := s.requireSchemaVersion(ctx, generationSchemaVersion); err != nil {
		return FileStateObservation{}, false, err
	}
	var observed FileStateObservation
	applied := false
	err := storage.InTx(ctx, s.db, func(ctx context.Context, tx *sql.Tx) error {
		current, err := readFileStateToken(ctx, tx, completed.token.path)
		if err != nil {
			return err
		}
		if current != completed.token {
			observed, err = current.observation()
			return err
		}
		// Retain the retired generation in the same transaction as deletion.
		// Recreating identical pending bytes must not recreate the old token.
		if _, err := tx.ExecContext(ctx, `INSERT INTO file_index_generations(path,generation) VALUES(?,?)
			ON CONFLICT(path) DO UPDATE SET generation=MAX(generation,excluded.generation)`, current.path, current.attempts); err != nil {
			return err
		}
		for _, table := range []string{"symbol_references", "symbol_imports", "symbols", "file_index_state"} {
			if _, err := tx.ExecContext(ctx, "DELETE FROM "+table+" WHERE unit_id = ? AND path = ?", current.unitID, current.path); err != nil {
				return err
			}
		}
		missing := fileStateToken{path: current.path}
		observed, err = missing.observation()
		if err == nil {
			applied = true
		}
		return err
	})
	if err != nil {
		return FileStateObservation{}, false, writeFailure(ctx, s.db, "file invalidation", err)
	}
	return observed, applied, nil
}
