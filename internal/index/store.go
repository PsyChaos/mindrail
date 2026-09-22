package index

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"time"

	"github.com/PsyChaos/mindrail/internal/app"
	"github.com/PsyChaos/mindrail/internal/identity"
	"github.com/PsyChaos/mindrail/internal/storage"
)

// UnitKind is the language scope of one analysis root. TSX is a parser grammar,
// not a unit kind: a package.json with tsconfig.json is a TypeScript unit.
type UnitKind string

const (
	UnitPython     UnitKind = "python"
	UnitTypeScript UnitKind = "typescript"
	UnitJavaScript UnitKind = "javascript"
)

type ProjectUnit struct {
	ID           string
	Path         string
	Kind         UnitKind
	DiscoveredAt time.Time
}

// FileState is the durable queue state. Unsupported is terminal; failed is
// still pending work and carries the partial facts that extraction produced.
type FileState string

const (
	StatePending     FileState = "pending"
	StateIndexed     FileState = "indexed"
	StateFailed      FileState = "failed"
	StateUnsupported FileState = "unsupported"
)

type FileIndexState struct {
	UnitID      string
	Path        string
	Language    string
	ContentHash string
	State       FileState
	Attempts    int
	LastError   string
	IndexedAt   *time.Time
}

// Symbol contains one declaration's three fingerprints and source span. ID is
// a SQLite row ID, not the durable symbol_uid that MR-006 allocates. UID is
// that allocation once stamped, or empty while the identity question is open.
type Symbol struct {
	ID            int64
	LogicalKey    string
	Kind          string
	Name          string
	Container     string
	StartLine     int
	StartCol      int
	EndLine       int
	EndCol        int
	SignatureHash string
	BodyHash      string
	StructureHash string
	UID           string
}

type Import struct {
	ImporterKey string
	Module      string
	Names       []string
	Alias       string
	IsRelative  bool
}

type Reference struct {
	ReferrerKey string
	TargetText  string
	// TargetLogicalKey is a non-persisted extraction hint. When exactly one
	// symbol in this unit has that key, ReplaceFileFacts sets its row ID in
	// the same transaction that inserts this file's facts.
	TargetLogicalKey string
	ScopeText        string
	Label            string
	Confidence       float64
	ResolvedSymbolID *int64
}

// FileFacts is a complete replacement of the facts for one file. State must
// be indexed or failed; a failed parse may still carry useful partial facts.
// ProjectID scopes the identities this completion mints or reuses: identity
// is per project (decision D-94), so a completion that cannot name its
// project is refused rather than minted into a shared namespace.
type FileFacts struct {
	ProjectID   string
	UnitID      string
	Path        string
	Language    string
	ContentHash string
	State       FileState
	LastError   string
	Symbols     []Symbol
	Imports     []Import
	References  []Reference
	// RenameHints carries Git-corroborated moves into this file: callers that
	// can afford one diff per batch fetch them, the indexer never runs git.
	// An empty set means structural-only matching.
	RenameHints []RenameHint
}

// Store owns only the SQLite facts; parser snapshots are a separate disk cache.
type Store struct {
	db    *sql.DB
	clock app.Clock
}

func NewStore(db *sql.DB, clock app.Clock) *Store { return &Store{db: db, clock: clock} }

// UpsertUnit keeps the first ID and discovery time for a path, even when a
// subsequent inventory pass refines a JavaScript unit to TypeScript.
func (s *Store) UpsertUnit(ctx context.Context, path string, kind UnitKind) (ProjectUnit, error) {
	if !filepath.IsAbs(path) || filepath.Clean(path) != path {
		return ProjectUnit{}, invalidInput("project unit path must be clean and absolute")
	}
	if kind != UnitPython && kind != UnitTypeScript && kind != UnitJavaScript {
		return ProjectUnit{}, invalidInput("project unit kind must be python, typescript or javascript")
	}
	if err := s.requireSchema(ctx); err != nil {
		return ProjectUnit{}, err
	}
	var unit ProjectUnit
	err := storage.InTx(ctx, s.db, func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `INSERT INTO project_units (id, path, kind, discovered_at)
			VALUES (?, ?, ?, ?) ON CONFLICT(path) DO UPDATE SET kind = excluded.kind
			WHERE kind <> excluded.kind`, identity.NewID("UNT"), path, kind, app.FormatTime(s.clock.Now()))
		if err != nil {
			return err
		}
		var stamped string
		if err := tx.QueryRowContext(ctx, `SELECT id, path, kind, discovered_at FROM project_units WHERE path = ?`, path).
			Scan(&unit.ID, &unit.Path, &unit.Kind, &stamped); err != nil {
			return err
		}
		unit.DiscoveredAt, err = app.ParseTime(stamped)
		return err
	})
	if err != nil {
		return ProjectUnit{}, writeFailure(ctx, s.db, "project unit", err)
	}
	return unit, nil
}

// ListUnits returns the ProjectUnits rooted in root, in deterministic path
// order. The runtime database belongs to a Git common directory, so several
// worktrees share it; a status read must never report a sibling worktree's
// inventory. root is supplied canonically by bootstrap/inventory and this
// method remains filesystem-free.
func (s *Store) ListUnits(ctx context.Context, root string) ([]ProjectUnit, error) {
	if err := validUnitRoot(root); err != nil {
		return nil, err
	}
	if err := s.requireSchema(ctx); err != nil {
		return nil, err
	}
	foreignRoots, err := registeredForeignRoots(ctx, s.db, root)
	if err != nil {
		return nil, corruptState(err)
	}
	where, args := pathInRootSQL("path", root)
	rows, err := s.db.QueryContext(ctx, `SELECT id, path, kind, discovered_at FROM project_units WHERE `+where+` ORDER BY path`, args...)
	if err != nil {
		return nil, corruptState(err)
	}
	defer rows.Close()

	var units []ProjectUnit
	for rows.Next() {
		var unit ProjectUnit
		var discoveredAt string
		if err := rows.Scan(&unit.ID, &unit.Path, &unit.Kind, &discoveredAt); err != nil {
			return nil, corruptState(err)
		}
		if containedByForeignWorktree(unit.Path, root, foreignRoots) {
			continue
		}
		parsed, err := app.ParseTime(discoveredAt)
		if err != nil {
			return nil, corruptState(err)
		}
		unit.DiscoveredAt = parsed
		units = append(units, unit)
	}
	if err := rows.Err(); err != nil {
		return nil, corruptState(err)
	}
	return units, nil
}

// ReconcileUnits removes units that a complete inventory scan no longer found
// below root. It removes only generated index facts, in one short transaction,
// and excludes paths inside another registered worktree even when that
// worktree is nested beneath root.
func (s *Store) ReconcileUnits(ctx context.Context, root string, discoveredPaths []string) error {
	if err := validUnitRoot(root); err != nil {
		return err
	}
	discovered := make(map[string]struct{}, len(discoveredPaths))
	for _, path := range discoveredPaths {
		if !isCleanAbsolutePath(path) || !pathInRoot(root, path) {
			return invalidInput("discovered project unit path is outside its inventory root")
		}
		discovered[path] = struct{}{}
	}
	if err := s.requireSchema(ctx); err != nil {
		return err
	}

	err := storage.InTx(ctx, s.db, func(ctx context.Context, tx *sql.Tx) error {
		foreignRoots, err := registeredForeignRoots(ctx, tx, root)
		if err != nil {
			return err
		}
		where, args := pathInRootSQL("path", root)
		rows, err := tx.QueryContext(ctx, `SELECT id, path FROM project_units WHERE `+where+` ORDER BY path`, args...)
		if err != nil {
			return err
		}
		defer rows.Close()

		var stale []string
		for rows.Next() {
			var id, path string
			if err := rows.Scan(&id, &path); err != nil {
				return err
			}
			if _, found := discovered[path]; found || containedByForeignWorktree(path, root, foreignRoots) {
				continue
			}
			stale = append(stale, id)
		}
		if err := rows.Err(); err != nil {
			return err
		}

		for _, unitID := range stale {
			for _, table := range []string{"symbol_references", "symbol_imports", "symbols", "file_index_state", "project_units"} {
				column := "unit_id"
				if table == "project_units" {
					column = "id"
				}
				if _, err := tx.ExecContext(ctx, "DELETE FROM "+table+" WHERE "+column+" = ?", unitID); err != nil {
					return err
				}
			}
		}
		return nil
	})
	if err != nil {
		return writeFailure(ctx, s.db, "project unit reconciliation", err)
	}
	return nil
}

// UpsertFileState registers a pending or unsupported file. Rediscovery with no
// new content hash preserves an already indexed or failed file; a changed hash
// makes it pending again. A supplied pending hash names target bytes, not a
// completed parse, and every update advances the per-path CAS generation.
func (s *Store) UpsertFileState(ctx context.Context, state FileIndexState) error {
	if state.State != StatePending && state.State != StateUnsupported {
		return invalidInput("file registration must be pending or unsupported")
	}
	if state.UnitID == "" || !filepath.IsAbs(state.Path) || filepath.Clean(state.Path) != state.Path {
		return invalidInput("file registration needs a unit ID and a clean absolute path")
	}
	if err := s.requireSchema(ctx); err != nil {
		return err
	}
	err := storage.InTx(ctx, s.db, func(ctx context.Context, tx *sql.Tx) error {
		if err := fileBelongsToUnit(ctx, tx, state.UnitID, state.Path); err != nil {
			return err
		}
		var previousUnit string
		err := tx.QueryRowContext(ctx, `SELECT unit_id FROM file_index_state WHERE path = ?`, state.Path).Scan(&previousUnit)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		if err == nil && previousUnit != state.UnitID {
			// A newly discovered nested unit took ownership of this path. Its
			// old-unit facts are no longer admissible and their symbol row IDs
			// cannot be kept as pointers (D-89 clears them on delete).
			for _, table := range []string{"symbol_references", "symbol_imports", "symbols"} {
				if _, err := tx.ExecContext(ctx, "DELETE FROM "+table+" WHERE path = ? AND unit_id = ?", state.Path, previousUnit); err != nil {
					return err
				}
			}
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO file_index_state
			(path, unit_id, language, content_hash, state)
			VALUES (?, ?, ?, NULLIF(?, ''), ?)
			ON CONFLICT(path) DO UPDATE SET
			  unit_id = excluded.unit_id,
			  language = excluded.language,
			  content_hash = excluded.content_hash,
			  state = excluded.state,
			  attempts = file_index_state.attempts + 1,
			  last_error = NULL,
			  indexed_at = NULL
			WHERE file_index_state.unit_id <> excluded.unit_id
			   OR (excluded.state = 'unsupported' AND file_index_state.state <> 'unsupported')
			   OR (excluded.state = 'pending' AND excluded.content_hash IS NOT NULL
			       AND (file_index_state.content_hash IS NULL OR file_index_state.content_hash <> excluded.content_hash))`,
			state.Path, state.UnitID, state.Language, state.ContentHash, state.State)
		return err
	})
	if err != nil {
		return writeFailure(ctx, s.db, "file index state", err)
	}
	return nil
}

// ReplaceFileFacts commits the state/hash and all three fact sets in one short
// transaction. The caller parses before this call; no parser or filesystem work
// runs while SQLite holds its write lock.
func (s *Store) ReplaceFileFacts(ctx context.Context, facts FileFacts) (storage.TxStats, error) {
	if err := validReplacement(facts); err != nil {
		return storage.TxStats{}, err
	}
	if err := s.requireSchema(ctx); err != nil {
		return storage.TxStats{}, err
	}
	stats, err := storage.InTxMeasured(ctx, s.db, func(ctx context.Context, tx *sql.Tx) error {
		return s.replaceFileFactsTx(ctx, tx, facts, 1)
	})
	if err != nil {
		return storage.TxStats{}, writeFailure(ctx, s.db, "structural facts", err)
	}
	return stats, nil
}

func validReplacement(facts FileFacts) error {
	if facts.ProjectID == "" || facts.UnitID == "" || !filepath.IsAbs(facts.Path) || filepath.Clean(facts.Path) != facts.Path || facts.Language == "" || facts.ContentHash == "" {
		return invalidInput("replacement needs a project ID, unit ID, clean absolute path, language and content hash")
	}
	if facts.State != StateIndexed && facts.State != StateFailed {
		return invalidInput("replacement state must be indexed or failed")
	}
	if facts.State == StateFailed && facts.LastError == "" {
		return invalidInput("a failed parse needs its error text")
	}
	for _, hint := range facts.RenameHints {
		if hint.OldPath == "" || hint.NewPath == "" || !filepath.IsAbs(hint.OldPath) || filepath.Clean(hint.OldPath) != hint.OldPath || !filepath.IsAbs(hint.NewPath) || filepath.Clean(hint.NewPath) != hint.NewPath {
			return invalidInput("rename hints need clean absolute old and new paths")
		}
	}
	return nil
}

func (s *Store) replaceFileFactsTx(ctx context.Context, tx *sql.Tx, facts FileFacts, attemptIncrement int) error {
	if err := fileBelongsToUnit(ctx, tx, facts.UnitID, facts.Path); err != nil {
		return err
	}
	var owner string
	if err := tx.QueryRowContext(ctx, `SELECT unit_id FROM file_index_state WHERE path = ?`, facts.Path).Scan(&owner); err != nil {
		return err
	}
	if owner != facts.UnitID {
		return invalidInput("file belongs to another project unit")
	}
	// Ancestor snapshot before the delete: migration matching reads the old
	// rows this commit replaces (same-file renames) plus hint-named files
	// (moves). Rows deleted here are exactly what "disappeared" means.
	ancestors, err := s.snapshotFileAncestorsTx(ctx, tx, facts.UnitID, facts.Language, facts.Path, facts.RenameHints)
	if err != nil {
		return err
	}
	for _, table := range []string{"symbol_references", "symbol_imports", "symbols"} {
		if _, err := tx.ExecContext(ctx, "DELETE FROM "+table+" WHERE unit_id = ? AND path = ?", facts.UnitID, facts.Path); err != nil {
			return err
		}
	}
	// Identity resolution shares the transaction with the facts it names
	// (decision D-95): lookup hits keep their uid, one confident heir
	// migrates, twins ambiguate, and only the truly new mints — all in the
	// same commit that replaces the file's symbols.
	staged := make([]stagedKey, 0, len(facts.Symbols))
	for _, sym := range facts.Symbols {
		containerLocal, err := localPart(sym.Container)
		if err != nil {
			return corruptState(err)
		}
		staged = append(staged, stagedKey{
			Key: sym.LogicalKey, Kind: sym.Kind, ContainerLocal: containerLocal,
			BodyHash: sym.BodyHash, Path: facts.Path,
		})
	}
	now := app.FormatTime(s.clock.Now())
	uids, ambiguous, err := s.resolveIdentitiesTx(ctx, tx, facts.ProjectID, facts.UnitID, facts.Language, staged, ancestors, facts.RenameHints, now)
	if err != nil {
		return err
	}
	for _, sym := range facts.Symbols {
		uid := sql.NullString{}
		if !ambiguous[sym.LogicalKey] {
			uid = sql.NullString{String: uids[sym.LogicalKey], Valid: true}
		}
		_, err := tx.ExecContext(ctx, `INSERT INTO symbols
				(unit_id, path, logical_key, kind, name, container, start_line, start_col, end_line, end_col, signature_hash, body_hash, structure_hash, symbol_uid)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			facts.UnitID, facts.Path, sym.LogicalKey, sym.Kind, sym.Name, sym.Container, sym.StartLine, sym.StartCol, sym.EndLine, sym.EndCol, sym.SignatureHash, sym.BodyHash, sym.StructureHash, uid)
		if err != nil {
			return err
		}
	}
	for _, imp := range facts.Imports {
		names, err := marshalNames(imp.Names)
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO symbol_imports
				(unit_id, path, importer_key, module, names, alias, is_relative)
				VALUES (?, ?, ?, ?, ?, ?, ?)`, facts.UnitID, facts.Path, imp.ImporterKey, imp.Module, names, imp.Alias, imp.IsRelative)
		if err != nil {
			return err
		}
	}
	for _, ref := range facts.References {
		resolved := ref.ResolvedSymbolID
		if ref.TargetLogicalKey != "" {
			var err error
			resolved, err = resolveUniqueSymbol(ctx, tx, facts.UnitID, ref.TargetLogicalKey)
			if err != nil {
				return err
			}
		} else if resolved != nil {
			var key string
			if err := tx.QueryRowContext(ctx, `SELECT logical_key FROM symbols WHERE id = ? AND unit_id = ?`, *resolved, facts.UnitID).Scan(&key); errors.Is(err, sql.ErrNoRows) {
				return invalidInput("resolved reference points outside its project unit")
			} else if err != nil {
				return err
			}
			unique, err := resolveUniqueSymbol(ctx, tx, facts.UnitID, key)
			if err != nil {
				return err
			}
			if unique == nil {
				resolved = nil
			} else if *unique != *resolved {
				return invalidInput("resolved reference does not name the unique symbol for its key")
			}
		}
		label := ref.Label
		if label == "" {
			label = "STRUCTURAL_NAME_MATCH"
		}
		_, err := tx.ExecContext(ctx, `INSERT INTO symbol_references
				(unit_id, path, referrer_key, target_text, scope_text, label, confidence, resolved_symbol_id)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, facts.UnitID, facts.Path, ref.ReferrerKey, ref.TargetText, ref.ScopeText, label, ref.Confidence, resolved)
		if err != nil {
			return err
		}
	}
	var lastError any
	if facts.State == StateFailed {
		lastError = facts.LastError
	}
	_, err = tx.ExecContext(ctx, `UPDATE file_index_state SET
			language = ?, content_hash = ?, state = ?, attempts = attempts + ?,
			last_error = ?, indexed_at = ? WHERE unit_id = ? AND path = ?`,
		facts.Language, facts.ContentHash, facts.State, attemptIncrement, lastError, app.FormatTime(s.clock.Now()), facts.UnitID, facts.Path)
	return err
}

// ListPending returns the durable resume set in path order. An empty unitID
// selects all units in one query for startup scheduling.
func (s *Store) ListPending(ctx context.Context, unitID string) ([]FileIndexState, error) {
	if err := s.requireSchema(ctx); err != nil {
		return nil, err
	}
	query := `SELECT unit_id, path, language, content_hash, state, attempts, last_error, indexed_at
		FROM file_index_state WHERE state IN ('pending', 'failed')`
	var args []any
	if unitID != "" {
		query += ` AND unit_id = ?`
		args = append(args, unitID)
	}
	query += ` ORDER BY path`
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, corruptState(err)
	}
	defer rows.Close()
	var states []FileIndexState
	for rows.Next() {
		state, err := scanFileState(rows)
		if err != nil {
			return nil, corruptState(err)
		}
		states = append(states, state)
	}
	if err := rows.Err(); err != nil {
		return nil, corruptState(err)
	}
	return states, nil
}

// CountByState reads readiness counts without walking the filesystem. An
// empty unitID counts all units in one SQL aggregate for status.
func (s *Store) CountByState(ctx context.Context, unitID string) (map[FileState]int, error) {
	if err := s.requireSchema(ctx); err != nil {
		return nil, err
	}
	query := `SELECT state, count(*) FROM file_index_state`
	var args []any
	if unitID != "" {
		query += ` WHERE unit_id = ?`
		args = append(args, unitID)
	}
	query += ` GROUP BY state`
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, corruptState(err)
	}
	defer rows.Close()
	counts := make(map[FileState]int, 4)
	for rows.Next() {
		var state FileState
		var count int
		if err := rows.Scan(&state, &count); err != nil {
			return nil, corruptState(err)
		}
		counts[state] = count
	}
	if err := rows.Err(); err != nil {
		return nil, corruptState(err)
	}
	return counts, nil
}

func fileBelongsToUnit(ctx context.Context, tx *sql.Tx, unitID, path string) error {
	var root string
	if err := tx.QueryRowContext(ctx, `SELECT path FROM project_units WHERE id = ?`, unitID).Scan(&root); err != nil {
		return err
	}
	rel, err := filepath.Rel(root, path)
	if err != nil || rel == "." || rel == ".." || filepath.IsAbs(rel) || len(rel) >= 3 && rel[:3] == ".."+string(filepath.Separator) {
		return invalidInput("file path is outside its project unit")
	}
	return nil
}

func validUnitRoot(root string) error {
	if !isCleanAbsolutePath(root) {
		return invalidInput("project unit root must be clean and absolute")
	}
	return nil
}

func isCleanAbsolutePath(path string) bool {
	return filepath.IsAbs(path) && filepath.Clean(path) == path
}

func pathInRoot(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	return err == nil && rel != ".." && !filepath.IsAbs(rel) && (rel == "." || len(rel) < 3 || rel[:3] != ".."+string(filepath.Separator))
}

// pathInRootSQL mirrors pathInRoot without relying on SQL LIKE: % and _ are
// legal path bytes, so a LIKE predicate would let a root containing either
// character leak into a sibling worktree. The extra separator check makes the
// /repo/web versus /repo/website boundary explicit.
func pathInRootSQL(column, root string) (string, []any) {
	if isFilesystemRoot(root) {
		// A volume root already ends in its separator: adding the normal
		// child-boundary check would look for a second slash (`//child`) and
		// exclude every descendant. Its prefix is the boundary.
		return "substr(" + column + ", 1, length(?)) = ?", []any{root, root}
	}
	return "(" + column + " = ? OR (substr(" + column + ", 1, length(?)) = ? AND substr(" + column + ", length(?) + 1, 1) = ?))", []any{root, root, root, root, string(filepath.Separator)}
}

func isFilesystemRoot(path string) bool {
	return path == filepath.VolumeName(path)+string(filepath.Separator)
}

type queryer interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}

func registeredForeignRoots(ctx context.Context, db queryer, root string) ([]string, error) {
	rows, err := db.QueryContext(ctx, `SELECT root_path FROM workspaces WHERE root_path <> ?`, root)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var roots []string
	for rows.Next() {
		var candidate string
		if err := rows.Scan(&candidate); err != nil {
			return nil, err
		}
		if isCleanAbsolutePath(candidate) {
			roots = append(roots, candidate)
		}
	}
	return roots, rows.Err()
}

func containedByForeignWorktree(path, currentRoot string, roots []string) bool {
	for _, root := range roots {
		// A parent worktree contains a nested checkout's path, but it does not
		// own that nested checkout's inventory. Conversely, a worktree nested
		// beneath the current root owns its own paths and must be excluded.
		if pathInRoot(root, path) && !pathInRoot(root, currentRoot) {
			return true
		}
	}
	return false
}

type fileScanner interface{ Scan(...any) error }

func scanFileState(row fileScanner) (FileIndexState, error) {
	var state FileIndexState
	var hash, lastError, indexedAt sql.NullString
	err := row.Scan(&state.UnitID, &state.Path, &state.Language, &hash, &state.State, &state.Attempts, &lastError, &indexedAt)
	if err != nil {
		return FileIndexState{}, err
	}
	state.ContentHash = hash.String
	state.LastError = lastError.String
	if indexedAt.Valid {
		parsed, err := app.ParseTime(indexedAt.String)
		if err != nil {
			return FileIndexState{}, err
		}
		state.IndexedAt = &parsed
	}
	return state, nil
}

func marshalNames(names []string) (string, error) {
	if names == nil {
		names = []string{}
	}
	encoded, err := json.Marshal(names)
	if err != nil {
		return "", err
	}
	return string(encoded), nil
}

func resolveUniqueSymbol(ctx context.Context, tx *sql.Tx, unitID, logicalKey string) (*int64, error) {
	rows, err := tx.QueryContext(ctx, `SELECT id FROM symbols WHERE unit_id = ? AND logical_key = ? LIMIT 2`, unitID, logicalKey)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(ids) != 1 {
		return nil, nil
	}
	return &ids[0], nil
}

// requireSchema asks the migration ledger once per store operation, never per
// row or fact. The store may survive `mindrail init`, so this cannot be cached
// at construction: a v3 database can become v4 while this Store is live.
func (s *Store) requireSchema(ctx context.Context) error {
	var applied sql.NullInt64
	if err := s.db.QueryRowContext(ctx, `SELECT max(version) FROM schema_migrations`).Scan(&applied); err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return err
		}
		// The ledger does not exist before the first init. Probe only on the
		// error path: no user tables is a healthy version 0, while any user
		// table without its ledger is damage that init cannot repair.
		var tables int
		if probeErr := s.db.QueryRowContext(ctx, `SELECT count(*) FROM sqlite_master WHERE type = 'table' AND name NOT GLOB 'sqlite_*'`).Scan(&tables); probeErr == nil && tables == 0 {
			return schemaBehind(0)
		}
		return corruptState(err)
	}
	if applied.Int64 < TableSchemaVersion {
		return schemaBehind(applied.Int64)
	}
	return nil
}

func invalidInput(why string) error {
	return app.NewError(app.CodeCommandLineInvalid, app.KindUsage, why,
		"No index facts were changed.", "Pass a valid project unit and file state.")
}

func writeFailure(ctx context.Context, db *sql.DB, what string, err error) error {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	if _, ok := app.PayloadOf(err); ok {
		return err
	}
	wrapped := fmt.Errorf("write %s: %w", what, err)
	if classified := storage.WriteFailure(ctx, db, what, wrapped); classified != nil {
		return classified
	}
	return wrapped
}
