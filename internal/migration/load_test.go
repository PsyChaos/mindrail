package migration_test

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"maps"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/PsyChaos/mindrail/internal/migration"
	"github.com/PsyChaos/mindrail/migrations"
)

var filenameGrammar = regexp.MustCompile(`^[0-9]{6}_[a-z0-9_]+\.sql$`)

// TestLoadEmbeddedMigrationsAreOrderedAndUnique checks the real embedded set,
// not a fixture: the ordering guarantee is only worth anything if it holds for
// the files that actually ship.
func TestLoadEmbeddedMigrationsAreOrderedAndUnique(t *testing.T) {
	loaded, err := migration.Load(migrations.FS)
	if err != nil {
		t.Fatalf("Load(migrations.FS) = %v, want no error", err)
	}
	if len(loaded) == 0 {
		t.Fatal("Load(migrations.FS) returned no migrations, want at least the initial schema")
	}

	entries, err := fs.ReadDir(migrations.FS, ".")
	if err != nil {
		t.Fatalf("ReadDir = %v, want no error", err)
	}
	sqlFiles := 0
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".sql") {
			sqlFiles++
		}
	}
	if len(loaded) != sqlFiles {
		t.Errorf("Load returned %d migrations for %d .sql files; a file was skipped", len(loaded), sqlFiles)
	}

	previous := int64(0)
	for i, m := range loaded {
		if m.Version <= previous {
			t.Errorf("migration %d has version %d, want strictly greater than %d", i, m.Version, previous)
		}
		previous = m.Version

		filename := fmt.Sprintf("%06d_%s.sql", m.Version, m.Name)
		if !filenameGrammar.MatchString(filename) {
			t.Errorf("migration %d reconstructs as %q, which does not match the filename grammar", m.Version, filename)
		}
		if m.SQL == "" {
			t.Errorf("migration %d has empty SQL", m.Version)
		}

		want := sha256.Sum256([]byte(m.SQL))
		if m.Checksum != hex.EncodeToString(want[:]) {
			t.Errorf("migration %d checksum = %q, want the hex sha256 of its SQL", m.Version, m.Checksum)
		}
	}

	// The names are part of the ledger: a migration recorded under one name and
	// shipped under another is the checksum condition arriving through the file
	// system instead of through an edit.
	for i, want := range []struct {
		version int64
		name    string
	}{
		{version: 1, name: "initial"},
		{version: 2, name: "coordination"},
	} {
		if i >= len(loaded) {
			t.Errorf("the embedded set has no migration %d_%s", want.version, want.name)
			continue
		}
		if loaded[i].Version != want.version || loaded[i].Name != want.name {
			t.Errorf("migration %d = %d_%s, want %d_%s",
				i, loaded[i].Version, loaded[i].Name, want.version, want.name)
		}
	}
}

// tablesPerMilestone is the scope boundary each migration draws, stated per
// migration rather than as one set.
//
// Symbol identities, bindings and ambiguities belong to MR-006; creating
// their tables early would make that MR's migration unnecessary and its
// schema decisions unreviewable. Keeping the map keyed by version is what
// makes the assertion below say *which* migration overreached rather than
// only that the union grew.
var tablesPerMilestone = map[int64][]string{
	1: {"projects", "workspaces"},                                                              // MR-001
	2: {"sessions", "tasks", "checkpoints"},                                                    // MR-003
	3: {"leases", "operations"},                                                                // MR-004; revision on tasks is an ALTER, not a table
	4: {"project_units", "file_index_state", "symbols", "symbol_imports", "symbol_references"}, // MR-005
	5: {"symbol_identities", "invariant_symbol_bindings", "symbol_identity_ambiguities"},       // MR-006; symbols.symbol_uid is an ALTER, not a table
	6: {"changes", "change_files", "change_symbols", "change_baselines", "change_operations"},  // MR-007
	7: {"scope_attributions"},                                                                  // MR-008
}

// TestEachMigrationCreatesOnlyItsMilestonesTables pins those boundaries.
func TestEachMigrationCreatesOnlyItsMilestonesTables(t *testing.T) {
	loaded, err := migration.Load(migrations.FS)
	if err != nil {
		t.Fatalf("Load = %v, want no error", err)
	}
	if len(loaded) != len(tablesPerMilestone) {
		t.Fatalf("the embedded set holds %d migrations, and this test knows the scope of %d",
			len(loaded), len(tablesPerMilestone))
	}

	createTable := regexp.MustCompile(`(?i)CREATE\s+TABLE\s+(?:IF\s+NOT\s+EXISTS\s+)?([a-z_]+)`)
	for _, m := range loaded {
		declared, known := tablesPerMilestone[m.Version]
		if !known {
			t.Errorf("migration %d is not in this test's scope map, so nothing pins what it may create", m.Version)
			continue
		}

		got := map[string]bool{}
		for _, match := range createTable.FindAllStringSubmatch(m.SQL, -1) {
			got[strings.ToLower(match[1])] = true
		}
		want := map[string]bool{}
		for _, table := range declared {
			want[table] = true
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("migration %d creates %v, want exactly %v", m.Version, keys(got), keys(want))
		}
	}
}

// TestNoDownMigrationsExist enforces tech-stack §24's forward-only rule from
// both directions: no reverse file may ship, and the migrator must expose no
// way to run one even if someone wrote it.
func TestNoDownMigrationsExist(t *testing.T) {
	entries, err := fs.ReadDir(migrations.FS, ".")
	if err != nil {
		t.Fatalf("ReadDir = %v, want no error", err)
	}
	for _, entry := range entries {
		name := strings.ToLower(entry.Name())
		if strings.Contains(name, ".down.") || strings.HasSuffix(name, "_down.sql") {
			t.Errorf("%s is a reverse migration; migrations are forward-only (tech-stack §24)", entry.Name())
		}
	}

	forbidden := []string{"down", "rollback", "revert", "downgrade", "undo"}
	migratorType := reflect.TypeOf(&migration.Migrator{})
	for i := range migratorType.NumMethod() {
		method := strings.ToLower(migratorType.Method(i).Name)
		for _, word := range forbidden {
			if strings.Contains(method, word) {
				t.Errorf("Migrator exposes %q; there is no downgrade entry point by design",
					migratorType.Method(i).Name)
			}
		}
	}
}

func TestLoadRejectsMalformedSets(t *testing.T) {
	tests := []struct {
		name    string
		files   fstest.MapFS
		wantErr string
	}{
		{
			name: "duplicate version",
			files: fstest.MapFS{
				"000001_initial.sql": &fstest.MapFile{Data: []byte("SELECT 1;")},
				"000001_other.sql":   &fstest.MapFile{Data: []byte("SELECT 2;")},
			},
			wantErr: "duplicate",
		},
		{
			name: "reverse migration",
			files: fstest.MapFS{
				"000001_initial.sql":      &fstest.MapFile{Data: []byte("SELECT 1;")},
				"000001_initial.down.sql": &fstest.MapFile{Data: []byte("DROP TABLE projects;")},
			},
			wantErr: "000001_initial.down.sql",
		},
		{
			name:    "wrong digit count",
			files:   fstest.MapFS{"00001_initial.sql": &fstest.MapFile{Data: []byte("SELECT 1;")}},
			wantErr: "00001_initial.sql",
		},
		{
			name:    "uppercase name",
			files:   fstest.MapFS{"000001_Initial.sql": &fstest.MapFile{Data: []byte("SELECT 1;")}},
			wantErr: "000001_Initial.sql",
		},
		{
			name:    "missing version prefix",
			files:   fstest.MapFS{"initial.sql": &fstest.MapFile{Data: []byte("SELECT 1;")}},
			wantErr: "initial.sql",
		},
		{
			name:    "version zero",
			files:   fstest.MapFS{"000000_initial.sql": &fstest.MapFile{Data: []byte("SELECT 1;")}},
			wantErr: "000000_initial.sql",
		},
		{
			name:    "empty body",
			files:   fstest.MapFS{"000001_initial.sql": &fstest.MapFile{Data: []byte("   \n")}},
			wantErr: "000001_initial.sql",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := migration.Load(tt.files)
			if err == nil {
				t.Fatalf("Load(%s) = nil error, want a rejection", tt.name)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("Load(%s) = %q, want it to mention %q", tt.name, err, tt.wantErr)
			}
		})
	}
}

func TestLoadSortsAscendingRegardlessOfDirectoryOrder(t *testing.T) {
	files := fstest.MapFS{
		"000010_ten.sql": &fstest.MapFile{Data: []byte("SELECT 10;")},
		"000002_two.sql": &fstest.MapFile{Data: []byte("SELECT 2;")},
		"000001_one.sql": &fstest.MapFile{Data: []byte("SELECT 1;")},
	}

	loaded, err := migration.Load(files)
	if err != nil {
		t.Fatalf("Load = %v, want no error", err)
	}

	want := []int64{1, 2, 10}
	if len(loaded) != len(want) {
		t.Fatalf("Load returned %d migrations, want %d", len(loaded), len(want))
	}
	for i, version := range want {
		if loaded[i].Version != version {
			t.Errorf("migration %d version = %d, want %d", i, loaded[i].Version, version)
		}
	}
}

func TestLoadIgnoresNonSQLFiles(t *testing.T) {
	files := fstest.MapFS{
		"000001_initial.sql": &fstest.MapFile{Data: []byte("SELECT 1;")},
		"README.md":          &fstest.MapFile{Data: []byte("not a migration")},
		"embed.go":           &fstest.MapFile{Data: []byte("package migrations")},
	}

	loaded, err := migration.Load(files)
	if err != nil {
		t.Fatalf("Load = %v, want no error", err)
	}
	if len(loaded) != 1 {
		t.Errorf("Load returned %d migrations, want 1", len(loaded))
	}
}

func keys(m map[string]bool) []string {
	return slices.Sorted(maps.Keys(m))
}
