package doctor

import (
	"github.com/PsyChaos/mindrail/internal/config"
	"github.com/PsyChaos/mindrail/internal/coordination"
	"github.com/PsyChaos/mindrail/internal/filesystem"
	"github.com/PsyChaos/mindrail/internal/git"
	"github.com/PsyChaos/mindrail/internal/index"
	"github.com/PsyChaos/mindrail/internal/knowledge/loader"
	"github.com/PsyChaos/mindrail/internal/knowledge/validate"
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

	// KnowledgeFindings is spec §95 steps 5-11 over the store above, run once
	// by bootstrap and filed here (decision D-42). It is deliberately a third
	// field beside Knowledge and KnowledgeErr rather than a fourth thing a check
	// could compute: validate.Check needs a compiled schema.Validator whose
	// construction returns an error, and a check that built one would be opening
	// and failing in a place this type promises does neither.
	//
	// It is data, never an error. A store full of invalid records leaves
	// KnowledgeErr nil (decision D-42, AC-08.3): the records the repository owns
	// are wrong and the binary is fine, so folding them into KnowledgeErr would
	// halt the startup sequence and make every later block report itself as
	// never taken — a fabricated absence rather than a finding.
	//
	// A nil slice here means the same thing a zero Knowledge does: bootstrap
	// never got as far as running the pipeline. That is why no check reads it
	// without first asking reached(stepValidateKnowledge).
	KnowledgeFindings []validate.Finding

	Workspace    workspace.Workspace
	WorkspaceErr error

	// Coordination is MR-003's session/task/checkpoint summary, and
	// CoordinationObserved says whether anybody looked.
	//
	// The two are separate for the reason every other pair in this struct is: a
	// project with no tasks and a startup that never reached the coordination
	// store both leave the counts at zero, and "there is no work in flight" is a
	// different answer from "nobody asked". No doctor check reads either — a
	// blocked task is a fact about work, not about the installation (decision
	// D-62) — so this is carried for `status` alone.
	// CoordinationErr is the third answer, and the report had only two. A read
	// that ran and failed — a damaged timestamp, a query the database could not
	// serve — was folded into the same false flag as a read that never
	// happened, and published as "not observed — startup stopped before this
	// subsystem was read" about a sequence that had not stopped (finding F46).
	// The Runtime block has had the distinction since MR-001; this is the pair
	// WorkspaceErr already forms with Workspace.
	Coordination         coordination.Summary
	CoordinationObserved bool
	CoordinationErr      error

	// Inventory is the persisted ProjectUnit count read at startup step 8.
	// It stays separate from doctor checks: discovery is non-blocking, but
	// status must distinguish a completed zero-unit inventory from a step that
	// did not run or could not read its persisted facts.
	Inventory         []index.ProjectUnit
	InventoryObserved bool
	InventoryErr      error

	// IndexCounts is the persisted file-state census read at startup step 8
	// beside the inventory, through one SQL aggregate — never a filesystem
	// walk, so a read-only status hashes nothing and reports the cold index
	// honestly instead of pretending otherwise. IndexObserved says the census
	// was read; IndexErr is the third answer, a census that ran and failed.
	IndexCounts   map[index.FileState]int
	IndexObserved bool
	IndexErr      error

	// Probes holds what doctor established for itself; see Probe.
	Probes Probes
}
