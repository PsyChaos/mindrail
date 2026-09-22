package index

import (
	"testing"
)

const migrationProject = "PRJ-TEST-MIGRATION"

// TestMatchBarSeparatesBodyKindContainerAndPath pins the D-96 bar as a pure
// function: body, kind and container must agree, and the pair must share a
// file or a Git-corroborated move. Structure is deliberately not compared.
func TestMatchBarSeparatesBodyKindContainerAndPath(t *testing.T) {
	base := ancestorRow{Key: "old", UID: "SYM-1", Kind: "function", ContainerLocal: "c", BodyHash: "b", Path: "/u/a.py"}
	added := stagedKey{Key: "new", Kind: "function", ContainerLocal: "c", BodyHash: "b", Path: "/u/a.py"}
	if !matchBar(added, base, "c", false) {
		t.Fatal("same-file identical rename rejected")
	}
	for _, tc := range []struct {
		name   string
		mutate func(*stagedKey, *ancestorRow)
		git    bool
		want   bool
	}{
		{"body differs", func(a *stagedKey, _ *ancestorRow) { a.BodyHash = "other" }, false, false},
		{"kind differs", func(a *stagedKey, _ *ancestorRow) { a.Kind = "class" }, false, false},
		{"container differs", func(a *stagedKey, _ *ancestorRow) { a.ContainerLocal = "other" }, false, false},
		{"empty body never matches", func(a *stagedKey, r *ancestorRow) { a.BodyHash, r.BodyHash = "", "" }, false, false},
		{"moved file without git", func(a *stagedKey, _ *ancestorRow) { a.Path = "/u/b.py" }, false, false},
		{"moved file with git", func(a *stagedKey, _ *ancestorRow) { a.Path = "/u/b.py" }, true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a, r := added, base
			tc.mutate(&a, &r)
			if got := matchBar(a, r, r.ContainerLocal, tc.git); got != tc.want {
				t.Fatalf("match = %t, want %t", got, tc.want)
			}
		})
	}
}

// TestOrderParentsFirstRootsBeforeChildren pins the cascade order: a class
// resolves before the methods that name it, deterministically.
func TestOrderParentsFirstRootsBeforeChildren(t *testing.T) {
	keys := []stagedKey{
		{Key: "method", ContainerLocal: "class"},
		{Key: "lone", ContainerLocal: ""},
		{Key: "class", ContainerLocal: ""},
	}
	ordered := orderParentsFirst(keys)
	if ordered[0].Key == "method" {
		t.Fatalf("order = %v, method must follow its container", ordered)
	}
	if ordered[len(ordered)-1].Key != "method" {
		t.Fatalf("order = %v, deepest last", ordered)
	}
}
