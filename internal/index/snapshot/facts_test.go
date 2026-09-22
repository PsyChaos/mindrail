package snapshot

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/PsyChaos/mindrail/internal/index/parser"
)

func TestCompleteFactsValidation(t *testing.T) {
	source := []byte("from .lib import tool as renamed\ndef helper(x): return x\ndef caller(): return helper(1)\n")
	valid := fixtureFacts(t, source)
	cases := map[string]func(*Facts){
		"missing_hash": func(f *Facts) { f.Symbols[0].BodyHash = "" },
		"invalid_hash": func(f *Facts) {
			f.Symbols[0].SignatureHash = "zzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzz"
		},
		"local_key": func(f *Facts) { f.Symbols[0].Name = "renamed_without_rekeying" },
		"container": func(f *Facts) {
			s := &f.Symbols[0]
			s.ContainerLocalKey = "foreign"
			s.LocalKey = parser.LocalKey(s.ContainerLocalKey, s.Kind, s.Name)
		},
		"signature_span":     func(f *Facts) { f.Symbols[0].SignatureRange.StartByte = 0 },
		"body_span":          func(f *Facts) { f.Symbols[0].BodyRange.EndByte = uint(len(source) + 1) },
		"overlapping_body":   func(f *Facts) { f.Symbols[0].BodyRange.StartByte = f.Symbols[0].Range.StartByte },
		"import_module":      func(f *Facts) { f.Imports[0].Module = "" },
		"import_name":        func(f *Facts) { f.Imports[0].Names[0] = "" },
		"import_scope":       func(f *Facts) { f.Imports[0].ImporterLocalKey = "foreign" },
		"import_span":        func(f *Facts) { f.Imports[0].Range.EndByte = uint(len(source) + 1) },
		"reference_target":   func(f *Facts) { f.References[0].TargetLocalKey = "foreign" },
		"reference_scope":    func(f *Facts) { f.References[0].ScopeLocalKey = "foreign" },
		"reference_referrer": func(f *Facts) { f.References[0].ReferrerLocalKey = "foreign" },
		"partial_success":    func(f *Facts) { f.HasSyntaxErrors = true },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			encoded, err := json.Marshal(valid)
			if err != nil {
				t.Fatal(err)
			}
			var bad Facts
			if err := json.Unmarshal(encoded, &bad); err != nil {
				t.Fatal(err)
			}
			mutate(&bad)
			paths := cachePaths(t)
			_, err = New(paths).GetOrCompute(t.Context(), pythonInfo(t), source, func(context.Context, []byte) (Facts, error) { return bad, nil })
			if !errors.Is(err, ErrInvalidFacts) || len(entryFiles(t, paths.CacheDir)) != 0 {
				t.Fatalf("malformed facts accepted: %v", err)
			}
		})
	}
}
