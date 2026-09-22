package index_test

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"path/filepath"
	"sync"
	"testing"

	"github.com/PsyChaos/mindrail/internal/index"
)

func casHash(content string) string {
	sum := sha256.Sum256([]byte(content))
	return hex.EncodeToString(sum[:])
}

func casSymbol(name, key string) index.Symbol {
	return index.Symbol{LogicalKey: key, Name: name, Kind: "function", SignatureHash: "sig", BodyHash: "body", StructureHash: "shape"}
}

func TestRegistrationCASStoresTargetHashAndRejectsLateStaleRegistration(t *testing.T) {
	store, db := indexStore(t)
	unit, err := store.UpsertUnit(t.Context(), t.TempDir(), index.UnitPython)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(unit.Path, "file.py")
	before, err := store.ReadFileState(t.Context(), path)
	if err != nil || before.Exists {
		t.Fatalf("initial observation = %+v, %v", before, err)
	}
	h2 := casHash("H2")
	registered, applied, err := store.RegisterFileCAS(t.Context(), before, unit.ID, path, "python", h2)
	if err != nil || !applied || !registered.Exists || registered.State.State != index.StatePending || registered.State.ContentHash != h2 || registered.State.Attempts != 1 {
		t.Fatalf("H2 registration = %+v, applied=%t, err=%v", registered, applied, err)
	}
	facts := index.FileFacts{UnitID: unit.ID, Path: path, Language: "python", ContentHash: h2, State: index.StateIndexed, Symbols: []index.Symbol{casSymbol("new", path+":new")}}
	completed, _, applied, err := store.ReplaceFileFactsCAS(t.Context(), registered, facts)
	if err != nil || !applied || completed.State.State != index.StateIndexed || completed.State.ContentHash != h2 {
		t.Fatalf("H2 completion = %+v, applied=%t, err=%v", completed, applied, err)
	}
	_, applied, err = store.RegisterFileCAS(t.Context(), before, unit.ID, path, "python", casHash("H1"))
	if err != nil || applied {
		t.Fatalf("late H1 registration applied=%t, err=%v", applied, err)
	}
	observed, err := store.ReadFileState(t.Context(), path)
	if err != nil || observed.State.State != index.StateIndexed || observed.State.ContentHash != h2 {
		t.Fatalf("late H1 changed state: %+v, %v", observed, err)
	}
	var name string
	if err := db.QueryRowContext(t.Context(), `SELECT name FROM symbols WHERE path = ?`, path).Scan(&name); err != nil || name != "new" {
		t.Fatalf("late H1 changed facts: name=%q err=%v", name, err)
	}
}

func TestCompletionCASRejectsStaleH1BeforeDeletingH2Facts(t *testing.T) {
	store, db := indexStore(t)
	unit, err := store.UpsertUnit(t.Context(), t.TempDir(), index.UnitPython)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(unit.Path, "file.py")
	before, err := store.ReadFileState(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	h1, h2 := casHash("H1"), casHash("H2")
	r1, applied, err := store.RegisterFileCAS(t.Context(), before, unit.ID, path, "python", h1)
	if err != nil || !applied {
		t.Fatalf("H1 registration: %v, %t", err, applied)
	}
	r2, applied, err := store.RegisterFileCAS(t.Context(), r1, unit.ID, path, "python", h2)
	if err != nil || !applied || r2.State.Attempts != r1.State.Attempts+1 {
		t.Fatalf("H2 registration: %+v, %v, %t", r2, err, applied)
	}
	f2 := index.FileFacts{UnitID: unit.ID, Path: path, Language: "python", ContentHash: h2, State: index.StateIndexed, Symbols: []index.Symbol{casSymbol("H2", path+":H2")}}
	_, _, applied, err = store.ReplaceFileFactsCAS(t.Context(), r2, f2)
	if err != nil || !applied {
		t.Fatalf("H2 completion: %v, %t", err, applied)
	}
	var originalID int64
	if err := db.QueryRowContext(t.Context(), `SELECT id FROM symbols WHERE path = ?`, path).Scan(&originalID); err != nil {
		t.Fatal(err)
	}
	f1 := index.FileFacts{UnitID: unit.ID, Path: path, Language: "python", ContentHash: h1, State: index.StateIndexed, Symbols: []index.Symbol{casSymbol("H1", path+":H1")}}
	_, _, applied, err = store.ReplaceFileFactsCAS(t.Context(), r1, f1)
	if err != nil || applied {
		t.Fatalf("stale H1 completion applied=%t, err=%v", applied, err)
	}
	var gotID int64
	var gotName string
	if err := db.QueryRowContext(t.Context(), `SELECT id, name FROM symbols WHERE path = ?`, path).Scan(&gotID, &gotName); err != nil || gotID != originalID || gotName != "H2" {
		t.Fatalf("stale H1 mutated H2 facts: id=%d name=%q err=%v", gotID, gotName, err)
	}
}

func TestCASCompletionResolvesOnlyUniqueSameKey(t *testing.T) {
	store, db := indexStore(t)
	unit, err := store.UpsertUnit(t.Context(), t.TempDir(), index.UnitPython)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(unit.Path, "file.py")
	key := path + ":function:target"
	write := func(source string, symbols []index.Symbol) {
		t.Helper()
		previous, err := store.ReadFileState(t.Context(), path)
		if err != nil {
			t.Fatal(err)
		}
		hash := casHash(source)
		registered, applied, err := store.RegisterFileCAS(t.Context(), previous, unit.ID, path, "python", hash)
		if err != nil || !applied {
			t.Fatalf("register: %v, %t", err, applied)
		}
		facts := index.FileFacts{UnitID: unit.ID, Path: path, Language: "python", ContentHash: hash, State: index.StateIndexed,
			Symbols: symbols, References: []index.Reference{{TargetText: "target", TargetLogicalKey: key, Label: "STRUCTURAL_NAME_MATCH", Confidence: 0.5}}}
		_, _, applied, err = store.ReplaceFileFactsCAS(t.Context(), registered, facts)
		if err != nil || !applied {
			t.Fatalf("complete: %v, %t", err, applied)
		}
	}
	sym := casSymbol("target", key)
	write("unique", []index.Symbol{sym})
	var pointer sql.NullInt64
	if err := db.QueryRowContext(t.Context(), `SELECT resolved_symbol_id FROM symbol_references WHERE path = ?`, path).Scan(&pointer); err != nil || !pointer.Valid {
		t.Fatalf("unique pointer=%v err=%v", pointer, err)
	}
	write("ambiguous", []index.Symbol{sym, sym})
	if err := db.QueryRowContext(t.Context(), `SELECT resolved_symbol_id FROM symbol_references WHERE path = ?`, path).Scan(&pointer); err != nil || pointer.Valid {
		t.Fatalf("ambiguous pointer=%v err=%v", pointer, err)
	}
}

func TestConcurrentRegistrationsFromOneObservationOnlyOneWins(t *testing.T) {
	store, _ := indexStore(t)
	unit, err := store.UpsertUnit(t.Context(), t.TempDir(), index.UnitPython)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(unit.Path, "file.py")
	before, err := store.ReadFileState(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	type outcome struct {
		applied bool
		err     error
	}
	results := make(chan outcome, 2)
	var ready sync.WaitGroup
	ready.Add(2)
	start := make(chan struct{})
	for _, hash := range []string{casHash("H1"), casHash("H2")} {
		go func(hash string) {
			ready.Done()
			<-start
			_, applied, err := store.RegisterFileCAS(t.Context(), before, unit.ID, path, "python", hash)
			results <- outcome{applied: applied, err: err}
		}(hash)
	}
	ready.Wait()
	close(start)
	wins := 0
	for range 2 {
		outcome := <-results
		if outcome.err != nil {
			t.Fatal(outcome.err)
		}
		if outcome.applied {
			wins++
		}
	}
	state, err := store.ReadFileState(t.Context(), path)
	if err != nil || wins != 1 || state.State.Attempts != 1 {
		t.Fatalf("wins=%d state=%+v err=%v", wins, state.State, err)
	}
}

func TestRegisterCASRejectsMalformedHashWithoutRows(t *testing.T) {
	store, db := indexStore(t)
	unit, err := store.UpsertUnit(t.Context(), t.TempDir(), index.UnitPython)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(unit.Path, "file.py")
	before, err := store.ReadFileState(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	for _, hash := range []string{"", "abcd", "not-hex-not-hex-not-hex-not-hex-not-hex-not-hex-not-hex-not-hex-", "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"} {
		if _, applied, err := store.RegisterFileCAS(t.Context(), before, unit.ID, path, "python", hash); err == nil || applied {
			t.Fatalf("hash %q accepted: applied=%t err=%v", hash, applied, err)
		}
	}
	var rows int
	if err := db.QueryRowContext(t.Context(), `SELECT count(*) FROM file_index_state WHERE path = ?`, path).Scan(&rows); err != nil || rows != 0 {
		t.Fatalf("rows=%d err=%v", rows, err)
	}
}

func TestLegacyRegistrationCannotRecreateOldCASTokenAfterStateABA(t *testing.T) {
	store, _ := indexStore(t)
	root := t.TempDir()
	unit, err := store.UpsertUnit(t.Context(), root, index.UnitPython)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "file.py")
	before, err := store.ReadFileState(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	hash := casHash("same")
	old, applied, err := store.RegisterFileCAS(t.Context(), before, unit.ID, path, "python", hash)
	if err != nil || !applied {
		t.Fatalf("initial registration=%v applied=%t", err, applied)
	}
	for _, state := range []index.FileState{index.StateUnsupported, index.StatePending} {
		registrationHash := hash
		if state == index.StateUnsupported {
			registrationHash = ""
		}
		if err := store.UpsertFileState(t.Context(), index.FileIndexState{UnitID: unit.ID, Path: path, Language: "python", ContentHash: registrationHash, State: state}); err != nil {
			t.Fatal(err)
		}
	}
	_, _, applied, err = store.ReplaceFileFactsCAS(t.Context(), old, index.FileFacts{UnitID: unit.ID, Path: path, Language: "python", ContentHash: hash, State: index.StateIndexed})
	if err != nil || applied {
		t.Fatalf("ABA completion applied=%t err=%v", applied, err)
	}
}

func TestFailedObservationMustRegisterRetryBeforeCompletion(t *testing.T) {
	store, _ := indexStore(t)
	unit, err := store.UpsertUnit(t.Context(), t.TempDir(), index.UnitPython)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(unit.Path, "file.py")
	before, err := store.ReadFileState(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	hash := casHash("partial")
	registered, applied, err := store.RegisterFileCAS(t.Context(), before, unit.ID, path, "python", hash)
	if err != nil || !applied {
		t.Fatalf("register=%v applied=%t", err, applied)
	}
	failed, _, applied, err := store.ReplaceFileFactsCAS(t.Context(), registered, index.FileFacts{UnitID: unit.ID, Path: path, Language: "python", ContentHash: hash, State: index.StateFailed, LastError: "partial"})
	if err != nil || !applied || failed.State.State != index.StateFailed {
		t.Fatalf("failure=%+v applied=%t err=%v", failed, applied, err)
	}
	complete := index.FileFacts{UnitID: unit.ID, Path: path, Language: "python", ContentHash: hash, State: index.StateIndexed}
	if _, _, applied, err := store.ReplaceFileFactsCAS(t.Context(), failed, complete); err == nil || applied {
		t.Fatalf("unregistered retry accepted=%t err=%v", applied, err)
	}
	retry, applied, err := store.RegisterFileCAS(t.Context(), failed, unit.ID, path, "python", hash)
	if err != nil || !applied || retry.State.Attempts != failed.State.Attempts+1 {
		t.Fatalf("retry=%+v applied=%t err=%v", retry, applied, err)
	}
	if _, _, applied, err := store.ReplaceFileFactsCAS(t.Context(), retry, complete); err != nil || !applied {
		t.Fatalf("completion=%t err=%v", applied, err)
	}
}

// TestRegistrationCASLeavesUnchangedIndexedWithoutNewGeneration pins the
// CAS fast path the restart proof leans on: re-registering an indexed file
// whose hash did not change is a no-op — same generation, nothing rewritten.
// Without it every repeat visit would mint a new attempt and reparse.
func TestRegistrationCASLeavesUnchangedIndexedWithoutNewGeneration(t *testing.T) {
	store, _ := indexStore(t)
	unit, err := store.UpsertUnit(t.Context(), t.TempDir(), index.UnitPython)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(unit.Path, "file.py")
	before, err := store.ReadFileState(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	hash := casHash("same")
	registered, applied, err := store.RegisterFileCAS(t.Context(), before, unit.ID, path, "python", hash)
	if err != nil || !applied {
		t.Fatalf("register=%v applied=%t", err, applied)
	}
	completed, _, applied, err := store.ReplaceFileFactsCAS(t.Context(), registered, index.FileFacts{UnitID: unit.ID, Path: path, Language: "python", ContentHash: hash, State: index.StateIndexed})
	if err != nil || !applied {
		t.Fatalf("complete=%v applied=%t", err, applied)
	}
	observed, err := store.ReadFileState(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	again, applied, err := store.RegisterFileCAS(t.Context(), observed, unit.ID, path, "python", hash)
	if err != nil || applied {
		t.Fatalf("unchanged re-registration applied=%t err=%v", applied, err)
	}
	if again.State.Attempts != completed.State.Attempts {
		t.Fatalf("attempts %d -> %d on an unchanged hash", completed.State.Attempts, again.State.Attempts)
	}
}
