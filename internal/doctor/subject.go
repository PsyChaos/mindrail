package doctor

import (
	"github.com/PsyChaos/mindrail/internal/config"
	"github.com/PsyChaos/mindrail/internal/filesystem"
	"github.com/PsyChaos/mindrail/internal/git"
	"github.com/PsyChaos/mindrail/internal/knowledge/loader"
	"github.com/PsyChaos/mindrail/internal/migration"
	"github.com/PsyChaos/mindrail/internal/storage"
	"github.com/PsyChaos/mindrail/internal/workspace"
)

// Subject is the already-resolved bootstrap state that checks report on.
//
// Every field is paired with the error the resolution step produced, because
// "this did not resolve" is exactly the condition doctor exists to report and a
// Subject that dropped the error would leave the check guessing why a value is
// zero. Bootstrap fills what it can and stops at the first hard failure; that
// is what makes a partially broken installation describable, and it is also why
// a zero field is never on its own evidence of anything. Which fields were
// populated at all is answered by haltedAt, not by inspecting them.
//
// Checks are pure functions of a Subject: they open nothing, create nothing and
// stat nothing, so a check can be exercised from a value and two checks can
// never disagree about what is on disk. The read-only observations bootstrap
// had no reason to make are gathered once, up front, by Probe — spec §83
// forbids doctor from mutating anything, and probing is not mutating
// (decision D-01).
type Subject struct {
	StartDir   string
	GitVersion string
	Repo       git.Repository
	RepoErr    error
	Paths      filesystem.RuntimePaths
	PathsErr   error

	// CacheErr is a cache directory bootstrap could not create, kept apart from
	// PathsErr because the two cost different things. MR-001 reads and writes
	// the runtime root on every command and never touches the cache, so an
	// unusable cache stops nothing; folding it into PathsErr is what drove a
	// fully working, initialised repository to BLOCKED and exit 4 over a
	// directory this binary never writes to (finding F13).
	CacheErr error

	Config       config.Loaded
	ConfigErr    error
	DBPresent    bool
	DBErr        error
	Pragmas      storage.Pragmas
	IntegrityErr error
	Migrations   []migration.Applied
	PendingCount int
	MigrateErr   error
	Knowledge    loader.Store
	KnowledgeErr error
	Workspace    workspace.Workspace
	WorkspaceErr error

	// Probes holds what doctor established for itself; see Probe.
	Probes Probes
}
