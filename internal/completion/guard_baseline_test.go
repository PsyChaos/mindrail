package completion_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/PsyChaos/mindrail/internal/app"
	"github.com/PsyChaos/mindrail/internal/changes"
	"github.com/PsyChaos/mindrail/internal/git"
)

func TestGuardBaselineConcurrentEmptySnapshotIsStable(t *testing.T) {
	f := newFixture(t)
	var wg sync.WaitGroup
	results := make(chan changes.GuardBaseline, 8)
	failures := make(chan error, 8)
	for range 8 {
		wg.Go(func() {
			baseline, err := f.changes.Store().EnsureGuardBaseline(t.Context(), f.project, f.root, git.NewExecRunner())
			results <- baseline
			failures <- err
		})
	}
	wg.Wait()
	close(results)
	close(failures)
	for err := range failures {
		if err != nil {
			t.Fatal(err)
		}
	}
	for baseline := range results {
		if baseline.HeadOID == "" || baseline.Mappings == nil || len(baseline.Mappings) != 0 {
			t.Fatalf("empty capture is not a real snapshot: %+v", baseline)
		}
	}
	var count int
	var body string
	if err := f.application.DB().QueryRow(`SELECT count(*), mappings_json FROM guard_baselines`).Scan(&count, &body); err != nil || count != 1 || body != "[]" {
		t.Fatalf("concurrent snapshots: count=%d JSON=%q error=%v", count, body, err)
	}
}

type beforeInsertRunner struct {
	git.CommandRunner
	reads int
	run   func(string)
}

func (r *beforeInsertRunner) Run(ctx context.Context, dir string, args ...string) ([]byte, []byte, error) {
	out, stderr, err := r.CommandRunner.Run(ctx, dir, args...)
	if err == nil && strings.Join(args, " ") == "rev-parse --verify --quiet HEAD^{commit}" {
		r.reads++
		if r.reads == 2 {
			r.run(strings.TrimSpace(string(out)))
		}
	}
	return out, stderr, err
}

func TestGuardBaselinePreservesConcurrentExistingEdges(t *testing.T) {
	f := newFixture(t)
	winner := []changes.ProofMapping{{Path: filepath.Join(f.root, "test_winner.py"), Test: "test_winner", ProductionUID: "SYM-WINNER", InvariantID: "INV-0001"}}
	encoded, err := json.Marshal(winner)
	if err != nil {
		t.Fatal(err)
	}
	runner := &beforeInsertRunner{CommandRunner: git.NewExecRunner(), run: func(head string) {
		// Another writer commits after this caller observed absence and
		// computed an empty set, but before its transaction. Existing edges
		// must be retained rather than replaced with this caller's empty set.
		_, err := f.application.DB().Exec(`INSERT INTO guard_baselines
			(project_id, workspace_id, head_oid, format_version, mappings_json, captured_at)
			VALUES (?, ?, ?, 1, ?, '2026-09-26T00:00:00Z')`, f.project, f.application.Subject().Workspace.ID, head, string(encoded))
		if err != nil {
			t.Fatal(err)
		}
	}}
	baseline, err := f.changes.Store().EnsureGuardBaseline(t.Context(), f.project, f.root, runner)
	if err != nil || !reflect.DeepEqual(baseline.Mappings, winner) {
		t.Fatalf("loser did not read winner: %+v, %v", baseline, err)
	}
}

func TestGuardBaselineInterleavedDisjointCapturesConvergeToUnion(t *testing.T) {
	f := newFixture(t)
	first := seedGuardMapping(t, f, "a")
	second := changes.ProofMapping{}
	runner := &beforeInsertRunner{CommandRunner: git.NewExecRunner(), run: func(string) {
		// The outer ensure has already computed A. A concurrent producer now
		// sees only B and commits it first; the eventual result must keep both.
		if _, err := f.application.DB().Exec(`DELETE FROM symbol_references`); err != nil {
			t.Fatal(err)
		}
		second = seedGuardMapping(t, f, "b")
		if _, err := f.changes.Store().EnsureGuardBaseline(t.Context(), f.project, f.root, git.NewExecRunner()); err != nil {
			t.Fatal(err)
		}
	}}
	baseline, err := f.changes.Store().EnsureGuardBaseline(t.Context(), f.project, f.root, runner)
	want := []changes.ProofMapping{first, second}
	if err != nil || !reflect.DeepEqual(baseline.Mappings, want) {
		t.Fatalf("disjoint captures lost an edge: %+v, want %+v, error=%v", baseline.Mappings, want, err)
	}
	repeated, err := f.changes.Store().EnsureGuardBaseline(t.Context(), f.project, f.root, git.NewExecRunner())
	if err != nil || !reflect.DeepEqual(baseline, repeated) {
		t.Fatalf("union was not idempotent: %+v vs %+v, %v", baseline, repeated, err)
	}
}

func seedGuardMapping(t *testing.T, f fixture, suffix string) changes.ProofMapping {
	t.Helper()
	db := f.application.DB()
	if _, err := db.Exec(`INSERT OR IGNORE INTO project_units(id,path,kind,discovered_at) VALUES ('UNT-PROOF',?,'python','2026-09-26T00:00:00Z')`, f.root); err != nil {
		t.Fatal(err)
	}
	mapping := changes.ProofMapping{Path: filepath.Join(f.root, "test_"+suffix+".py"), Test: "test_" + suffix, ProductionUID: "SYM-" + suffix, InvariantID: "INV-" + suffix}
	if _, err := db.Exec(`INSERT INTO symbol_identities(symbol_uid,project_id,unit_id,language,logical_key,previous_keys,created_at)
		VALUES (?,?,'UNT-PROOF','python',?,'[]','2026-09-26T00:00:00Z')`, mapping.ProductionUID, f.project, "prod-"+suffix); err != nil {
		t.Fatal(err)
	}
	var productionID int64
	for _, item := range []struct{ path, key, name, uid string }{
		{filepath.Join(f.root, "prod_"+suffix+".py"), "prod-" + suffix, "prod_" + suffix, mapping.ProductionUID},
		{mapping.Path, mapping.Test, mapping.Test, ""},
	} {
		result, err := db.Exec(`INSERT INTO symbols(unit_id,path,logical_key,kind,name,start_line,start_col,end_line,end_col,signature_hash,body_hash,structure_hash,symbol_uid)
			VALUES ('UNT-PROOF',?,?,'function',?,1,0,2,0,'s','b','t',NULLIF(?,''))`, item.path, item.key, item.name, item.uid)
		if err != nil {
			t.Fatal(err)
		}
		if item.uid != "" {
			productionID, err = result.LastInsertId()
			if err != nil {
				t.Fatal(err)
			}
		}
	}
	if _, err := db.Exec(`INSERT INTO symbol_references(unit_id,path,referrer_key,target_text,confidence,resolved_symbol_id)
		VALUES ('UNT-PROOF',?,?,?,0.5,?)`, mapping.Path, mapping.Test, "prod_"+suffix, productionID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO invariant_symbol_bindings(invariant_id,symbol_uid,status,updated_at)
		VALUES (?,?,'bound','2026-09-26T00:00:00Z')`, mapping.InvariantID, mapping.ProductionUID); err != nil {
		t.Fatal(err)
	}
	return mapping
}

func TestGuardBaselineRefusesAlreadyMutatedIndexWithoutSnapshot(t *testing.T) {
	f := newFixture(t)
	path := filepath.Join(f.root, "source.txt")
	if _, err := f.application.DB().Exec(`INSERT INTO project_units (id,path,kind,discovered_at) VALUES ('UNT-GUARD',?,'python','2026-09-26T00:00:00Z')`, f.root); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256([]byte("already reindexed edit\n"))
	if _, err := f.application.DB().Exec(`INSERT INTO file_index_state (path,unit_id,language,content_hash,state) VALUES (?,'UNT-GUARD','python',?,'indexed')`, path, hex.EncodeToString(sum[:])); err != nil {
		t.Fatal(err)
	}
	_, err := f.changes.Store().EnsureGuardBaseline(t.Context(), f.project, f.root, git.NewExecRunner())
	payload, ok := app.PayloadOf(err)
	if !ok || payload.Code != app.CodeIndexStateCorrupt || !strings.Contains(payload.Why, "pre-mutation") {
		t.Fatalf("upgraded dirty index was trusted: %v", err)
	}
	var count int
	if err := f.application.DB().QueryRow(`SELECT count(*) FROM guard_baselines`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("unsafe snapshot stored: %d %v", count, err)
	}
}

func TestGuardBaselineDetectsHeadMoveAndSeparatesSnapshots(t *testing.T) {
	f := newFixture(t)
	before, err := f.changes.Store().EnsureGuardBaseline(t.Context(), f.project, f.root, git.NewExecRunner())
	if err != nil {
		t.Fatal(err)
	}
	gitCommand(t, f.root, "-c", "user.name=test", "-c", "user.email=test@example.invalid", "commit", "--allow-empty", "--quiet", "-m", "new HEAD")
	if err := before.CheckHead(t.Context(), f.root, git.NewExecRunner()); err == nil {
		t.Fatal("HEAD movement was silently certified")
	}
	after, err := f.changes.Store().EnsureGuardBaseline(t.Context(), f.project, f.root, git.NewExecRunner())
	if err != nil || after.HeadOID == before.HeadOID {
		t.Fatalf("new HEAD reused old snapshot: before=%+v after=%+v error=%v", before, after, err)
	}
}

// Every production task-scoped discovery caller must preserve proof edges
// first and recheck HEAD afterwards. Adding a caller changes this explicit
// architecture inventory so a new route cannot quietly omit the protocol.
func TestTaskDiscoveryProductionCallersPreserveGuardBaseline(t *testing.T) {
	repo := filepath.Clean(filepath.Join("..", ".."))
	want := map[string]bool{
		"internal/completion/service.go": false,
		"internal/mcp/lifecycle.go":      false,
		"internal/mcp/discovery.go":      false,
		"internal/workflow/journal.go":   false,
	}
	err := filepath.WalkDir(filepath.Join(repo, "internal"), func(path string, entry os.DirEntry, err error) error {
		if err != nil || entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return err
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		source := string(body)
		for _, method := range []string{".AfterChange(", ".Reconcile("} {
			call := strings.Index(source, method)
			if call < 0 {
				continue
			}
			rel, err := filepath.Rel(repo, path)
			if err != nil {
				return err
			}
			rel = filepath.ToSlash(rel)
			if _, known := want[rel]; !known {
				t.Errorf("new task discovery caller needs guard-baseline wiring: %s", rel)
			}
			want[rel] = true
			capture := strings.LastIndex(source[:call], ".EnsureGuardBaseline(")
			check := strings.Index(source[call:], ".CheckHead(")
			if capture < 0 || check < 0 {
				t.Errorf("%s %s lacks ensure-before/check-after order", rel, method)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	for path, seen := range want {
		if !seen {
			t.Errorf("expected production discovery caller disappeared: %s", path)
		}
	}
}
