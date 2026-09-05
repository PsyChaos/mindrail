package app

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"testing"
)

// TestDomainErrorCarriesFiveFields pins tech-stack §72: the five pieces of an
// error are typed fields, so callers branch on them instead of string-matching
// a formatted message.
func TestDomainErrorCarriesFiveFields(t *testing.T) {
	wantFields := []struct {
		name string
		typ  reflect.Type
	}{
		{"Code", reflect.TypeOf(Code(""))},
		{"Kind", reflect.TypeOf(Kind(0))},
		{"Why", reflect.TypeOf("")},
		{"Impact", reflect.TypeOf("")},
		{"NextAction", reflect.TypeOf([]string(nil))},
		{"Metadata", reflect.TypeOf(map[string]string(nil))},
		{"Cause", reflect.TypeOf((*error)(nil)).Elem()},
	}

	typ := reflect.TypeOf(DomainError{})
	for _, wf := range wantFields {
		t.Run(wf.name, func(t *testing.T) {
			field, ok := typ.FieldByName(wf.name)
			if !ok {
				t.Fatalf("DomainError has no field %s", wf.name)
			}
			if field.Type != wf.typ {
				t.Errorf("DomainError.%s is %s, want %s", wf.name, field.Type, wf.typ)
			}
		})
	}

	cause := errors.New("permission denied")
	err := NewError(
		CodeRuntimePathUnwritable,
		KindUnavailable,
		"the runtime directory cannot be written",
		"mindrail cannot create its database",
		"check the permissions on the Git common-dir",
		"re-run mindrail init",
	).WithCause(cause).WithMetadata("path", "/tmp/x")

	if err.Code != CodeRuntimePathUnwritable {
		t.Errorf("Code = %q, want %q", err.Code, CodeRuntimePathUnwritable)
	}
	if err.Kind != KindUnavailable {
		t.Errorf("Kind = %v, want %v", err.Kind, KindUnavailable)
	}
	if err.Why == "" || err.Impact == "" {
		t.Errorf("Why = %q, Impact = %q, want both non-empty", err.Why, err.Impact)
	}
	if len(err.NextAction) != 2 {
		t.Errorf("NextAction = %v, want 2 elements", err.NextAction)
	}
	if got := err.Metadata["path"]; got != "/tmp/x" {
		t.Errorf("Metadata[path] = %q, want %q", got, "/tmp/x")
	}
	if !errors.Is(err, cause) {
		t.Errorf("errors.Is(err, cause) = false, want true")
	}
	if want := "RUNTIME_PATH_UNWRITABLE: the runtime directory cannot be written"; err.Error() != want {
		t.Errorf("Error() = %q, want %q", err.Error(), want)
	}
}

// TestDomainErrorWrappingSurvivesErrorsIsAndAs proves the carrier is still
// reachable after the error has crossed three package boundaries.
func TestDomainErrorWrappingSurvivesErrorsIsAndAs(t *testing.T) {
	cause := errors.New("no such file")
	base := NewError(
		CodeRuntimeDBUnavailable,
		KindUnavailable,
		"the runtime database could not be opened",
		"no command that touches state can run",
		"run mindrail init",
	).WithCause(cause).WithMetadata("db_path", "/tmp/mindrail.db")

	wrapped := fmt.Errorf("bootstrap: %w", fmt.Errorf("storage: %w", fmt.Errorf("open: %w", error(base))))

	var got *DomainError
	if !errors.As(wrapped, &got) {
		t.Fatalf("errors.As through three levels found no *DomainError")
	}
	if got.Code != CodeRuntimeDBUnavailable {
		t.Errorf("Code = %q, want %q", got.Code, CodeRuntimeDBUnavailable)
	}
	if got.Metadata["db_path"] != "/tmp/mindrail.db" {
		t.Errorf("Metadata = %v, want db_path preserved", got.Metadata)
	}
	if !errors.Is(wrapped, cause) {
		t.Errorf("errors.Is(wrapped, cause) = false, want true")
	}

	payload, ok := PayloadOf(wrapped)
	if !ok {
		t.Fatalf("PayloadOf(wrapped) = _, false; want true")
	}
	if payload.Code != CodeRuntimeDBUnavailable {
		t.Errorf("payload.Code = %q, want %q", payload.Code, CodeRuntimeDBUnavailable)
	}

	if _, ok := PayloadOf(errors.New("plain")); ok {
		t.Errorf("PayloadOf(plain error) = _, true; want false")
	}
	if _, ok := PayloadOf(nil); ok {
		t.Errorf("PayloadOf(nil) = _, true; want false")
	}
}

// TestErrorPayloadJSONHasAllFourKeys walks the whole registry so a new code
// cannot ship without the four fields spec §84 promises to every consumer.
func TestErrorPayloadJSONHasAllFourKeys(t *testing.T) {
	for _, code := range RegisteredCodes() {
		t.Run(string(code), func(t *testing.T) {
			err := NewError(code, KindFailed, "why "+string(code), "impact "+string(code), "next "+string(code))

			raw, marshalErr := json.Marshal(err.Payload())
			if marshalErr != nil {
				t.Fatalf("json.Marshal: %v", marshalErr)
			}

			var decoded map[string]json.RawMessage
			if unmarshalErr := json.Unmarshal(raw, &decoded); unmarshalErr != nil {
				t.Fatalf("json.Unmarshal: %v", unmarshalErr)
			}

			for _, key := range []string{"code", "why", "impact", "next_action"} {
				value, present := decoded[key]
				if !present {
					t.Fatalf("payload %s is missing key %q", raw, key)
				}
				switch string(value) {
				case `""`, `[]`, "null":
					t.Errorf("payload key %q is empty (%s)", key, value)
				}
			}

			if _, present := decoded["metadata"]; present {
				t.Errorf("payload %s carries a metadata key with no metadata set", raw)
			}
			if _, present := decoded["cause"]; present {
				t.Errorf("payload %s carries a cause key with no cause set", raw)
			}
		})
	}
}

// TestPayloadCarriesTheCause is the fix for an error surface that set Cause
// everywhere and rendered it nowhere.
//
// Why, impact and next_action are written for the person who has to act, and
// they are deliberately the same sentence for every instance of a code. That is
// what makes them useless for telling two instances apart: "the runtime database
// could not be opened" is the report for a lock held for two milliseconds and
// for a directory nobody can write, and only the underlying message
// distinguishes them.
func TestPayloadCarriesTheCause(t *testing.T) {
	cause := errors.New("database is locked (5) (SQLITE_BUSY)")
	err := NewError(
		CodeRuntimeDBUnavailable,
		KindUnavailable,
		"the runtime database could not be opened",
		"no command that touches state can run",
		"retry the command",
	).WithCause(cause)

	if got := err.Payload().Cause; got != cause.Error() {
		t.Errorf("payload cause = %q, want %q", got, cause.Error())
	}

	raw, marshalErr := json.Marshal(err.Payload())
	if marshalErr != nil {
		t.Fatalf("json.Marshal: %v", marshalErr)
	}
	var decoded map[string]any
	if unmarshalErr := json.Unmarshal(raw, &decoded); unmarshalErr != nil {
		t.Fatalf("json.Unmarshal: %v", unmarshalErr)
	}
	if decoded["cause"] != cause.Error() {
		t.Errorf("JSON cause = %v, want %q\n%s", decoded["cause"], cause.Error(), raw)
	}

	// The four §84 keys are untouched by the addition: a consumer written
	// against the old shape still finds everything it bound to.
	for _, key := range []string{"code", "why", "impact", "next_action"} {
		if _, present := decoded[key]; !present {
			t.Errorf("payload %s lost key %q", raw, key)
		}
	}
}

// TestCauseOfReachesThroughTheWrapChain covers the accessor the CLI and the
// startup logger use. A cause that is only reachable by a type assertion on an
// unwrapped error is a cause no boundary can print.
func TestCauseOfReachesThroughTheWrapChain(t *testing.T) {
	cause := errors.New("unable to open database file (14)")
	base := NewError(CodeRuntimeDBUnavailable, KindUnavailable, "why", "impact", "next").WithCause(cause)
	wrapped := fmt.Errorf("bootstrap: %w", error(base))

	if got := CauseOf(wrapped); got != cause {
		t.Errorf("CauseOf(wrapped) = %v, want %v", got, cause)
	}
	if got := CauseOf(errors.New("plain")); got != nil {
		t.Errorf("CauseOf(plain) = %v, want nil", got)
	}
	if got := CauseOf(nil); got != nil {
		t.Errorf("CauseOf(nil) = %v, want nil", got)
	}

	causeless := NewError(CodeRuntimeDBUnavailable, KindUnavailable, "why", "impact", "next")
	if got := CauseOf(causeless); got != nil {
		t.Errorf("CauseOf(error with no cause) = %v, want nil", got)
	}
}

// TestAdoptCauseNeverOverwritesAnExistingCause pins the one rule that keeps
// re-attachment honest: a carrier that already knows why it failed is never
// told a different story by a caller that knows less.
func TestAdoptCauseNeverOverwritesAnExistingCause(t *testing.T) {
	original := errors.New("the real reason")
	substitute := errors.New("a guess")

	withCause := NewError(CodeMigrationFailed, KindFailed, "why", "impact", "next").WithCause(original)
	if got := CauseOf(AdoptCause(withCause, substitute)); got != original {
		t.Errorf("AdoptCause overwrote an existing cause with %v", got)
	}

	empty := NewError(CodeMigrationFailed, KindFailed, "why", "impact", "next")
	if got := CauseOf(AdoptCause(empty, original)); got != original {
		t.Errorf("AdoptCause(empty) cause = %v, want %v", got, original)
	}

	if got := AdoptCause(nil, original); got != nil {
		t.Errorf("AdoptCause(nil, cause) = %v, want nil", got)
	}
	plain := errors.New("no payload")
	if got := AdoptCause(plain, original); got != plain {
		t.Errorf("AdoptCause(plain) = %v, want the error unchanged", got)
	}
}

// TestRenderErrorBlockLayout is the spec §84 golden: a bare code line, then
// Why / Impact / Next in that order.
func TestRenderErrorBlockLayout(t *testing.T) {
	err := NewError(
		CodeNotAGitRepository,
		KindUsage,
		"the current directory is not inside a Git repository",
		"mindrail cannot resolve a workspace root",
		"cd into a Git repository",
		"run git init",
	)

	const want = "NOT_A_GIT_REPOSITORY\n" +
		"\n" +
		"Why:\n" +
		"the current directory is not inside a Git repository\n" +
		"\n" +
		"Impact:\n" +
		"mindrail cannot resolve a workspace root\n" +
		"\n" +
		"Next:\n" +
		"cd into a Git repository\n" +
		"run git init\n"

	var buf bytes.Buffer
	if renderErr := RenderError(&buf, err); renderErr != nil {
		t.Fatalf("RenderError: %v", renderErr)
	}
	if got := buf.String(); got != want {
		t.Errorf("RenderError wrote:\n%q\nwant:\n%q", got, want)
	}

	t.Run("plain error falls back to its message", func(t *testing.T) {
		var plain bytes.Buffer
		if renderErr := RenderError(&plain, errors.New("disk full")); renderErr != nil {
			t.Fatalf("RenderError: %v", renderErr)
		}
		if got := plain.String(); got != "disk full\n" {
			t.Errorf("RenderError(plain) = %q, want %q", got, "disk full\n")
		}
	})

	t.Run("nil error writes nothing", func(t *testing.T) {
		var empty bytes.Buffer
		if renderErr := RenderError(&empty, nil); renderErr != nil {
			t.Fatalf("RenderError: %v", renderErr)
		}
		if empty.Len() != 0 {
			t.Errorf("RenderError(nil) wrote %q, want nothing", empty.String())
		}
	})
}

// TestRenderErrorShowsTheProducersDetails is finding F17.
//
// The metadata is where the layer that detected the failure puts the facts only
// it knows — the directory git was asked about above all. It reached the JSON
// envelope and stopped there, so a machine could answer "which directory did
// Mindrail probe?" and the person running the binary under -C could not.
//
// The block is byte-exact because two things about it are load-bearing: the
// keys are sorted, so the same failure renders identically twice, and Next stays
// last, so the remedy is still the final thing on the screen.
func TestRenderErrorShowsTheProducersDetails(t *testing.T) {
	err := NewError(
		CodeNotAGitRepository,
		KindUsage,
		"the directory is not inside a Git repository worktree",
		"Mindrail has nothing to attach to here",
		"change into a Git repository and run the command again",
	).WithMetadata("start_dir", "/srv/checkout").
		WithMetadata("check", "git").
		// An empty value is not a detail; printing "probed_path: " would be a
		// blank claim where the reader expects a path.
		WithMetadata("probed_path", "")

	const want = "NOT_A_GIT_REPOSITORY\n" +
		"\n" +
		"Why:\n" +
		"the directory is not inside a Git repository worktree\n" +
		"\n" +
		"Impact:\n" +
		"Mindrail has nothing to attach to here\n" +
		"\n" +
		"Details:\n" +
		"check: git\n" +
		"start_dir: /srv/checkout\n" +
		"\n" +
		"Next:\n" +
		"change into a Git repository and run the command again\n"

	var buf bytes.Buffer
	if renderErr := RenderError(&buf, err); renderErr != nil {
		t.Fatalf("RenderError: %v", renderErr)
	}
	if got := buf.String(); got != want {
		t.Errorf("RenderError wrote:\n%q\nwant:\n%q", got, want)
	}

	// Repeated renders are identical: Go randomises map iteration, and a block
	// that reordered itself between two runs of the same command is one no
	// reader and no golden could rely on.
	for range 20 {
		var again bytes.Buffer
		if renderErr := RenderError(&again, err); renderErr != nil {
			t.Fatalf("RenderError: %v", renderErr)
		}
		if again.String() != want {
			t.Fatalf("RenderError is not deterministic:\n%q", again.String())
		}
	}
}
