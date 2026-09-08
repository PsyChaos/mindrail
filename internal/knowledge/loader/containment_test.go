package loader_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/PsyChaos/mindrail/internal/app"
	"github.com/PsyChaos/mindrail/internal/filesystem"
	"github.com/PsyChaos/mindrail/internal/knowledge/schema"
)

// The record path the whole file talks about, in the one spelling Mindrail
// stores and prints (tech-stack §74).
const escapingRecordPath = ".mindrail/knowledge/decisions/DEC-0002.json"

// TestLoadRefusesARecordThatResolvesOutsideTheRepository is finding BA-10.
//
// Containment was applied to the store directory and to each bucket, so a
// bucket that left the repository was refused — but the entries inside a
// contained bucket were opened by joining the entry name onto the resolved
// directory, which resolves no link of its own. A record file that was a
// symbolic link out of the worktree was therefore read, counted and, once
// MR-002 began deriving verdicts from those bytes, judged as though the
// repository had committed it.
//
// The three targets are the three ways out: an absolute path, a relative path
// climbing past the root, and a link whose target does not exist — the last
// because a dangling link is reported by the operating system as "no such
// file", which named the wrong condition entirely.
func TestLoadRefusesARecordThatResolvesOutsideTheRepository(t *testing.T) {
	requireSymlinks(t)

	for _, tc := range []struct {
		name   string
		target func(t *testing.T, worktree, outside string) string
	}{
		{
			name: "absolute target",
			target: func(t *testing.T, _, outside string) string {
				t.Helper()
				return writeOutsideRecord(t, outside, "evil.json")
			},
		},
		{
			name: "relative target climbing out of the worktree",
			target: func(t *testing.T, worktree, outside string) string {
				t.Helper()
				absolute := writeOutsideRecord(t, outside, "evil.json")
				bucket := filepath.Join(worktree, ".mindrail", "knowledge", "decisions")
				relative, err := filepath.Rel(bucket, absolute)
				if err != nil {
					t.Fatalf("relative path from %q to %q: %v", bucket, absolute, err)
				}
				return relative
			},
		},
		{
			name: "dangling target outside the worktree",
			target: func(t *testing.T, _, outside string) string {
				t.Helper()
				return filepath.Join(outside, "never-written.json")
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			worktree := t.TempDir()
			outside := t.TempDir()
			makeKnowledgeDirs(t, worktree)
			linkRecord(t, worktree, "decisions", "DEC-0002.json", tc.target(t, worktree, outside))

			store, err := newLoader(t, worktree).Load(context.Background())

			// One record must cost one record (decision D-06): a planted link
			// may not make the store unreadable.
			if err != nil {
				t.Fatalf("Load() error = %v, want nil: a single escaping record is a Problem, not a store failure", err)
			}
			if store.Count() != 0 {
				t.Fatalf("Count() = %d, want 0: the record resolves outside the repository", store.Count())
			}
			if len(store.Problems) != 1 {
				t.Fatalf("Problems = %v, want exactly one", store.Problems)
			}

			problem := store.Problems[0]
			if problem.Code != app.CodePathEscapesRoot {
				t.Errorf("Problem.Code = %q, want %q: the file reads perfectly, so it is not KNOWLEDGE_UNREADABLE",
					problem.Code, app.CodePathEscapesRoot)
			}
			if problem.Fatal {
				t.Error("Problem.Fatal = true, want false: only an unreadable schema_version blocks the store")
			}
			if problem.Path != escapingRecordPath {
				t.Errorf("Problem.Path = %q, want %q", problem.Path, escapingRecordPath)
			}
			if !strings.Contains(problem.Message, "resolves outside the repository root") {
				t.Errorf("Problem.Message = %q, want it to name the containment condition", problem.Message)
			}
			// The message is persisted and printed, so no machine-local
			// absolute path may appear in it.
			for _, leak := range []string{worktree, outside} {
				if strings.Contains(problem.Message, leak) {
					t.Errorf("Problem.Message = %q, want no absolute path; it leaked %q", problem.Message, leak)
				}
			}
			if store.HasFatalProblem() {
				t.Error("HasFatalProblem() = true, want false")
			}
		})
	}
}

// TestLoadKeepsTheRecordsBesideAnEscapingOne is decision D-06's price list. The
// remedy for BA-10 must cost the repository the one record that left it and
// nothing else, so the good record filed next to the link is still counted.
func TestLoadKeepsTheRecordsBesideAnEscapingOne(t *testing.T) {
	requireSymlinks(t)

	worktree := t.TempDir()
	outside := t.TempDir()
	makeKnowledgeDirs(t, worktree)
	writeRecord(t, worktree, "decisions", "DEC-0001.json", decisionJSON("DEC-0001", schema.WriteVersion))
	writeRecord(t, worktree, "invariants", "INV-0001.json", invariantJSON("INV-0001", schema.WriteVersion))
	linkRecord(t, worktree, "decisions", "DEC-0002.json", writeOutsideRecord(t, outside, "evil.json"))

	store, err := newLoader(t, worktree).Load(context.Background())
	if err != nil {
		t.Fatalf("Load() error = %v, want nil", err)
	}
	if store.Count() != 2 {
		t.Errorf("Count() = %d, want 2: the escaping record costs one record, not the store", store.Count())
	}
	if len(store.Decisions) != 1 || store.Decisions[0].ID != "DEC-0001" {
		t.Errorf("Decisions = %v, want only DEC-0001", store.Decisions)
	}
	if len(store.Problems) != 1 {
		t.Errorf("Problems = %v, want exactly one", store.Problems)
	}
}

// TestLoadAcceptsRecordsReachedThroughLinksInsideTheRepository is the over-fire
// guard for the fix above, and it is the reason the remedy is Root.Resolve
// rather than a "reject any directory entry that is a symbolic link" test.
//
// Decision D-32's policy — stated on filesystem.Root — is that links inside the
// root are allowed and links that leave it are refused. Refusing every link
// would refuse each layout below, all of which stay inside the repository, and
// a guard that makes a working repository unreadable is a worse defect than the
// one it closes.
func TestLoadAcceptsRecordsReachedThroughLinksInsideTheRepository(t *testing.T) {
	requireSymlinks(t)

	body := decisionJSON("DEC-0002", schema.WriteVersion)

	for _, tc := range []struct {
		name  string
		setup func(t *testing.T, worktree string)
	}{
		{
			name: "record links to a file inside the repository by absolute path",
			setup: func(t *testing.T, worktree string) {
				t.Helper()
				makeKnowledgeDirs(t, worktree)
				linkRecord(t, worktree, "decisions", "DEC-0002.json", writeInsideRecord(t, worktree, body))
			},
		},
		{
			name: "record links to a file inside the repository by relative path",
			setup: func(t *testing.T, worktree string) {
				t.Helper()
				makeKnowledgeDirs(t, worktree)
				writeInsideRecord(t, worktree, body)
				linkRecord(t, worktree, "decisions", "DEC-0002.json",
					filepath.Join("..", "..", "..", "records", "DEC-0002.json"))
			},
		},
		{
			name: "record is a hard link to a file inside the repository",
			setup: func(t *testing.T, worktree string) {
				t.Helper()
				makeKnowledgeDirs(t, worktree)
				source := writeInsideRecord(t, worktree, body)
				target := filepath.Join(worktree, ".mindrail", "knowledge", "decisions", "DEC-0002.json")
				if err := os.Link(source, target); err != nil {
					t.Skipf("hard links unsupported here: %v", err)
				}
			},
		},
		{
			name: "the whole knowledge store is a link to a directory inside the repository",
			setup: func(t *testing.T, worktree string) {
				t.Helper()
				shared := filepath.Join(worktree, "shared-knowledge")
				mustMkdirAll(t, filepath.Join(shared, "decisions"))
				mustMkdirAll(t, filepath.Join(shared, "invariants"))
				mustWriteFile(t, filepath.Join(shared, "decisions", "DEC-0002.json"), body)
				mustMkdirAll(t, filepath.Join(worktree, ".mindrail"))
				mustSymlink(t, shared, filepath.Join(worktree, ".mindrail", "knowledge"))
			},
		},
		{
			name: "a bucket is a link to a directory inside the repository",
			setup: func(t *testing.T, worktree string) {
				t.Helper()
				shared := filepath.Join(worktree, "shared-decisions")
				mustMkdirAll(t, shared)
				mustWriteFile(t, filepath.Join(shared, "DEC-0002.json"), body)
				mustMkdirAll(t, filepath.Join(worktree, ".mindrail", "knowledge"))
				mustSymlink(t, shared, filepath.Join(worktree, ".mindrail", "knowledge", "decisions"))
			},
		},
		{
			name: "a linked record inside a linked bucket",
			setup: func(t *testing.T, worktree string) {
				t.Helper()
				shared := filepath.Join(worktree, "shared-decisions")
				mustMkdirAll(t, shared)
				source := writeInsideRecord(t, worktree, body)
				mustSymlink(t, source, filepath.Join(shared, "DEC-0002.json"))
				mustMkdirAll(t, filepath.Join(worktree, ".mindrail", "knowledge"))
				mustSymlink(t, shared, filepath.Join(worktree, ".mindrail", "knowledge", "decisions"))
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			worktree := t.TempDir()
			tc.setup(t, worktree)

			store, err := newLoader(t, worktree).Load(context.Background())
			if err != nil {
				t.Fatalf("Load() error = %v, want nil: this layout never leaves the repository", err)
			}
			if len(store.Problems) != 0 {
				t.Fatalf("Problems = %v, want none: a link inside the repository is allowed (D-32)", store.Problems)
			}
			if store.Count() != 1 {
				t.Fatalf("Count() = %d, want 1", store.Count())
			}
			if got := string(store.Decisions[0].Body); got != body {
				t.Errorf("Body = %q, want the bytes on disk %q", got, body)
			}
			if store.Decisions[0].Path != escapingRecordPath {
				t.Errorf("Path = %q, want %q: the reported path is where the record lives in the repository, not where the link points",
					store.Decisions[0].Path, escapingRecordPath)
			}
		})
	}
}

// TestLoadAcceptsAWorktreeReachedThroughASymlinkedComponent is the over-fire
// guard for the case a containment fix breaks most easily: the repository is
// perfectly ordinary, but the path used to reach it passes through a link.
// macOS's /tmp -> /private/tmp is this shape, and so is every checkout under a
// symlinked home or scratch directory. Every record here is a plain file.
func TestLoadAcceptsAWorktreeReachedThroughASymlinkedComponent(t *testing.T) {
	requireSymlinks(t)

	base := t.TempDir()
	real := filepath.Join(base, "real")
	mustMkdirAll(t, real)
	mustSymlink(t, real, filepath.Join(base, "link"))

	// The worktree is addressed through the link, so a component of the root's
	// own path is a symbolic link.
	worktree := filepath.Join(base, "link", "repo")
	mustMkdirAll(t, worktree)
	makeKnowledgeDirs(t, worktree)
	writeRecord(t, worktree, "decisions", "DEC-0001.json", decisionJSON("DEC-0001", schema.WriteVersion))
	writeRecord(t, worktree, "invariants", "INV-0001.json", invariantJSON("INV-0001", schema.WriteVersion))

	store, err := newLoader(t, worktree).Load(context.Background())
	if err != nil {
		t.Fatalf("Load() error = %v, want nil: a link on the way to the root is not an escape from it", err)
	}
	if len(store.Problems) != 0 {
		t.Fatalf("Problems = %v, want none", store.Problems)
	}
	if store.Count() != 2 {
		t.Errorf("Count() = %d, want 2", store.Count())
	}
}

// TestEscapeClassificationDoesNotSwallowAnUnreadableRecord is the second
// over-fire guard, on the diagnosis rather than on the outcome.
//
// MR-001's repeated shape is a widened classification that starts answering
// questions that were already answered correctly. A record that genuinely
// cannot be read still has to say KNOWLEDGE_UNREADABLE: "resolves outside the
// repository root" sends a reader hunting for a link that is not there.
func TestEscapeClassificationDoesNotSwallowAnUnreadableRecord(t *testing.T) {
	requireSymlinks(t)

	for _, tc := range []struct {
		name  string
		setup func(t *testing.T, worktree string)
	}{
		{
			name: "a record that is not JSON",
			setup: func(t *testing.T, worktree string) {
				t.Helper()
				writeRecord(t, worktree, "decisions", "DEC-0002.json", "{not json")
			},
		},
		{
			name: "a link dangling inside the repository",
			setup: func(t *testing.T, worktree string) {
				t.Helper()
				linkRecord(t, worktree, "decisions", "DEC-0002.json",
					filepath.Join("..", "..", "..", "records", "never-written.json"))
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			worktree := t.TempDir()
			makeKnowledgeDirs(t, worktree)
			tc.setup(t, worktree)

			store, err := newLoader(t, worktree).Load(context.Background())
			if err != nil {
				t.Fatalf("Load() error = %v, want nil", err)
			}
			if len(store.Problems) != 1 {
				t.Fatalf("Problems = %v, want exactly one", store.Problems)
			}
			if got := store.Problems[0].Code; got != app.CodeKnowledgeUnreadable {
				t.Errorf("Problem.Code = %q, want %q: nothing here leaves the repository",
					got, app.CodeKnowledgeUnreadable)
			}
			if strings.Contains(store.Problems[0].Message, "outside the repository") {
				t.Errorf("Problem.Message = %q, want it not to blame containment", store.Problems[0].Message)
			}
		})
	}
}

// TestLoadStillFailsTheStoreWhenABucketLeavesTheRepository pins the boundary
// the fix must not move. A bucket that resolves outside cannot be walked at
// all, so there is no store to report and the containment sentinel travels out
// unchanged — that is what makes `status`, `doctor` and `init` name one
// condition identically. Only a single record is demoted to a Problem.
func TestLoadStillFailsTheStoreWhenABucketLeavesTheRepository(t *testing.T) {
	requireSymlinks(t)

	worktree := t.TempDir()
	outside := t.TempDir()
	mustMkdirAll(t, filepath.Join(worktree, ".mindrail", "knowledge"))
	mustSymlink(t, outside, filepath.Join(worktree, ".mindrail", "knowledge", "decisions"))

	store, err := newLoader(t, worktree).Load(context.Background())
	if !errors.Is(err, filesystem.ErrEscapesRoot) {
		t.Fatalf("Load() error = %v, want it to wrap filesystem.ErrEscapesRoot", err)
	}
	if len(store.Problems) != 0 || store.Count() != 0 {
		t.Errorf("Load() = %+v, want the zero store beside the error", store)
	}
}

func requireSymlinks(t *testing.T) {
	t.Helper()

	dir := t.TempDir()
	if err := os.Symlink(filepath.Join(dir, "target"), filepath.Join(dir, "link")); err != nil {
		t.Skipf("symbolic links unsupported on %s: %v", runtime.GOOS, err)
	}
}

func mustSymlink(t *testing.T, target, link string) {
	t.Helper()
	if err := os.Symlink(target, link); err != nil {
		t.Fatalf("linking %s -> %s: %v", link, target, err)
	}
}

func mustWriteFile(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("writing %s: %v", path, err)
	}
}

// linkRecord puts a symbolic link where a record file belongs.
func linkRecord(t *testing.T, worktreeRoot, subdir, name, target string) {
	t.Helper()
	mustSymlink(t, target, filepath.Join(worktreeRoot, ".mindrail", "knowledge", subdir, name))
}

// writeOutsideRecord writes a perfectly valid record somewhere the repository
// does not reach, and returns its absolute path. It is valid on purpose: the
// point of BA-10 is that this file was accepted, not that it was malformed.
func writeOutsideRecord(t *testing.T, outsideDir, name string) string {
	t.Helper()
	path := filepath.Join(outsideDir, name)
	mustWriteFile(t, path, decisionJSON("DEC-0002", schema.WriteVersion))
	return path
}

// writeInsideRecord writes a record under the repository but outside the
// knowledge store, so a link to it stays within the containment boundary.
func writeInsideRecord(t *testing.T, worktreeRoot, contents string) string {
	t.Helper()
	dir := filepath.Join(worktreeRoot, "records")
	mustMkdirAll(t, dir)
	path := filepath.Join(dir, "DEC-0002.json")
	mustWriteFile(t, path, contents)
	return path
}
