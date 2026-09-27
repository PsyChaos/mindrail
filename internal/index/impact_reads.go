package index

import (
	"context"
	"database/sql"
	"sort"
	"strings"

	"github.com/PsyChaos/mindrail/internal/app"
)

// GuardReference is one invariant-bound production identity referenced by a
// named test declaration in a changed file. The set-based read keeps commit
// guard cost proportional to changed test paths instead of all bindings.
type GuardReference struct {
	InvariantID   string
	ProductionUID string
	Path          string
	Test          string
}

// LogicalBinding is one invariant binding resolved through an identity's
// current or previous exact logical key.
type LogicalBinding struct {
	LogicalKey    string
	InvariantID   string
	ProductionUID string
}

// BindingsForLogicalKeys resolves exact current and historical identity keys
// in bounded chunks. Previous keys keep renamed baseline references stable.
func (s *Store) BindingsForLogicalKeys(ctx context.Context, keys []string) ([]LogicalBinding, error) {
	if err := s.requireSchema(ctx); err != nil {
		return nil, err
	}
	unique := map[string]bool{}
	for _, key := range keys {
		if key != "" {
			unique[key] = true
		}
	}
	ordered := make([]string, 0, len(unique))
	for key := range unique {
		ordered = append(ordered, key)
	}
	sort.Strings(ordered)
	var out []LogicalBinding
	const chunkSize = 100
	for start := 0; start < len(ordered); start += chunkSize {
		end := start + chunkSize
		if end > len(ordered) {
			end = len(ordered)
		}
		placeholders := make([]string, end-start)
		args := make([]any, 0, 2*(end-start))
		for _, key := range ordered[start:end] {
			placeholders[len(args)] = "?"
			args = append(args, key)
		}
		for _, key := range ordered[start:end] {
			args = append(args, key)
		}
		in := strings.Join(placeholders, ",")
		rows, err := s.db.QueryContext(ctx, `WITH matched(logical_key, symbol_uid) AS (
			SELECT logical_key, symbol_uid FROM symbol_identities WHERE logical_key IN (`+in+`)
			UNION
			SELECT previous.value, identities.symbol_uid
			FROM symbol_identities identities, json_each(identities.previous_keys) previous
			WHERE previous.value IN (`+in+`)
		)
		SELECT DISTINCT matched.logical_key, b.invariant_id, b.symbol_uid
		FROM matched JOIN invariant_symbol_bindings b ON b.symbol_uid = matched.symbol_uid
		ORDER BY matched.logical_key, b.invariant_id, b.symbol_uid`, args...)
		if err != nil {
			return nil, corruptState(err)
		}
		for rows.Next() {
			var row LogicalBinding
			if err := rows.Scan(&row.LogicalKey, &row.InvariantID, &row.ProductionUID); err != nil {
				rows.Close()
				return nil, corruptState(err)
			}
			out = append(out, row)
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return nil, corruptState(err)
		}
		if err := rows.Close(); err != nil {
			return nil, corruptState(err)
		}
	}
	return out, nil
}

// GuardReferencesInFiles returns guard mappings for exact test paths. Paths
// are queried in bounded chunks to stay below SQLite variable limits.
func (s *Store) GuardReferencesInFiles(ctx context.Context, paths []string) ([]GuardReference, error) {
	if err := s.requireSchema(ctx); err != nil {
		return nil, err
	}
	unique := map[string]bool{}
	for _, path := range paths {
		if path != "" {
			unique[path] = true
		}
	}
	ordered := make([]string, 0, len(unique))
	for path := range unique {
		ordered = append(ordered, path)
	}
	sort.Strings(ordered)
	var out []GuardReference
	const chunkSize = 200
	for start := 0; start < len(ordered); start += chunkSize {
		end := start + chunkSize
		if end > len(ordered) {
			end = len(ordered)
		}
		placeholders := make([]string, end-start)
		args := make([]any, end-start)
		for i, path := range ordered[start:end] {
			placeholders[i] = "?"
			args[i] = path
		}
		rows, err := s.db.QueryContext(ctx, `SELECT DISTINCT b.invariant_id, b.symbol_uid,
			r.path, ref.name
			FROM invariant_symbol_bindings b
			JOIN symbols target ON target.symbol_uid = b.symbol_uid
			JOIN symbol_references r ON r.resolved_symbol_id = target.id
			JOIN symbols ref ON ref.unit_id = r.unit_id AND ref.path = r.path
				AND ref.logical_key = r.referrer_key
			WHERE r.path IN (`+strings.Join(placeholders, ",")+`)
			ORDER BY b.invariant_id, b.symbol_uid, r.path, ref.name`, args...)
		if err != nil {
			return nil, corruptState(err)
		}
		for rows.Next() {
			var row GuardReference
			if err := rows.Scan(&row.InvariantID, &row.ProductionUID, &row.Path, &row.Test); err != nil {
				rows.Close()
				return nil, corruptState(err)
			}
			out = append(out, row)
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return nil, corruptState(err)
		}
		if err := rows.Close(); err != nil {
			return nil, corruptState(err)
		}
	}
	return out, nil
}

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

// UnresolvedReferringTo returns every unresolved reference row naming one
// identifier, in unit/path/key order. Cross-file callers live here by
// construction (decision D-90): their rows carry target_text but no
// resolved_symbol_id, so MR-009's name layer reads them as weak signals
// instead of re-resolving them.
func (s *Store) UnresolvedReferringTo(ctx context.Context, targetText string) ([]Referrer, error) {
	if targetText == "" {
		return nil, invalidInput("reference lookup needs a target text")
	}
	if err := s.requireSchema(ctx); err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT unit_id, path, referrer_key,
		target_text, label, confidence FROM symbol_references
		WHERE target_text = ? AND resolved_symbol_id IS NULL
		ORDER BY unit_id, path, referrer_key`, targetText)
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

// BindingsWithStatus lists every binding of one durable identity with its
// status — bound, ambiguous and orphaned alike. The gate (MR-013 via
// MR-016's complete) judges hardness from status + severity; this read
// stays neutral and reports all three (decision D-211).
func (s *Store) BindingsWithStatus(ctx context.Context, uid string) ([]Binding, error) {
	if uid == "" {
		return nil, invalidInput("binding listing needs a symbol uid")
	}
	if err := s.requireSchema(ctx); err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT invariant_id, symbol_uid, status, reason, updated_at
		FROM invariant_symbol_bindings WHERE symbol_uid = ? ORDER BY invariant_id`, uid)
	if err != nil {
		return nil, corruptState(err)
	}
	defer rows.Close()
	var out []Binding
	for rows.Next() {
		var binding Binding
		var stamped string
		if err := rows.Scan(&binding.InvariantID, &binding.UID, &binding.Status,
			&binding.Reason, &stamped); err != nil {
			return nil, corruptState(err)
		}
		parsed, err := app.ParseTime(stamped)
		if err != nil {
			return nil, corruptState(err)
		}
		binding.UpdatedAt = parsed
		out = append(out, binding)
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
