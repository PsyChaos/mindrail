package app

import "slices"

// Code is the stable, machine-readable identity of a failure mode. Agents and
// CI branch on it, so it is the one part of an error that must never be
// reworded: prose can be improved, a code cannot.
//
// Codes are unprefixed SCREAMING_SNAKE (decision D-18). The MINDRAIL_ prefix
// stays reserved for process and infrastructure conditions, so a domain code
// can never be mistaken for one.
type Code string

// The codes this binary may emit. Every constant here must also appear in
// allCodes; TestCodeRegistryIsUniqueAndExhaustive fails otherwise.
const (
	CodeNotAGitRepository           Code = "NOT_A_GIT_REPOSITORY"
	CodeBareRepository              Code = "BARE_REPOSITORY_UNSUPPORTED"
	CodeGitUnavailable              Code = "GIT_UNAVAILABLE"
	CodeGitTimeout                  Code = "GIT_TIMEOUT"
	CodePathEscapesRoot             Code = "PATH_ESCAPES_ROOT"
	CodePathNotRepresentable        Code = "PATH_NOT_REPRESENTABLE"
	CodeRuntimePathUnwritable       Code = "RUNTIME_PATH_UNWRITABLE"
	CodeRuntimeDBUnavailable        Code = "RUNTIME_DB_UNAVAILABLE"
	CodeRuntimeDBCorrupt            Code = "RUNTIME_DB_CORRUPT"
	CodeRuntimeDBSchemaTooNew       Code = "RUNTIME_DB_SCHEMA_TOO_NEW"
	CodeMigrationFailed             Code = "MIGRATION_FAILED"
	CodeMigrationChecksumMismatch   Code = "MIGRATION_CHECKSUM_MISMATCH"
	CodeWorkspaceNotInitialized     Code = "WORKSPACE_NOT_INITIALIZED"
	CodeWorkspaceRegistrationFailed Code = "WORKSPACE_REGISTRATION_FAILED"
	CodeConfigInvalid               Code = "CONFIG_INVALID"
	CodeConfigUnknownEnvVar         Code = "CONFIG_UNKNOWN_ENV_VAR"
	CodeKnowledgeUnreadable         Code = "KNOWLEDGE_UNREADABLE"
	CodeKnowledgeSchemaUnsupported  Code = "KNOWLEDGE_SCHEMA_UNSUPPORTED"

	// CodeKnowledgeInvalid marks a knowledge record this binary read and found
	// wrong: it breaks its own schema document, its file name disagrees with the
	// id inside it, it names a supersede target that is not there, or its scope
	// is not spelled the way tech-stack §74 requires.
	//
	// It is deliberately not KNOWLEDGE_UNREADABLE. That code means "this binary
	// could not read the record", and the two have opposite remedies: one is
	// fixed by upgrading Mindrail, this one by editing a file the repository
	// owns (decision D-38). Collapsing them would make the component code
	// ambiguous at the one place a consumer branches on it.
	CodeKnowledgeInvalid Code = "KNOWLEDGE_INVALID"

	// CodeKnowledgeSupersedeCycle marks a supersede lineage with no end (spec
	// §95 step 9).
	//
	// It is separate from KNOWLEDGE_INVALID rather than the same code carrying a
	// flag because `status` publishes exactly one Code per component, and the two
	// conditions differ in state, exit class and remedy: an invalid record costs
	// the repository the records it names, while a cycle makes every answer to
	// "which Decision is current" wrong (decision D-39, D-43).
	CodeKnowledgeSupersedeCycle Code = "KNOWLEDGE_SUPERSEDE_CYCLE"

	// CodeCommandLineInvalid marks a command line this binary could not
	// understand: an unknown subcommand, an unknown flag, a surplus argument.
	// It is distinct from CONFIG_INVALID because the two have different
	// remedies — one is fixed in the shell, the other in a file — and because a
	// caller branching on "was my invocation wrong?" cannot tell them apart
	// from the exit code alone (findings F04, F08).
	CodeCommandLineInvalid Code = "COMMAND_LINE_INVALID"

	// CodeStartupIncomplete marks a reading that was never taken because the
	// tech-stack §87 startup sequence aborted before the step that would have
	// populated it. It is the machine-readable spelling of "nobody looked",
	// which a report must be able to say without borrowing the vocabulary of
	// "we looked and it is not there".
	CodeStartupIncomplete Code = "STARTUP_INCOMPLETE"

	// CodeSessionNotFound marks a session handle that names no session this
	// repository has ever minted.
	//
	// It is a refusal rather than a mint (decision D-61). A caller who mistypes
	// the handle they were given must be told, because the alternative — minting
	// a session under the typo — turns one agent into two and puts the next
	// checkpoint under an identity nothing else will ever refer to.
	CodeSessionNotFound Code = "SESSION_NOT_FOUND"

	// CodeTaskNotFound marks a task id no row carries. It is separate from
	// SESSION_NOT_FOUND because the remedies differ: one is `mindrail task list`,
	// the other is the id the caller's own `session open` printed.
	CodeTaskNotFound Code = "TASK_NOT_FOUND"

	// CodeTaskStateInvalid marks a state change spec §58's lifecycle does not
	// have: completing a task nobody claimed, reopening an abandoned one,
	// claiming one that is already claimed.
	//
	// It is its own code because it is the only refusal in this milestone that a
	// caller can act on without changing anything else — the task exists, the
	// session exists, and the answer is that this move is not available from
	// where the task is. The message names both states and the moves that are
	// (decision D-55).
	CodeTaskStateInvalid Code = "TASK_STATE_INVALID"

	// CodeCheckpointNotFound marks a task with no checkpoint on it where one was
	// required. It is not the answer to `task show` on a fresh task: a task
	// nobody has checkpointed yet is a normal state, reported as an absent
	// checkpoint rather than as a failure.
	CodeCheckpointNotFound Code = "CHECKPOINT_NOT_FOUND"

	// CodeCoordinationUnavailable marks a coordination command run against a
	// repository that has no runtime store yet.
	//
	// It is deliberately not WORKSPACE_NOT_INITIALIZED, which names the same
	// condition and is the right code for `status` and `doctor`. That one is
	// decision D-03's single zero-exit row: it is a *state to report*, and the
	// exit-class table's comment is explicit that it must never appear in an
	// error object at all. A command that was asked to open a task and did not
	// open one has not reported a state — it has failed to do the thing — and
	// exiting 0 there would make `mindrail task open && ...` run its second half
	// against a task that does not exist.
	//
	// The remedy is the same sentence `status` prints, because the fix is the
	// same: run `mindrail init`.
	CodeCoordinationUnavailable Code = "COORDINATION_UNAVAILABLE"

	// CodeCoordinationWriteFailed marks a coordination row that did not reach
	// the runtime database, after storage.WriteFailure has had its say. It is
	// the coordination counterpart of WORKSPACE_REGISTRATION_FAILED and exists
	// for the same reason: the generic remedy is only correct for the failures
	// that really are about the rows being written, and the named storage
	// conditions carry remedies that can succeed.
	CodeCoordinationWriteFailed Code = "COORDINATION_WRITE_FAILED"

	// CodeCoordinationReadFailed marks a coordination row the runtime database
	// holds and could not answer for: a timestamp column that does not parse, a
	// query that failed for a reason the storage layer does not name.
	//
	// It exists because the read paths had no code at all. Every write in
	// internal/coordination was wrapped in a domain error and every read
	// returned a bare fmt.Errorf, so one damaged `created_at` reached the user
	// as `{"code":"","why":"look up the last checkpoint of \"TSK-…\": parsing
	// time \"yesterday\" as \"2006\"…","impact":"","next_action":[]}` — a Go
	// parser message with a format string in it, and nothing a caller can
	// branch on (finding F44).
	//
	// The counterpart of COORDINATION_WRITE_FAILED, and ExitFailed for the same
	// reason: the storage conditions that really are "come back later" are
	// named by the storage layer before this code is reached.
	CodeCoordinationReadFailed Code = "COORDINATION_READ_FAILED"

	// CodeBusyRetryable marks a command that waited the whole busy budget for
	// the runtime database's write lock and was still refused — at BEGIN, or
	// while opening a database another process was creating.
	//
	// It is the one code with the MINDRAIL_ prefix, which decision D-18
	// reserved for process and infrastructure conditions and named this value
	// as the example. Nothing is wrong with the database or the repository, and
	// the only remedy is to run the command again; it is KindUnavailable (exit
	// 4) so a caller that retries on that class retries here. Until MR-004 the
	// condition shared RUNTIME_DB_UNAVAILABLE with a corrupt path and a missing
	// directory, whose remedies cannot succeed against a lock (decision D-74).
	CodeBusyRetryable Code = "MINDRAIL_BUSY_RETRYABLE"

	// The lease conditions (MR-004, decision D-76). Three codes rather than one
	// with a flag, because a caller's next action differs: on a conflict it
	// waits for the expiry it is told or asks the holder to release; on a lease
	// it no longer holds it acquires again; on an unknown id it looks the id up.

	// CodeLeaseConflict marks a target whose active lease another session
	// holds. It is the milestone's scenario refused by name: the second agent
	// is told the holder, the lease id and the expiry, never to force.
	CodeLeaseConflict Code = "LEASE_CONFLICT"

	// CodeLeaseNotHeld marks a renew or a release of a lease this session no
	// longer holds because it expired or was released. The remedy is to acquire
	// again, which is why it is not LEASE_CONFLICT: nobody holds it.
	CodeLeaseNotHeld Code = "LEASE_NOT_HELD"

	// CodeLeaseNotFound marks a lease id no row carries.
	CodeLeaseNotFound Code = "LEASE_NOT_FOUND"

	// CodeStateRevisionConflict marks a task move decided on a reading of the
	// task that is no longer current: the caller expected revision n and the
	// row has moved on (spec §11, decision D-72). It is the refusal that
	// replaces silent last-write-wins, and it is its own code because the
	// caller's next action — re-read, decide again — differs from every other
	// refusal's.
	CodeStateRevisionConflict Code = "STATE_REVISION_CONFLICT"

	// CodeOperationIDConflict marks an operation id used before for a
	// different request (decision D-71). The recorded result answers another
	// request, so it is not returned; the caller minted one id for two
	// requests, or changed a request under one, and either way the next
	// action is a new id.
	CodeOperationIDConflict Code = "OPERATION_ID_CONFLICT"

	// CodeSyntaxLanguageUnsupported marks a file whose extension maps to no
	// grammar this binary carries (decision D-88; the task list's MR-005
	// scope is Python, TypeScript and JavaScript). It is its own code because
	// the condition is permanent and the remedy is honesty: nothing clears
	// it, the file is recorded out of scope rather than pending, and the next
	// action is to leave it alone rather than to retry it.
	CodeSyntaxLanguageUnsupported Code = "SYNTAX_LANGUAGE_UNSUPPORTED"

	// CodeSyntaxParseFailed marks a file whose grammar this binary carries
	// but whose parse produced a partial tree or an extraction error
	// (spec-1.0 §108, decision D-81). It is its own code because the file's
	// syntax is the thing to fix and nothing else is: the index continues
	// around the file, which stays failed and pending, and the remedy names
	// the file rather than the installation.
	CodeSyntaxParseFailed Code = "SYNTAX_PARSE_FAILED"

	// CodeIndexStateCorrupt marks an index state the binary cannot answer
	// from — a ledger that holds version rows for tables that do not exist,
	// or state rows whose content cannot be read back. It is its own code
	// because the runtime database's index half, not the repository, is the
	// thing that is wrong, and the remedy is to rebuild that half rather than
	// to parse anything.
	CodeIndexStateCorrupt Code = "INDEX_STATE_CORRUPT"

	// CodeSymbolIdentityAmbiguous marks a removed symbol that meets the
	// migration bar for two or more added symbols (decision D-96). It is its
	// own code because the condition is a blocked decision, not a broken
	// installation: nothing is corrupt, but no guess is taken, and the remedy
	// names the candidates rather than the database.
	CodeSymbolIdentityAmbiguous Code = "SYMBOL_IDENTITY_AMBIGUOUS"

	// CodeOrphanedProtectedSymbol marks a tracked invariant whose symbol is
	// gone with no confident heir (decision D-99). It is its own code because
	// the invariant is still active and still means something: the remedy is
	// to migrate, supersede or retire it explicitly, never to let it go
	// silent.
	CodeOrphanedProtectedSymbol Code = "ORPHANED_PROTECTED_SYMBOL"

	// CodeScopeDrift marks a changed file outside the task's declared scope
	// (spec §62, decision D-139). It is its own code because the file is
	// genuinely changed and genuinely out of scope: the remedy extends the
	// baseline or moves the edit, and completion must not silently include
	// what no task declared.
	CodeScopeDrift Code = "SCOPE_DRIFT"

	// CodeUnregisteredChange marks a symbol no open change claims (spec §64,
	// decision D-133). It is its own code because the work exists and the
	// ownership does not: the remedy declares the scope or records an
	// explicit attribution.
	CodeUnregisteredChange Code = "UNREGISTERED_CHANGE"

	// CodeReconcileAmbiguous marks a symbol two or more open changes claim
	// (spec §66, decision D-133). It is its own code because choosing would
	// launder one task's edit as another's: the remedy names every
	// candidate, and a human assigns.
	CodeReconcileAmbiguous Code = "RECONCILE_AMBIGUOUS"

	// CodeTestGuardWeakened marks a weakened verification test: removed
	// assertions, added skip/xfail/disable markers, or a removed test that
	// verified an invariant (spec §103, decision D-173). It is its own code
	// because the threat is a green run that proves nothing: the remedy
	// restores the guard or records an explicit allowance, and a removed
	// CRITICAL verification test blocks completion.
	CodeTestGuardWeakened Code = "TEST_GUARD_WEAKENED"
)

// allCodes is the registry itself, sorted once at init so RegisteredCodes can
// hand out a copy without re-sorting on every call.
var allCodes = sortedCodes([]Code{
	CodeNotAGitRepository,
	CodeBareRepository,
	CodeGitUnavailable,
	CodeGitTimeout,
	CodePathEscapesRoot,
	CodePathNotRepresentable,
	CodeRuntimePathUnwritable,
	CodeRuntimeDBUnavailable,
	CodeRuntimeDBCorrupt,
	CodeRuntimeDBSchemaTooNew,
	CodeMigrationFailed,
	CodeMigrationChecksumMismatch,
	CodeWorkspaceNotInitialized,
	CodeWorkspaceRegistrationFailed,
	CodeConfigInvalid,
	CodeConfigUnknownEnvVar,
	CodeKnowledgeUnreadable,
	CodeKnowledgeSchemaUnsupported,
	CodeKnowledgeInvalid,
	CodeKnowledgeSupersedeCycle,
	CodeCommandLineInvalid,
	CodeStartupIncomplete,
	CodeSessionNotFound,
	CodeTaskNotFound,
	CodeTaskStateInvalid,
	CodeCheckpointNotFound,
	CodeCoordinationUnavailable,
	CodeCoordinationWriteFailed,
	CodeCoordinationReadFailed,
	CodeBusyRetryable,
	CodeLeaseConflict,
	CodeLeaseNotHeld,
	CodeLeaseNotFound,
	CodeStateRevisionConflict,
	CodeOperationIDConflict,
	CodeSyntaxLanguageUnsupported,
	CodeSyntaxParseFailed,
	CodeIndexStateCorrupt,
	CodeSymbolIdentityAmbiguous,
	CodeOrphanedProtectedSymbol,
	CodeScopeDrift,
	CodeUnregisteredChange,
	CodeReconcileAmbiguous,
	CodeTestGuardWeakened,
})

var codeSet = indexCodes(allCodes)

// RegisteredCodes returns every code this binary may emit, sorted. The result
// is a copy: the registry backs IsRegistered and must survive a caller that
// sorts or truncates what it was given.
func RegisteredCodes() []Code {
	return slices.Clone(allCodes)
}

// IsRegistered reports whether c is a code this binary owns. It exists so that
// a code arriving from persisted state or another process can be rejected
// instead of being echoed back as a plausible-looking unknown.
func IsRegistered(c Code) bool {
	_, ok := codeSet[c]
	return ok
}

func sortedCodes(codes []Code) []Code {
	slices.Sort(codes)
	return codes
}

func indexCodes(codes []Code) map[Code]struct{} {
	set := make(map[Code]struct{}, len(codes))
	for _, c := range codes {
		set[c] = struct{}{}
	}
	return set
}
