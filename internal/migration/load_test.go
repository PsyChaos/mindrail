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

	if loaded[0].Version != 1 || loaded[0].Name != "initial" {
		t.Errorf("first migration = %d_%s, want 1_initial", loaded[0].Version, loaded[0].Name)
	}
}

// TestInitialMigrationCreatesOnlyMR001Tables pins the MR-001 scope boundary.
// Sessions, tasks, leases, symbols, changes and evidence belong to later MRs;
// creating their tables early would make those MRs' migrations unnecessary and
// their schema decisions unreviewable.
func TestInitialMigrationCreatesOnlyMR001Tables(t *testing.T) {
	loaded, err := migration.Load(migrations.FS)
	if err != nil {
		t.Fatalf("Load = %v, want no error", err)
	}

	createTable := regexp.MustCompile(`(?i)CREATE\s+TABLE\s+(?:IF\s+NOT\s+EXISTS\s+)?([a-z_]+)`)
	got := map[string]bool{}
	for _, m := range loaded {
		for _, match := range createTable.FindAllStringSubmatch(m.SQL, -1) {
			got[strings.ToLower(match[1])] = true
		}
	}

	want := map[string]bool{"projects": true, "workspaces": true}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("embedded migrations create %v, want exactly %v", keys(got), keys(want))
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
