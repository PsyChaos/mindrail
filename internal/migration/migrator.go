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
// confirms that the objects the recorded migrations create are still there.
// Without that confirmation a database whose tables were dropped -- by a
// half-finished manual repair, or by a tool that mistook the runtime store for
// scratch space -- reports "Schema up to date (version 1)", and every command
// that trusts the report then fails on a table the report said existed.
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
		return nil, fmt.Errorf("read %s: %w", bookkeepingTable, err)
	}
	defer func() { _ = rows.Close() }()

	var ledger []Applied
	for rows.Next() {
		var (
			row       Applied
			appliedAt string
		)
		if err := rows.Scan(&row.Version, &row.Name, &row.Checksum, &appliedAt); err != nil {
			return nil, fmt.Errorf("scan %s row: %w", bookkeepingTable, err)
		}
		row.AppliedAt, err = app.ParseTime(appliedAt)
		if err != nil {
			return nil, fmt.Errorf("%s version %d: %w", bookkeepingTable, row.Version, err)
		}
		ledger = append(ledger, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read %s: %w", bookkeepingTable, err)
	}

	return ledger, nil
}

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
			return applyFailure(pending, err)
		}
		if found {
			if existing.Checksum != pending.Checksum {
				return checksumFailure(pending, existing)
			}
			row = existing
			return nil
		}

		if _, err := tx.ExecContext(ctx, pending.SQL); err != nil {
			return applyFailure(pending, err)
		}

		now := m.clock.Now().UTC()
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO schema_migrations (version, name, checksum, applied_at) VALUES (?, ?, ?, ?)`,
			pending.Version, pending.Name, pending.Checksum, app.FormatTime(now)); err != nil {
			return applyFailure(pending, err)
		}

		row.AppliedAt = now
		applied = true
		return nil
	})
	if err != nil {
		// InTx's own begin/commit failures are not domain errors yet.
		if _, isDomain := app.PayloadOf(err); !isDomain {
			return Applied{}, false, applyFailure(pending, err)
		}
		return Applied{}, false, err
	}

	return row, applied, nil
}

// verifySchemaObjects checks that the schema still looks the way the recorded
// version says it should: the objects are in sqlite_master, and the tables
// among them still carry the columns their migration declared.
//
// Two queries with a handful of bound names, on purpose: this runs on the warm
// path of every command, so the check has to cost less than the report it
// protects. Existence is asked first because a missing object is the commoner
// damage and names it better; the column pass is a containment assertion and
// nothing more. Types, defaults, constraints and collations are still not
// compared -- that would be the schema diff this package refuses to be, and the
// parsing it does need happens once at Load against embedded files, never here.
func (m *Migrator) verifySchemaObjects(ctx context.Context, ledger []Applied) error {
	if err := m.verifyObjectsExist(ctx, m.declaredObjects(ledger)); err != nil {
		return err
	}
	return m.verifyTableShapes(ctx, m.declaredColumns(ledger))
}

func (m *Migrator) verifyObjectsExist(ctx context.Context, declared map[string]SchemaObject) error {
	if len(declared) == 0 {
		return nil
	}

	names := make([]any, 0, len(declared))
	placeholders := make([]string, 0, len(declared))
	for name := range declared {
		names = append(names, name)
		placeholders = append(placeholders, "?")
	}

	rows, err := m.db.QueryContext(ctx,
		`SELECT type, name FROM sqlite_master WHERE name IN (`+strings.Join(placeholders, ", ")+`)`, names...)
	if err != nil {
		return fmt.Errorf("read sqlite_master: %w", err)
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var kind, name string
		if err := rows.Scan(&kind, &name); err != nil {
			return fmt.Errorf("scan sqlite_master row: %w", err)
		}
		if object, ok := declared[name]; ok && object.Kind == kind {
			delete(declared, name)
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("read sqlite_master: %w", err)
	}
	if len(declared) != 0 {
		return schemaObjectFailure(declared)
	}

	return nil
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
		return fmt.Errorf("read table columns: %w", err)
	}
	defer func() { _ = rows.Close() }()

	present := make(map[string]map[string]struct{}, len(expected))
	for rows.Next() {
		var table, column string
		if err := rows.Scan(&table, &column); err != nil {
			return fmt.Errorf("scan table column row: %w", err)
		}
		if present[table] == nil {
			present[table] = make(map[string]struct{})
		}
		present[table][strings.ToLower(column)] = struct{}{}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("read table columns: %w", err)
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

	return schemaShapeFailure(missing)
}

// declaredColumns replays the recorded migrations and returns the columns each
// surviving table is expected to have.
//
// Two things are deliberately forgotten. A table a later recorded migration
// dropped or renamed is not expected at all -- the rename case loses the shape
// because the new name has no CREATE to read it from, and guessing would be the
// F4 trap again. And a table any recorded migration ALTERed is dropped from the
// map entirely: ALTER can add, drop or rename a column, and a checker that
// modelled all three would be the schema diff this package refuses to be.
// Forgetting fails open, which for a secondary check is the right direction.
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
		for _, table := range candidate.Altered {
			delete(columns, table)
		}
	}

	if len(columns) == 0 {
		return nil
	}
	return columns
}

// declaredObjects replays the recorded migrations' schema effects in version
// order and returns what the schema is expected to contain at that version,
// keyed by name.
//
// It is a fold rather than a union, and that is the whole point (finding F4).
// The union of every create is not a description of any schema: the first
// migration that legitimately drops or renames an earlier object would make the
// check demand something the schema is correct not to have, and no remedy could
// clear it -- rebuilding replays the same migrations and reaches the same state.
// Replaying the effects instead asks the only question that has an answer:
// "does this database look the way version N is supposed to look?"
//
// Only recorded versions are replayed: an object a pending migration will
// create is legitimately absent, and one a pending migration will drop is
// legitimately still there.
//
// Names are unique across types in sqlite_master, so one map keyed by name
// cannot conflate a table with an index of the same name.
func (m *Migrator) declaredObjects(ledger []Applied) map[string]SchemaObject {
	declared := make(map[string]SchemaObject)
	for _, candidate := range m.replayable(ledger) {
		for _, effect := range candidate.Effects {
			if effect.Removed {
				delete(declared, effect.Object.Name)
				continue
			}
			declared[effect.Object.Name] = effect.Object
		}
	}
	return declared
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

	ordered := slices.SortedFunc(slices.Values(m.set), func(a, b Migration) int {
		return cmp.Compare(a.Version, b.Version)
	})

	replayable := make([]Migration, 0, len(ordered))
	for _, candidate := range ordered {
		if checksum, applied := recorded[candidate.Version]; applied && checksum == candidate.Checksum {
			replayable = append(replayable, candidate)
		}
	}
	return replayable
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

	err = storage.InTx(ctx, m.db, func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, bookkeepingDDL)
		return err
	})
	if err != nil {
		return app.NewError(
			app.CodeMigrationFailed,
			app.KindFailed,
			"the migration bookkeeping table could not be created",
			"Mindrail cannot tell which schema migrations have run, so it will not write to this database.",
			"Check that the runtime database is writable and not held open by another process.",
		).WithCause(fmt.Errorf("%w: create %s: %w", ErrApplyFailed, bookkeepingTable, err))
	}

	return nil
}

func (m *Migrator) bookkeepingExists(ctx context.Context) (bool, error) {
	var count int
	err := m.db.QueryRowContext(ctx,
		`SELECT count(*) FROM sqlite_master WHERE type = 'table' AND name = ?`, bookkeepingTable).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("look up %s: %w", bookkeepingTable, err)
	}
	return count > 0, nil
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

func applyFailure(pending Migration, cause error) error {
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
func schemaObjectFailure(missing map[string]SchemaObject) error {
	names := slices.Sorted(maps.Keys(missing))

	detail := make([]string, 0, len(names))
	for _, name := range names {
		detail = append(detail, missing[name].Kind+" "+name)
	}
	listed := strings.Join(detail, ", ")

	return app.NewError(
		app.CodeMigrationFailed,
		app.KindFailed,
		"the schema does not contain what the applied migrations created: "+listed,
		"The recorded schema version does not describe this database, so any command that trusted it would fail on a missing object.",
		"Move the runtime database aside and run `mindrail init` to rebuild it.",
	).
		WithMetadata("missing", listed).
		WithMetadata("missing_count", strconv.Itoa(len(names))).
		WithCause(ErrSchemaObjectMissing)
}

// schemaShapeFailure reports a table that kept its name and lost its columns.
// The remedy is the rebuild rather than `mindrail init`, for the same reason
// schemaObjectFailure's is: every version is already recorded, so init would
// apply nothing and leave the table exactly as it is.
func schemaShapeFailure(missing map[string][]string) error {
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
		"Move the runtime database aside and run `mindrail init` to rebuild it.",
	).
		WithMetadata("missing_columns", listed).
		WithMetadata("missing_table_count", strconv.Itoa(len(tables))).
		WithCause(ErrSchemaShapeChanged)
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
