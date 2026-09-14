package coordination

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/PsyChaos/mindrail/internal/app"
	"github.com/PsyChaos/mindrail/internal/migration"
	"github.com/PsyChaos/mindrail/internal/storage"
	"github.com/PsyChaos/mindrail/migrations"
)

// TestTheNewestCheckpointQueryDoesNotSortTheProject is finding F48.
//
// `status` runs this query on every start, and it used to be a join driven from
// tasks: the plan searched idx_tasks_project, probed the checkpoint index once
// per task and then built a temp b-tree to sort what came back. The work was
// therefore proportional to the number of tasks in the project even when the
// project had no checkpoints at all — 1.2 ms at 1,000 tasks and 7–29 ms at
// 20,000 depending on what is timed around the query, on a command whose whole
// budget is 150 ms. (This comment said "1.2 ms at 20,000" until audit round 2,
// §4.11, traced the figure to the wrong row of round 1's table.)
//
// The plan is asserted rather than the timing. A timing test on a query this
// fast measures the machine it runs on; the temp b-tree is the thing that makes
// the cost grow with the task count, and its absence is what has to hold.
func TestTheNewestCheckpointQueryDoesNotSortTheProject(t *testing.T) {
	plan := queryPlan(t, selectNewestCheckpointOfProject)

	if strings.Contains(plan, "TEMP B-TREE") {
		t.Errorf("the newest-checkpoint query sorts, so its cost grows with the project:\n%s", plan)
	}
	// The other half: the scan has to be over checkpoints, which is what makes
	// LIMIT 1 stop at the first row rather than after the whole project.
	if !strings.Contains(plan, "SCAN c") {
		t.Errorf("the plan no longer walks the checkpoints in row order:\n%s", plan)
	}
}

// queryPlan returns SQLite's plan for a statement, against a database carrying
// the real schema.
func queryPlan(t *testing.T, query string) string {
	t.Helper()

	db := schemaOnlyDatabase(t)

	rows, err := db.QueryContext(t.Context(), "EXPLAIN QUERY PLAN "+query, "PRJ-1")
	if err != nil {
		t.Fatalf("EXPLAIN QUERY PLAN: %v", err)
	}
	defer func() { _ = rows.Close() }()

	var plan strings.Builder
	for rows.Next() {
		var (
			id, parent, notUsed int
			detail              string
		)
		if err := rows.Scan(&id, &parent, &notUsed, &detail); err != nil {
			t.Fatalf("scanning the plan: %v", err)
		}
		plan.WriteString(detail)
		plan.WriteString("\n")
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("reading the plan: %v", err)
	}
	if plan.Len() == 0 {
		t.Fatal("SQLite returned no plan, so this proves nothing")
	}
	return plan.String()
}

// schemaOnlyDatabase is the real schema with no rows in it. The plan SQLite
// picks depends on the indexes, which the migrations create, and not on what
// the tables hold — and a fixture with rows would invite a test that measured
// them instead.
func schemaOnlyDatabase(t *testing.T) *storage.DB {
	t.Helper()

	db, err := storage.Open(t.Context(), storage.Options{
		Path: filepath.Join(t.TempDir(), "mindrail.db"),
	})
	if err != nil {
		t.Fatalf("storage.Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	set, err := migration.Load(migrations.FS)
	if err != nil {
		t.Fatalf("migration.Load: %v", err)
	}
	clock := app.FixedClock{Instant: time.Date(2026, 3, 14, 9, 26, 53, 0, time.UTC)}
	if _, err := migration.New(db.DB, set, clock).Up(t.Context()); err != nil {
		t.Fatalf("migration Up: %v", err)
	}
	return db
}
