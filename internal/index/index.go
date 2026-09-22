// Package index owns the structural index: the project units discovery
// found, the per-file index state that is the cold queue's durable state,
// and the structural facts one Tree-sitter parse produced (symbols, imports,
// references, fingerprints). Tech-stack §7 names the layout; decision D-80
// records which of those packages 0.1 creates and why.
//
// The facts live in the runtime database under migration 000004; durable
// symbol identity lives under migration 000005. The parse snapshots are a
// disk cache beside the database and are owned by internal/index/snapshot,
// not here (decision D-82).
package index

// TableSchemaVersion is the migration that creates the project_units,
// file_index_state, symbols, symbol_imports and symbol_references tables,
// plus the symbol identity tables and the symbols.symbol_uid column.
//
// It exists for the same reason workspace.TableSchemaVersion and
// coordination.TableSchemaVersion do: a reader of these tables must be able
// to tell "the schema does not hold them yet" apart from "they hold
// nothing", from the migration ledger, which is what the migrator itself is
// answerable for (decision D-80's store, MR-003's pattern).
const TableSchemaVersion = 5
