package coordination

import (
	"fmt"
	"path"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/PsyChaos/mindrail/internal/app"
)

// LeaseTTL is how long a lease binds its holder after the last acquisition or
// renewal: spec §59's default, and the only place the number appears (decision
// D-69). Nothing reads it from configuration and no flag overrides it; a holder
// that needs longer renews, and a test that needs shorter moves the clock.
const LeaseTTL = 20 * time.Minute

// leaseIDPrefix is part of the persisted value, like the other three prefixes,
// so a lease id read out of a transcript says what it names.
const leaseIDPrefix = "LSE"

// TargetKind is what a lease is over. MR-004 has two; `symbol` arrives with
// MR-006, when there is a symbol_uid to name (decision D-64).
type TargetKind string

const (
	TargetTask TargetKind = "task"
	TargetFile TargetKind = "file"
)

// targetKinds is every kind, in the order the design lists them.
var targetKinds = []TargetKind{TargetTask, TargetFile}

// ParseTargetKind reads a kind off the wire and refuses anything that is not
// one. The error is plain because whether a bad value is a usage mistake is a
// question about the caller, as it is for ParseState.
func ParseTargetKind(raw string) (TargetKind, error) {
	for _, kind := range targetKinds {
		if string(kind) == raw {
			return kind, nil
		}
	}
	return "", fmt.Errorf("%q is not a lease target kind; the kinds are task and file", raw)
}

// Target is the pair a lease is over: a kind, and the key that names one thing
// of that kind — a task id, or a repository-relative path.
type Target struct {
	Kind TargetKind `json:"kind"`
	Key  string     `json:"key"`
}

// String renders a target the way a message names it: `task TSK-…`,
// `file src/auth.go`.
func (t Target) String() string { return string(t.Kind) + " " + t.Key }

// TaskTarget names a task.
func TaskTarget(taskID string) Target { return Target{Kind: TargetTask, Key: taskID} }

// FileTarget names a file by its repository-relative path, cleaned so that
// `a/./b` and `a/b` are one target (decision D-77).
//
// The file need not exist: a lease on a path is a statement about who is
// writing there. What is refused is a key that could name something outside
// the repository or nothing inside it — an absolute path, a path that climbs
// above the root, the root itself, an empty string — and one that is not valid
// UTF-8, which JSON could not carry unchanged. The refusal is a usage error, as
// OpenTask's empty title is: this package is the surface MR-015's tools call
// without a command line in front of it, so the rule lives here and the CLI
// meets it by calling this.
func FileTarget(raw string) (Target, error) {
	switch {
	case strings.TrimSpace(raw) == "":
		return Target{}, fileTargetInvalid(raw, "is empty")
	case !utf8.ValidString(raw):
		return Target{}, fileTargetInvalid(raw, "is not valid UTF-8")
	case strings.TrimSpace(raw) != raw:
		// Two keys that differ only by a trailing space would be two targets
		// on one file — two agents each holding "the" lease (TASK-03's
		// Breaker). A name that really begins or ends with whitespace is not
		// a name this rule can tell from a stray one, and is refused.
		return Target{}, fileTargetInvalid(raw, "begins or ends with whitespace")
	case strings.ContainsFunc(raw, func(r rune) bool { return r < ' ' || r == 0x7f }):
		return Target{}, fileTargetInvalid(raw, "contains a control character")
	case len(raw) >= 2 && raw[1] == ':' && isASCIILetter(raw[0]):
		// A drive prefix is absolute where it comes from, and names nothing
		// relative to a repository root anywhere.
		return Target{}, fileTargetInvalid(raw, "is absolute; a lease names a path relative to the repository root")
	}

	// Backslashes are read as separators and the path is cleaned; nothing
	// else is rewritten — a space inside a name is the name's.
	key := path.Clean(strings.ReplaceAll(raw, `\`, "/"))
	switch {
	case strings.HasPrefix(key, "/"):
		return Target{}, fileTargetInvalid(raw, "is absolute; a lease names a path relative to the repository root")
	case key == "..", strings.HasPrefix(key, "../"):
		return Target{}, fileTargetInvalid(raw, "climbs above the repository root")
	case key == ".":
		return Target{}, fileTargetInvalid(raw, "names the repository root rather than a file in it")
	}
	return Target{Kind: TargetFile, Key: key}, nil
}

func isASCIILetter(b byte) bool { return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') }

// ShellArgument renders a target key for a remedy that is a command line to
// run: quoted when the shell would otherwise split it or read it as a flag,
// bare when it is already safe. `--file=<key>` in the remedy keeps a key that
// begins with a dash from being read as an option (TASK-03's Breaker: the
// remedy for `with space/file.go` leased `with`).
func ShellArgument(key string) string {
	safe := true
	for _, r := range key {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '.', r == '/', r == '_', r == '-':
		default:
			safe = false
		}
	}
	if safe && !strings.HasPrefix(key, "-") {
		return key
	}
	return "'" + strings.ReplaceAll(key, "'", `'\''`) + "'"
}

func fileTargetInvalid(raw, what string) error {
	return app.NewError(
		app.CodeCommandLineInvalid,
		app.KindUsage,
		fmt.Sprintf("the file path %q %s", raw, what),
		"Nothing was read and nothing was written.",
		"Pass --file as a path relative to the repository root, such as src/auth/session.go.",
	).WithMetadata("target_key", raw)
}

// LeaseStatus is what a lease row is at the moment it was read, judged against
// the store's clock (decision D-79): held, run out, or given up.
type LeaseStatus string

const (
	LeaseActive   LeaseStatus = "active"
	LeaseExpired  LeaseStatus = "expired"
	LeaseReleased LeaseStatus = "released"
)

// The reasons a tenure ends, stored in release_reason. They are the lease
// history spec §59 keeps: how each tenure closed, on the row that was it.
const (
	ReleaseReasonReleased = "released" // the holder gave it up, or a task went back to OPEN
	ReleaseReasonHandoff  = "handoff"  // the holder's handoff checkpoint
	ReleaseReasonFinished = "finished" // the task reached COMPLETED or ABANDONED
	ReleaseReasonExpired  = "expired"  // found run out by the next acquirer, and closed by it
)

// Lease is one tenure: one session holding one target from an acquisition to
// a release (decision D-64). A row is never turned into a different tenure —
// a renewal moves RenewedAt and ExpiresAt, a release or an expiry closes it,
// and a takeover after expiry closes the old row and opens a new one — so the
// table is the lease history, one row per tenure.
//
// Status is computed when the row is read, not stored: an expired row stays
// unreleased until an acquirer closes it, and a reader must still say what it
// is (decision D-65).
type Lease struct {
	ID            string      `json:"lease_id"`
	ProjectID     string      `json:"project_id"`
	TargetKind    TargetKind  `json:"target_kind"`
	TargetKey     string      `json:"target_key"`
	Holder        string      `json:"holder"`
	AcquiredAt    time.Time   `json:"acquired_at"`
	RenewedAt     time.Time   `json:"renewed_at"`
	ExpiresAt     time.Time   `json:"expires_at"`
	ReleasedAt    *time.Time  `json:"released_at"`
	ReleaseReason string      `json:"release_reason,omitempty"`
	Status        LeaseStatus `json:"status"`
}

// Target is the pair this lease is over.
func (l Lease) Target() Target { return Target{Kind: l.TargetKind, Key: l.TargetKey} }

// statusAt judges a row against an instant: released rows are released
// whatever the clock says, and an unreleased one is active until its expiry
// has passed. The comparison is in Go; the column is TEXT and not ordered
// (decision D-65).
func (l Lease) statusAt(now time.Time) LeaseStatus {
	switch {
	case l.ReleasedAt != nil:
		return LeaseReleased
	case !now.Before(l.ExpiresAt):
		return LeaseExpired
	default:
		return LeaseActive
	}
}

// Acquisition is what AcquireLease reports: the lease now held; whether the
// caller already held it and the call renewed it under the same id; the
// expired tenure this acquisition closed, when it took one over (decision
// D-67 — a takeover is reported, never silent); and, for a task target, the
// task as the claim left it — its claimant and its revision, which the claim
// raised. The task is carried because a claimant that read the task before
// claiming it holds a stale revision afterwards, and its own next move under
// D-72 would be refused for it (TASK-04's Breaker). Nil for a file target.
type Acquisition struct {
	Lease      Lease  `json:"lease"`
	Renewed    bool   `json:"renewed"`
	Superseded *Lease `json:"superseded"`
	Task       *Task  `json:"task,omitempty"`
}
