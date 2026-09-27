package index

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

// TestTimingNeverReachesTheWire is REQ-12/D-86: parse, extract and write
// timings are collected for logs and never enter a JSON envelope. Every field
// of Timing is string-tagged out, and an IndexResult marshals without any
// timing key — a field added later without the tag fails this test instead of
// leaking onto the wire in MR-019.
func TestTimingNeverReachesTheWire(t *testing.T) {
	typ := reflect.TypeOf(Timing{})
	for i := range typ.NumField() {
		if tag := typ.Field(i).Tag.Get("json"); tag != "-" {
			t.Errorf("Timing.%s json tag = %q, want %q", typ.Field(i).Name, tag, "-")
		}
	}
	encoded, err := json.Marshal(IndexResult{})
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"timing", "parse", "extract", "write", "stats", "waited", "held"} {
		if strings.Contains(string(encoded), `"`+key+`"`) {
			t.Errorf("IndexResult JSON carries timing key %q: %s", key, encoded)
		}
	}
}
