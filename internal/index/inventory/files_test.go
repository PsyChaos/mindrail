package inventory_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/PsyChaos/mindrail/internal/index"
	"github.com/PsyChaos/mindrail/internal/index/inventory"
)

func TestFilesDeterministicallyAssignsNestedUnitsAndSkipsUnsafePaths(t *testing.T) {
	root := t.TempDir()
	outer := index.ProjectUnit{ID: "outer", Path: root, Kind: index.UnitPython}
	innerRoot := filepath.Join(root, "nested")
	if err := os.MkdirAll(innerRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	inner := index.ProjectUnit{ID: "inner", Path: innerRoot, Kind: index.UnitTypeScript}
	for _, rel := range []string{"z.py", "nested/a.ts", "nested/b.tsx", "nested/c.js", "nested/README.md", "node_modules/pkg/skip.py", ".git/skip.py", ".mindrail/skip.py", "linked/.git", "linked/skip.py"} {
		path := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("source"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	outside := filepath.Join(t.TempDir(), "outside.py")
	if err := os.WriteFile(outside, []byte("outside"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "link.py")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	files, err := inventory.Files(t.Context(), root, []index.ProjectUnit{inner, outer})
	if err != nil {
		t.Fatal(err)
	}
	want := []inventory.File{
		{Unit: inner, Path: filepath.Join(innerRoot, "a.ts")},
		{Unit: inner, Path: filepath.Join(innerRoot, "b.tsx")},
		{Unit: inner, Path: filepath.Join(innerRoot, "c.js")},
		{Unit: outer, Path: filepath.Join(root, "z.py")},
	}
	if !reflect.DeepEqual(files, want) {
		t.Fatalf("files=%+v want=%+v", files, want)
	}
}

func TestFilesCancellationBeforeWalk(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := inventory.Files(ctx, t.TempDir(), nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("error=%v", err)
	}
}
