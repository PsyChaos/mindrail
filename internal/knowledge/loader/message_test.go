package loader_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/PsyChaos/mindrail/internal/app"
	"github.com/PsyChaos/mindrail/internal/knowledge/schema"
)

// problemShape is one way to make the loader refuse a record, named so a failure
// says which one.
type problemShape struct {
	name string
	// setup plants the shape and returns the directories whose absolute paths
	// must not appear in any message it produces.
	setup func(t *testing.T, worktree string) []string
	// wantCode, when set, pins the diagnosis as well as the spelling.
	wantCode app.Code
	// wantReason, when set, must survive into the message.
	wantReason string
	// unwantedReason, when set, must not appear: it is the neighbouring
	// diagnosis this shape must not be confused with.
	unwantedReason string
}

// everyProblemShape is every way a single record becomes a Problem. New shapes
// belong here rather than in a test of their own, because the property below is
// about all of them at once.
func everyProblemShape() []problemShape {
	body := decisionJSON("DEC-0002", schema.WriteVersion)

	return []problemShape{
		{
			name: "a record whose links loop",
			setup: func(t *testing.T, worktree string) []string {
				t.Helper()
				// The two links that form the loop deliberately carry no
				// .json suffix, so the walk ignores them and the store holds
				// exactly one broken record.
				bucket := filepath.Join(worktree, ".mindrail", "knowledge", "decisions")
				mustSymlink(t, filepath.Join(bucket, "loop-b"), filepath.Join(bucket, "loop-a"))
				mustSymlink(t, filepath.Join(bucket, "loop-a"), filepath.Join(bucket, "loop-b"))
				mustSymlink(t, filepath.Join(bucket, "loop-a"), filepath.Join(bucket, "DEC-0002.json"))
				return nil
			},
			wantCode: app.CodeKnowledgeUnreadable,
			// Naming the condition is the point: "could not be resolved" plus
			// a link loop is what the reader has to go and look for.
			wantReason: "symbolic link that loops",
			// It resolves to nothing, not to somewhere outside.
			unwantedReason: "outside the repository",
		},
		{
			name: "a record that leaves the repository",
			setup: func(t *testing.T, worktree string) []string {
				t.Helper()
				outside := t.TempDir()
				linkRecord(t, worktree, "decisions", "DEC-0002.json",
					writeOutsideRecord(t, outside, "evil.json"))
				return []string{outside}
			},
			wantCode:   app.CodePathEscapesRoot,
			wantReason: "resolves outside the repository root",
		},
		{
			name: "a record whose link dangles inside the repository",
			setup: func(t *testing.T, worktree string) []string {
				t.Helper()
				linkRecord(t, worktree, "decisions", "DEC-0002.json",
					filepath.Join("..", "..", "..", "records", "never-written.json"))
				return nil
			},
			wantCode: app.CodeKnowledgeUnreadable,
			// The operating system's own reason for this one is exact and
			// path-free, and it must not be flattened into the generic one.
			wantReason:     "no such file or directory",
			unwantedReason: "could not be resolved",
		},
		{
			name: "a record that is not JSON",
			setup: func(t *testing.T, worktree string) []string {
				t.Helper()
				writeRecord(t, worktree, "decisions", "DEC-0002.json", "{not json")
				return nil
			},
			wantCode:   app.CodeKnowledgeUnreadable,
			wantReason: "is not a JSON object",
		},
		{
			name: "a record with no schema_version",
			setup: func(t *testing.T, worktree string) []string {
				t.Helper()
				writeRecord(t, worktree, "decisions", "DEC-0002.json", `{"id":"DEC-0002"}`)
				return nil
			},
			wantCode:   app.CodeKnowledgeUnreadable,
			wantReason: "has no schema_version",
		},
		{
			name: "a record with a schema_version that is not an integer",
			setup: func(t *testing.T, worktree string) []string {
				t.Helper()
				writeRecord(t, worktree, "decisions", "DEC-0002.json", `{"schema_version":"one"}`)
				return nil
			},
			wantCode:   app.CodeKnowledgeUnreadable,
			wantReason: "not an integer",
		},
		{
			name: "a record from a newer writer",
			setup: func(t *testing.T, worktree string) []string {
				t.Helper()
				writeRecord(t, worktree, "decisions", "DEC-0002.json",
					decisionJSON("DEC-0002", schema.WriteVersion+1000))
				return nil
			},
			wantCode:   app.CodeKnowledgeSchemaUnsupported,
			wantReason: "reader window",
		},
		{
			name: "a record that cannot be opened",
			setup: func(t *testing.T, worktree string) []string {
				t.Helper()
				if os.Geteuid() == 0 {
					t.Skip("root reads files regardless of their mode")
				}
				writeRecord(t, worktree, "decisions", "DEC-0002.json", body)
				record := filepath.Join(worktree, ".mindrail", "knowledge", "decisions", "DEC-0002.json")
				if err := os.Chmod(record, 0o000); err != nil {
					t.Fatalf("make the record unreadable: %v", err)
				}
				t.Cleanup(func() { _ = os.Chmod(record, 0o600) })
				return nil
			},
			wantCode:   app.CodeKnowledgeUnreadable,
			wantReason: "permission denied",
		},
	}
}

// TestNoProblemMessageCarriesAnAbsolutePath is finding R-01, and it is written
// as a property over every shape rather than as an assertion about the one
// message that leaked.
//
// Problem.Message is persisted and printed, and tech-stack §74 says Mindrail
// stores and prints repository-relative forward-slash paths. Two errors from the
// containment layer are neither *fs.PathError nor the containment sentinel — a
// link loop, reported by filepath.EvalSymlinks as a plain errors.New, and the
// hop cap, reported by the canonicalizer as a plain fmt.Errorf — so
// readFailureReason's *fs.PathError branch did not catch them and their text
// went out verbatim, absolute path and all:
//
//	.mindrail/knowledge/decisions/DEC-0002.json cannot be read: filesystem:
//	canonicalize "/tmp/repo/.mindrail/knowledge/decisions/DEC-0002.json":
//	EvalSymlinks: too many links
//
// Asserting on that one sentence would have fixed that one sentence. The
// property is what holds for the messages nobody has written yet: whatever a
// record's problem turns out to be, the only path in the message is the one the
// loader put there itself.
func TestNoProblemMessageCarriesAnAbsolutePath(t *testing.T) {
	requireSymlinks(t)

	for _, shape := range everyProblemShape() {
		t.Run(shape.name, func(t *testing.T) {
			worktree := t.TempDir()
			makeKnowledgeDirs(t, worktree)
			forbidden := append([]string{worktree}, shape.setup(t, worktree)...)

			store, err := newLoader(t, worktree).Load(context.Background())
			if err != nil {
				t.Fatalf("Load() error = %v, want nil: one bad record costs one record (D-06)", err)
			}
			if len(store.Problems) != 1 {
				t.Fatalf("Problems = %+v, want exactly one", store.Problems)
			}

			problem := store.Problems[0]
			for _, leak := range forbidden {
				if strings.Contains(problem.Message, leak) {
					t.Errorf("Problem.Message = %q leaks the machine-local path %q;"+
						" only the repo-relative spelling belongs there (tech-stack §74)",
						problem.Message, leak)
				}
			}
			// Not only this machine's directories: no absolute path at all. A
			// message assembled on one machine is read on another, and a
			// temporary directory that happens not to appear in this run's
			// prefix is still a leak.
			for _, word := range strings.Fields(problem.Message) {
				if looksAbsolute(strings.Trim(word, `"'.,;:`)) {
					t.Errorf("Problem.Message = %q contains the absolute path %q;"+
						" Mindrail prints repo-relative forward-slash paths (tech-stack §74)",
						problem.Message, word)
				}
			}
			// The one path a message may carry is the record's own, in that
			// spelling. A message that names no path at all is fine — the
			// reader window one does, and Problem.Path carries the record.
			if strings.ContainsRune(problem.Message, '/') &&
				!strings.Contains(problem.Message, escapingRecordPath) {
				t.Errorf("Problem.Message = %q carries a path that is not the record's own %q",
					problem.Message, escapingRecordPath)
			}

			if shape.wantCode != "" && problem.Code != shape.wantCode {
				t.Errorf("Problem.Code = %q, want %q", problem.Code, shape.wantCode)
			}
			if shape.wantReason != "" && !strings.Contains(problem.Message, shape.wantReason) {
				t.Errorf("Problem.Message = %q, want it to keep the reason %q;"+
					" a message with no path must still say what went wrong",
					problem.Message, shape.wantReason)
			}
			if shape.unwantedReason != "" && strings.Contains(problem.Message, shape.unwantedReason) {
				t.Errorf("Problem.Message = %q, want it not to say %q: that is a different condition",
					problem.Message, shape.unwantedReason)
			}
			if problem.Path != escapingRecordPath {
				t.Errorf("Problem.Path = %q, want %q", problem.Path, escapingRecordPath)
			}
		})
	}
}

// looksAbsolute reports whether a word from a message is a rooted path in any
// spelling Mindrail could meet: a unix path, a Windows drive path, or a UNC one.
func looksAbsolute(word string) bool {
	switch {
	case strings.HasPrefix(word, "/") && len(word) > 1:
		return true
	case strings.HasPrefix(word, `\\`):
		return true
	case len(word) > 2 && word[1] == ':' && (word[2] == '\\' || word[2] == '/'):
		return true
	}
	return false
}

// TestProblemMessagesStillCarryTheOperatingSystemsReason is the over-fire guard
// for the fix above.
//
// The remedy for R-01 is to stop pasting an error's own text into the message
// when that error is not an *fs.PathError. Applied one step too widely — to
// *fs.PathError as well — it would replace "permission denied" and "no such file
// or directory" with one sentence about symbolic links, and send every reader of
// a chmod-ed record hunting for a link that is not there. Those two reasons come
// from the kernel, name the condition exactly, and contain no path of their own,
// so they must survive.
func TestProblemMessagesStillCarryTheOperatingSystemsReason(t *testing.T) {
	requireSymlinks(t)

	for _, tc := range []struct {
		name   string
		setup  func(t *testing.T, worktree string)
		reason string
	}{
		{
			name: "a record the process may not open",
			setup: func(t *testing.T, worktree string) {
				t.Helper()
				if os.Geteuid() == 0 {
					t.Skip("root reads files regardless of their mode")
				}
				writeRecord(t, worktree, "decisions", "DEC-0002.json",
					decisionJSON("DEC-0002", schema.WriteVersion))
				record := filepath.Join(worktree, ".mindrail", "knowledge", "decisions", "DEC-0002.json")
				if err := os.Chmod(record, 0o000); err != nil {
					t.Fatalf("make the record unreadable: %v", err)
				}
				t.Cleanup(func() { _ = os.Chmod(record, 0o600) })
			},
			reason: "permission denied",
		},
		{
			name: "a record whose link points at nothing",
			setup: func(t *testing.T, worktree string) {
				t.Helper()
				linkRecord(t, worktree, "decisions", "DEC-0002.json",
					filepath.Join("..", "..", "..", "records", "never-written.json"))
			},
			reason: "no such file or directory",
		},
		{
			name: "a record that is a directory",
			setup: func(t *testing.T, worktree string) {
				t.Helper()
				mustMkdirAll(t, filepath.Join(worktree, "elsewhere"))
				linkRecord(t, worktree, "decisions", "DEC-0002.json",
					filepath.Join("..", "..", "..", "elsewhere"))
			},
			reason: "is a directory",
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
				t.Fatalf("Problems = %+v, want exactly one", store.Problems)
			}

			problem := store.Problems[0]
			if !strings.Contains(problem.Message, tc.reason) {
				t.Errorf("Problem.Message = %q, want it to keep the kernel's reason %q",
					problem.Message, tc.reason)
			}
			if strings.Contains(problem.Message, "could not be resolved") {
				t.Errorf("Problem.Message = %q: this condition has an exact reason and"+
					" must not be flattened into the link-resolution one", problem.Message)
			}
			if problem.Code != app.CodeKnowledgeUnreadable {
				t.Errorf("Problem.Code = %q, want %q", problem.Code, app.CodeKnowledgeUnreadable)
			}
		})
	}
}
