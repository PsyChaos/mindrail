// Package record holds the two typed knowledge records — Decision and
// Invariant — their construction, and the supersede transition (spec §53, §54,
// §93, §94).
//
// Two properties of this package are load-bearing rather than incidental.
//
// It depends on nothing else under internal/knowledge (design §2). That is what
// lets internal/knowledge/validate import it while it stays ignorant of the
// pipeline that judges its output, and it is why the constants below are
// declared here instead of read out of internal/knowledge/schema.
//
// It touches no filesystem, and imports nothing that could. Supersede's "writes
// nothing and touches no filesystem" (AC-01.4) is therefore a property of the
// whole package rather than a claim about one function body a later edit could
// quietly falsify; TestRecordPackageImportsNothingThatCanReachTheFilesystem
// asserts the entire non-test import set, so adding "os" fails the suite.
//
// The schema documents under schemas/knowledge are the contract, not these
// structs (decision D-36). Every rule that is spelled in both places — the id
// patterns, the three enumerations, the schema_version constant, the property
// set and which properties are required — is read back out of the shipped
// document by a test and compared, so the Go side cannot drift into disagreeing
// with the JSON about what a record is.
package record

import "time"

// The two values a record's "kind" field may carry (spec §53, §54).
//
// They are untyped string constants because the frozen struct (design §3)
// spells Kind as a plain string. internal/knowledge/loader and
// internal/knowledge/schema each declare their own typed spelling of the same
// two words for their own import-cycle reasons, which makes three copies of one
// pair of strings; TestKindConstantsAgreeWithTheLoaderAndTheSchemaPackage holds
// all three together, and reads the values out of the schema documents' "const"
// so the documents remain the authority.
const (
	KindDecision  = "decision"
	KindInvariant = "invariant"
)

// writeVersion is the schema_version stamped on every record these constructors
// build (kernel-scope §3: 0.1 writes 1 and reads [1]).
//
// It is a local constant rather than a read of schema.WriteVersion because this
// package depends on nothing else under internal/knowledge, and it is
// unexported because nothing outside needs to name it — a caller reads it off
// the record it was given. TestConstructorsStampTheVersionTheRegistryWrites
// keeps the two numbers equal.
const writeVersion = 1

// Status is where a record sits in its supersede lineage. A superseded record
// stays in the repository so the lineage survives (spec §93, §94); it is never
// deleted and never overwritten.
type Status string

const (
	StatusActive     Status = "active"
	StatusSuperseded Status = "superseded"
)

// Severity drives how hard validation pushes back on a change that touches an
// Invariant's scope (spec §51). It exists only on Invariant: decision.v1
// declares no such property, and WithSeverity refuses a Decision for that
// reason rather than dropping the value.
type Severity string

const (
	SeverityCritical Severity = "CRITICAL"
	SeverityHigh     Severity = "HIGH"
	SeverityMedium   Severity = "MEDIUM"
	SeverityLow      Severity = "LOW"
)

// severities is every value invariant.v1 admits, in the document's own order so
// that an error listing them reads the same way twice.
var severities = []Severity{SeverityCritical, SeverityHigh, SeverityMedium, SeverityLow}

// Decision answers "why did we choose this path?" (spec §53). Records are
// immutable: a newer Decision supersedes an older one instead of overwriting
// it, which is what Supersede exists to express.
//
// The field order is the schema document's property order, and the JSON tags
// are its property names. Neither is cosmetic — AC-01.6 round-trips a golden
// document through this struct and compares bytes, so a renamed tag or a
// dropped ",omitempty" fails.
type Decision struct {
	SchemaVersion int       `json:"schema_version"`
	Kind          string    `json:"kind"`
	ID            string    `json:"id"`
	Status        Status    `json:"status"`
	CreatedAt     time.Time `json:"created_at"`
	Title         string    `json:"title"`
	Context       string    `json:"context,omitempty"`
	Decision      string    `json:"decision"`
	Consequences  string    `json:"consequences,omitempty"`
	Supersedes    []string  `json:"supersedes,omitempty"`
	Scope         *Scope    `json:"scope,omitempty"`
	Tags          []string  `json:"tags,omitempty"`
}

// Invariant answers "what must stay true after a change?" (spec §54).
//
// It is not a Decision with different words. invariant.v1 requires "statement",
// "scope" and "severity", carries "rationale" where a Decision carries
// "context" and "consequences", and has no "title" at all. Mirroring the
// document rather than the sibling struct is decision D-36 applied to Go: a
// field this schema does not define would be rejected by its
// additionalProperties:false the moment the record was validated.
//
// Scope is a value, not a pointer, precisely because invariant.v1 lists it as
// required while decision.v1 does not. A *Scope can be nil, and a nil one
// marshals as "null", which is a document that says the scope is absent in the
// one place absence is not allowed. A value cannot be absent, so the Go type
// carries the requirement instead of merely hoping a constructor enforced it.
type Invariant struct {
	SchemaVersion int       `json:"schema_version"`
	Kind          string    `json:"kind"`
	ID            string    `json:"id"`
	Status        Status    `json:"status"`
	CreatedAt     time.Time `json:"created_at"`
	Statement     string    `json:"statement"`
	Rationale     string    `json:"rationale,omitempty"`
	Severity      Severity  `json:"severity"`
	Supersedes    []string  `json:"supersedes,omitempty"`
	Scope         Scope     `json:"scope"`
	Tags          []string  `json:"tags,omitempty"`
}
