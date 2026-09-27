package index_test

import (
	"fmt"
	"path/filepath"
	"sync"
	"testing"

	"github.com/PsyChaos/mindrail/internal/app"
	"github.com/PsyChaos/mindrail/internal/index"
)

func TestRemoveFileCASRetiresOnlyExactObservedState(t *testing.T) {
	store, db := indexStore(t)
	unit, err := store.UpsertUnit(t.Context(), t.TempDir(), index.UnitPython)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(unit.Path, "file.py")
	missing, err := store.ReadFileState(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	first, applied, err := store.RegisterFileCAS(t.Context(), missing, unit.ID, path, "python", casHash("one"))
	if err != nil || !applied {
		t.Fatalf("register: %v %v", applied, err)
	}
	facts := index.FileFacts{ProjectID: testProjectID, UnitID: unit.ID, Path: path, Language: "python", ContentHash: casHash("one"), State: index.StateIndexed,
		Symbols: []index.Symbol{casSymbol("f", path+":f")}, Imports: []index.Import{{Module: "os"}}, References: []index.Reference{{TargetText: "f", Confidence: 0.5}}}
	indexed, _, applied, err := store.ReplaceFileFactsCAS(t.Context(), first, facts)
	if err != nil || !applied {
		t.Fatalf("index: %v %v", applied, err)
	}
	pending, applied, err := store.RegisterFileCAS(t.Context(), indexed, unit.ID, path, "python", casHash("two"))
	if err != nil || !applied {
		t.Fatalf("register: %v %v", applied, err)
	}
	if got, applied, err := store.RemoveFileCAS(t.Context(), indexed); err != nil || applied || got.State.ContentHash != casHash("two") {
		t.Fatalf("stale deletion changed newer state: %+v %v %v", got, applied, err)
	}
	if _, applied, err := store.InvalidateFileCAS(t.Context(), pending); err == nil || applied {
		t.Fatal("legacy invalidation accepted pending state")
	}
	if _, err := db.ExecContext(t.Context(), `CREATE TRIGGER reject_remove BEFORE DELETE ON file_index_state BEGIN SELECT RAISE(ABORT,'injected removal failure'); END`); err != nil {
		t.Fatal(err)
	}
	if _, applied, err := store.RemoveFileCAS(t.Context(), pending); err == nil || applied {
		t.Fatal("removal failure swallowed")
	}
	if got := rowCount(t, db, "file_index_generations", path); got != 0 {
		t.Fatalf("failed removal retained generation tombstone: %d", got)
	}
	for _, table := range []string{"symbols", "symbol_imports", "symbol_references", "file_index_state"} {
		var count int
		if err := db.QueryRowContext(t.Context(), "SELECT count(*) FROM "+table+" WHERE path=?", path).Scan(&count); err != nil || count != 1 {
			t.Fatalf("%s not preserved atomically: %d %v", table, count, err)
		}
	}
	if _, err := db.ExecContext(t.Context(), `DROP TRIGGER reject_remove`); err != nil {
		t.Fatal(err)
	}
	if got, applied, err := store.RemoveFileCAS(t.Context(), pending); err != nil || !applied || got.Exists {
		t.Fatalf("remove pending: %+v %v %v", got, applied, err)
	}
	queued, err := store.ListPending(t.Context(), unit.ID)
	if err != nil || len(queued) != 0 {
		t.Fatalf("removed file still queued: %+v %v", queued, err)
	}
	counts, err := store.CountByState(t.Context(), unit.ID)
	if err != nil || len(counts) != 0 {
		t.Fatalf("tombstone counted as live state: %+v %v", counts, err)
	}
	for _, table := range []string{"symbols", "symbol_imports", "symbol_references", "file_index_state"} {
		var count int
		if err := db.QueryRowContext(t.Context(), "SELECT count(*) FROM "+table+" WHERE path=?", path).Scan(&count); err != nil || count != 0 {
			t.Fatalf("%s not removed: %d %v", table, count, err)
		}
	}
	if _, _, applied, err := store.ReplaceFileFactsCAS(t.Context(), pending, index.FileFacts{ProjectID: testProjectID, UnitID: unit.ID, Path: path, Language: "python", ContentHash: casHash("two"), State: index.StateIndexed}); err != nil || applied {
		t.Fatalf("old pending result resurrected deleted file: %v %v", applied, err)
	}
}

func TestRemoveFileCASCompetesWithRegistration(t *testing.T) {
	for attempt := 0; attempt < 10; attempt++ {
		store, _ := indexStore(t)
		unit, err := store.UpsertUnit(t.Context(), t.TempDir(), index.UnitPython)
		if err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(unit.Path, "file.py")
		missing, err := store.ReadFileState(t.Context(), path)
		if err != nil {
			t.Fatal(err)
		}
		pending, applied, err := store.RegisterFileCAS(t.Context(), missing, unit.ID, path, "python", casHash("one"))
		if err != nil || !applied {
			t.Fatalf("register: %v %v", applied, err)
		}
		start := make(chan struct{})
		var wg sync.WaitGroup
		wg.Add(2)
		var removed, registered bool
		var removeErr, registerErr error
		go func() { defer wg.Done(); <-start; _, removed, removeErr = store.RemoveFileCAS(t.Context(), pending) }()
		go func() {
			defer wg.Done()
			<-start
			_, registered, registerErr = store.RegisterFileCAS(t.Context(), pending, unit.ID, path, "python", casHash("two"))
		}()
		close(start)
		wg.Wait()
		if removeErr != nil || registerErr != nil || removed == registered {
			t.Fatalf("expected one winner: remove=%v (%v), register=%v (%v)", removed, removeErr, registered, registerErr)
		}
		got, err := store.ReadFileState(t.Context(), path)
		if err != nil || removed && got.Exists || registered && (!got.Exists || got.State.ContentHash != casHash("two")) {
			t.Fatalf("wrong winning state: %+v %v", got, err)
		}
	}
}

func TestRemoveFileCASRejectsObserverFromBeforeRecreation(t *testing.T) {
	for _, legacy := range []bool{false, true} {
		for _, restart := range []bool{false, true} {
			t.Run(fmt.Sprintf("legacy=%v/restart=%v", legacy, restart), func(t *testing.T) {
				testRemoveFileCASRejectsObserverFromBeforeRecreation(t, legacy, restart)
			})
		}
	}
}

func testRemoveFileCASRejectsObserverFromBeforeRecreation(t *testing.T, legacy, restart bool) {
	store, db := indexStore(t)
	unit, err := store.UpsertUnit(t.Context(), t.TempDir(), index.UnitPython)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(unit.Path, "file.py")
	missing, err := store.ReadFileState(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	first, applied, err := store.RegisterFileCAS(t.Context(), missing, unit.ID, path, "python", casHash("same bytes"))
	if err != nil || !applied {
		t.Fatalf("register: %v %v", applied, err)
	}
	missing, applied, err = store.RemoveFileCAS(t.Context(), first)
	if err != nil || !applied {
		t.Fatalf("remove: %v %v", applied, err)
	}
	if restart {
		store = index.NewStore(db, app.SystemClock{})
	}
	if legacy {
		err = store.UpsertFileState(t.Context(), index.FileIndexState{UnitID: unit.ID, Path: path, Language: "python", ContentHash: casHash("same bytes"), State: index.StatePending})
	} else {
		_, applied, err = store.RegisterFileCAS(t.Context(), missing, unit.ID, path, "python", casHash("same bytes"))
		if !applied {
			t.Fatal("recreation was not applied")
		}
	}
	if err != nil {
		t.Fatal(err)
	}
	recreated, err := store.ReadFileState(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	if _, applied, err := store.RemoveFileCAS(t.Context(), first); err != nil || applied {
		t.Fatalf("pre-removal observer removed new registration: %v %v", applied, err)
	}
	if _, _, applied, err := store.ReplaceFileFactsCAS(t.Context(), first, index.FileFacts{ProjectID: testProjectID, UnitID: unit.ID, Path: path, Language: "python", ContentHash: casHash("same bytes"), State: index.StateIndexed}); err != nil || applied {
		t.Fatalf("pre-removal completion replaced new registration: %v %v", applied, err)
	}
	if recreated.State.Attempts <= first.State.Attempts {
		t.Fatalf("generation reused: %d -> %d", first.State.Attempts, recreated.State.Attempts)
	}
}
