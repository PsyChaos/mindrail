package index

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/PsyChaos/mindrail/internal/app"
	"github.com/PsyChaos/mindrail/internal/identity"
	"github.com/PsyChaos/mindrail/internal/storage"
)

// Identity is one durable symbol lineage: the uid that survives renames and
// moves while logical_key follows the current name (decision D-94).
type Identity struct {
	UID          string
	ProjectID    string
	UnitID       string
	Language     string
	Key          string
	ContainerUID *string
	PreviousKeys []string
	CreatedAt    time.Time
}

// LookupIdentity reads the identity for an allocation key, if one was ever
// minted. The boolean is false, not an error, when nothing was allocated.
func (s *Store) LookupIdentity(ctx context.Context, projectID, unitID, language, key string) (Identity, bool, error) {
	if projectID == "" || unitID == "" || language == "" || key == "" {
		return Identity{}, false, invalidInput("identity lookup needs a project, unit, language and key")
	}
	if err := s.requireSchema(ctx); err != nil {
		return Identity{}, false, err
	}
	identity, err := queryIdentity(ctx, s.db, projectID, unitID, language, key)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Identity{}, false, nil
		}
		return Identity{}, false, corruptState(err)
	}
	return identity, true, nil
}

// MintIdentity allocates the key's uid exactly once per project (decision
// D-94, spec §24): INSERT ... ON CONFLICT DO NOTHING, then read back whoever
// won. A unique violation is not an error — it is the loser learning the
// winner's uid (decision D-109). Callers wanting migration-before-mint use
// EnsureIdentity in internal/index/symbol instead of calling this directly.
func (s *Store) MintIdentity(ctx context.Context, projectID, unitID, language, key string) (Identity, error) {
	if projectID == "" || unitID == "" || language == "" || key == "" {
		return Identity{}, invalidInput("identity allocation needs a project, unit, language and key")
	}
	if err := s.requireSchema(ctx); err != nil {
		return Identity{}, err
	}
	minted := Identity{
		UID:       identity.NewID("SYM"),
		ProjectID: projectID,
		UnitID:    unitID,
		Language:  language,
		Key:       key,
		CreatedAt: s.clock.Now(),
	}
	err := storage.InTx(ctx, s.db, func(ctx context.Context, tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `INSERT INTO symbol_identities
			(symbol_uid, project_id, unit_id, language, logical_key, created_at)
			VALUES (?, ?, ?, ?, ?, ?)
			ON CONFLICT(project_id, unit_id, language, logical_key) DO NOTHING`,
			minted.UID, projectID, unitID, language, key, app.FormatTime(minted.CreatedAt)); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return Identity{}, writeFailure(ctx, s.db, "symbol identity", err)
	}
	identity, err := queryIdentity(ctx, s.db, projectID, unitID, language, key)
	if err != nil {
		return Identity{}, corruptState(err)
	}
	return identity, nil
}

// ensureIdentityTx is the transaction-scoped lookup-or-mint the completion
// path uses so facts and uids commit atomically (decision D-95).
func (s *Store) ensureIdentityTx(ctx context.Context, tx *sql.Tx, projectID, unitID, language, key string) (string, error) {
	var uid string
	err := tx.QueryRowContext(ctx, `SELECT symbol_uid FROM symbol_identities
		WHERE project_id = ? AND unit_id = ? AND language = ? AND logical_key = ?`,
		projectID, unitID, language, key).Scan(&uid)
	if err == nil {
		return uid, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return "", err
	}
	minted := identity.NewID("SYM")
	if _, err := tx.ExecContext(ctx, `INSERT INTO symbol_identities
		(symbol_uid, project_id, unit_id, language, logical_key, created_at)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(project_id, unit_id, language, logical_key) DO NOTHING`,
		minted, projectID, unitID, language, key, app.FormatTime(s.clock.Now())); err != nil {
		return "", err
	}
	if err := tx.QueryRowContext(ctx, `SELECT symbol_uid FROM symbol_identities
		WHERE project_id = ? AND unit_id = ? AND language = ? AND logical_key = ?`,
		projectID, unitID, language, key).Scan(&uid); err != nil {
		return "", err
	}
	return uid, nil
}

type identityRowQuerier interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func queryIdentity(ctx context.Context, db identityRowQuerier, projectID, unitID, language, key string) (Identity, error) {
	var identity Identity
	var container sql.NullString
	var previous, created string
	err := db.QueryRowContext(ctx, `SELECT symbol_uid, project_id, unit_id, language, logical_key,
		container_uid, previous_keys, created_at FROM symbol_identities
		WHERE project_id = ? AND unit_id = ? AND language = ? AND logical_key = ?`,
		projectID, unitID, language, key).
		Scan(&identity.UID, &identity.ProjectID, &identity.UnitID, &identity.Language,
			&identity.Key, &container, &previous, &created)
	if err != nil {
		return Identity{}, err
	}
	if container.Valid {
		identity.ContainerUID = &container.String
	}
	if err := json.Unmarshal([]byte(previous), &identity.PreviousKeys); err != nil {
		return Identity{}, err
	}
	stamped, err := app.ParseTime(created)
	if err != nil {
		return Identity{}, err
	}
	identity.CreatedAt = stamped
	return identity, nil
}

// BackfillUnit stamps every uid-less symbol row of a unit from lookup-or-mint
// without reparse (decision D-100): every such row's key is present, so
// nothing disappeared and there is nothing to migrate. It returns how many
// rows gained a uid; a second run stamps zero.
func (s *Store) BackfillUnit(ctx context.Context, projectID, unitID string) (int64, error) {
	if projectID == "" || unitID == "" {
		return 0, invalidInput("identity backfill needs a project and a unit")
	}
	if err := s.requireSchema(ctx); err != nil {
		return 0, err
	}
	var stamped int64
	_, err := storage.InTxMeasured(ctx, s.db, func(ctx context.Context, tx *sql.Tx) error {
		// The language travels with the file row, not the symbol row, so the
		// key set is read through a join. A uid-less key with no file row is
		// damage, not work: backfill refuses it rather than minting into a
		// language it cannot name.
		rows, err := tx.QueryContext(ctx, `SELECT DISTINCT s.logical_key, f.language FROM symbols s
			JOIN file_index_state f ON f.unit_id = s.unit_id AND f.path = s.path
			WHERE s.unit_id = ? AND s.symbol_uid IS NULL ORDER BY s.logical_key`, unitID)
		if err != nil {
			return err
		}
		type keyLanguage struct {
			key      string
			language string
		}
		var keys []keyLanguage
		for rows.Next() {
			var entry keyLanguage
			if err := rows.Scan(&entry.key, &entry.language); err != nil {
				rows.Close()
				return err
			}
			if entry.language == "" {
				rows.Close()
				return invalidInput("backfill found a uid-less key with no language")
			}
			keys = append(keys, entry)
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return err
		}
		if err := rows.Close(); err != nil {
			return err
		}
		for _, entry := range keys {
			uid, err := s.ensureIdentityTx(ctx, tx, projectID, unitID, entry.language, entry.key)
			if err != nil {
				return err
			}
			result, err := tx.ExecContext(ctx, `UPDATE symbols SET symbol_uid = ?
				WHERE unit_id = ? AND logical_key = ? AND symbol_uid IS NULL`,
				uid, unitID, entry.key)
			if err != nil {
				return err
			}
			n, err := result.RowsAffected()
			if err != nil {
				return err
			}
			stamped += n
		}
		return nil
	})
	if err != nil {
		return 0, writeFailure(ctx, s.db, "identity backfill", err)
	}
	return stamped, nil
}
