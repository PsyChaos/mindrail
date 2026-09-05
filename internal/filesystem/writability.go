package filesystem

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/PsyChaos/mindrail/internal/app"
)

// RootKind names which of the directories Mindrail writes into an answer is
// about.
//
// It exists because "unusable" is not one fact. The runtime root holds the
// database and the state every command reads; the cache directory holds
// derived data MR-001 never writes; the repository config directory holds
// committed repository content and is not machine-local at all. A report that
// labelled all of them "the runtime directory" told a user to fix a path that
// was not the one that failed, and a caller that could not tell them apart had
// to treat them as equally fatal (finding F7). This package states which root
// failed and what it is for, and leaves the severity to the caller.
type RootKind string

const (
	// RootRuntime is the directory the runtime database lives in.
	RootRuntime RootKind = "runtime root"

	// RootCache is the directory derived data lives in.
	RootCache RootKind = "cache directory"

	// RootRepository is <worktree>/.mindrail, the repository's own
	// configuration and knowledge scaffolding.
	//
	// It is a third kind rather than a variant of the runtime root because
	// everything a consumer would grade it on differs: it lives in the working
	// tree, it is version-controlled, it is shared by every contributor, and no
	// environment variable relocates it. Without its own kind an unwritable
	// checkout arrived carrying no root_kind at all, so a caller grading by root
	// kind had to file it under whichever root it guessed.
	RootRepository RootKind = "repository config directory"
)

// Purpose says what the directory is used for, in a noun phrase a report can
// put after "Mindrail stores ... here".
func (k RootKind) Purpose() string {
	switch k {
	case RootCache:
		return "derived cache data"
	case RootRepository:
		return "the repository's configuration and the knowledge scaffolding committed with it"
	default:
		return "the runtime database and the workspace state recorded in it"
	}
}

// Impact states what is lost when this root is unusable, without ruling on how
// fatal that is: MR-001 reads and writes the runtime root on every command and
// never touches the cache, and which of those should stop a command is the
// caller's call, not this package's.
//
// It is exported so that a package which creates one of these roots itself
// describes the loss in the same words the probe does, instead of keeping a
// second copy of the sentence that can drift from this one.
func (k RootKind) Impact() string {
	switch k {
	case RootCache:
		return "Mindrail cannot store derived cache data for this repository"
	case RootRepository:
		return "Mindrail cannot lay down the repository scaffolding, so knowledge records have nowhere to live"
	default:
		return "Mindrail cannot store its runtime state, so no command that needs the database can run"
	}
}

// override names the environment variable that relocates this root, so a remedy
// can offer the escape hatch that actually applies to it. The empty string means
// there is none: the repository config directory is where the repository says it
// is, and offering MINDRAIL_RUNTIME_DIR for it would send a user to relocate a
// root that is not the one that failed (decision D-07 names those two overrides
// and only those two).
func (k RootKind) override() string {
	switch k {
	case RootCache:
		return "MINDRAIL_CACHE_DIR"
	case RootRepository:
		return ""
	default:
		return "MINDRAIL_RUNTIME_DIR"
	}
}

// RootKindOf reports which runtime root a RUNTIME_PATH_UNWRITABLE error is
// about, so a caller can grade it.
//
// It is the other half of leaving severity to the caller: EnsureDirs returns
// one error for whichever root stopped it, and without this a caller has to
// treat an unusable cache directory exactly as it treats an unusable runtime
// root -- which is how a directory MR-001 never writes to came to stop every
// command (finding F7).
func RootKindOf(err error) (RootKind, bool) {
	payload, ok := app.PayloadOf(err)
	if !ok {
		return "", false
	}
	switch RootKind(payload.Metadata["root_kind"]) {
	case RootRuntime:
		return RootRuntime, true
	case RootCache:
		return RootCache, true
	case RootRepository:
		return RootRepository, true
	default:
		return "", false
	}
}

// ErrDanglingSymlink reports a runtime directory that is a symlink resolving to
// nothing.
//
// It is its own condition because it is invisible to os.Stat, which follows the
// link and reports the absence at the far end. A probe built on Stat alone
// therefore called the path "not there yet, and creatable" while MkdirAll --
// which cannot write through a link to nowhere -- refused it, so doctor said
// "run `mindrail init`" and init exited 4 with a permissions remedy that could
// not clear a link (finding F6). Presence.probeUnresolved answers the same
// question one directory level down for the database file itself.
var ErrDanglingSymlink = errors.New("path is a symlink that resolves to nothing")

// Writability is the answer to "could Mindrail use this runtime directory?"
// asked without touching it.
//
// It exists because ResolveRuntimePaths deliberately answers a different
// question — where the state *would* live — and a caller that treats a
// successful derivation as "the path is usable" reports an unwritable
// repository as healthy. Resolution and usability are separate facts and this
// type keeps them separate.
type Writability struct {
	// Dir is the runtime directory the answer is about.
	Dir string
	// Kind says which of Mindrail's roots Dir is, so a caller never labels one
	// with the other's name.
	Kind RootKind
	// Purpose says what that root is used for, so a report can explain what is
	// lost without knowing the layout.
	Purpose string
	// Probed is the directory that was actually inspected: Dir when it already
	// exists, otherwise its nearest existing ancestor, which is the directory
	// the create would have to happen in.
	Probed string
	// Exists reports whether Dir itself is already there. A directory that does
	// not exist yet can still be usable, which is why this is not Usable.
	Exists bool
	// Usable reports whether Mindrail could create or write inside Dir.
	Usable bool
	// Err is the structured RUNTIME_PATH_UNWRITABLE error, non-nil exactly when
	// Usable is false.
	Err error
}

// root pairs a derived location with what it is for.
type root struct {
	kind RootKind
	dir  string
}

// roots lists the directories EnsureDirs creates, in the order it creates them.
// It is the single definition of "Mindrail's runtime directories", so the probe
// and the create cannot disagree about which directories those are.
func (p RuntimePaths) roots() []root {
	all := make([]root, 0, 2)
	for _, candidate := range []root{{RootRuntime, p.RuntimeRoot}, {RootCache, p.CacheDir}} {
		if candidate.dir != "" {
			all = append(all, candidate)
		}
	}
	return all
}

// ProbeWritable reports whether the runtime root is usable, creating nothing.
//
// Doctor and status must be able to ask this: decision D-01 forbids them from
// mutating anything, so the only honest way for them to distinguish "the
// runtime path is fine and the repository is simply not initialised yet" from
// "the runtime path cannot be written at all" is a probe that does not write.
// Without it an unwritable .git is rendered as a missing database and remedied
// with `mindrail init`, which then fails.
//
// It answers about the runtime root and nothing else. It used to return
// whichever of the two roots failed first, which meant a caller labelling the
// answer `runtime_root_usable` published the cache directory's verdict under
// the runtime root's name (finding F7). A caller that wants every root asks
// ProbeRoots and decides for itself what each one's failure is worth.
//
// The second result is false when there was nothing to probe — no runtime root
// was ever derived. The Writability is then zero, and a caller must not read
// that as "unwritable": a check whose input was never populated has not run.
func (p RuntimePaths) ProbeWritable() (Writability, bool) {
	if p.RuntimeRoot == "" {
		return Writability{}, false
	}
	return probeDir(RootRuntime, p.RuntimeRoot), true
}

// ProbeRoots answers for every runtime directory, in the order EnsureDirs would
// create them, creating nothing.
//
// Grading the answers is the caller's job. MR-001 stores nothing in the cache
// directory, so an unusable one does not stop the same commands an unusable
// runtime root does; deciding that here would bake one milestone's needs into
// the layer that only knows what is on disk.
func (p RuntimePaths) ProbeRoots() []Writability {
	all := p.roots()
	answers := make([]Writability, 0, len(all))
	for _, r := range all {
		answers = append(answers, probeDir(r.kind, r.dir))
	}
	return answers
}

// probeDir answers for one directory.
func probeDir(kind RootKind, dir string) Writability {
	clean := filepath.Clean(dir)
	result := Writability{Dir: dir, Kind: kind, Purpose: kind.Purpose()}

	if dangling(clean) {
		result.Probed = clean
		result.Err = unwritablePathError(kind, dir, clean, ErrDanglingSymlink)
		return result
	}

	existing, info, err := nearestExisting(clean)
	result.Probed = existing
	result.Exists = err == nil && existing == clean

	switch {
	case err != nil:
		// Either nothing on the path exists, or the stat itself was refused
		// because an ancestor is not searchable. Neither leaves a usable dir.
		result.Err = unwritablePathError(kind, dir, existing, err)
	case !info.IsDir():
		result.Err = unwritablePathError(kind, dir, existing, ErrNotDirectory)
	default:
		if accessErr := accessWritable(existing); accessErr != nil {
			result.Err = unwritablePathError(kind, dir, existing, accessErr)
			break
		}
		result.Usable = true
	}
	return result
}

// dangling reports whether dir is a symlink that does not resolve.
//
// Both calls are needed and neither alone answers it: Lstat sees the link but
// not where it goes, Stat sees where it goes but not that it was a link. A
// symlink that resolves to a real directory is not dangling and this returns
// false for it, which is the case that must keep working — pointing the runtime
// root at storage elsewhere is a supported layout, not a fault.
func dangling(dir string) bool {
	link, err := os.Lstat(dir)
	if err != nil || link.Mode()&fs.ModeSymlink == 0 {
		return false
	}
	_, err = os.Stat(dir)
	return err != nil
}

// nearestExisting returns the longest prefix of dir that is on disk, with its
// stat. When the directory itself is absent that prefix is the one a create
// would have to happen in, which is where the permission has to be and so what
// a remedy has to name: telling a user to fix the mode of a directory that is
// not there is not a remedy.
func nearestExisting(dir string) (string, os.FileInfo, error) {
	current := filepath.Clean(dir)
	for {
		info, err := os.Stat(current)
		if err == nil {
			return current, info, nil
		}
		if !errors.Is(err, fs.ErrNotExist) {
			return current, nil, err
		}
		parent := filepath.Dir(current)
		if parent == current {
			return current, nil, err
		}
		current = parent
	}
}

// unwritablePathError reports a runtime location Mindrail cannot use.
//
// Two things are decided from the cause rather than assumed. The sentence names
// the directory that actually carries the permission, which for a location that
// does not exist yet is its parent rather than the location itself. And the
// first next action is the one that clears *this* obstruction: "check the
// permissions" was printed even for a file sitting in the way and a link
// pointing at nothing, neither of which any mode change can fix, which sent the
// user to chmod a path that was already correct (finding F8).
//
// The same function serves the probe and the create, so `doctor` and `init`
// cannot describe one condition two ways.
func unwritablePathError(kind RootKind, dir, probed string, cause error) error {
	label := string(kind)
	why := fmt.Sprintf("the %s %q could not be created", label, dir)
	next := []string{fmt.Sprintf("check the permissions on %q", probed)}

	switch {
	case errors.Is(cause, ErrDanglingSymlink):
		why = fmt.Sprintf("the %s %q is a symlink that points at nothing", label, dir)
		next = []string{fmt.Sprintf("remove or repoint the link at %q", dir)}
	case errors.Is(cause, ErrNotDirectory):
		why = fmt.Sprintf("%q is in the way of the %s and is not a directory", probed, label)
		next = []string{fmt.Sprintf("remove or move aside %q", probed)}
	case errors.Is(cause, fs.ErrPermission) && probed == filepath.Clean(dir):
		why = fmt.Sprintf("the %s %q is not writable", label, dir)
	case errors.Is(cause, fs.ErrPermission):
		why = fmt.Sprintf("the %s %q cannot be created because %q is not writable", label, dir, probed)
	}

	if override := kind.override(); override != "" {
		next = append(next, fmt.Sprintf("or point %s at a usable location", override))
	}

	return app.NewError(
		app.CodeRuntimePathUnwritable,
		app.KindUnavailable,
		why,
		kind.Impact(),
		next...,
	).WithMetadata("path", dir).
		WithMetadata("probed_path", probed).
		WithMetadata("root_kind", label).
		WithMetadata("detail", cause.Error()).
		WithCause(cause)
}
