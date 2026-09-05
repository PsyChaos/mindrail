package app

import (
	"errors"
	"fmt"
	"io"
	"maps"
	"slices"
)

// DomainError is the structured error carrier of tech-stack §72. The five
// pieces a caller needs — what failed, why, what it costs, what to do next and
// the underlying cause — are separate typed fields rather than a formatted
// sentence, so control flow never has to match on message text.
//
// Kind is what maps the failure onto a process exit code; see ExitCode.
type DomainError struct {
	Code       Code
	Kind       Kind
	Why        string
	Impact     string
	NextAction []string
	Metadata   map[string]string
	Cause      error
}

// NewError builds a domain error. Why, impact and at least one next action are
// positional because spec §84 promises all four to every consumer: an error
// with no stated impact or remedy is the failure mode this type exists to
// prevent.
func NewError(code Code, kind Kind, why, impact string, next ...string) *DomainError {
	return &DomainError{
		Code:       code,
		Kind:       kind,
		Why:        why,
		Impact:     impact,
		NextAction: slices.Clone(next),
	}
}

func (e *DomainError) Error() string {
	if e.Why == "" {
		return string(e.Code)
	}
	return string(e.Code) + ": " + e.Why
}

func (e *DomainError) Unwrap() error { return e.Cause }

// WithCause attaches the underlying failure so errors.Is can still reach a
// sentinel from a lower layer. It decorates the receiver in place and returns
// it, which keeps `return NewError(...).WithCause(err)` and the two-statement
// form equivalent; only ever call it on an error you just constructed.
func (e *DomainError) WithCause(err error) *DomainError {
	e.Cause = err
	return e
}

// WithMetadata records a machine-readable detail — a path, a version, an id.
// Values land verbatim in the JSON payload, so nothing secret belongs here
// (spec §19).
func (e *DomainError) WithMetadata(key, value string) *DomainError {
	if e.Metadata == nil {
		e.Metadata = make(map[string]string, 1)
	}
	e.Metadata[key] = value
	return e
}

// Payload projects the carrier onto the wire shape. The result owns its slice
// and map, so serializing an error cannot expose the live carrier to mutation.
func (e *DomainError) Payload() ErrorPayload {
	next := slices.Clone(e.NextAction)
	if next == nil {
		// next_action is always an array on the wire; a null would force every
		// consumer to special-case it.
		next = []string{}
	}

	cause := ""
	if e.Cause != nil {
		cause = e.Cause.Error()
	}

	return ErrorPayload{
		Code:       e.Code,
		Why:        e.Why,
		Impact:     e.Impact,
		NextAction: next,
		Cause:      cause,
		Metadata:   maps.Clone(e.Metadata),
	}
}

// ErrorPayload is the JSON projection of a DomainError. The four keys are spec
// §84's contract; next_action is a list because §72 makes remedies plural and
// §84's own example prints two of them (decision D-12).
//
// Cause is a fifth, optional key and a deliberate widening of design §2.1. Why,
// impact and next_action are written for the person who has to act; the cause is
// the sentence the failing library produced, and without it on the wire a
// transient `SQLITE_BUSY` is indistinguishable from a permanently unwritable
// directory — both arrive as "the runtime database could not be opened" with a
// remedy that does not apply. It is omitted when empty, so an error carrying no
// cause serializes exactly as it did before.
type ErrorPayload struct {
	Code       Code              `json:"code"`
	Why        string            `json:"why"`
	Impact     string            `json:"impact"`
	NextAction []string          `json:"next_action"`
	Cause      string            `json:"cause,omitempty"`
	Metadata   map[string]string `json:"metadata,omitempty"`
}

// PayloadOf recovers the payload from anywhere in the wrap chain. Command code
// uses it instead of a type assertion so an error that crossed three package
// boundaries still renders with its code intact.
func PayloadOf(err error) (ErrorPayload, bool) {
	var domain *DomainError
	if !errors.As(err, &domain) {
		return ErrorPayload{}, false
	}
	return domain.Payload(), true
}

// CauseOf returns the underlying failure the first DomainError in err's chain
// carries, or nil when there is none.
//
// A caller wants this when the domain sentence is not enough to act: "the
// runtime database could not be opened" is the same sentence for a lock held for
// two milliseconds and for a directory nobody can write, and only the cause tells
// them apart.
func CauseOf(err error) error {
	var domain *DomainError
	if !errors.As(err, &domain) {
		return nil
	}
	return domain.Cause
}

// AdoptCause attaches source as the cause of the DomainError in err, but only
// when err carries none of its own.
//
// It exists because a verdict is frequently re-derived rather than propagated: a
// health check reports a reading, and the error built from that reading inherits
// the code, the remedy and the metadata but not the library message underneath.
// Re-attaching that message is what keeps the answer to "but why?" reachable.
//
// It mutates the carrier in place, so only call it on an error the caller owns —
// one it just constructed, or one a constructor just handed it.
func AdoptCause(err, source error) error {
	if err == nil || source == nil {
		return err
	}

	var domain *DomainError
	if !errors.As(err, &domain) || domain.Cause != nil {
		return err
	}
	domain.Cause = source
	return err
}

// RenderError writes the human error block of spec §84: a bare code line, then
// Why, Impact, Details and Next in that order. An error with no domain payload
// degrades to its message rather than printing an empty block.
//
// Details is the metadata the layer that detected the failure attached, and it
// is here because the human surface was the only one that dropped it. `--json`
// has carried `start_dir` since the last pass, so a machine could answer "which
// directory did Mindrail actually probe?" and a person could not — and the
// person is the one who runs the binary under `-C` or out of a Git hook, where
// the shell prompt is not the answer (acceptance criterion 3, finding F17).
//
// It sits before Next so that the remedy stays the last thing on the screen.
func RenderError(w io.Writer, err error) error {
	if err == nil {
		return nil
	}

	payload, ok := PayloadOf(err)
	if !ok {
		_, writeErr := fmt.Fprintln(w, err.Error())
		return writeErr
	}

	if _, writeErr := fmt.Fprintf(w, "%s\n", payload.Code); writeErr != nil {
		return writeErr
	}
	if writeErr := renderSection(w, "Why", payload.Why); writeErr != nil {
		return writeErr
	}
	if writeErr := renderSection(w, "Impact", payload.Impact); writeErr != nil {
		return writeErr
	}
	if writeErr := renderSection(w, "Details", detailLines(payload.Metadata)...); writeErr != nil {
		return writeErr
	}
	return renderSection(w, "Next", payload.NextAction...)
}

// detailLines renders the metadata as sorted `key: value` lines. Sorting is
// what makes the block reproducible: Go map iteration is randomised, and a
// human block that reordered itself between two runs of the same command is one
// no golden test and no reader can rely on.
func detailLines(metadata map[string]string) []string {
	lines := make([]string, 0, len(metadata))
	for _, key := range slices.Sorted(maps.Keys(metadata)) {
		if metadata[key] == "" {
			continue
		}
		lines = append(lines, key+": "+metadata[key])
	}
	return lines
}

// renderSection writes one labelled block, one line per value, preceded by a
// blank line. Sections with nothing to say are skipped instead of printing a
// label over emptiness.
func renderSection(w io.Writer, label string, lines ...string) error {
	present := make([]string, 0, len(lines))
	for _, line := range lines {
		if line != "" {
			present = append(present, line)
		}
	}
	if len(present) == 0 {
		return nil
	}

	if _, err := fmt.Fprintf(w, "\n%s:\n", label); err != nil {
		return err
	}
	for _, line := range present {
		if _, err := fmt.Fprintln(w, line); err != nil {
			return err
		}
	}
	return nil
}
