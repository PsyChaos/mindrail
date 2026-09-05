package storage

import (
	"errors"
	"strings"
	"testing"

	"github.com/PsyChaos/mindrail/internal/app"
)

// TestClassifyPragmaMismatchKeepsTheDriverDiagnosisForTheDriver is the
// over-fire guard for finding F5.
//
// The fix reclassifies exactly one condition: a read-only handle finding the
// file in the wrong journal mode. Everything else the read-back can catch is
// still a driver that ignored its DSN, and still has to say so -- a
// reclassification that swallowed a genuinely broken driver would hide the
// silent foreign-key loss decision D-22 exists to catch.
func TestClassifyPragmaMismatchKeepsTheDriverDiagnosisForTheDriver(t *testing.T) {
	want := ExpectedPragmas(DefaultBusyTimeout)

	notWAL := want
	notWAL.JournalMode = "delete"

	foreignKeysOff := want
	foreignKeysOff.ForeignKeys = 0

	bothWrong := want
	bothWrong.JournalMode = "delete"
	bothWrong.ForeignKeys = 0

	busyWrong := want
	busyWrong.BusyTimeout = 0

	syncWrong := want
	syncWrong.Synchronous = 0

	cases := []struct {
		name        string
		readOnly    bool
		got         Pragmas
		wantNotWAL  bool
		wantCode    app.Code
		wantSentine error
	}{
		{
			name: "read-only handle against a file that is not in WAL", readOnly: true, got: notWAL,
			wantNotWAL: true, wantCode: app.CodeRuntimeDBUnavailable, wantSentine: ErrNotWAL,
		},
		{
			name: "read-write handle that asked for WAL and did not get it", readOnly: false, got: notWAL,
			wantNotWAL: false, wantCode: app.CodeRuntimeDBUnavailable, wantSentine: ErrPragma,
		},
		{
			name: "read-only handle with foreign keys off", readOnly: true, got: foreignKeysOff,
			wantNotWAL: false, wantCode: app.CodeRuntimeDBUnavailable, wantSentine: ErrPragma,
		},
		{
			name: "read-only handle with the journal mode and foreign keys both wrong", readOnly: true, got: bothWrong,
			wantNotWAL: false, wantCode: app.CodeRuntimeDBUnavailable, wantSentine: ErrPragma,
		},
		{
			name: "read-only handle that did not get its busy timeout", readOnly: true, got: busyWrong,
			wantNotWAL: false, wantCode: app.CodeRuntimeDBUnavailable, wantSentine: ErrPragma,
		},
		{
			name: "read-only handle that did not get its synchronous setting", readOnly: true, got: syncWrong,
			wantNotWAL: false, wantCode: app.CodeRuntimeDBUnavailable, wantSentine: ErrPragma,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := classifyPragmaMismatch(
				Options{Path: "/repo/.git/mindrail/mindrail.db", ReadOnly: tc.readOnly, BusyTimeout: DefaultBusyTimeout},
				want, tc.got)

			if !errors.Is(err, tc.wantSentine) {
				t.Fatalf("classifyPragmaMismatch = %v, want it to unwrap to %v", err, tc.wantSentine)
			}

			payload, ok := app.PayloadOf(err)
			if !ok {
				t.Fatalf("classifyPragmaMismatch(%+v) carries no domain payload: %v", tc.got, err)
			}
			if payload.Code != tc.wantCode {
				t.Fatalf("payload.Code = %q, want %q", payload.Code, tc.wantCode)
			}

			reportsDriver := false
			for _, action := range payload.NextAction {
				if strings.Contains(action, "Report this") {
					reportsDriver = true
				}
			}
			if tc.wantNotWAL && reportsDriver {
				t.Errorf("next action %v asks for a bug report for a condition `mindrail init` fixes", payload.NextAction)
			}
			if !tc.wantNotWAL && !reportsDriver {
				t.Errorf("next action %v no longer reports a driver that ignored its DSN", payload.NextAction)
			}
		})
	}
}
