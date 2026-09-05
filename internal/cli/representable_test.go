package cli

import (
	"strings"
	"testing"

	"github.com/PsyChaos/mindrail/internal/app"
)

// invalidUTF8 is a byte sequence no valid UTF-8 string contains. It stands in
// for the repository path a filesystem is perfectly happy to hold and JSON is
// not.
const invalidUTF8 = "bad\xff name"

// TestRefusalFindsTheOffenderWhereverItIs walks the shapes MR-001 actually
// serialises.
//
// The walk exists because by the time there are marshalled bytes the evidence is
// gone: the encoder has already replaced the offending byte with U+FFFD, and a
// U+FFFD in the output is indistinguishable from one a repository legitimately
// contains. Each case below is a place a path really does travel in these
// reports — a struct field, a map value, a member name, an element of a
// next_action list — so a walk that covered only the first would let the others
// through.
func TestRefusalFindsTheOffenderWhereverItIs(t *testing.T) {
	type nested struct {
		Path    string
		Details map[string]string
		Actions []string
	}

	cases := map[string]any{
		"struct field":     nested{Path: invalidUTF8},
		"map value":        nested{Details: map[string]string{"db_path": invalidUTF8}},
		"map key":          nested{Details: map[string]string{invalidUTF8: "ok"}},
		"slice element":    nested{Actions: []string{"fine", invalidUTF8}},
		"pointer":          &nested{Path: invalidUTF8},
		"interface member": map[string]any{"runtime": nested{Path: invalidUTF8}},
	}

	for name, data := range cases {
		t.Run(name, func(t *testing.T) {
			refusal := refuseUnrepresentableJSON(data, nil)
			if refusal == nil {
				t.Fatalf("a document carrying %q in a %s was accepted, so JSON would publish a path "+
					"that does not exist on disk", invalidUTF8, name)
			}

			payload, ok := app.PayloadOf(refusal)
			if !ok {
				t.Fatalf("the refusal carries no payload: %v", refusal)
			}
			if payload.Code != app.CodePathNotRepresentable {
				t.Errorf("code = %q, want %q", payload.Code, app.CodePathNotRepresentable)
			}
			// The diagnosis must survive the delivery it is complaining about: a
			// message that had to be mangled to be read is the same defect one
			// layer up.
			if strings.ContainsRune(payload.Why, '�') {
				t.Errorf("the refusal's own message was mangled: %q", payload.Why)
			}
			if !strings.Contains(payload.Why, `bad\xff name`) {
				t.Errorf("why = %q does not name the offending value losslessly", payload.Why)
			}
			if len(payload.NextAction) == 0 {
				t.Error("the refusal offers no way forward")
			}
		})
	}
}

// TestRefusalReadsTheErrorPayloadToo covers the other half of the envelope. A
// failure's remedy is rendered into the same document as the report, and a
// next_action naming a mangled path is exactly as unusable as a field holding
// one.
func TestRefusalReadsTheErrorPayloadToo(t *testing.T) {
	verdict := app.NewError(app.CodeRuntimeDBUnavailable, app.KindUnavailable,
		"the runtime database at "+invalidUTF8+" could not be opened",
		"nothing can run", "Move "+invalidUTF8+" aside.")

	if refuseUnrepresentableJSON(nil, verdict) == nil {
		t.Fatal("a failure whose remedy names an unrepresentable path was accepted")
	}
}

// TestRepresentableDocumentsAreNotRefused is the over-fire guard, and it is the
// adjacent condition the refusal must not swallow.
//
// Non-ASCII is not the condition. A repository under a Cyrillic, Japanese or
// accented path is ordinary, valid UTF-8 and JSON carries it exactly; refusing
// it would break `--json` for a large part of the world over a defect that is
// not present. Raw bytes in a []byte are not the condition either: the encoder
// base64s them, which is lossless whatever they hold.
func TestRepresentableDocumentsAreNotRefused(t *testing.T) {
	type nested struct {
		Path    string
		Details map[string]string
		Blob    []byte
	}

	cases := map[string]any{
		"ascii":                nested{Path: "/repo/.git"},
		"cyrillic":             nested{Path: "/проект/.git"},
		"japanese":             nested{Path: "/リポジトリ/.git"},
		"accented":             nested{Path: "/café/.git"},
		"emoji in a map value": nested{Details: map[string]string{"root": "/repo/🚀/.git"}},
		"raw bytes in a blob":  nested{Blob: []byte{0xff, 0xfe}},
		"nothing at all":       nil,
	}

	for name, data := range cases {
		t.Run(name, func(t *testing.T) {
			if refusal := refuseUnrepresentableJSON(data, nil); refusal != nil {
				t.Errorf("a representable document was refused: %v", refusal)
			}
		})
	}
}
