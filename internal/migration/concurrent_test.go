package migration_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/PsyChaos/mindrail/internal/migration"
	"github.com/PsyChaos/mindrail/internal/storage"
	"github.com/PsyChaos/mindrail/migrations"
)

// helperDBEnv turns this test binary into the racer described below. It has to
// be a re-exec rather than a goroutine: SQLite's file locks are held by the
// process, so two goroutines in one process never contend for the lock that
// decision D-24 is about, and a WAL conversion performed once inside the parent
// is already finished before any goroutine can observe it. That is exactly why
// the earlier in-process version of this test passed against an implementation
// with no retry at all.
const helperDBEnv = "MINDRAIL_TEST_MIGRATION_RACER_DB"

// Exit codes the racer uses to tell the parent what it did. They are distinct
// values rather than a shared "0 means fine" because the assertion below is
// about *which* process applied the set, not merely that nobody crashed.
const (
	racerApplied  = 10 // this process ran the migrations
	racerObserved = 11 // another process had already run them
	racerOpen     = 12 // storage.Open refused
	racerMigrate  = 13 // Migrator.Up refused
	racerLoad     = 14 // the embedded set would not load
)

func TestMain(m *testing.M) {
	if path := os.Getenv(helperDBEnv); path != "" {
		os.Exit(runRacer(path))
	}
	os.Exit(m.Run())
}

// runRacer is one contender: open the database and bring it to the current
// schema, the same two calls `mindrail init` makes.
func runRacer(path string) int {
	ctx := context.Background()

	set, err := migration.Load(migrations.FS)
	if err != nil {
		fmt.Fprintf(os.Stderr, "load: %v\n", err)
		return racerLoad
	}

	db, err := storage.Open(ctx, storage.Options{Path: path})
	if err != nil {
		fmt.Fprintf(os.Stderr, "open: %v\n", err)
		return racerOpen
	}
	defer func() { _ = db.Close() }()

	result, err := migration.New(db.DB, set, fixedClock()).Up(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "up: %v\n", err)
		return racerMigrate
	}

	if len(result.Applied) > 0 {
		return racerApplied
	}
	return racerObserved
}

// racerRounds and racersPerRound size the race. One round is one brand-new
// database, which is the only moment the WAL conversion happens and therefore
// the only moment the contention exists; a single round is flaky by nature, so
// the test buys its confidence with several.
const (
	racerRounds     = 5
	racersPerRound  = 5
	racerTotalCount = racerRounds * racersPerRound
)

// TestConcurrentInitAcrossProcessesWaits is decision D-24's actual promise:
// several processes reaching a brand-new database at once all succeed, because
// the loser blocks until the winner is done instead of giving up.
//
// The assertion is deliberately absolute -- every process exits cleanly, in
// every round. A single SQLITE_BUSY that escaped as a hard failure is the
// production symptom (`mindrail init` exiting 4 on a repository it shares with
// a hook or a second shell), so tolerating "most of them" here would tolerate
// exactly the bug.
func TestConcurrentInitAcrossProcessesWaits(t *testing.T) {
	if testing.Short() {
		t.Skip("spawns processes")
	}

	binary, err := os.Executable()
	if err != nil {
		t.Fatalf("os.Executable = %v, want no error", err)
	}

	set := embeddedSet(t)
	appliers := 0

	for round := range racerRounds {
		path := filepath.Join(t.TempDir(), "mindrail.db")

		var (
			wg     sync.WaitGroup
			mu     sync.Mutex
			codes  = make([]int, racersPerRound)
			output = make([]string, racersPerRound)
		)
		for racer := range racersPerRound {
			wg.Add(1)
			go func() {
				defer wg.Done()

				cmd := exec.Command(binary)
				cmd.Env = append(os.Environ(), helperDBEnv+"="+path)
				out, runErr := cmd.CombinedOutput()

				code := cmd.ProcessState.ExitCode()
				var exitErr *exec.ExitError
				if runErr != nil && !errors.As(runErr, &exitErr) {
					code = -1
					out = append(out, []byte("spawn: "+runErr.Error())...)
				}

				mu.Lock()
				codes[racer], output[racer] = code, strings.TrimSpace(string(out))
				mu.Unlock()
			}()
		}
		wg.Wait()

		for racer, code := range codes {
			switch code {
			case racerApplied:
				appliers++
			case racerObserved:
			default:
				t.Errorf("round %d racer %d exited %d, want %d (applied) or %d (observed): %s",
					round, racer, code, racerApplied, racerObserved, output[racer])
			}
		}

		assertLedgerAppliedOnce(t, path, len(set))
	}

	if appliers != racerRounds {
		t.Errorf("%d of %d processes applied the set across %d fresh databases, want exactly %d",
			appliers, racerTotalCount, racerRounds, racerRounds)
	}
}

// assertLedgerAppliedOnce is the other half of D-24: waiting is only correct if
// the winner's work is what the loser then observes, so no migration may be
// recorded twice and the schema must really be there.
func assertLedgerAppliedOnce(t *testing.T, path string, want int) {
	t.Helper()

	db := openDB(t, path)

	var rows int
	if err := db.QueryRowContext(t.Context(), `SELECT count(*) FROM schema_migrations`).Scan(&rows); err != nil {
		t.Fatalf("count schema_migrations = %v, want no error", err)
	}
	if rows != want {
		t.Errorf("schema_migrations has %d rows, want %d; a migration was applied more than once", rows, want)
	}

	for _, table := range []string{"projects", "workspaces"} {
		if !tableExists(t, db, table) {
			t.Errorf("table %q is missing after the race; the winner's schema was not committed", table)
		}
	}
}
