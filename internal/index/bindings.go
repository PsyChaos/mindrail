package index

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"github.com/PsyChaos/mindrail/internal/app"
	"github.com/PsyChaos/mindrail/internal/storage"
)

// Binding statuses. Bound is the only quiet one: ambiguous and orphaned both
// carry findings, and block while an active HIGH/CRITICAL invariant points at
// them (decision D-98).
const (
	BindingBound     = "bound"
	BindingAmbiguous = "ambiguous"
	BindingOrphaned  = "orphaned"
)

// Binding joins one repository-owned invariant to one durable identity. The
// pair is the grain: a multi-target scope binds each uid, and one uid may
// serve several invariants.
type Binding struct {
	InvariantID string
	UID         string
	Status      string
	Reason      string
	UpdatedAt   time.Time
}

// ListSymbolsInFile returns every symbol row for one file, including uids.
// Callers walk declaration order only if they sort: storage order is row id.
func (s *Store) ListSymbolsInFile(ctx context.Context, unitID, path string) ([]Symbol, error) {
	if unitID == "" || path == "" {
		return nil, invalidInput("symbol listing needs a unit and a path")
	}
	if err := s.requireSchema(ctx); err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT logical_key, kind, name, container,
		start_line, start_col, end_line, end_col, signature_hash, body_hash, structure_hash, symbol_uid
		FROM symbols WHERE unit_id = ? AND path = ? ORDER BY id`, unitID, path)
	if err != nil {
		return nil, corruptState(err)
	}
	defer rows.Close()
	var symbols []Symbol
	for rows.Next() {
		var symbol Symbol
		var uid sql.NullString
		if err := rows.Scan(&symbol.LogicalKey, &symbol.Kind, &symbol.Name, &symbol.Container,
			&symbol.StartLine, &symbol.StartCol, &symbol.EndLine, &symbol.EndCol,
			&symbol.SignatureHash, &symbol.BodyHash, &symbol.StructureHash, &uid); err != nil {
			return nil, corruptState(err)
		}
		symbol.UID = uid.String
		symbols = append(symbols, symbol)
	}
	if err := rows.Err(); err != nil {
		return nil, corruptState(err)
	}
	return symbols, nil
}

// DistinctUIDsInFile returns the allocated uids with live rows in one file,
// in first-seen order. Unallocated (NULL) rows contribute nothing: callers
// that need the open-identity case read the rows, not this set.
func (s *Store) DistinctUIDsInFile(ctx context.Context, unitID, path string) ([]string, error) {
	if unitID == "" || path == "" {
		return nil, invalidInput("uid listing needs a unit and a path")
	}
	if err := s.requireSchema(ctx); err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT symbol_uid FROM symbols
		WHERE unit_id = ? AND path = ? AND symbol_uid IS NOT NULL GROUP BY symbol_uid ORDER BY min(id)`, unitID, path)
	if err != nil {
		return nil, corruptState(err)
	}
	defer rows.Close()
	var uids []string
	for rows.Next() {
		var uid string
		if err := rows.Scan(&uid); err != nil {
			return nil, corruptState(err)
		}
		uids = append(uids, uid)
	}
	if err := rows.Err(); err != nil {
		return nil, corruptState(err)
	}
	return uids, nil
}

// SymbolsPaths returns every path with live symbol rows in a unit, in path
// order. Prefix scopes (MODULE, PACKAGE) filter this set in Go: SQL LIKE
// cannot tell "%" the wildcard from "%" the path byte (the D-84 lesson).
func (s *Store) SymbolsPaths(ctx context.Context, unitID string) ([]string, error) {
	if unitID == "" {
		return nil, invalidInput("path listing needs a unit")
	}
	if err := s.requireSchema(ctx); err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT DISTINCT path FROM symbols
		WHERE unit_id = ? ORDER BY path`, unitID)
	if err != nil {
		return nil, corruptState(err)
	}
	defer rows.Close()
	var paths []string
	for rows.Next() {
		var path string
		if err := rows.Scan(&path); err != nil {
			return nil, corruptState(err)
		}
		paths = append(paths, path)
	}
	if err := rows.Err(); err != nil {
		return nil, corruptState(err)
	}
	return paths, nil
}

// UidHasLiveRows reports whether any symbol row still carries a uid. The
// binding refresh's sticky rule reads this, not the target text: a lineage
// with live rows keeps its bindings whatever the text now says.
func (s *Store) UidHasLiveRows(ctx context.Context, uid string) (bool, error) {
	if uid == "" {
		return false, invalidInput("liveness check needs a uid")
	}
	if err := s.requireSchema(ctx); err != nil {
		return false, err
	}
	var count int
	if err := s.db.QueryRowContext(ctx, `SELECT count(*) FROM symbols WHERE symbol_uid = ?`, uid).Scan(&count); err != nil {
		return false, corruptState(err)
	}
	return count > 0, nil
}

// Ambiguity names one blocked identity decision for a removed lineage.
type Ambiguity struct {
	ID            int64
	UnitID        string
	RemovedUID    string
	RemovedKey    string
	CandidateKeys []string
	CreatedAt     time.Time
}

// ListAmbiguitiesForUID returns the blocking decisions naming a removed
// lineage, newest first. Refresh consults them before orphaning: an
// explicitly undecided lineage is ambiguous, not vanished.
func (s *Store) ListAmbiguitiesForUID(ctx context.Context, uid string) ([]Ambiguity, error) {
	if uid == "" {
		return nil, invalidInput("ambiguity listing needs a uid")
	}
	if err := s.requireSchema(ctx); err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT id, unit_id, removed_uid, removed_key, candidate_keys, created_at
		FROM symbol_identity_ambiguities WHERE removed_uid = ? ORDER BY id DESC`, uid)
	if err != nil {
		return nil, corruptState(err)
	}
	defer rows.Close()
	var out []Ambiguity
	for rows.Next() {
		var ambiguity Ambiguity
		var candidates, created string
		if err := rows.Scan(&ambiguity.ID, &ambiguity.UnitID, &ambiguity.RemovedUID,
			&ambiguity.RemovedKey, &candidates, &created); err != nil {
			return nil, corruptState(err)
		}
		if err := json.Unmarshal([]byte(candidates), &ambiguity.CandidateKeys); err != nil {
			return nil, corruptState(err)
		}
		stamped, err := app.ParseTime(created)
		if err != nil {
			return nil, corruptState(err)
		}
		ambiguity.CreatedAt = stamped
		out = append(out, ambiguity)
	}
	if err := rows.Err(); err != nil {
		return nil, corruptState(err)
	}
	return out, nil
}

// UpsertBinding records one binding outcome.// UpsertBinding records one binding outcome. Re-resolution overwrites the
// previous status for the pair: the table describes the world now, while
// symbol_identity_ambiguities keeps the audit trail.
func (s *Store) UpsertBinding(ctx context.Context, invariantID, uid, status, reason string) error {
	if invariantID == "" || uid == "" {
		return invalidInput("binding needs an invariant and a uid")
	}
	switch status {
	case BindingBound, BindingAmbiguous, BindingOrphaned:
	default:
		return invalidInput("binding status must be bound, ambiguous or orphaned")
	}
	if err := s.requireSchema(ctx); err != nil {
		return err
	}
	err := storage.InTx(ctx, s.db, func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `INSERT INTO invariant_symbol_bindings
			(invariant_id, symbol_uid, status, reason, updated_at)
			VALUES (?, ?, ?, ?, ?)
			ON CONFLICT(invariant_id, symbol_uid) DO UPDATE SET
			status = excluded.status, reason = excluded.reason, updated_at = excluded.updated_at`,
			invariantID, uid, status, reason, app.FormatTime(s.clock.Now()))
		return err
	})
	if err != nil {
		return writeFailure(ctx, s.db, "identity binding", err)
	}
	return nil
}

// DeleteBindingsExcept drops an invariant's bindings outside the keep set and
// returns how many rows it removed. Refresh calls it after upserting the
// current outcomes so a binding the world stopped supporting cannot linger.
func (s *Store) DeleteBindingsExcept(ctx context.Context, invariantID string, keep []string) (int64, error) {
	if invariantID == "" {
		return 0, invalidInput("binding prune needs an invariant")
	}
	if err := s.requireSchema(ctx); err != nil {
		return 0, err
	}
	query := `DELETE FROM invariant_symbol_bindings WHERE invariant_id = ?`
	args := []any{invariantID}
	if len(keep) > 0 {
		query += ` AND symbol_uid NOT IN (`
		for i, uid := range keep {
			if i > 0 {
				query += `, `
			}
			query += `?`
			args = append(args, uid)
		}
		query += `)`
	}
	var removed int64
	err := storage.InTx(ctx, s.db, func(ctx context.Context, tx *sql.Tx) error {
		result, err := tx.ExecContext(ctx, query, args...)
		if err != nil {
			return err
		}
		removed, err = result.RowsAffected()
		return err
	})
	if err != nil {
		return 0, writeFailure(ctx, s.db, "identity binding prune", err)
	}
	return removed, nil
}

// ListBindingsForInvariant returns one invariant's bindings in uid order.
func (s *Store) ListBindingsForInvariant(ctx context.Context, invariantID string) ([]Binding, error) {
	if invariantID == "" {
		return nil, invalidInput("binding listing needs an invariant")
	}
	if err := s.requireSchema(ctx); err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT invariant_id, symbol_uid, status, reason, updated_at
		FROM invariant_symbol_bindings WHERE invariant_id = ? ORDER BY symbol_uid`, invariantID)
	if err != nil {
		return nil, corruptState(err)
	}
	defer rows.Close()
	var bindings []Binding
	for rows.Next() {
		var binding Binding
		var updated string
		if err := rows.Scan(&binding.InvariantID, &binding.UID, &binding.Status, &binding.Reason, &updated); err != nil {
			return nil, corruptState(err)
		}
		stamped, err := app.ParseTime(updated)
		if err != nil {
			return nil, corruptState(err)
		}
		binding.UpdatedAt = stamped
		bindings = append(bindings, binding)
	}
	if err := rows.Err(); err != nil {
		return nil, corruptState(err)
	}
	return bindings, nil
}
