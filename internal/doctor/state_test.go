package doctor

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestStateEnumIsExactlyFiveValues pins the spec §84 health vocabulary. The
// count is asserted as well as the members because a sixth state would silently
// widen a contract that status, the CLI and every later MR branch on.
func TestStateEnumIsExactlyFiveValues(t *testing.T) {
	want := []State{StateOK, StateDegraded, StateError, StateUnavailable, StateNotApplicable}

	got := States()
	if len(got) != len(want) {
		t.Fatalf("States() has %d values %v, want exactly %d", len(got), got, len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("States()[%d] = %q, want %q", i, got[i], want[i])
		}
	}

	markers := make(map[string]State, len(got))
	for _, state := range got {
		t.Run(string(state), func(t *testing.T) {
			if !state.Valid() {
				t.Errorf("%q is in States() but Valid() is false", state)
			}
			if string(state) != strings.ToUpper(string(state)) {
				t.Errorf("state %q is not uppercase on the wire", state)
			}

			encoded, err := json.Marshal(state)
			if err != nil {
				t.Fatalf("marshal %q: %v", state, err)
			}
			if want := `"` + string(state) + `"`; string(encoded) != want {
				t.Errorf("json.Marshal(%q) = %s, want %s", state, encoded, want)
			}

			marker := state.Marker()
			if marker == "" {
				t.Errorf("state %q has no human marker", state)
			}
			if other, clash := markers[marker]; clash {
				t.Errorf("state %q shares marker %q with %q", state, marker, other)
			}
			markers[marker] = state
		})
	}

	// The registry backs Valid(); a caller that sorts or rewrites the result
	// must not be able to reach it.
	got[0] = State("MUTATED")
	if States()[0] != StateOK {
		t.Errorf("States() returned its backing array; the enum is mutable")
	}
}

// TestStateUnmarshalRejectsUnknown proves the decoder fails closed. A state
// read from a persisted report or another process must be rejected rather than
// echoed back as a plausible-looking unknown.
func TestStateUnmarshalRejectsUnknown(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    State
		wantErr bool
	}{
		{name: "ok", input: `"OK"`, want: StateOK},
		{name: "degraded", input: `"DEGRADED"`, want: StateDegraded},
		{name: "error", input: `"ERROR"`, want: StateError},
		{name: "unavailable", input: `"UNAVAILABLE"`, want: StateUnavailable},
		{name: "not applicable", input: `"NOT_APPLICABLE"`, want: StateNotApplicable},
		{name: "lowercase", input: `"ok"`, wantErr: true},
		{name: "mixed case", input: `"Ok"`, wantErr: true},
		{name: "spec §83 per-unit vocabulary", input: `"HEALTHY"`, wantErr: true},
		{name: "misconfigured", input: `"MISCONFIGURED"`, wantErr: true},
		{name: "readiness value", input: `"PARTIAL_READY"`, wantErr: true},
		{name: "empty", input: `""`, wantErr: true},
		{name: "number", input: `1`, wantErr: true},
		{name: "null", input: `null`, wantErr: true},
		{name: "object", input: `{"state":"OK"}`, wantErr: true},
		{name: "padded", input: `" OK"`, wantErr: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var got State
			err := json.Unmarshal([]byte(tc.input), &got)

			if tc.wantErr {
				if err == nil {
					t.Fatalf("unmarshal %s succeeded with %q, want an error", tc.input, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unmarshal %s: %v", tc.input, err)
			}
			if got != tc.want {
				t.Errorf("unmarshal %s = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

// TestStateUnmarshalRejectsUnknownInsideAResult guards the field the decoder
// actually protects: a Result arriving over the wire with a fabricated state.
func TestStateUnmarshalRejectsUnknownInsideAResult(t *testing.T) {
	var result Result
	if err := json.Unmarshal([]byte(`{"name":"git","state":"FINE"}`), &result); err == nil {
		t.Fatalf("decoded a Result with state %q, want an error", result.State)
	}
}
