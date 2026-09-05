// Package migration applies the embedded SQL schema files in order, exactly
// once each, and records what it did in schema_migrations.
//
// It is deliberately not a migration framework (tech-stack §24): numbered
// files, forward-only, one transaction per migration, fail fast. There is no
// downgrade entry point, because a downgrade that ran against data written by
// the newer schema would have to invent what to do with it.
package migration

import (
	"cmp"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"path"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

// Migration is one embedded SQL file, addressed by version.
type Migration struct {
	Version  int64
	Name     string
	SQL      string
	Checksum string // hex sha256 of SQL
	Objects  []SchemaObject
	Effects  []SchemaEffect

	// Columns lists the column names each CREATE TABLE in this file declares,
	// keyed by table name. It is what makes "the table exists" separable from
	// "the table is the table this migration made" (finding F9).
	Columns map[string][]string

	// Altered names the tables this file changes with ALTER TABLE, in any of
	// its forms. It is a deliberate bail-out, not a description: a recorded
	// migration that alters a table means the shape recorded at CREATE time is
	// no longer what the schema should look like, and the checker gives up on
	// that table rather than growing a column-level replay of its own -- which
	// is exactly the trap finding F4 was about, one level down.
	Altered []string
}

// SchemaObject is one persistent database object a migration's SQL creates.
// Kind is spelled the way sqlite_master.type spells it, so verifying an object
// is a comparison rather than a translation.
type SchemaObject struct {
	Kind string // table | index | view | trigger
	Name string
}

// SchemaEffect is one change a migration makes to the set of objects the schema
// is expected to contain, in the order the file makes it.
//
// Creates alone are not enough to say what a schema should look like. A
// migration that drops or renames an object created by an earlier one leaves
// that earlier object legitimately absent, and a verification built from the
// union of every create would call the resulting database damaged forever --
// including the rebuild its own remedy asked for, which replays the same set
// and lands in the same state (finding F4). Order matters within a file too: a
// migration that drops an object and recreates it ends with the object present.
type SchemaEffect struct {
	Object  SchemaObject
	Removed bool

	// IfNotExists marks a create the author wrote as `CREATE ... IF NOT
	// EXISTS`, which is a statement about what the migration tolerates: an
	// object already standing under that name is not an error and the statement
	// is a no-op. Anything that reasons about what must be *absent* before a
	// migration runs has to honour that, or it refuses an init the database
	// would have accepted.
	IfNotExists bool
}

// Removes lists the objects this migration takes away, in file order. It is the
// mirror of Objects and exists for the same reason: so a caller can state what
// a migration does to the schema without re-reading its SQL.
func (m Migration) Removes() []SchemaObject {
	removed := make([]SchemaObject, 0, len(m.Effects))
	for _, effect := range m.Effects {
		if effect.Removed {
			removed = append(removed, effect.Object)
		}
	}
	return removed
}

// filenamePattern is the whole naming contract. The six-digit zero-padded
// version keeps directory listings and the sort order in agreement, and the
// restricted name charset means a migration cannot be renamed into a different
// one by case alone on a case-insensitive filesystem.
var filenamePattern = regexp.MustCompile(`^([0-9]{6})_([a-z0-9_]+)\.sql$`)

// Load reads every migration in the root of fsys, sorted by ascending version.
//
// It rejects rather than skips: a file that does not match the grammar is far
// more likely to be a migration someone misnamed -- 000001_initial.down.sql,
// or 00001_initial.sql -- than a stray note, and silently ignoring it would
// mean the schema quietly differed from what the author wrote.
func Load(fsys fs.FS) ([]Migration, error) {
	entries, err := fs.ReadDir(fsys, ".")
	if err != nil {
		return nil, fmt.Errorf("read migration directory: %w", err)
	}

	loaded := make([]Migration, 0, len(entries))
	seen := make(map[int64]string, len(entries))

	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || path.Ext(name) != ".sql" {
			continue
		}

		match := filenamePattern.FindStringSubmatch(name)
		if match == nil {
			return nil, fmt.Errorf("migration file %q does not match %s", name, filenamePattern)
		}

		version, err := strconv.ParseInt(match[1], 10, 64)
		if err != nil {
			return nil, fmt.Errorf("migration file %q has an unparseable version: %w", name, err)
		}
		if version < 1 {
			return nil, fmt.Errorf("migration file %q has version %d; versions start at 1", name, version)
		}
		if previous, duplicate := seen[version]; duplicate {
			return nil, fmt.Errorf("duplicate migration version %d in %q and %q", version, previous, name)
		}

		body, err := fs.ReadFile(fsys, name)
		if err != nil {
			return nil, fmt.Errorf("read migration file %q: %w", name, err)
		}
		if strings.TrimSpace(string(body)) == "" {
			return nil, fmt.Errorf("migration file %q is empty", name)
		}

		checksum := sha256.Sum256(body)
		seen[version] = name
		effects := schemaEffects(string(body))
		loaded = append(loaded, Migration{
			Version:  version,
			Name:     match[2],
			SQL:      string(body),
			Checksum: hex.EncodeToString(checksum[:]),
			Objects:  createdObjects(effects),
			Effects:  effects,
			Columns:  tableColumns(string(body)),
			Altered:  alteredTables(string(body)),
		})
	}

	slices.SortFunc(loaded, func(a, b Migration) int {
		return cmp.Compare(a.Version, b.Version)
	})

	return loaded, nil
}

// effectPattern finds the persistent objects a migration adds and removes.
//
// It reads the statement heads only, which is all the verification needs: the
// question is "does the ledger's claim that this migration ran describe this
// database?", and an object that the recorded migrations should have left
// behind but is missing from sqlite_master answers it without parsing a single
// column definition. The anchor keeps it from matching a statement mentioned
// inside a `--` comment or a string literal on a continuation line.
//
// The three alternatives are the three ways the expected set changes: a CREATE
// adds, a DROP removes, and an `ALTER TABLE ... RENAME TO` does both. There is
// deliberately no fourth: everything else a migration can write leaves the set
// of object names alone.
var effectPattern = regexp.MustCompile(`(?im)^[\t ]*(?:` +
	`CREATE[\t ]+((?:(?:UNIQUE|TEMP|TEMPORARY|VIRTUAL)[\t ]+)*)(TABLE|INDEX|VIEW|TRIGGER)[\t ]+(IF[\t ]+NOT[\t ]+EXISTS[\t ]+)?(` + identifier + `)` +
	`|DROP[\t ]+(TABLE|INDEX|VIEW|TRIGGER)[\t ]+(?:IF[\t ]+EXISTS[\t ]+)?(` + identifier + `)` +
	`|ALTER[\t ]+TABLE[\t ]+(` + identifier + `)[\t ]+RENAME[\t ]+TO[\t ]+(` + identifier + `)` +
	`)`)

// identifier is a bare or double-quoted SQLite object name.
const identifier = `"[^"]+"|[A-Za-z_][A-Za-z0-9_$]*`

// Submatch indices of effectPattern, named because seven positional groups in
// one alternation is where an off-by-one hides.
const (
	groupCreateModifiers   = 1
	groupCreateKind        = 2
	groupCreateIfNotExists = 3
	groupCreateName        = 4
	groupDropKind          = 5
	groupDropName          = 6
	groupRenameFrom        = 7
	groupRenameTo          = 8
)

// schemaEffects lists what body does to the expected object set, in file order.
// TEMP objects are skipped: they live only for the connection that made them,
// so their absence later says nothing about the schema.
func schemaEffects(body string) []SchemaEffect {
	matches := effectPattern.FindAllStringSubmatch(body, -1)
	effects := make([]SchemaEffect, 0, len(matches))

	for _, match := range matches {
		switch {
		case match[groupCreateKind] != "":
			if strings.Contains(strings.ToUpper(match[groupCreateModifiers]), "TEMP") {
				continue
			}
			effects = append(effects, SchemaEffect{
				Object: SchemaObject{
					Kind: strings.ToLower(match[groupCreateKind]),
					Name: unquote(match[groupCreateName]),
				},
				IfNotExists: match[groupCreateIfNotExists] != "",
			})
		case match[groupDropKind] != "":
			effects = append(effects, SchemaEffect{
				Object: SchemaObject{
					Kind: strings.ToLower(match[groupDropKind]),
					Name: unquote(match[groupDropName]),
				},
				Removed: true,
			})
		case match[groupRenameFrom] != "":
			effects = append(effects,
				SchemaEffect{
					Object:  SchemaObject{Kind: "table", Name: unquote(match[groupRenameFrom])},
					Removed: true,
				},
				SchemaEffect{
					Object: SchemaObject{Kind: "table", Name: unquote(match[groupRenameTo])},
				},
			)
		}
	}

	return effects
}

// createdObjects is the creates of effects, in file order and without
// duplicates. It is derived rather than parsed a second time so the two views
// of one migration cannot drift apart.
func createdObjects(effects []SchemaEffect) []SchemaObject {
	objects := make([]SchemaObject, 0, len(effects))
	seen := make(map[SchemaObject]struct{}, len(effects))

	for _, effect := range effects {
		if effect.Removed {
			continue
		}
		if _, duplicate := seen[effect.Object]; duplicate {
			continue
		}
		seen[effect.Object] = struct{}{}
		objects = append(objects, effect.Object)
	}

	return objects
}

func unquote(name string) string { return strings.Trim(name, `"`) }
