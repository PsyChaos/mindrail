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

	// CodeStartupIncomplete marks a reading that was never taken because the
	// tech-stack §87 startup sequence aborted before the step that would have
	// populated it. It is the machine-readable spelling of "nobody looked",
	// which a report must be able to say without borrowing the vocabulary of
	// "we looked and it is not there".
	CodeStartupIncomplete Code = "STARTUP_INCOMPLETE"
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
	CodeStartupIncomplete,
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
