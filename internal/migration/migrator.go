package migration

import (
	"cmp"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/PsyChaos/mindrail/internal/app"
	"github.com/PsyChaos/mindrail/internal/storage"
)

// bookkeepingDDL creates the ledger the migrator keeps about itself. It is not
// a migration file: a migration cannot create the table that records whether
// migrations have run (decision D-23).
//
// checksum is the column that makes forward-only enforceable. Without it,
// editing 000001 after it shipped would leave every existing database silently
// running the old schema.
const bookkeepingDDL = `CREATE TABLE IF NOT EXISTS schema_migrations (
    version    INTEGER PRIMARY KEY,
    name       TEXT NOT NULL,
    checksum   TEXT NOT NULL,
    applied_at TEXT NOT NULL
)`

const bookkeepingTable = "schema_migrations"

// tableKind is the way sqlite_master spells a table, and the only object kind
// the schema verification in this file draws conclusions from. See
// verifySchemaObjects for why the others are out.
const tableKind = "table"

// Applied is one row of the ledger.
type Applied struct {
	Version   int64     `json:"version"`
	Name      string    `json:"name"`
	Checksum  string    `json:"checksum"`
	AppliedAt time.Time `json:"applied_at"`
}

// Result summarises one Up call. Applied lists what this run did, which is
// empty on a re-run; CurrentVersion and Total describe the database either way,
// so a caller can report state without a second query.
type Result struct {
	Applied        []Applied `json:"applied"` // this run only; empty on re-run
	CurrentVersion int64     `json:"current_version"`
	Total          int       `json:"total"`
}

// Migrator applies a fixed migration set to one database. The set and the
// clock are constructor parameters rather than globals so a test can inject a
// deliberately broken set or a frozen instant (decision D-34).
type Migrator struct {
	db    *sql.DB
	set   []Migration
	clock app.Clock
}

// New builds a migrator over set, which Load has already ordered.
func New(db *sql.DB, set []Migration, clock app.Clock) *Migrator {
	return &Migrator{db: db, set: set, clock: clock}
}

// rebuildRemedy names the database the reader has to move aside.
//
// The path is asked of the connection rather than threaded in, the way storage's
// own write remedies ask for it. Without it the sentence said "the runtime
// database" and stopped, while the corruption remedy for the file next to it
// printed the full path — and the database lives under `.git`, which most users
// have never opened. The second cost is the one that matters more: a remedy
// naming no absolute path cannot be read by the invariant that compares what two
// documents say about one path, so it could never be caught contradicting the
// error object it travels with (finding F13).
//
// An empty answer is a normal outcome, not a failure. It costs the sentence its
// path, and the sentence is being built because something has already gone
// wrong.
func (m *Migrator) rebuildRemedy(ctx context.Context) string {
	if path := storage.MainDatabaseFile(ctx, m.db); path != "" {
		return "Move " + path + " aside and run `mindrail init` to rebuild it."
	}
	return "Move the runtime database aside and run `mindrail init` to rebuild it."
}

var (
	// ErrApplyFailed means a migration's SQL did not run. Nothing it did was
	// kept and nothing was recorded.
	ErrApplyFailed = errors.New("migration failed")

	// ErrChecksumMismatch means an already-applied migration's file has
	// changed since it ran.
	ErrChecksumMismatch = errors.New("applied migration content changed")

	// ErrSchemaAhead means the database records a migration this binary has
	// never heard of.
	ErrSchemaAhead = errors.New("database schema is newer than this binary")

	// ErrSchemaObjectMissing means the ledger records a migration as applied
	// but the objects it creates are not in the database.
	ErrSchemaObjectMissing = errors.New("an applied migration's schema objects are missing")

	// ErrSchemaShapeChanged means an object is present under the right name but
	// is missing columns the applied migrations declared.
	ErrSchemaShapeChanged = errors.New("an applied migration's table no longer has the columns it declared")

	// ErrSchemaBehindLedger means the ledger does not record a migration whose
	// tables the database already holds -- the schema is ahead of the record of
	// it, so applying the migration would fail on a table that exists.
	ErrSchemaBehindLedger = errors.New("the schema holds tables the migration ledger does not record")

	// ErrBookkeepingOccupied means something that is not the ledger table
	// already stands under the ledger table's name.
	ErrBookkeepingOccupied = errors.New("another object occupies the bookkeeping table's name")
)

// Up applies every pending migration in ascending order and returns what it
// did. Each migration's check-then-apply -- "is this version recorded? no,
// then run it and record it" -- happens inside a single BEGIN IMMEDIATE
// transaction, so two processes starting at the same time cannot both conclude
// the migration is pending (decision D-24). The loser waits out busy_timeout
// and then reads the winner's row.
//
// Up stops at the first failure and returns the migrations it had already
// committed. It does not unwind them: they are each independently valid, and
// undoing a committed schema change is the downgrade this package refuses to
// implement.
func (m *Migrator) Up(ctx context.Context) (Result, error) {
	result := Result{Applied: []Applied{}, Total: len(m.set)}

	if err := m.ensureBookkeeping(ctx); err != nil {
		return result, err
	}

	ledger, err := m.Status(ctx)
	if err != nil {
		return result, err
	}
	result.CurrentVersion = highestVersion(ledger)

	// Checked before anything is applied: a database that has drifted from
	// this binary's expectations must not be written to at all.
	if err := m.verifyLedger(ledger); err != nil {
		return result, err
	}

	recorded := make(map[int64]struct{}, len(ledger))
	for _, row := range ledger {
		recorded[row.Version] = struct{}{}
	}

	for _, pending := range m.set {
		if _, done := recorded[pending.Version]; done {
			continue
		}

		row, applied, err := m.apply(ctx, pending)
		if err != nil {
			return result, err
		}
		if applied {
			result.Applied = append(result.Applied, row)
		}
		if pending.Version > result.CurrentVersion {
			result.CurrentVersion = pending.Version
		}
	}

	return result, nil
}

// Status returns the ledger in ascending version order. A database with no
// ledger table is a database no migration has run against, which is a state,
// not an error -- doctor reports it rather than failing on it.
//
// The ledger is a claim about the schema, not the schema, so Status also
// confirms that the two still describe each other -- in both directions. A
// database whose tables were dropped, by a half-finished manual repair or by a
// tool that mistook the runtime store for scratch space, would otherwise report
// "Schema up to date (version 1)" and then fail every command on a table the
// report said existed. A database whose ledger rows were lost while its tables
// survived would otherwise report "Schema is behind this binary" and send the
// user to an `init` that cannot ever succeed.
//
// Status deliberately does not run verifyLedger. A ledger this binary is behind
// -- decision D-25's newer database -- has to stay *reportable*, or doctor
// cannot explain the very condition it exists to explain; refusing to hand back
// the rows would turn the report into the failure. Up runs verifyLedger before
// it writes, which is where fail-closed belongs.
func (m *Migrator) Status(ctx context.Context) ([]Applied, error) {
	ledger, err := m.readLedger(ctx)
	if err != nil {
		return nil, err
	}
	if err := m.verifySchemaObjects(ctx, ledger); err != nil {
		return nil, err
	}
	return ledger, nil
}

func (m *Migrator) readLedger(ctx context.Context) ([]Applied, error) {
	present, err := m.bookkeepingExists(ctx)
	if err != nil {
		return nil, err
	}
	if !present {
		return nil, nil
	}

	rows, err := m.db.QueryContext(ctx,
		`SELECT version, name, checksum, applied_at FROM schema_migrations ORDER BY version`)
	if err != nil {
		return nil, m.unreadableFailure(ctx, ledgerDescription, fmt.Errorf("read %s: %w", bookkeepingTable, err))
	}
	defer func() { _ = rows.Close() }()

	var ledger []Applied
	for rows.Next() {
		var (
			row       Applied
			appliedAt string
		)
		if err := rows.Scan(&row.Version, &row.Name, &row.Checksum, &appliedAt); err != nil {
			return nil, m.unreadableFailure(ctx, ledgerDescription, fmt.Errorf("scan %s row: %w", bookkeepingTable, err))
		}
		row.AppliedAt, err = app.ParseTime(appliedAt)
		if err != nil {
			return nil, m.unreadableFailure(ctx, ledgerDescription,
				fmt.Errorf("%s version %d: %w", bookkeepingTable, row.Version, err))
		}
		ledger = append(ledger, row)
	}
	if err := rows.Err(); err != nil {
		return nil, m.unreadableFailure(ctx, ledgerDescription, fmt.Errorf("read %s: %w", bookkeepingTable, err))
	}

	return ledger, nil
}

// ledgerDescription names the ledger the way a user should read about it. It is
// spelled once so the three ways reading it can fail cannot describe it three
// different ways.
const ledgerDescription = "the migration ledger in the runtime database"

// columnsDescription is the same for the table shapes.
const columnsDescription = "the runtime database's table columns"

// Pending returns the migrations Up would apply next, in order.
func (m *Migrator) Pending(ctx context.Context) ([]Migration, error) {
	ledger, err := m.Status(ctx)
	if err != nil {
		return nil, err
	}

	recorded := make(map[int64]struct{}, len(ledger))
	for _, row := range ledger {
		recorded[row.Version] = struct{}{}
	}

	pending := make([]Migration, 0, len(m.set))
	for _, candidate := range m.set {
		if _, done := recorded[candidate.Version]; !done {
			pending = append(pending, candidate)
		}
	}
	return pending, nil
}

// apply runs one migration and records it atomically. The bool reports whether
// this call did the work; false means another process won the race and the
// version was already recorded when the transaction took the write lock.
func (m *Migrator) apply(ctx context.Context, pending Migration) (Applied, bool, error) {
	row := Applied{Version: pending.Version, Name: pending.Name, Checksum: pending.Checksum}
	applied := false

	err := storage.InTx(ctx, m.db, func(ctx context.Context, tx *sql.Tx) error {
		existing, found, err := readLedgerRow(ctx, tx, pending.Version)
		if err != nil {
			return m.applyFailure(ctx, pending, err)
		}
		if found {
			if existing.Checksum != pending.Checksum {
				return checksumFailure(pending, existing)
			}
			row = existing
			return nil
		}

		if _, err := tx.ExecContext(ctx, pending.SQL); err != nil {
			return m.applyFailure(ctx, pending, err)
		}

		now := m.clock.Now().UTC()
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO schema_migrations (version, name, checksum, applied_at) VALUES (?, ?, ?, ?)`,
			pending.Version, pending.Name, pending.Checksum, app.FormatTime(now)); err != nil {
			return m.applyFailure(ctx, pending, err)
		}

		row.AppliedAt = now
		applied = true
		return nil
	})
	if err != nil {
		// InTx's own begin/commit failures are not domain errors yet. BEGIN is
		// where contention surfaces on this path: _txlock=immediate takes the
		// write lock at the door, so a second `mindrail init` that outlasts
		// busy_timeout fails here and nowhere else.
		if _, isDomain := app.PayloadOf(err); !isDomain {
			return Applied{}, false, m.applyFailure(ctx, pending, err)
		}
		return Applied{}, false, err
	}

	return row, applied, nil
}

// verifySchemaObjects checks the two ways the ledger and the schema can
// contradict each other, and then the one way a table can contradict itself.
//
// The ledger is behind: a table a migration the ledger does not record would
// create is already standing (H2). The ledger is ahead: a table the recorded
// migrations should have left behind is gone (F4/S4). And the shape pass, which
// catches the table that kept its name and lost its columns (F9).
//
// # Only tables
//
// Indexes, views and triggers are deliberately not verified, and that narrowing
// is what makes the check safe rather than merely correct today (finding H1).
//
// The expected set is a fold of the recorded migrations' effects, and a fold is
// only sound for objects whose removal is always written down. In SQLite a
// table is the only such object: nothing removes a table as a side effect of
// something else. Everything else is removed implicitly -- `DROP TABLE` takes
// the table's indexes and triggers with it, `DROP VIEW` takes the view's
// triggers -- so the fold keeps demanding an index the schema is *correct* not
// to have the moment any migration retires a table. That is not a hypothetical:
// it is how the standard 12-step table rebuild is written, and it bricked every
// command, `mindrail init` included, with a remedy that rebuilt the database,
// replayed both migrations and landed in the identical state.
//
// The alternative was to model the cascade -- parse `CREATE INDEX ... ON t` and
// `CREATE TRIGGER ... ON t`, track ownership across renames, and subtract the
// blast radius of every drop. That is a second guess layered on the first, and a
// wrong guess brings back exactly the bricking this narrowing removes. It also
// buys nothing against the failure this check exists for: a missing index costs
// query time, never a failed command, while a missing table or column is the
// `no such table` that made `doctor` report health over a database no command
// could use. Load still records every kind, so a later milestone that needs
// index or trigger verification has the material -- and will have to argue the
// cascade separately.
//
// # Cost
//
// Two queries with a handful of bound names on the warm path of every command,
// and a third only when a contradiction is found. Types, defaults, constraints
// and collations are not compared -- that would be the schema diff this package
// refuses to be, and the parsing it does need happens once at Load against
// embedded files, never here.
func (m *Migrator) verifySchemaObjects(ctx context.Context, ledger []Applied) error {
	expected := m.expectedTables(ledger)
	unrecorded := m.unrecordedTables(ledger, expected)

	present, err := m.presentTables(ctx, expected, unrecorded)
	if err != nil {
		return err
	}

	if missing := namesAbsentFrom(expected, present); len(missing) != 0 {
		return m.schemaObjectFailure(ctx, missing)
	}

	if conflicts := namesPresentIn(unrecorded, present); len(conflicts) != 0 {
		// One re-read closes the D-24 race. Another process applying the first
		// migration commits its tables and its ledger rows in one transaction,
		// but this check reads the two with separate statements, so a ledger read
		// from before that commit can meet a table list from after it. The tables
		// were observed present, so the winner's commit is already durable, so a
		// ledger read *now* cannot miss it: a ledger that changed means the
		// contradiction was this process's snapshot rather than the database's
		// state.
		unchanged, err := m.ledgerUnchanged(ctx, ledger)
		if err != nil {
			return err
		}
		if unchanged {
			return m.schemaBehindLedgerFailure(ctx, conflicts, unrecorded)
		}
	}

	return m.verifyTableShapes(ctx, m.declaredColumns(ledger))
}

// presentTables reports which of the named tables sqlite_master actually holds.
//
// Both directions are asked in one query because both are about the same list
// of names on the same warm path, and asking twice would double the cost of the
// cheaper half of the check.
func (m *Migrator) presentTables(
	ctx context.Context,
	expected map[string]struct{},
	unrecorded map[string]Migration,
) (map[string]struct{}, error) {
	names := make([]any, 0, len(expected)+len(unrecorded))
	for name := range expected {
		names = append(names, name)
	}
	for name := range unrecorded {
		if _, duplicate := expected[name]; !duplicate {
			names = append(names, name)
		}
	}
	if len(names) == 0 {
		return nil, nil
	}

	placeholders := make([]string, len(names))
	for i := range placeholders {
		placeholders[i] = "?"
	}

	rows, err := m.db.QueryContext(ctx,
		`SELECT name FROM sqlite_master WHERE type = 'table' AND name IN (`+
			strings.Join(placeholders, ", ")+`)`, names...)
	if err != nil {
		return nil, m.unreadableFailure(ctx, "the runtime database's table list", err)
	}
	defer func() { _ = rows.Close() }()

	present := make(map[string]struct{}, len(names))
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, m.unreadableFailure(ctx, "the runtime database's table list", err)
		}
		present[name] = struct{}{}
	}
	if err := rows.Err(); err != nil {
		return nil, m.unreadableFailure(ctx, "the runtime database's table list", err)
	}

	return present, nil
}

// ledgerUnchanged re-reads the ledger and reports whether it is still the one a
// contradiction was computed against. Only the identifying columns are compared:
// a row cannot change its name or its applied_at without changing its version or
// its checksum too, and comparing the parsed timestamp would make the answer
// depend on formatting.
func (m *Migrator) ledgerUnchanged(ctx context.Context, before []Applied) (bool, error) {
	after, err := m.readLedger(ctx)
	if err != nil {
		return false, err
	}
	return slices.EqualFunc(before, after, func(a, b Applied) bool {
		return a.Version == b.Version && a.Checksum == b.Checksum
	}), nil
}

func namesAbsentFrom(expected, present map[string]struct{}) []string {
	absent := make([]string, 0, len(expected))
	for name := range expected {
		if _, found := present[name]; !found {
			absent = append(absent, name)
		}
	}
	slices.Sort(absent)
	return absent
}

func namesPresentIn(candidates map[string]Migration, present map[string]struct{}) []string {
	found := make([]string, 0, len(candidates))
	for name := range candidates {
		if _, exists := present[name]; exists {
			found = append(found, name)
		}
	}
	slices.Sort(found)
	return found
}

// verifyTableShapes checks that every column the recorded migrations declared
// is still in the table that carries its name.
//
// It is the answer to finding F9: existence alone let a table that was dropped
// and recreated by hand pass, so `doctor` reported the schema healthy, the next
// write failed on a column, and that failure's remedy was "run `mindrail
// doctor`" -- a loop through the check that had just said everything was fine.
//
// It is one query for the whole schema, and it asserts containment rather than
// equality: a column the live table has and the migration did not declare is
// not a fault, because a later migration is allowed to add one. The equality
// the check does *not* assert is what keeps it from becoming a schema diff.
func (m *Migrator) verifyTableShapes(ctx context.Context, expected map[string][]string) error {
	if len(expected) == 0 {
		return nil
	}

	names := make([]any, 0, len(expected))
	placeholders := make([]string, 0, len(expected))
	for name := range expected {
		names = append(names, name)
		placeholders = append(placeholders, "?")
	}

	// table_xinfo rather than table_info: it also lists generated and hidden
	// columns, and a check that asserts containment is safer with the larger
	// set on the "present" side.
	rows, err := m.db.QueryContext(ctx,
		`SELECT m.name, p.name FROM sqlite_master m JOIN pragma_table_xinfo(m.name) p `+
			`WHERE m.type = 'table' AND m.name IN (`+strings.Join(placeholders, ", ")+`)`, names...)
	if err != nil {
		return m.unreadableFailure(ctx, columnsDescription, fmt.Errorf("read table columns: %w", err))
	}
	defer func() { _ = rows.Close() }()

	present := make(map[string]map[string]struct{}, len(expected))
	for rows.Next() {
		var table, column string
		if err := rows.Scan(&table, &column); err != nil {
			return m.unreadableFailure(ctx, columnsDescription, fmt.Errorf("scan table column row: %w", err))
		}
		if present[table] == nil {
			present[table] = make(map[string]struct{})
		}
		present[table][strings.ToLower(column)] = struct{}{}
	}
	if err := rows.Err(); err != nil {
		return m.unreadableFailure(ctx, columnsDescription, fmt.Errorf("read table columns: %w", err))
	}

	missing := make(map[string][]string)
	for table, columns := range expected {
		for _, column := range columns {
			if _, found := present[table][column]; !found {
				missing[table] = append(missing[table], column)
			}
		}
	}
	if len(missing) == 0 {
		return nil
	}

	return m.schemaShapeFailure(ctx, missing)
}

// declaredColumns replays the recorded migrations and returns the columns each
// surviving table is expected to have.
//
// Two things are deliberately forgotten. A table a later recorded migration
// dropped or renamed is not expected at all -- the rename case loses the shape
// because the new name has no CREATE to read it from, and guessing would be the
// F4 trap again. And a table any recorded migration ALTERed in a form other
// than ADD COLUMN is dropped from the map entirely: DROP COLUMN and RENAME
// COLUMN would need a column-level replay, and a checker that modelled them
// would be the schema diff this package refuses to be. Forgetting fails open,
// which for a secondary check is the right direction.
//
// ADD COLUMN is followed (decision D-73). It is the one form that only adds a
// name, so the expectation is the CREATE's names plus the added one — and it is
// only applied to a table still being tracked: a table an earlier form made
// the checker forget stays forgotten, and a name that has no CREATE to extend
// gets nothing invented for it.
func (m *Migrator) declaredColumns(ledger []Applied) map[string][]string {
	columns := make(map[string][]string)
	for _, candidate := range m.replayable(ledger) {
		for _, effect := range candidate.Effects {
			if effect.Object.Kind != "table" {
				continue
			}
			if effect.Removed {
				delete(columns, effect.Object.Name)
				continue
			}
			if declared, ok := candidate.Columns[effect.Object.Name]; ok {
				columns[effect.Object.Name] = declared
			} else {
				delete(columns, effect.Object.Name)
			}
		}
		for table, added := range candidate.Added {
			if tracked, ok := columns[table]; ok {
				columns[table] = append(slices.Clone(tracked), added...)
			}
		}
		for _, table := range candidate.Altered {
			delete(columns, table)
		}
	}

	if len(columns) == 0 {
		return nil
	}
	return columns
}

// expectedTables replays the recorded migrations' table effects in version
// order and returns the tables the schema is expected to contain at that
// version.
//
// It is a fold rather than a union, and that is the whole point (finding F4).
// The union of every create is not a description of any schema: the first
// migration that legitimately drops or renames an earlier table would make the
// check demand something the schema is correct not to have, and no remedy could
// clear it -- rebuilding replays the same migrations and reaches the same state.
// Replaying the effects instead asks the only question that has an answer:
// "does this database look the way version N is supposed to look?"
//
// Only recorded versions are replayed: a table a pending migration will create
// is legitimately absent, and one a pending migration will drop is legitimately
// still there.
//
// Non-table objects are skipped; verifySchemaObjects explains why the fold is
// only sound for tables.
func (m *Migrator) expectedTables(ledger []Applied) map[string]struct{} {
	expected := make(map[string]struct{})
	for _, candidate := range m.replayable(ledger) {
		for _, effect := range candidate.Effects {
			if effect.Object.Kind != tableKind {
				continue
			}
			if effect.Removed {
				delete(expected, effect.Object.Name)
				continue
			}
			expected[effect.Object.Name] = struct{}{}
		}
	}
	return expected
}

// unrecordedTables returns the tables that must *not* already exist for the
// pending migrations to run, mapped to the migration that would create each.
//
// This is the other direction of disagreement (finding H2). A ledger that lost
// its rows while the schema kept its tables reads as "not migrated yet", so
// `doctor` called it merely DEGRADED and prescribed `mindrail init`, while
// `init` failed on `table projects already exists` every single time and printed
// a remedy that sent the user back to `init`. The tool's own instructions were a
// loop, and CI stayed green over a repository whose `init` was dead.
//
// Nothing here is healed. Re-inserting the missing rows would record a claim --
// "migration 000001 ran" -- from circumstantial evidence, which is the exact
// kind of lie the ledger verification was added to catch; the tables could have
// been made by hand or by another tool. What the tool owes the user instead is
// an honest name for the condition and a remedy that actually clears it. At
// MR-001 the runtime database holds only the project and workspace rows `init`
// re-derives, so rebuilding it costs nothing that cannot be recreated; a
// milestone that stores something irreplaceable has to revisit this remedy.
//
// The walk is a simulation, not a union, for the same reason the fold above is:
//
//   - A name the recorded fold already expects is not a conflict. The 12-step
//     table rebuild drops a table and recreates it under the same name, and
//     reading that create on its own would refuse a perfectly good upgrade over
//     the table the migration is about to drop itself.
//   - A name any earlier pending effect already created *or removed* is not a
//     conflict either, for the same reason one step further out: the sequence
//     has already dealt with whatever was standing there.
//   - `CREATE TABLE IF NOT EXISTS` is never a conflict. The author said an
//     existing table is acceptable, and refusing would block an init that would
//     have succeeded.
//
// The whole pass is skipped unless every ledger row is one this binary can read
// -- a known version whose file still checksums the same. A database written by
// a newer Mindrail (decision D-25) has tables from migrations this binary cannot
// see, and answering "your schema is ahead of its ledger, rebuild it" there
// would bury RUNTIME_DB_SCHEMA_TOO_NEW under a remedy that destroys the newer
// database. Every other disagreement already has its own diagnosis, so failing
// open whenever one of them is in play is the right bias.
func (m *Migrator) unrecordedTables(ledger []Applied, expected map[string]struct{}) map[string]Migration {
	replayable := m.replayable(ledger)
	if len(replayable) != len(ledger) {
		return nil
	}

	recorded := make(map[int64]struct{}, len(ledger))
	for _, row := range ledger {
		recorded[row.Version] = struct{}{}
	}

	// settled is every name the sequence has already accounted for: the tables
	// the recorded fold expects, plus every table an earlier pending statement
	// creates or drops.
	settled := maps.Clone(expected)
	if settled == nil {
		settled = make(map[string]struct{})
	}

	conflicts := make(map[string]Migration)
	for _, candidate := range m.ordered() {
		if _, applied := recorded[candidate.Version]; applied {
			continue
		}
		for _, effect := range candidate.Effects {
			if effect.Object.Kind != tableKind {
				continue
			}
			name := effect.Object.Name
			if _, accounted := settled[name]; !accounted && !effect.Removed && !effect.IfNotExists {
				conflicts[name] = candidate
			}
			settled[name] = struct{}{}
		}
	}

	if len(conflicts) == 0 {
		return nil
	}
	return conflicts
}

// replayable returns the migrations whose files still describe what the ledger
// says was applied, in ascending version order.
//
// Three exclusions, each for its own reason. A version the ledger does not
// record has not run, so its objects are legitimately absent. A version the
// ledger records that this binary does not have is decision D-25's newer
// database, and this binary cannot say what it created. And a version whose
// file checksum no longer matches the recorded one has been edited since it
// ran: its text is no longer a description of this schema, and deriving
// expectations from it reports an edited migration file as a damaged database
// -- with "rebuild" as the remedy, when the fix is to revert the edit. The
// checksum mismatch itself is Up's to report, and reporting it is only possible
// if this check does not fire first.
//
// Load orders the set, but the fold's correctness depends on the order rather
// than merely benefiting from it, so it is established here too.
func (m *Migrator) replayable(ledger []Applied) []Migration {
	if len(ledger) == 0 {
		return nil
	}

	recorded := make(map[int64]string, len(ledger))
	for _, row := range ledger {
		recorded[row.Version] = row.Checksum
	}

	replayable := make([]Migration, 0, len(m.set))
	for _, candidate := range m.ordered() {
		if checksum, applied := recorded[candidate.Version]; applied && checksum == candidate.Checksum {
			replayable = append(replayable, candidate)
		}
	}
	return replayable
}

// ordered returns the migration set in ascending version order. Load already
// orders it, but every fold in this file is only correct in version order, so
// the order is established where it is depended on rather than assumed.
func (m *Migrator) ordered() []Migration {
	return slices.SortedFunc(slices.Values(m.set), func(a, b Migration) int {
		return cmp.Compare(a.Version, b.Version)
	})
}

// verifyLedger is the fail-closed gate of decisions D-23 and D-25. It runs on
// every start, not only when something is pending, because both conditions it
// detects are invisible from the schema alone.
func (m *Migrator) verifyLedger(ledger []Applied) error {
	known := make(map[int64]Migration, len(m.set))
	for _, candidate := range m.set {
		known[candidate.Version] = candidate
	}

	for _, row := range ledger {
		expected, ok := known[row.Version]
		if !ok {
			return schemaAheadFailure(row)
		}
		if expected.Checksum != row.Checksum {
			return checksumFailure(expected, row)
		}
	}

	return nil
}

// ensureBookkeeping creates the ledger table if it is missing. The existence
// probe comes first so a routine `mindrail init` on an initialised repository
// does not take the write lock just to run a no-op DDL.
func (m *Migrator) ensureBookkeeping(ctx context.Context) error {
	present, err := m.bookkeepingExists(ctx)
	if err != nil {
		return err
	}
	if present {
		return nil
	}

	// The name has to be free, not merely free of tables. `CREATE TABLE IF NOT
	// EXISTS` matches on the name and not on the kind, so a view standing under
	// schema_migrations makes the create a silent no-op; the INSERT that records
	// the first migration then fails inside the apply transaction, and the whole
	// thing is reported as a broken migration with "inspect the migration SQL and
	// re-run `mindrail init`" -- which fails identically, forever. The lookup
	// costs one query on the only path that reaches it: an `init` that has no
	// ledger table yet.
	kind, occupied, err := m.bookkeepingOccupant(ctx)
	if err != nil {
		return m.unreadableFailure(ctx, ledgerDescription, fmt.Errorf("look up %s: %w", bookkeepingTable, err))
	}
	if occupied {
		return m.bookkeepingOccupiedFailure(ctx, kind)
	}

	err = storage.InTx(ctx, m.db, func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, bookkeepingDDL)
		return err
	})
	if err != nil {
		return bookkeepingCreateFailure(ctx, m.db, err)
	}

	return nil
}

// bookkeepingCreateFailure reports a ledger table that could not be created.
//
// The generic remedy it falls back to -- "check that the runtime database is
// writable and not held open by another process" -- names both conditions
// because it can distinguish neither. When the driver can, the specific answer
// wins: contention is a wait rather than a fault, an unwritable file is a chmod
// on a named path, and neither is worth an exit 1 (finding W1).
func bookkeepingCreateFailure(ctx context.Context, q storage.Querier, cause error) error {
	if named := storage.WriteFailure(ctx, q, "create the migration bookkeeping table", cause); named != nil {
		return named
	}

	return app.NewError(
		app.CodeMigrationFailed,
		app.KindFailed,
		"the migration bookkeeping table could not be created",
		"Mindrail cannot tell which schema migrations have run, so it will not write to this database.",
		"Check that the runtime database is writable and not held open by another process.",
	).WithCause(fmt.Errorf("%w: create %s: %w", ErrApplyFailed, bookkeepingTable, cause))
}

func (m *Migrator) bookkeepingExists(ctx context.Context) (bool, error) {
	var count int
	err := m.db.QueryRowContext(ctx,
		`SELECT count(*) FROM sqlite_master WHERE type = ? AND name = ?`, tableKind, bookkeepingTable).Scan(&count)
	if err != nil {
		return false, m.unreadableFailure(ctx, ledgerDescription, fmt.Errorf("look up %s: %w", bookkeepingTable, err))
	}
	return count > 0, nil
}

// bookkeepingOccupant reports what is standing under the ledger table's name
// when it is not a table.
func (m *Migrator) bookkeepingOccupant(ctx context.Context) (string, bool, error) {
	var kind string
	err := m.db.QueryRowContext(ctx,
		`SELECT type FROM sqlite_master WHERE name = ? AND type <> ?`, bookkeepingTable, tableKind).Scan(&kind)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return "", false, nil
	case err != nil:
		return "", false, err
	}
	return kind, true, nil
}

func readLedgerRow(ctx context.Context, q storage.Querier, version int64) (Applied, bool, error) {
	row := Applied{Version: version}

	var appliedAt string
	err := q.QueryRowContext(ctx,
		`SELECT name, checksum, applied_at FROM schema_migrations WHERE version = ?`, version).
		Scan(&row.Name, &row.Checksum, &appliedAt)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return Applied{}, false, nil
	case err != nil:
		return Applied{}, false, err
	}

	row.AppliedAt, err = app.ParseTime(appliedAt)
	if err != nil {
		return Applied{}, false, err
	}
	return row, true, nil
}

func highestVersion(ledger []Applied) int64 {
	var highest int64
	for _, row := range ledger {
		if row.Version > highest {
			highest = row.Version
		}
	}
	return highest
}

// applyFailure reports a migration that did not run.
//
// The driver's result code is consulted first, because three of the reasons a
// write fails are not about the migration at all. A database another process
// holds, one whose file refuses writes, and a full disk were every one of them
// reported as MIGRATION_FAILED at exit 1 -- "this operation is broken", with a
// remedy telling the user to inspect SQL that is fine -- when decision D-03 rates
// contention and an unwritable runtime path at exit 4, the code a caller retries
// on (finding W1). What is left over is a genuine migration failure and keeps
// the diagnosis below.
func (m *Migrator) applyFailure(ctx context.Context, pending Migration, cause error) error {
	if named := storage.WriteFailure(ctx, m.db,
		fmt.Sprintf("apply migration %06d_%s", pending.Version, pending.Name), cause); named != nil {
		return named
	}

	return app.NewError(
		app.CodeMigrationFailed,
		app.KindFailed,
		fmt.Sprintf("migration %06d_%s could not be applied", pending.Version, pending.Name),
		"The runtime database is left on the previous schema; Mindrail will not write to it.",
		"Inspect the migration SQL and the database, then re-run `mindrail init`.",
	).
		WithMetadata("version", strconv.FormatInt(pending.Version, 10)).
		WithMetadata("name", pending.Name).
		WithCause(fmt.Errorf("%w: %w", ErrApplyFailed, cause))
}

func checksumFailure(expected Migration, recorded Applied) error {
	return app.NewError(
		app.CodeMigrationChecksumMismatch,
		app.KindFailed,
		fmt.Sprintf("migration %06d_%s has changed since it was applied", expected.Version, expected.Name),
		"The database schema no longer matches the migration files, so later migrations cannot be trusted.",
		"Revert the edit to the applied migration file and add a new numbered migration instead.",
	).
		WithMetadata("version", strconv.FormatInt(expected.Version, 10)).
		WithMetadata("recorded_checksum", recorded.Checksum).
		WithMetadata("file_checksum", expected.Checksum).
		WithCause(ErrChecksumMismatch)
}

// schemaObjectFailure reports a ledger that describes a schema the database no
// longer has. The remedy is a rebuild rather than `mindrail init`, because init
// would find every version already recorded, apply nothing, and leave the
// database in exactly the state that produced this error.
func (m *Migrator) schemaObjectFailure(ctx context.Context, names []string) error {
	detail := make([]string, 0, len(names))
	for _, name := range names {
		detail = append(detail, tableKind+" "+name)
	}
	listed := strings.Join(detail, ", ")

	return app.NewError(
		app.CodeMigrationFailed,
		app.KindFailed,
		"the schema does not contain what the applied migrations created: "+listed,
		"The recorded schema version does not describe this database, so any command that trusted it would fail on a missing object.",
		m.rebuildRemedy(ctx),
	).
		WithMetadata("missing", listed).
		WithMetadata("missing_count", strconv.Itoa(len(names))).
		WithCause(ErrSchemaObjectMissing)
}

// schemaShapeFailure reports a table that kept its name and lost its columns.
// The remedy is the rebuild rather than `mindrail init`, for the same reason
// schemaObjectFailure's is: every version is already recorded, so init would
// apply nothing and leave the table exactly as it is.
func (m *Migrator) schemaShapeFailure(ctx context.Context, missing map[string][]string) error {
	tables := slices.Sorted(maps.Keys(missing))

	detail := make([]string, 0, len(tables))
	for _, table := range tables {
		slices.Sort(missing[table])
		detail = append(detail, table+" ("+strings.Join(missing[table], ", ")+")")
	}
	listed := strings.Join(detail, "; ")

	return app.NewError(
		app.CodeMigrationFailed,
		app.KindFailed,
		"the schema is missing columns the applied migrations declared: "+listed,
		"The recorded schema version does not describe this database, so the next write would fail on a column this report called healthy.",
		m.rebuildRemedy(ctx),
	).
		WithMetadata("missing_columns", listed).
		WithMetadata("missing_table_count", strconv.Itoa(len(tables))).
		WithCause(ErrSchemaShapeChanged)
}

// schemaBehindLedgerFailure reports the ledger that lost rows the schema still
// reflects (finding H2).
//
// The migration named is the lowest-versioned unrecorded one that collides,
// because that is the one `mindrail init` reaches first and therefore the one
// whose failure the user would otherwise be reading.
//
// The remedy is the rebuild and not `mindrail init`, because init is the command
// that cannot work here: it would take the missing rows as work to do and fail
// on the first `CREATE TABLE`, every time, forever. A rebuilt database has no
// tables to collide with, so this is a remedy that ends the loop rather than
// re-entering it.
func (m *Migrator) schemaBehindLedgerFailure(ctx context.Context, conflicts []string, unrecorded map[string]Migration) error {
	first := unrecorded[conflicts[0]]
	for _, name := range conflicts[1:] {
		if unrecorded[name].Version < first.Version {
			first = unrecorded[name]
		}
	}

	listed := strings.Join(conflicts, ", ")
	migrationName := fmt.Sprintf("%06d_%s", first.Version, first.Name)

	return app.NewError(
		app.CodeMigrationFailed,
		app.KindFailed,
		"the migration ledger does not record migration "+migrationName+
			", but the tables it creates are already in this database: "+listed,
		"The ledger no longer describes this database, so `mindrail init` would try to create tables that already exist and would fail the same way every time it ran.",
		m.rebuildRemedy(ctx),
	).
		WithMetadata("unrecorded_version", strconv.FormatInt(first.Version, 10)).
		WithMetadata("unrecorded_name", first.Name).
		WithMetadata("existing_tables", listed).
		WithMetadata("existing_table_count", strconv.Itoa(len(conflicts))).
		WithCause(ErrSchemaBehindLedger)
}

// bookkeepingOccupiedFailure reports a runtime database in which something that
// is not the ledger holds the ledger's name. Every run of `mindrail init` fails
// on the same `CREATE TABLE IF NOT EXISTS`, so the remedy has to be the rebuild
// rather than another init.
func (m *Migrator) bookkeepingOccupiedFailure(ctx context.Context, kind string) error {
	return app.NewError(
		app.CodeMigrationFailed,
		app.KindFailed,
		"the runtime database has a "+kind+" named "+bookkeepingTable+", which is the name the migration ledger needs",
		"Mindrail cannot record which schema migrations have run, so `mindrail init` would fail on the same name every time it ran.",
		m.rebuildRemedy(ctx),
	).
		WithMetadata("occupying_kind", kind).
		WithCause(fmt.Errorf("%w: sqlite_master holds a %s named %s", ErrBookkeepingOccupied, kind, bookkeepingTable))
}

// unreadableFailure turns a failure to read the runtime database's own
// bookkeeping into the four things a user can act on (finding H3).
//
// Without it the driver's own words -- `no such column: checksum (1)`, or
// database/sql's `converting driver.Value type string ("one") to a int64` --
// arrived as the user-facing `why` of a MIGRATION_FAILED, under a `next_action`
// recommending the `mindrail init` that had just failed in exactly the same way.
// The raw text stays on as the cause, which is where `--verbose` prints it and
// where a maintainer can still read it.
//
// A cancelled or expired context is told apart because it is not damage: telling
// someone who pressed Ctrl-C to move their database aside would be a worse
// remedy than the one this function exists to replace.
func (m *Migrator) unreadableFailure(ctx context.Context, what string, cause error) error {
	if errors.Is(cause, context.Canceled) || errors.Is(cause, context.DeadlineExceeded) {
		return app.NewError(
			app.CodeMigrationFailed,
			app.KindFailed,
			"reading "+what+" was interrupted before it finished",
			"Mindrail cannot confirm which schema migrations have run, so it will not write to this database.",
			"Run the command again.",
		).WithCause(cause)
	}

	return app.NewError(
		app.CodeMigrationFailed,
		app.KindFailed,
		what+" could not be read",
		"Mindrail cannot confirm which schema migrations have run, so it will not trust or write to this database.",
		m.rebuildRemedy(ctx),
	).WithCause(cause)
}

func schemaAheadFailure(row Applied) error {
	return app.NewError(
		app.CodeRuntimeDBSchemaTooNew,
		app.KindFailed,
		fmt.Sprintf("the runtime database records migration %d, which this binary does not know", row.Version),
		"A newer Mindrail wrote this database; continuing could corrupt data this binary cannot interpret.",
		"Upgrade Mindrail to a version that includes migration "+strconv.FormatInt(row.Version, 10)+".",
	).
		WithMetadata("database_version", strconv.FormatInt(row.Version, 10)).
		WithCause(ErrSchemaAhead)
}
