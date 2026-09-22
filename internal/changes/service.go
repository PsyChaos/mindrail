package changes

import (
	"context"
	"database/sql"
	"fmt"
	"os"

	"github.com/PsyChaos/mindrail/internal/index"
	"github.com/PsyChaos/mindrail/internal/index/inventory"
	"github.com/PsyChaos/mindrail/internal/storage"
)

// Symbol change kinds ride the CHECK vocabulary of change_symbols.
const (
	SymbolAdded    = "added"
	SymbolRemoved  = "removed"
	SymbolModified = "modified"
)

// SymbolChange is one symbol delta, ready to upsert.
type SymbolChange struct {
	Key        string
	UID        string
	Kind       string
	Body       bool
	Signature  bool
	Structure  bool
	SigHash    string
	BodyHash   string
	StructHash string
	Via        string
}

// Service composes discovery: baselines, Git, extraction, indexing and
// identity following. It holds the index store and indexer beside its own
// store; callers thread project and repository root explicitly.
type Service struct {
	store   *Store
	indexes *index.Store
	indexer *index.Indexer
}

// New builds a Service over the three stores it composes.
func New(store *Store, indexes *index.Store, indexer *index.Indexer) (*Service, error) {
	if store == nil || indexes == nil || indexer == nil {
		return nil, fmt.Errorf("changes: service needs a change store, an index store and an indexer")
	}
	return &Service{store: store, indexes: indexes, indexer: indexer}, nil
}

// unitFor resolves the owning unit for an absolute path, or reports that no
// unit owns it. Files outside every unit carry file rows but no symbol
// delta — callers treat that as a complete answer for the file, not as a
// failure.
func (s *Service) unitFor(path string, units []index.ProjectUnit) (index.ProjectUnit, bool) {
	unit, ok := inventory.Owner(path, units)
	if !ok {
		return index.ProjectUnit{}, false
	}
	return unit, true
}

// SyncFileSymbols diffs one file's fresh extraction against stored facts,
// indexes the file (facts update, uids migrate), and upserts the symbol
// deltas with post-indexing uids (decision D-129: deltas never mint, they
// name what indexing assigned). Deleted paths skip extraction and mark every
// stored row removed. Unsupported or out-of-unit files contribute nothing
// beyond their file row.
func (s *Service) SyncFileSymbols(ctx context.Context, projectID, repoRoot, changeID, path, content string, deleted bool, hints []index.RenameHint, via string, units []index.ProjectUnit) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if projectID == "" || changeID == "" || path == "" {
		return invalidInput("symbol sync needs a project, a change and a path")
	}
	switch via {
	case ViaBaseline, ViaReconcile:
	default:
		return invalidInput("symbol sync provenance must be baseline or reconcile")
	}
	unit, ok := s.unitFor(path, units)
	if !ok {
		return nil
	}
	stored, err := s.indexes.ListSymbolsInFile(ctx, unit.ID, path)
	if err != nil {
		return err
	}
	if deleted {
		return s.store.UpsertSymbolRows(ctx, changeID, removedSymbols(stored, via))
	}
	current, err := s.indexer.ExtractCurrent(ctx, unit, path, []byte(content))
	if err != nil {
		return err
	}
	if current == nil {
		return nil
	}
	delta := diffSymbols(stored, current, via)
	if _, err := s.indexer.IndexFile(ctx, projectID, unit, path, hints...); err != nil {
		return err
	}
	resolved, err := s.attachUIDs(ctx, unit.ID, path, delta)
	if err != nil {
		return err
	}
	return s.store.UpsertSymbolRows(ctx, changeID, resolved)
}

// removedSymbols marks every stored row removed with its last-known hashes.
func removedSymbols(stored []index.Symbol, via string) []SymbolChange {
	delta := make([]SymbolChange, 0, len(stored))
	for _, row := range stored {
		delta = append(delta, SymbolChange{Key: row.LogicalKey, UID: row.UID,
			Kind: SymbolRemoved, SigHash: row.SignatureHash, BodyHash: row.BodyHash, StructHash: row.StructureHash, Via: via})
	}
	return delta
}

// diffSymbols matches fresh extraction against stored facts by logical key:
// missing rows are added, extra stored rows are removed, and present pairs
// compare the three hashes independently.
func diffSymbols(stored []index.Symbol, current []index.Symbol, via string) []SymbolChange {
	byKey := make(map[string]index.Symbol, len(stored))
	for _, row := range stored {
		byKey[row.LogicalKey] = row
	}
	seen := make(map[string]bool, len(current))
	var delta []SymbolChange
	for _, sym := range current {
		seen[sym.LogicalKey] = true
		old, ok := byKey[sym.LogicalKey]
		if !ok {
			delta = append(delta, SymbolChange{Key: sym.LogicalKey, Kind: SymbolAdded,
				SigHash: sym.SignatureHash, BodyHash: sym.BodyHash, StructHash: sym.StructureHash, Via: via})
			continue
		}
		if old.SignatureHash == sym.SignatureHash && old.BodyHash == sym.BodyHash && old.StructureHash == sym.StructureHash {
			continue
		}
		delta = append(delta, SymbolChange{Key: sym.LogicalKey, Kind: SymbolModified,
			Body: old.BodyHash != sym.BodyHash, Signature: old.SignatureHash != sym.SignatureHash,
			Structure: old.StructureHash != sym.StructureHash,
			SigHash:   sym.SignatureHash, BodyHash: sym.BodyHash, StructHash: sym.StructureHash, Via: via})
	}
	for _, row := range stored {
		if !seen[row.LogicalKey] {
			delta = append(delta, SymbolChange{Key: row.LogicalKey, UID: row.UID, Kind: SymbolRemoved,
				SigHash: row.SignatureHash, BodyHash: row.BodyHash, StructHash: row.StructureHash, Via: via})
		}
	}
	return delta
}

// attachUIDs resolves post-indexing uids for delta rows by key. Added rows
// pick up minted or migrated uids; modified rows keep theirs; removed rows
// already carry the stored one.
func (s *Service) attachUIDs(ctx context.Context, unitID, path string, delta []SymbolChange) ([]SymbolChange, error) {
	rows, err := s.indexes.ListSymbolsInFile(ctx, unitID, path)
	if err != nil {
		return nil, err
	}
	uids := make(map[string]string, len(rows))
	for _, row := range rows {
		uids[row.LogicalKey] = row.UID
	}
	for i := range delta {
		if delta[i].Kind == SymbolRemoved {
			continue
		}
		delta[i].UID = uids[delta[i].Key]
	}
	return delta, nil
}

// UpsertSymbolRows accumulates symbol deltas into a Change by natural key.
func (s *Store) UpsertSymbolRows(ctx context.Context, changeID string, rows []SymbolChange) error {
	if changeID == "" {
		return invalidInput("symbol rows need a change")
	}
	if err := s.requireSchema(ctx); err != nil {
		return err
	}
	err := storage.InTx(ctx, s.db, func(ctx context.Context, tx *sql.Tx) error {
		for _, row := range rows {
			if row.Key == "" {
				return invalidInput("symbol row needs a key")
			}
			switch row.Kind {
			case SymbolAdded, SymbolRemoved, SymbolModified:
			default:
				return invalidInput("symbol row kind must be added, removed or modified")
			}
			switch row.Via {
			case ViaBaseline, ViaReconcile:
			default:
				return invalidInput("symbol row provenance must be baseline or reconcile")
			}
			if _, err := tx.ExecContext(ctx, `INSERT INTO change_symbols
				(change_id, logical_key, symbol_uid, kind, body_changed, signature_changed, structure_changed,
				signature_hash, body_hash, structure_hash, discovered_via)
				VALUES (?, ?, NULLIF(?, ''), ?, ?, ?, ?, ?, ?, ?, ?)
				ON CONFLICT(change_id, logical_key) DO UPDATE SET
				symbol_uid = excluded.symbol_uid, kind = excluded.kind,
				body_changed = excluded.body_changed, signature_changed = excluded.signature_changed,
				structure_changed = excluded.structure_changed, signature_hash = excluded.signature_hash,
				body_hash = excluded.body_hash, structure_hash = excluded.structure_hash,
				discovered_via = excluded.discovered_via`,
				changeID, row.Key, row.UID, row.Kind, boolInt(row.Body), boolInt(row.Signature),
				boolInt(row.Structure), row.SigHash, row.BodyHash, row.StructHash, row.Via); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return writeFailure(ctx, s.db, "change symbols", err)
	}
	return nil
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// ReadChangeSymbols returns one change's symbol rows in key order.
func (s *Store) ReadChangeSymbols(ctx context.Context, changeID string) ([]SymbolChange, error) {
	if changeID == "" {
		return nil, invalidInput("symbol listing needs a change")
	}
	if err := s.requireSchema(ctx); err != nil {
		return nil, err
	}
	dbRows, err := s.db.QueryContext(ctx, `SELECT logical_key, symbol_uid, kind, body_changed,
		signature_changed, structure_changed, signature_hash, body_hash, structure_hash, discovered_via
		FROM change_symbols WHERE change_id = ? ORDER BY logical_key`, changeID)
	if err != nil {
		return nil, corruptState(err)
	}
	defer dbRows.Close()
	var rows []SymbolChange
	for dbRows.Next() {
		var row SymbolChange
		var uid sql.NullString
		var body, signature, structure int
		if err := dbRows.Scan(&row.Key, &uid, &row.Kind, &body, &signature, &structure,
			&row.SigHash, &row.BodyHash, &row.StructHash, &row.Via); err != nil {
			return nil, corruptState(err)
		}
		row.UID = uid.String
		row.Body, row.Signature, row.Structure = body != 0, signature != 0, structure != 0
		rows = append(rows, row)
	}
	if err := dbRows.Err(); err != nil {
		return nil, corruptState(err)
	}
	return rows, nil
}

// AfterChange composes the baseline path end to end: baseline delta files,
// symbol sync per file, all into the task's open change. Calling it twice
// converges (AC-04.3): natural-key upserts make redelivery idempotent.
func (s *Service) AfterChange(ctx context.Context, projectID, repoRoot, taskID, operationID string) (Change, error) {
	if err := ctx.Err(); err != nil {
		return Change{}, err
	}
	if projectID == "" || taskID == "" {
		return Change{}, invalidInput("after_change needs a project and a task")
	}
	baseline, err := s.store.ReadBaseline(ctx, taskID)
	if err != nil {
		return Change{}, err
	}
	scope := make([]string, 0, len(baseline))
	for path := range baseline {
		scope = append(scope, path)
	}
	delta, err := s.store.BaselineFileDelta(ctx, taskID, scope)
	if err != nil {
		return Change{}, err
	}
	change, err := s.store.EnsureOpenChange(ctx, taskID, operationID)
	if err != nil {
		return Change{}, err
	}
	if err := s.store.UpsertFileRows(ctx, change.ID, delta); err != nil {
		return Change{}, err
	}
	units, err := s.indexes.ListUnits(ctx, repoRoot)
	if err != nil {
		return Change{}, err
	}
	for _, file := range delta {
		content, deleted := readScopeContent(file)
		if err := s.SyncFileSymbols(ctx, projectID, repoRoot, change.ID, file.Path, content, deleted, nil, ViaBaseline, units); err != nil {
			return Change{}, err
		}
	}
	return s.store.readChange(ctx, change.ID)
}

// readScopeContent loads a delta file's current bytes for extraction.
// Deleted and unhashable paths sync with empty content and the deleted flag,
// resolving to stored-row removal or file-only rows downstream.
func readScopeContent(file FileChange) (string, bool) {
	if file.Kind == FileDeleted || file.Hash == "" {
		return "", true
	}
	content, err := os.ReadFile(file.Path)
	if err != nil {
		return "", true
	}
	return string(content), false
}
