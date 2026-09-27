package changes

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/PsyChaos/mindrail/internal/app"
	"github.com/PsyChaos/mindrail/internal/index"
	"github.com/PsyChaos/mindrail/internal/storage"
)

// restoreAutomaticFiles also visits the task's previous discoveries: restoring
// exact baseline bytes removes a path from Git's delta, not from the index.
// Keep those rows until every refresh succeeds so interrupted passes retry them.
// Only this task's current ownership rows are retired; completed task changes,
// baselines, operation journals and durable symbol identities remain historical.
func (s *Service) restoreAutomaticFiles(ctx context.Context, projectID, changeID string, entries []FileChange, units []index.ProjectUnit) error {
	prior, err := s.store.ReadChangeFiles(ctx, changeID)
	if err != nil {
		return err
	}
	active := make(map[string]bool, len(entries))
	for _, file := range entries {
		active[file.Path] = true
	}
	foreign := make(map[string]bool, len(s.automatic.ForeignPaths))
	for _, path := range s.automatic.ForeignPaths {
		foreign[path] = true
	}
	var restored []string
	for _, file := range prior {
		if active[file.Path] {
			continue
		}
		// Do not change an index currently owned by another run. Removing our
		// superseded attribution must not also overwrite their current bytes.
		if !foreign[file.Path] {
			if err := s.refreshAutomaticFile(ctx, projectID, file.Path, units); err != nil {
				return err
			}
		}
		restored = append(restored, file.Path)
	}
	if len(restored) == 0 {
		return nil
	}
	return s.store.retireAutomaticFiles(ctx, changeID, restored, active, units)
}

func (s *Service) refreshAutomaticFile(ctx context.Context, projectID, path string, units []index.ProjectUnit) error {
	unit, ok := s.unitFor(path, units)
	if !ok {
		return nil
	}
	observed, err := s.indexes.ReadFileState(ctx, path)
	if err != nil {
		return err
	}
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		if !observed.Exists {
			return nil
		}
		_, applied, err := s.indexes.RemoveFileCAS(ctx, observed)
		if err != nil {
			return err
		}
		if !applied {
			return fmt.Errorf("index changed while restoring %s; retry reconciliation", path)
		}
		return nil
	}
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("restored source %s is not a regular file", path)
	}
	result, err := s.indexer.IndexFile(ctx, projectID, unit, path)
	if payload, ok := app.PayloadOf(err); ok && payload.Code == app.CodeSyntaxLanguageUnsupported {
		return nil
	}
	if err != nil {
		return err
	}
	if result.Stale {
		return fmt.Errorf("source changed while restoring %s; retry reconciliation", path)
	}
	return nil
}

func (s *Store) retireAutomaticFiles(ctx context.Context, changeID string, restored []string, active map[string]bool, units []index.ProjectUnit) error {
	restoredPaths := make(map[string]bool, len(restored))
	for _, path := range restored {
		restoredPaths[path] = true
	}
	err := storage.InTx(ctx, s.db, func(ctx context.Context, tx *sql.Tx) error {
		// The key records the path at discovery, even when the UID has since
		// migrated to another name. Resolve its unit from the durable identity,
		// not from the live symbols which may already have disappeared.
		rows, err := tx.QueryContext(ctx, `SELECT c.logical_key, COALESCE(u.path,'')
			FROM change_symbols c LEFT JOIN symbol_identities i ON i.symbol_uid=c.symbol_uid
			LEFT JOIN project_units u ON u.id=i.unit_id WHERE c.change_id=?`, changeID)
		if err != nil {
			return err
		}
		var retired []string
		for rows.Next() {
			var key, root string
			if err := rows.Scan(&key, &root); err != nil {
				rows.Close()
				return err
			}
			var parts []string
			if err := json.Unmarshal([]byte(key), &parts); err != nil || len(parts) != 2 {
				rows.Close()
				return fmt.Errorf("invalid file-qualified change symbol key %q", key)
			}
			remove := len(active) == 0
			if root != "" {
				remove = remove || restoredPaths[filepath.Join(root, filepath.FromSlash(parts[0]))]
			} else {
				// Ambiguous symbols have no UID. Retain their blocking row if
				// any still-active unit/path could own the same qualified key.
				keep := false
				for _, unit := range units {
					path := filepath.Join(unit.Path, filepath.FromSlash(parts[0]))
					remove = remove || restoredPaths[path]
					keep = keep || active[path]
				}
				remove = remove && !keep
			}
			if remove {
				retired = append(retired, key)
			}
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return err
		}
		if err := rows.Close(); err != nil {
			return err
		}
		for _, key := range retired {
			if _, err := tx.ExecContext(ctx, `DELETE FROM change_symbols WHERE change_id=? AND logical_key=?`, changeID, key); err != nil {
				return err
			}
		}
		for _, path := range restored {
			if _, err := tx.ExecContext(ctx, `DELETE FROM change_files WHERE change_id=? AND path=?`, changeID, path); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return writeFailure(ctx, s.db, "restored automatic changes", err)
	}
	return nil
}
