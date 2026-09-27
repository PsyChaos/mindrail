package changes_test

import (
	"encoding/json"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/PsyChaos/mindrail/internal/app"
	"github.com/PsyChaos/mindrail/internal/changes"
	"github.com/PsyChaos/mindrail/internal/git"
)

// BR-05B: corrupt persisted rows do not have to pass the writer first. Each
// empty-field payload below is canonical JSON, so only the completeness guard
// can reject it; canonicalization alone must not appear to cover that rule.
func TestGuardBaselineRejectsIncompletePersistedMappings(t *testing.T) {
	for _, test := range []struct {
		name  string
		field string
		valid bool
	}{
		{name: "valid_empty_set", valid: true},
		{name: "valid_mapping", valid: true},
		{name: "empty_path", field: "path"},
		{name: "empty_test", field: "test"},
		{name: "empty_production_uid", field: "production_uid"},
		{name: "empty_invariant_id", field: "invariant_id"},
		{name: "malformed_json"},
	} {
		t.Run(test.name, func(t *testing.T) {
			store, db, _ := changesFixtureDB(t)
			seedTaskChain(t, db, "TSK-1")
			root := t.TempDir()
			if _, err := db.DB.ExecContext(t.Context(), `UPDATE workspaces SET root_path = ? WHERE workspace_id = 'WS-1'`, root); err != nil {
				t.Fatal(err)
			}
			const head = "0123456789abcdef0123456789abcdef01234567"
			runner := &git.FakeRunner{Responses: map[string]git.FakeResponse{
				"rev-parse --verify --quiet HEAD^{commit}": {Stdout: head + "\n"},
			}}
			mapping := changes.ProofMapping{
				Path: filepath.Join(root, "test_guard.py"), Test: "test_guard",
				ProductionUID: "SYM-PRODUCTION", InvariantID: "INV-0001",
			}
			switch test.field {
			case "path":
				mapping.Path = ""
			case "test":
				mapping.Test = ""
			case "production_uid":
				mapping.ProductionUID = ""
			case "invariant_id":
				mapping.InvariantID = ""
			}
			mappings := []changes.ProofMapping{mapping}
			if test.name == "valid_empty_set" {
				mappings = []changes.ProofMapping{}
			}
			encoded, err := json.Marshal(mappings)
			if err != nil {
				t.Fatal(err)
			}
			body := string(encoded)
			if test.name == "malformed_json" {
				body = "["
			}
			if _, err := db.DB.ExecContext(t.Context(), `INSERT INTO guard_baselines
				(project_id, workspace_id, head_oid, format_version, mappings_json, captured_at)
				VALUES ('PRJ-1', 'WS-1', ?, 1, ?, '2026-09-26T00:00:00Z')`, head, body); err != nil {
				t.Fatal(err)
			}
			fresh, err := changes.NewStore(db.DB, app.FixedClock{})
			if err != nil {
				t.Fatal(err)
			}
			for _, reader := range []*changes.Store{store, fresh} {
				got, err := reader.EnsureGuardBaseline(t.Context(), "PRJ-1", root, runner)
				if test.valid {
					if err != nil || got.HeadOID != head || !reflect.DeepEqual(got.Mappings, mappings) {
						t.Fatalf("valid persisted control: baseline=%+v, error=%v", got, err)
					}
					continue
				}
				payload, ok := app.PayloadOf(err)
				if !ok || payload.Code != app.CodeIndexStateCorrupt || len(payload.NextAction) == 0 {
					t.Fatalf("incomplete/corrupt persisted mapping was not refused: baseline=%+v, payload=%+v, error=%v", got, payload, err)
				}
				if got.HeadOID != "" || len(got.Mappings) != 0 {
					t.Fatalf("refusal leaked an accepted baseline: %+v", got)
				}
			}
			var persisted string
			if err := db.DB.QueryRowContext(t.Context(), `SELECT mappings_json FROM guard_baselines
				WHERE project_id = 'PRJ-1' AND workspace_id = 'WS-1' AND head_oid = ? AND format_version = 1`, head).Scan(&persisted); err != nil {
				t.Fatal(err)
			}
			if persisted != body {
				t.Fatalf("reader rewrote persisted evidence: got %q, want %q", persisted, body)
			}
		})
	}
}
