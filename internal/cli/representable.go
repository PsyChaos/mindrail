package cli

import (
	"reflect"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/PsyChaos/mindrail/internal/app"
	"github.com/PsyChaos/mindrail/internal/filesystem"
	"github.com/PsyChaos/mindrail/internal/git"
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
	if found, ok := firstUnrepresentable(data); ok {
		return unrepresentableError(found)
	}
	if payload, ok := app.PayloadOf(verdict); ok {
		if found, ok := firstUnrepresentable(payload); ok {
			// An error object's strings are this repository's own locations and
			// the remedies that name them — every code that embeds a value
			// embeds a path — so the payload half keeps the location form
			// whatever the walk was able to work out about the member.
			found.location = true
			return unrepresentableError(found)
		}
	}
	return nil
}

// offender is one value a document cannot carry, and where it was found.
type offender struct {
	// value is the offending string itself, byte for byte.
	value string
	// member is the JSON member path it sits at, "task.title" rather than
	// "somewhere in this report". Empty when the document is a bare string.
	member string
	// location says the value is a filesystem location, which is what decides
	// whether renaming something on disk is a remedy or a red herring.
	location bool
}

// locationTypes are the two structs that carry filesystem locations into a
// report. A string inside one of them is a path; a string anywhere else came
// from the configuration, from a command line or out of the runtime database,
// and no rename reaches it.
var locationTypes = map[reflect.Type]bool{
	reflect.TypeFor[filesystem.RuntimePaths](): true,
	reflect.TypeFor[git.Repository]():          true,
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
//
// There are two forms because there are two conditions, and for five audits
// there was one message for both. A task title holding a stray byte — written
// by a binary that did not yet refuse it at the boundary — was reported as a
// path that does not exist on disk, with "rename the offending path" as the
// remedy for a value in the `tasks` table that no rename can reach, and with
// nothing naming which value it was (finding F30). The stored form says where
// the value is instead.
func unrepresentableError(found offender) error {
	quoted := strconv.Quote(found.value)

	if found.location {
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

	return app.NewError(
		app.CodePathNotRepresentable,
		app.KindUsage,
		"the value "+quoted+" at "+found.where()+" is not valid UTF-8, so JSON cannot carry it unchanged",
		"Emitting it would publish something other than what this repository holds: JSON encoding "+
			"replaces every byte that is not valid UTF-8, so a consumer would read back a value that "+
			"does not match the one stored.",
		"Run this command without --json; the human report carries these bytes through unchanged.",
		"Or correct the stored value at "+found.where()+". It is not a path, so renaming anything on "+
			"disk does not reach it; a `--json` command cannot write such a value any more.",
	).WithMetadata("unrepresentable_value", quoted).
		WithMetadata("unrepresentable_member", found.where())
}

// where names the member the offending value sits at, for a document that turns
// out to be a bare string and has no member to name.
func (o offender) where() string {
	if o.member == "" {
		return "this report"
	}
	return o.member
}

// firstUnrepresentable walks a value and returns the first string in it that is
// not valid UTF-8.
//
// It walks the Go value rather than the marshalled bytes because by the time
// there are marshalled bytes the evidence is gone: the encoder has already
// replaced the offending bytes with U+FFFD, and a U+FFFD in the output is
// indistinguishable from one a repository legitimately contains.
func firstUnrepresentable(value any) (offender, bool) {
	if value == nil {
		return offender{}, false
	}
	return walkForInvalidUTF8(reflect.ValueOf(value), 0, offender{})
}

// walkForInvalidUTF8 is the recursion. Only the kinds that can hold a string are
// descended into; everything else is representable by construction.
//
// at carries the two things the caller cannot work out from the value alone:
// which member the walk has reached, and whether it is inside one of the structs
// that hold filesystem locations.
func walkForInvalidUTF8(v reflect.Value, depth int, at offender) (offender, bool) {
	if depth > maxWalkDepth || !v.IsValid() {
		return offender{}, false
	}

	switch v.Kind() {
	case reflect.String:
		if s := v.String(); !utf8.ValidString(s) {
			at.value = s
			return at, true
		}
	case reflect.Pointer, reflect.Interface:
		if !v.IsNil() {
			return walkForInvalidUTF8(v.Elem(), depth+1, at)
		}
	case reflect.Slice, reflect.Array:
		// A []byte is not a document string: the encoder base64s it, which is
		// lossless whatever bytes it holds.
		if v.Kind() == reflect.Slice && v.Type().Elem().Kind() == reflect.Uint8 {
			return offender{}, false
		}
		for i := range v.Len() {
			element := at.descend(strconv.Itoa(i))
			if found, ok := walkForInvalidUTF8(v.Index(i), depth+1, element); ok {
				return found, true
			}
		}
	case reflect.Map:
		for _, key := range v.MapKeys() {
			// A key is published as a JSON member name and is mangled exactly as
			// a value would be.
			if found, ok := walkForInvalidUTF8(key, depth+1, at); ok {
				return found, true
			}
			member := at
			if key.Kind() == reflect.String {
				member = at.descend(key.String())
			}
			if found, ok := walkForInvalidUTF8(v.MapIndex(key), depth+1, member); ok {
				return found, true
			}
		}
	case reflect.Struct:
		if locationTypes[v.Type()] {
			at.location = true
		}
		for i := range v.NumField() {
			// Unexported fields are not serialised, and reading them through
			// reflection is not permitted anyway.
			field := v.Type().Field(i)
			if !field.IsExported() {
				continue
			}
			if found, ok := walkForInvalidUTF8(v.Field(i), depth+1, at.descend(memberName(field))); ok {
				return found, true
			}
		}
	}

	return offender{}, false
}

// descend returns the position one member further in.
func (o offender) descend(member string) offender {
	if o.member != "" {
		member = o.member + "." + member
	}
	o.member = member
	return o
}

// memberName is the name a field is published under, which is the json tag
// where there is one and the Go name where there is not.
func memberName(field reflect.StructField) string {
	tag, ok := field.Tag.Lookup("json")
	if !ok {
		return field.Name
	}
	name, _, _ := strings.Cut(tag, ",")
	if name == "" || name == "-" {
		return field.Name
	}
	return name
}
