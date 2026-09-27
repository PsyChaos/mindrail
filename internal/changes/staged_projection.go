package changes

import (
	"context"
	"database/sql"

	"github.com/PsyChaos/mindrail/internal/app"
	"github.com/PsyChaos/mindrail/internal/storage"
)

// replaceChangeProjection atomically publishes one complete materialized
// staged snapshot. All fallible discovery and symbol construction precedes it.
func (s *Store) replaceChangeProjection(ctx context.Context, changeID string, files []FileChange, symbols []SymbolChange) error {
	if changeID == "" {
		return invalidInput("projection replacement needs a target change")
	}
	if err := s.requireSchema(ctx); err != nil {
		return err
	}
	err := storage.InTx(ctx, s.db, func(ctx context.Context, tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `DELETE FROM change_symbols WHERE change_id=?`, changeID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM change_files WHERE change_id=?`, changeID); err != nil {
			return err
		}
		for _, row := range files {
			if row.Path == "" {
				return invalidInput("file row needs a path")
			}
			switch row.Kind {
			case FileAdded, FileModified, FileDeleted, FileRenamed:
			default:
				return invalidInput("file row kind must be added, modified, deleted or renamed")
			}
			if row.Via != ViaReconcile {
				return invalidInput("staged file row provenance must be reconcile")
			}
			if _, err := tx.ExecContext(ctx, `INSERT INTO change_files
				(change_id, path, kind, old_path, content_hash, discovered_via)
				VALUES (?, ?, ?, ?, ?, ?)`, changeID, row.Path, row.Kind, row.OldPath, row.Hash, row.Via); err != nil {
				return err
			}
		}
		for _, row := range symbols {
			if row.Key == "" {
				return invalidInput("symbol row needs a key")
			}
			switch row.Kind {
			case SymbolAdded, SymbolRemoved, SymbolModified:
			default:
				return invalidInput("symbol row kind must be added, removed or modified")
			}
			if row.Via != ViaReconcile {
				return invalidInput("staged symbol row provenance must be reconcile")
			}
			if _, err := tx.ExecContext(ctx, `INSERT INTO change_symbols
				(change_id, logical_key, symbol_uid, kind, body_changed, signature_changed,
				 structure_changed, signature_hash, body_hash, structure_hash, discovered_via)
				VALUES (?, ?, NULLIF(?, ''), ?, ?, ?, ?, ?, ?, ?, ?)`, changeID, row.Key, row.UID,
				row.Kind, boolInt(row.Body), boolInt(row.Signature), boolInt(row.Structure),
				row.SigHash, row.BodyHash, row.StructHash, row.Via); err != nil {
				return err
			}
		}
		result, err := tx.ExecContext(ctx, `UPDATE changes SET updated_at=? WHERE change_id=?`,
			app.FormatTime(s.clock.Now()), changeID)
		if err != nil {
			return err
		}
		affected, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if affected != 1 {
			return corruptState(sql.ErrNoRows)
		}
		return nil
	})
	if err != nil {
		return writeFailure(ctx, s.db, "staged projection replacement", err)
	}
	return nil
}
