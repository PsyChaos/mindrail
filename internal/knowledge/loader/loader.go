// Package loader reads the repository-owned knowledge store under
// .mindrail/knowledge and reports what it found.
//
// Scope is deliberately narrow. Spec §95 defines a fourteen-step validation
// pipeline; this package performs steps 1-4 only — read the file, parse its
// JSON syntax, read schema_version, check it against the reader window
// (decision D-27). Performing any later step here would put a diagnostic in the
// wrong milestone and would reject records this package has no authority to
// judge.
//
// Steps 5-11 belong to MR-002 and live in internal/knowledge/validate: JSON
// Schema validation, filename/id agreement, unique ids, supersede targets,
// supersede cycles, duplicate active lineage, and scope *syntax*.
//
// Steps 12, 13 and 14 are deferred, not forgotten, and each names the milestone
// that unblocks it (AC-12.2). Step 12 resolves a scope target against the
// repository's structure and needs MR-005's index; step 13 follows a referenced
// validation profile and needs MR-010; step 14 checks evidence and test-mapping
// syntax and needs MR-011/MR-012. That list is stated once, authoritatively, in
// internal/knowledge/validate's package doc; it is repeated here only far enough
// to keep a reader of this file from concluding that MR-002 covered all ten
// remaining steps, which an earlier wording of this comment said and which was
// never true.
package loader

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/PsyChaos/mindrail/internal/app"
	"github.com/PsyChaos/mindrail/internal/filesystem"
	"github.com/PsyChaos/mindrail/internal/knowledge/schema"
)

// StoreRoot is the repository-relative location of the knowledge store
// (spec §8). It is spelled with forward slashes because that is the only
// spelling Mindrail stores or prints (tech-stack §74).
const StoreRoot = ".mindrail/knowledge"

// recordSuffix is the only file extension the store recognises. Anything else
// in the directory — a README, the .gitkeep decision D-05 writes — is ignored
// rather than reported, because scaffolding is not a broken record.
const recordSuffix = ".json"

// RecordKind names the two kinds of knowledge record (spec §53, §54).
type RecordKind string

const (
	KindDecision  RecordKind = "decision"
	KindInvariant RecordKind = "invariant"
)

// kindDirs binds each kind to its directory, in the order the store is walked.
// The kind of a record is decided by where it lives, not by the "kind" field
// inside it: reconciling the two is spec §95 step 6, and that is MR-002's.
var kindDirs = []struct {
	kind RecordKind
	dir  string
}{
	{kind: KindDecision, dir: "decisions"},
	{kind: KindInvariant, dir: "invariants"},
}

// RecordRef is a record this binary was able to read. It is a reference plus
// the bytes: MR-001 needed only the count and the version window, and carrying
// a body it could not yet interpret would have invited premature use of it.
// MR-002 has the interpreter, so decision D-46 hands the bytes over rather than
// making every later step re-read a file the loader has already opened — two
// reads of one path can disagree, and a verdict computed from bytes nobody else
// saw is not reproducible from the store the report shows.
type RecordRef struct {
	Kind          RecordKind `json:"kind"`
	ID            string     `json:"id"`
	Path          string     `json:"path"` // repo-relative, slash
	SchemaVersion int        `json:"schema_version"`

	// Body is the exact bytes step 1 read. It is what steps 5-11 evaluate, and
	// it is excluded from JSON so the store's published shape is unchanged.
	//
	// Excluded rather than merely omitted when empty: .mindrail/knowledge is
	// repository content a reader already has on disk, so republishing it inside
	// every status and doctor document would double the payload to say nothing
	// new. Adding it to the wire is a deliberate change to a published shape,
	// not a side effect of a field appearing in a struct.
	Body []byte `json:"-"`
}

// Problem is one record the loader could not accept.
//
// Fatal separates the two very different failures: a file that cannot be
// parsed costs the repository one record, while a record written by a newer
// Mindrail means this binary cannot see the whole store and must not pretend
// otherwise.
type Problem struct {
	Path    string   `json:"path"` // repo-relative, slash
	Code    app.Code `json:"code"`
	Message string   `json:"message"`
	Fatal   bool     `json:"fatal"` // true => unreadable schema_version
}

// Store is the result of one load.
type Store struct {
	Present                bool        `json:"present"`
	Root                   string      `json:"root"` // ".mindrail/knowledge"
	Decisions              []RecordRef `json:"decisions"`
	Invariants             []RecordRef `json:"invariants"`
	Problems               []Problem   `json:"problems"`
	WriteSchemaVersion     int         `json:"write_schema_version"`
	ReadableSchemaVersions []int       `json:"readable_schema_versions"`
}

// Count returns the number of records this binary can read. Records the loader
// refused are in Problems, never here.
func (s Store) Count() int {
	return len(s.Decisions) + len(s.Invariants)
}

// HasFatalProblem reports whether the store contains a record this binary
// cannot read. It is what drives readiness to BLOCKED: acting on a partial
// view of the invariants is worse than refusing to act.
func (s Store) HasFatalProblem() bool {
	for _, problem := range s.Problems {
		if problem.Fatal {
			return true
		}
	}
	return false
}

// Loader reads one repository's knowledge store.
type Loader struct {
	root     filesystem.Root
	registry *schema.Registry
}

// New binds a loader to a worktree root and a schema registry. The registry is
// injected rather than reached for so that the version window a report shows
// is provably the one the load enforced.
func New(root filesystem.Root, reg *schema.Registry) *Loader {
	return &Loader{root: root, registry: reg}
}

// Load performs spec §95 steps 1-4 only: read file, parse JSON syntax, read
// schema_version, check the reader window. Steps 5-14 belong to MR-002.
//
// A missing tree is success with Present=false (decision D-06): a clean clone
// of a repository that has no records must not look broken. An unreadable
// record is data in Problems, not an error — one bad file must not hide every
// good one beside it. Only a store that cannot be walked at all is an error.
func (l *Loader) Load(ctx context.Context) (Store, error) {
	store := Store{
		Root:                   StoreRoot,
		Decisions:              []RecordRef{},
		Invariants:             []RecordRef{},
		Problems:               []Problem{},
		WriteSchemaVersion:     schema.WriteVersion,
		ReadableSchemaVersions: schema.ReadableVersions(),
	}
	if l.registry != nil {
		store.WriteSchemaVersion = l.registry.WriteVersion()
		store.ReadableSchemaVersions = l.registry.ReadableVersions()
	}

	rootDir, err := l.root.Resolve(StoreRoot)
	if err != nil {
		return Store{}, unreadableStore(StoreRoot, err)
	}

	info, err := os.Stat(rootDir)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return store, nil
	case err != nil:
		return Store{}, unreadableStore(StoreRoot, err)
	case !info.IsDir():
		return Store{}, unreadableStore(StoreRoot, filesystem.ErrNotDirectory)
	}
	store.Present = true

	for _, bucket := range kindDirs {
		rel := path.Join(StoreRoot, bucket.dir)

		dir, resolveErr := l.root.Resolve(rel)
		if resolveErr != nil {
			return Store{}, unreadableStore(rel, resolveErr)
		}

		entries, readErr := os.ReadDir(dir)
		if errors.Is(readErr, fs.ErrNotExist) {
			// A repository with decisions but no invariants yet is healthy.
			continue
		}
		if readErr != nil {
			return Store{}, unreadableStore(rel, readErr)
		}

		// os.ReadDir sorts by name, which is what makes the reported order
		// reproducible across machines and filesystems.
		for _, entry := range entries {
			if ctxErr := ctx.Err(); ctxErr != nil {
				return Store{}, ctxErr
			}
			if !isRecordFile(entry) {
				continue
			}

			ref, problem := l.readRecord(bucket.kind, dir, rel, entry.Name())
			switch {
			case problem != nil:
				store.Problems = append(store.Problems, *problem)
			case bucket.kind == KindDecision:
				store.Decisions = append(store.Decisions, *ref)
			default:
				store.Invariants = append(store.Invariants, *ref)
			}
		}
	}

	return store, nil
}

// isRecordFile keeps the walk to the flat layout of spec §8. Sub-directories
// are not descended into: file name and id have to agree (spec §95 step 6),
// which only makes sense for a flat store.
func isRecordFile(entry fs.DirEntry) bool {
	return !entry.IsDir() && strings.HasSuffix(entry.Name(), recordSuffix)
}

// recordPath is what a caller sees, so it is built from the repo-relative
// directory rather than from the resolved absolute one: an absolute path would
// leak a machine-local location into persisted and printed output.
func recordPath(relDir, name string) string {
	return path.Join(relDir, name)
}

// readRecord performs steps 1-4 for one file. Exactly one of the two returns
// is non-nil. absDir is the canonical, already-contained bucket directory the
// entry was enumerated from; relDir is the same directory in the repo-relative
// slash spelling everything printed uses.
func (l *Loader) readRecord(kind RecordKind, absDir, relDir, name string) (*RecordRef, *Problem) {
	rel := recordPath(relDir, name)

	// Step 1: file read.
	abs, problem := l.recordFile(absDir, rel, name)
	if problem != nil {
		return nil, problem
	}

	data, err := os.ReadFile(abs)
	if err != nil {
		return nil, unreadableRecord(rel, "cannot be read: "+readFailureReason(err))
	}

	// Step 2: JSON syntax. Decoding into raw messages rather than a typed
	// struct keeps this step honest — a wrongly typed "id" or "status" is a
	// schema violation (step 5, MR-002's), not a syntax error, and must not be
	// reported here.
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return nil, unreadableRecord(rel, "is not a JSON object: "+err.Error())
	}

	// Step 3: schema_version parse.
	raw, ok := fields["schema_version"]
	if !ok {
		return nil, unreadableRecord(rel, "has no schema_version, so this binary cannot tell whether it may read it")
	}
	var version int
	if err := json.Unmarshal(raw, &version); err != nil {
		return nil, unreadableRecord(rel, "has a schema_version that is not an integer: "+string(raw))
	}

	// Step 4: reader compatibility. Fail closed — a record from a newer writer
	// may carry meaning this binary would silently drop (kernel-scope §3).
	if !l.supports(version) {
		return nil, &Problem{
			Path: rel,
			Code: app.CodeKnowledgeSchemaUnsupported,
			Message: fmt.Sprintf(
				"schema_version %d is outside this binary's reader window %v; %v",
				version, l.readableVersions(), schema.ErrUnsupportedVersion),
			Fatal: true,
		}
	}

	return &RecordRef{
		Kind: kind,
		// Read verbatim. Whether it agrees with the file name is spec §95
		// step 6, and that belongs to MR-002.
		ID:            stringField(fields, "id"),
		Path:          rel,
		SchemaVersion: version,
		// The bytes step 1 read, handed on unchanged (D-46). Not re-encoded from
		// fields: a round trip through map[string]json.RawMessage would reorder
		// the keys and drop the duplicates that make a document ambiguous, and
		// step 5 has to judge the file the repository actually committed. Safe to
		// store without copying because os.ReadFile allocates a fresh slice per
		// call and this is its only surviving reference — json.Unmarshal above
		// copied into the RawMessage values rather than aliasing this array.
		Body: data,
	}, nil
}

// recordFile decides which absolute path — if any — the loader may open for one
// directory entry, and returns the problem that stops it otherwise.
//
// Why containment is checked here at all. Resolving the bucket directory proves
// the directory is inside the repository; it proves nothing about an entry
// inside it, and a record file that is a symbolic link out of the worktree was
// therefore read and counted as repository content (finding BA-10). That
// mattered little while the loader only counted records; MR-002 derives verdicts
// from these bytes, so an unvetted file now supplies the evidence a report
// reasons from.
//
// Why it is not Root.Resolve for every entry. It was, and Resolve canonicalizes
// every component of the path it is given — the worktree root, .mindrail,
// knowledge, the bucket, and only then the record — so the answer already known
// about the first four was recomputed once per record. That cost about 3.9 µs a
// record and 45% of the loader's per-record time at scale, and moved the 150 ms
// warm-path budget for `status --json` from roughly 8,700 records to roughly
// 5,700 (finding B-A3). The four components are answered once, by the Resolve of
// the bucket that produced absDir, and answering them again cannot change the
// verdict: absDir is canonical, so nothing on it is a link any more.
//
// What is left to decide is the final component, and one os.Lstat decides it:
//
//   - Not a symbolic link. Then filepath.Join(absDir, name) IS the canonical
//     path of the entry — a canonical parent plus a non-link leaf — and absDir
//     is already known to be inside the root, so the entry is too. This is the
//     ordinary record, and it is the whole of the saving.
//   - A symbolic link. Then where it lands is genuinely unknown and the full
//     boundary decides it, exactly as before. That is decision D-32's policy —
//     "links inside the root are allowed, links that leave it are refused", not
//     "no links" — so a linked record inside the repository stays readable, and
//     so does every ordinary layout that reaches the worktree through a link.
//
// The two branches are the same function of the filesystem that Root.Resolve
// alone was, so no layout changes verdict; only the syscalls spent on the
// unremarkable case do.
//
// The os.Lstat is deliberate and not free: os.ReadDir has already reported a
// type for every entry, and reading the answer off that instead measured about
// 0.7 µs a record cheaper again. It is not taken, because that type was observed
// when the directory was listed, and the last record of a ten-thousand-record
// store is read some seventy milliseconds later. Deciding containment from it
// would widen the gap between the check and the open from microseconds to the
// whole walk — wider than the code this replaces, and wider than the code before
// that. A fresh stat of the entry about to be opened is what the remaining cost
// buys.
//
// The path handed to os.ReadFile is canonical in both branches — in the second
// because Resolve returns it that way, in the first because a non-link leaf on a
// canonical parent is already canonical — and that is a statement about the
// filesystem as it was when the check ran, not a lock on it.
//
// It is worth being exact about what that does not buy, because the sentence
// this replaces claimed the opposite and an audit measured it wrong. os.ReadFile
// follows symbolic links. A link substituted for the entry between the Lstat
// above and the open below is followed, and a record outside the worktree is
// ingested — the containment check is point-in-time, not time-of-use. The window
// is the same one every earlier version of this function had, including the one
// that called Root.Resolve per record, since Resolve likewise answers about the
// disk it saw; the fast path narrows it to one syscall pair and does not close
// it. Closing it means opening the entry without following links and stating
// what the loader does when that fails, which is a containment policy in
// internal/filesystem and a decision no MR-002 requirement asked for.
//
// Reading from absDir rather than re-deriving the directory from the root also
// means the record is read out of the very directory os.ReadDir enumerated. A
// bucket swapped for an escaping link midway through the walk cannot make the
// loader read entries from somewhere else; it could only ever have made the
// remaining entries fail, which is a diagnosis about a directory that is no
// longer the one the listing came from.
func (l *Loader) recordFile(absDir, rel, name string) (string, *Problem) {
	// The equivalence above rests on name being a single path component, which
	// is all os.ReadDir yields. Anything else is not reasoned about here: it
	// goes to the boundary, which is written to reason about whole paths.
	//
	// No caller can violate that today, which is why this branch went two
	// remediations without a test that could turn it red (finding RD-05).
	// TestTheFastPathIsNotTakenForANameThatIsNotOneComponent calls this function
	// directly and is the one thing holding it.
	if filepath.Base(name) == name {
		candidate := filepath.Join(absDir, name)
		info, err := os.Lstat(candidate)
		if err != nil {
			return "", unreadableRecord(rel, "cannot be read: "+readFailureReason(err))
		}
		if info.Mode()&fs.ModeSymlink == 0 {
			return candidate, nil
		}
	}

	abs, err := l.root.Resolve(rel)
	if err != nil {
		if errors.Is(err, filesystem.ErrEscapesRoot) {
			return "", escapingRecord(rel)
		}
		return "", unreadableRecord(rel, "cannot be read: "+readFailureReason(err))
	}
	return abs, nil
}

func (l *Loader) supports(version int) bool {
	if l.registry != nil {
		return l.registry.Supports(version)
	}
	return schema.WriteVersion == version
}

func (l *Loader) readableVersions() []int {
	if l.registry != nil {
		return l.registry.ReadableVersions()
	}
	return schema.ReadableVersions()
}

// stringField returns a string-valued field, or "" when it is absent or of
// another type. Judging the type is step 5's job.
func stringField(fields map[string]json.RawMessage, name string) string {
	raw, ok := fields[name]
	if !ok {
		return ""
	}
	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		return ""
	}
	return value
}

// unresolvableReason is the reason for a path the containment layer could not
// canonicalize at all. See readFailureReason for why it is a fixed sentence.
const unresolvableReason = "its path could not be resolved; check it for a symbolic link that loops" +
	" or for a chain of links that is too long to follow"

// readFailureReason reduces a file error to its reason, in a spelling that
// carries no path at all.
//
// The caller has already put the path in the message in the repo-relative
// forward-slash form that is the only one Mindrail stores or prints (tech-stack
// §74). Every path an error of its own carries is the machine-local absolute
// one, so none of them may reach a Problem.Message, which is persisted and
// printed.
//
// An *fs.PathError is unwrapped to its inner error, which is a syscall errno —
// "permission denied", "no such file or directory" — and path-free by
// construction.
//
// Everything else becomes the fixed sentence above rather than err.Error().
// Pasting the error in was finding R-01: filepath.EvalSymlinks reports a link
// loop as a plain errors.New, and canonicalizeHop reports its own hop cap as a
// plain fmt.Errorf, so neither is an *fs.PathError and both fell through — and
// both quote the absolute component they gave up on, which is how
// `.mindrail/knowledge/decisions/DEC-0002.json cannot be read: filesystem:
// canonicalize "/tmp/repo/.mindrail/..."` came to be printed. Those two are also
// the only errors the boundary can return here that are not *fs.PathError and
// not the containment sentinel, which is why the sentence may name what it names.
//
// It is a fixed sentence and not a scrubbing of err.Error() because the property
// has to hold for errors nobody has written yet: a filter can only remove the
// shapes of path someone thought of, while a sentence with no path in it cannot
// acquire one.
func readFailureReason(err error) string {
	var pathErr *fs.PathError
	if errors.As(err, &pathErr) && pathErr.Err != nil {
		return pathErr.Err.Error()
	}
	return unresolvableReason
}

// unreadableRecord builds a non-fatal problem. It costs the repository one
// record and nothing else, so the workspace stays usable.
func unreadableRecord(rel, message string) *Problem {
	return &Problem{
		Path:    rel,
		Code:    app.CodeKnowledgeUnreadable,
		Message: rel + " " + message,
		Fatal:   false,
	}
}

// escapingRecord builds the problem for a record that resolves outside the
// repository.
//
// It is a Problem and not an error because decision D-06 prices one bad record
// at one record: a single planted or mistaken link must not make every decision
// and invariant beside it invisible. That is the difference from
// unreadableStore, which does return an error — a bucket that leaves the
// repository cannot be walked at all, so there is no store left to report.
//
// The code is PATH_ESCAPES_ROOT and deliberately not KNOWLEDGE_UNREADABLE.
// KNOWLEDGE_UNREADABLE means "this binary could not read the record", and no read
// was attempted: recordFile decides this from the resolved path, so a link
// pointing at a deleted or unopenable target arrives here exactly like one
// pointing at a perfect record. Mindrail declines to treat any of them as
// repository content, and says so without claiming anything about their bytes.
// Conflating the two is the mistake unreadableStore below stopped making for the
// directory case, where it produced the remedy "check that it is a readable
// directory" for a link the loader had walked into without difficulty.
//
// This paragraph said "this record reads perfectly" until audit round 4 found
// the same sentence standing in three places after the pass that removed it from
// the fourth (finding R4-M22). Decision D-51's rule that escapingRecord "keeps
// its comment" recorded that D-51 changed nothing here; it is not a licence for
// the sentence to stay wrong. The code, the Fatal flag and the published Message
// are untouched.
//
// The message is built from the repo-relative path rather than from the
// containment error's prose, because that prose quotes the machine-local
// absolute path of the escaping component and Problem.Message is persisted and
// printed, where only the slash-spelled repository-relative form belongs
// (tech-stack §74).
func escapingRecord(rel string) *Problem {
	return &Problem{
		Path: rel,
		Code: app.CodePathEscapesRoot,
		Message: rel + " resolves outside the repository root, so Mindrail will not read it as" +
			" repository content; replace the link with the record itself or remove it",
		Fatal: false,
	}
}

// unreadableStore builds the error for a store that cannot be walked at all.
// It is KindUnavailable rather than KindUsage: an unreadable directory is an
// environment condition (decision D-03), and reporting it as invalid usage
// would send the user looking for a mistake they did not make.
//
// The underlying Go error travels in the cause and in the "detail" metadata,
// never in why. why is prose a person reads: pasting a wrapped error into it
// leaks internal package names and repeats the absolute path the reader already
// knows, and it says nothing the three fields below do not say better.
func unreadableStore(rel string, cause error) error {
	// A directory that resolves outside the repository is not unreadable. It was
	// refused on its path rather than read — which is why "check that
	// .mindrail/knowledge is a readable directory" cleared nothing and the reader
	// was left carrying out an instruction that changes nothing about the
	// condition — and the containment layer has
	// already named the condition in the words `init` prints for the same disk.
	// Returning that error unchanged is what makes one condition one diagnosis
	// whichever command met it: before this, `status` and `doctor` said
	// KNOWLEDGE_UNREADABLE at exit 1 while `init` said PATH_ESCAPES_ROOT at
	// exit 2.
	if errors.Is(cause, filesystem.ErrEscapesRoot) {
		return cause
	}

	why := fmt.Sprintf("the knowledge store directory %q could not be read", rel)
	switch {
	case errors.Is(cause, filesystem.ErrNotDirectory):
		why = fmt.Sprintf("%q is not a directory", rel)
	case errors.Is(cause, fs.ErrPermission):
		why = fmt.Sprintf("the knowledge store directory %q is not readable", rel)
	}

	return app.NewError(
		app.CodeKnowledgeUnreadable,
		app.KindUnavailable,
		why,
		"Mindrail cannot tell which decisions and invariants apply, so it would be working from an unknown subset of the project's knowledge.",
		"Check that "+rel+" is a readable directory",
		"Remove it if this repository is not meant to carry knowledge records",
	).WithMetadata("path", rel).
		WithMetadata("detail", cause.Error()).
		WithCause(cause)
}
