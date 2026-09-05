package cli

import (
	"reflect"
	"strconv"
	"unicode/utf8"

	"github.com/PsyChaos/mindrail/internal/app"
)

// maxWalkDepth bounds the representability walk. The report shapes MR-001
// serialises are a handful of levels deep; the bound exists so a future payload
// that somehow contains a cycle costs a truncated answer rather than the stack.
const maxWalkDepth = 32

// RefuseUnrepresentableJSON reports a document that JSON cannot carry without
// changing it, and returns nil when there is nothing wrong.
//
// A repository path is a sequence of bytes on every Unix, and nothing requires
// those bytes to be valid UTF-8. JSON strings are Unicode, and Go's encoder
// resolves the mismatch silently: every invalid byte is replaced with U+FFFD.
// The result is well-formed JSON stating a path that does not exist on disk —
// `--json` publishing, as a fact, a location a consumer cannot stat, open or
// pass back to git (finding W9).
//
// Of the two honest answers, this is the refusal. A lossless encoding was the
// alternative and it is the worse one here: any escaping scheme that survives
// JSON changes the bytes a consumer reads back, so it fixes the problem only for
// consumers that have been told about the scheme — and silently keeps lying to
// every one that has not. Refusing costs the machine-readable view of one
// unusual repository and never publishes a false path. The human view is
// unaffected: it writes the bytes through untouched, which is why the remedy can
// send the reader there.
//
// Both halves of the document are inspected. The payload of a failure is
// rendered into the same envelope as the report, and a remedy naming a mangled
// path is exactly as unusable as a field holding one.
func refuseUnrepresentableJSON(data any, verdict error) error {
	if offender, found := firstUnrepresentable(data); found {
		return unrepresentableError(offender)
	}
	if payload, ok := app.PayloadOf(verdict); ok {
		if offender, found := firstUnrepresentable(payload); found {
			return unrepresentableError(offender)
		}
	}
	return nil
}

// unrepresentableError describes the refusal.
//
// The offending value is named with strconv.Quote, which escapes every invalid
// byte as \xNN: that is lossless, it is readable, and — the part that matters —
// it is itself valid UTF-8, so the envelope reporting the problem does not
// reproduce it. A diagnosis that had to be mangled to be delivered would be the
// same defect one layer up.
//
// KindUsage puts it at decision D-03's exit 2, alongside PATH_ESCAPES_ROOT: the
// condition is deterministic, it is about where the repository was placed, and
// the reader can correct it. Both remedies can be carried out and both were:
// the human report renders this repository in full, and a directory whose name
// is valid UTF-8 makes `--json` work again.
func unrepresentableError(offender string) error {
	quoted := strconv.Quote(offender)

	return app.NewError(
		app.CodePathNotRepresentable,
		app.KindUsage,
		"the value "+quoted+" in this report is not valid UTF-8, so JSON cannot carry it unchanged",
		"Emitting it would publish a path that does not exist on disk: JSON encoding replaces every "+
			"byte that is not valid UTF-8, so a consumer would read back a location it cannot open.",
		"Run this command without --json; the human report carries these bytes through unchanged.",
		"Or rename the offending path so that every byte of it is valid UTF-8.",
	).WithMetadata("unrepresentable_value", quoted)
}

// firstUnrepresentable walks a value and returns the first string in it that is
// not valid UTF-8.
//
// It walks the Go value rather than the marshalled bytes because by the time
// there are marshalled bytes the evidence is gone: the encoder has already
// replaced the offending bytes with U+FFFD, and a U+FFFD in the output is
// indistinguishable from one a repository legitimately contains.
func firstUnrepresentable(value any) (string, bool) {
	if value == nil {
		return "", false
	}
	return walkForInvalidUTF8(reflect.ValueOf(value), 0)
}

// walkForInvalidUTF8 is the recursion. Only the kinds that can hold a string are
// descended into; everything else is representable by construction.
func walkForInvalidUTF8(v reflect.Value, depth int) (string, bool) {
	if depth > maxWalkDepth || !v.IsValid() {
		return "", false
	}

	switch v.Kind() {
	case reflect.String:
		if s := v.String(); !utf8.ValidString(s) {
			return s, true
		}
	case reflect.Pointer, reflect.Interface:
		if !v.IsNil() {
			return walkForInvalidUTF8(v.Elem(), depth+1)
		}
	case reflect.Slice, reflect.Array:
		// A []byte is not a document string: the encoder base64s it, which is
		// lossless whatever bytes it holds.
		if v.Kind() == reflect.Slice && v.Type().Elem().Kind() == reflect.Uint8 {
			return "", false
		}
		for i := range v.Len() {
			if offender, found := walkForInvalidUTF8(v.Index(i), depth+1); found {
				return offender, true
			}
		}
	case reflect.Map:
		for _, key := range v.MapKeys() {
			// A key is published as a JSON member name and is mangled exactly as
			// a value would be.
			if offender, found := walkForInvalidUTF8(key, depth+1); found {
				return offender, true
			}
			if offender, found := walkForInvalidUTF8(v.MapIndex(key), depth+1); found {
				return offender, true
			}
		}
	case reflect.Struct:
		for i := range v.NumField() {
			// Unexported fields are not serialised, and reading them through
			// reflection is not permitted anyway.
			if !v.Type().Field(i).IsExported() {
				continue
			}
			if offender, found := walkForInvalidUTF8(v.Field(i), depth+1); found {
				return offender, true
			}
		}
	}

	return "", false
}
