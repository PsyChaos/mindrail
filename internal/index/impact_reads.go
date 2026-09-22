package index

import (
	"context"
	"database/sql"
)

// Referrer is one resolved reference edge into a symbol: who refers, from
// which file, with the words the extraction saw. MR-009's traversal reads
// these; it never re-resolves (decision D-146).
type Referrer struct {
	UnitID     string
	Path       string
	Key        string
	TargetText string
	Label      string
	Confidence float64
}

// ReferrersOfUID returns every resolved reference edge into one durable
// identity, in path/key order. Unresolved rows (NULL resolved_symbol_id)
// never appear here: cross-file name users ride the TASK-02 fallback, not
// this edge set.
func (s *Store) ReferrersOfUID(ctx context.Context, uid string) ([]Referrer, error) {
	if uid == "" {
		return nil, invalidInput("referrer listing needs a symbol uid")
	}
	if err := s.requireSchema(ctx); err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT r.unit_id, r.path, r.referrer_key,
		r.target_text, r.label, r.confidence
		FROM symbol_references r JOIN symbols s ON s.id = r.resolved_symbol_id
		WHERE s.symbol_uid = ? ORDER BY r.path, r.referrer_key`, uid)
	if err != nil {
		return nil, corruptState(err)
	}
	defer rows.Close()
	var out []Referrer
	for rows.Next() {
		var referrer Referrer
		if err := rows.Scan(&referrer.UnitID, &referrer.Path, &referrer.Key,
			&referrer.TargetText, &referrer.Label, &referrer.Confidence); err != nil {
			return nil, corruptState(err)
		}
		out = append(out, referrer)
	}
	if err := rows.Err(); err != nil {
		return nil, corruptState(err)
	}
	return out, nil
}

// UIDForKey resolves one file-scoped logical key to its durable identity.
// The key embeds its file (decision D-90), so unit, path and key name
// exactly one row; rows without an allocated uid resolve nothing, which
// stops a traversal climb by absence rather than by error.
func (s *Store) UIDForKey(ctx context.Context, unitID, path, key string) (string, bool, error) {
	if unitID == "" || path == "" || key == "" {
		return "", false, invalidInput("uid resolution needs a unit, a path and a key")
	}
	if err := s.requireSchema(ctx); err != nil {
		return "", false, err
	}
	var uid sql.NullString
	err := s.db.QueryRowContext(ctx, `SELECT symbol_uid FROM symbols
		WHERE unit_id = ? AND path = ? AND logical_key = ? ORDER BY id LIMIT 1`,
		unitID, path, key).Scan(&uid)
	if err != nil {
		if err == sql.ErrNoRows {
			return "", false, nil
		}
		return "", false, corruptState(err)
	}
	if !uid.Valid || uid.String == "" {
		return "", false, nil
	}
	return uid.String, true, nil
}

// FactsForUID resolves one durable identity to the fact row it lives on:
// key, name and path. Rows without a live fact resolve nothing, so callers
// describe the edge they rode rather than a target they cannot show.
func (s *Store) FactsForUID(ctx context.Context, uid string) (key, name, path string, found bool, err error) {
	if uid == "" {
		return "", "", "", false, nil
	}
	if err := s.requireSchema(ctx); err != nil {
		return "", "", "", false, err
	}
	err = s.db.QueryRowContext(ctx, `SELECT logical_key, name, path FROM symbols
		WHERE symbol_uid = ? ORDER BY id LIMIT 1`, uid).Scan(&key, &name, &path)
	if err != nil {
		if err == sql.ErrNoRows {
			return "", "", "", false, nil
		}
		return "", "", "", false, corruptState(err)
	}
	return key, name, path, true, nil
}

// KeyToUIDs resolves a logical key through the identities table, where the
// key is the mutable current one (decision D-94). Several rows may share a
// key across units: every uid is returned in uid order, and callers analyze
// all of them instead of choosing silently.
func (s *Store) KeyToUIDs(ctx context.Context, key string) ([]string, error) {
	if key == "" {
		return nil, invalidInput("uid resolution needs a key")
	}
	if err := s.requireSchema(ctx); err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT symbol_uid FROM symbol_identities
		WHERE logical_key = ? ORDER BY symbol_uid`, key)
	if err != nil {
		return nil, corruptState(err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var uid string
		if err := rows.Scan(&uid); err != nil {
			return nil, corruptState(err)
		}
		out = append(out, uid)
	}
	if err := rows.Err(); err != nil {
		return nil, corruptState(err)
	}
	return out, nil
}
