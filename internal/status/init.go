package status

import "github.com/PsyChaos/mindrail/internal/migration"

// TerminalState is the two-valued verdict spec §82 requires init to end with.
//
// It is deliberately not doctor.State and not Readiness (decision D-16): those
// describe a component and an installation, while this is the sentence a human
// reads last and a CI job greps for. The strings are the spec's literals, not
// descriptions of them, so they must not be reworded.
type TerminalState string

const (
	TerminalReady   TerminalState = "READY FOR TARGETED WORK"
	TerminalBlocked TerminalState = "BLOCKED"
)

// InitReport is one `mindrail init` run.
//
// ConfigCreated and KnowledgeDirsCreated record what init actually did rather
// than what it would have done: spec §82 forbids silently overwriting an
// existing configuration, and a report that could not tell "created" from
// "left alone" would make that promise unverifiable.
type InitReport struct {
	Command              string              `json:"command"`
	TerminalState        TerminalState       `json:"terminal_state"`
	Reason               string              `json:"reason,omitempty"`
	ConfigPath           string              `json:"config_path"`
	ConfigCreated        bool                `json:"config_created"`
	KnowledgeDirsCreated []string            `json:"knowledge_dirs_created"`
	MigrationsApplied    []migration.Applied `json:"migrations_applied"`

	// ConfigPresent and KnowledgeDirsPresent report that the scaffold is on
	// disk, whoever put it there. Neither is derivable from the two fields above
	// it: a run that stopped before the scaffold step also creates no config and
	// no directories, and reading that silence as "already present" is how init
	// came to report a configuration file and two knowledge directories that do
	// not exist (finding H13).
	ConfigPresent        bool `json:"config_present"`
	KnowledgeDirsPresent bool `json:"knowledge_dirs_present"`

	// SchemaCurrent reports that the migration ledger was read and found
	// complete. It is not derivable from an empty MigrationsApplied: a run that
	// stopped before a database existed also applies no migration, and reading
	// the empty slice as "nothing to do" is how init came to print "schema
	// already current" about a database it never opened (finding F14).
	SchemaCurrent bool `json:"schema_current"`

	Status     Report `json:"status"`
	DurationMS int64  `json:"duration_ms"`
}
