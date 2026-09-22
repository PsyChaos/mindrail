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

// NamedSymbol is one declaration sharing a name: the fallback's candidate
// set. Same-name declarations ambiguate instead of collapsing (decision
// D-154), so callers get every candidate with its file and uid.
type NamedSymbol struct {
	UnitID string
	Path   string
	Key    string
	Name   string
	UID    string
}

// SymbolsNamed returns every live declaration with one name, in
// unit/path/key order. Rows without an allocated uid contribute nothing:
// an unidentified declaration cannot candidate.
func (s *Store) SymbolsNamed(ctx context.Context, name string) ([]NamedSymbol, error) {
	if name == "" {
		return nil, invalidInput("name lookup needs a name")
	}
	if err := s.requireSchema(ctx); err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT unit_id, path, logical_key, name, symbol_uid
		FROM symbols WHERE name = ? AND symbol_uid IS NOT NULL AND symbol_uid != ''
		ORDER BY unit_id, path, logical_key`, name)
	if err != nil {
		return nil, corruptState(err)
	}
	defer rows.Close()
	var out []NamedSymbol
	for rows.Next() {
		var candidate NamedSymbol
		if err := rows.Scan(&candidate.UnitID, &candidate.Path, &candidate.Key,
			&candidate.Name, &candidate.UID); err != nil {
			return nil, corruptState(err)
		}
		out = append(out, candidate)
	}
	if err := rows.Err(); err != nil {
		return nil, corruptState(err)
	}
	return out, nil
}

// UnitForUID resolves one durable identity to its unit and file: the
// file/module fallback's floor. Rows without a live fact resolve nothing.
func (s *Store) UnitForUID(ctx context.Context, uid string) (unitID, path string, found bool, err error) {
	if uid == "" {
		return "", "", false, nil
	}
	if err := s.requireSchema(ctx); err != nil {
		return "", "", false, err
	}
	err = s.db.QueryRowContext(ctx, `SELECT unit_id, path FROM symbols
		WHERE symbol_uid = ? ORDER BY id LIMIT 1`, uid).Scan(&unitID, &path)
	if err != nil {
		if err == sql.ErrNoRows {
			return "", "", false, nil
		}
		return "", "", false, corruptState(err)
	}
	return unitID, path, true, nil
}

// BindingsForUID returns the bound invariant ids for one durable identity.
// Only bound rows count as scope: ambiguous and orphaned bindings are MR-006
// findings, not MR-009 scope (decision D-98).
func (s *Store) BindingsForUID(ctx context.Context, uid string) ([]string, error) {
	if uid == "" {
		return nil, invalidInput("binding listing needs a symbol uid")
	}
	if err := s.requireSchema(ctx); err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT invariant_id FROM invariant_symbol_bindings
		WHERE symbol_uid = ? AND status = ? ORDER BY invariant_id`, uid, BindingBound)
	if err != nil {
		return nil, corruptState(err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, corruptState(err)
		}
		out = append(out, id)
	}
	if err := rows.Err(); err != nil {
		return nil, corruptState(err)
	}
	return out, nil
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
