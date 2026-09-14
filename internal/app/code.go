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
