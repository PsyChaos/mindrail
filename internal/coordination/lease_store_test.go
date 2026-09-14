package coordination_test

import (
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/PsyChaos/mindrail/internal/app"
	"github.com/PsyChaos/mindrail/internal/coordination"
	"github.com/PsyChaos/mindrail/internal/storage"
)

// settableClock is a clock a test moves by hand, so expiry can be crossed
// without sleeping and without the one-minute step stepClock takes on every
// read: a lease judged "active" and then "expired" has to be judged against
// two instants the test chose.
type settableClock struct {
	mu  sync.Mutex
	now time.Time
}

func newSettableClock() *settableClock { return &settableClock{now: baseInstant} }

func (c *settableClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now.UTC()
}

func (c *settableClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

// leaseFixture is a fixture over a settable clock.
func leaseFixture(t *testing.T) (fixture, *settableClock) {
	t.Helper()
	clock := newSettableClock()
	return openFixture(t, filepath.Join(t.TempDir(), "mindrail.db"), clock), clock
}

func mustFile(t *testing.T, raw string) coordination.Target {
	t.Helper()
	target, err := coordination.FileTarget(raw)
	if err != nil {
		t.Fatalf("FileTarget(%q) = %v, want no error", raw, err)
	}
	return target
}

func requireCode(t *testing.T, err error, want app.Code) app.ErrorPayload {
	t.Helper()
	if err == nil {
		t.Fatalf("want %s, got no error", want)
	}
	payload, ok := app.PayloadOf(err)
	if !ok {
		t.Fatalf("want a %s payload, got a bare error: %v", want, err)
	}
	if payload.Code != want {
		t.Fatalf("code = %s, want %s: %s", payload.Code, want, payload.Why)
	}
	return payload
}

// sameRow compares two readings of one row column by column, ignoring Status
// — which follows the clock rather than the row — and comparing released_at
// by value rather than by pointer.
func sameRow(a, b coordination.Lease) bool {
	releasedAlike := (a.ReleasedAt == nil) == (b.ReleasedAt == nil) &&
		(a.ReleasedAt == nil || a.ReleasedAt.Equal(*b.ReleasedAt))
	return a.ID == b.ID && a.ProjectID == b.ProjectID && a.TargetKind == b.TargetKind &&
		a.TargetKey == b.TargetKey && a.Holder == b.Holder &&
		a.AcquiredAt.Equal(b.AcquiredAt) && a.RenewedAt.Equal(b.RenewedAt) &&
		a.ExpiresAt.Equal(b.ExpiresAt) && releasedAlike && a.ReleaseReason == b.ReleaseReason
}

// readLeaseRow reads the row as stored, judged at the fixture clock, so a test
// asserts on what the database holds rather than on what a method returned.
func readLeaseRow(t *testing.T, f fixture, id string) coordination.Lease {
	t.Helper()
	lease, err := f.store.FindLease(t.Context(), id)
	if err != nil {
		t.Fatalf("FindLease(%s) = %v, want no error", id, err)
	}
	return lease
}

// TestFileTargetIsNormalisedAndRefusedByTheRule is AC-03.2 in both directions
// (decision D-77): the keys that are one target, and the ones that name
// nothing inside the repository.
func TestFileTargetIsNormalisedAndRefusedByTheRule(t *testing.T) {
	for raw, want := range map[string]string{
		"src/auth.go":        "src/auth.go",
		"./src/auth.go":      "src/auth.go",
		"src/./auth.go":      "src/auth.go",
		"src//auth.go":       "src/auth.go",
		"src/../lib/x.go":    "lib/x.go",
		`src\auth.go`:        "src/auth.go",
		"new/file/not/yet":   "new/file/not/yet",
		"src/auth.go/":       "src/auth.go",
		"UPPER/Case.go":      "UPPER/Case.go",
		"with space/file.go": "with space/file.go",
	} {
		target, err := coordination.FileTarget(raw)
		if err != nil {
			t.Errorf("FileTarget(%q) = %v, want the key %q", raw, err, want)
			continue
		}
		if target.Kind != coordination.TargetFile || target.Key != want {
			t.Errorf("FileTarget(%q) = %+v, want file %q", raw, target, want)
		}
	}

	for _, raw := range []string{"", "   ", "/etc/passwd", "..", "../x.go", "a/../../x.go", ".", "./", "src/\xff.go"} {
		_, err := coordination.FileTarget(raw)
		payload := requireCode(t, err, app.CodeCommandLineInvalid)
		if app.ExitCode(err) != app.ExitUsage {
			t.Errorf("FileTarget(%q) exits %d, want %d", raw, app.ExitCode(err), app.ExitUsage)
		}
		if !strings.Contains(strings.Join(payload.NextAction, " "), "--file") {
			t.Errorf("FileTarget(%q) remedy %v does not name --file", raw, payload.NextAction)
		}
	}

	if _, err := coordination.ParseTargetKind("task"); err != nil {
		t.Errorf("ParseTargetKind(task) = %v, want the kind", err)
	}
	if _, err := coordination.ParseTargetKind("file"); err != nil {
		t.Errorf("ParseTargetKind(file) = %v, want the kind", err)
	}
	for _, raw := range []string{"symbol", "File", "", "tasks"} {
		if _, err := coordination.ParseTargetKind(raw); err == nil {
			t.Errorf("ParseTargetKind(%q) = nil error, want a refusal: symbol is MR-006's", raw)
		}
	}
}

// TestTheLeaseTTLIsTwentyMinutes is AC-03.3's number, asserted on the row the
// store writes rather than on the constant alone.
func TestTheLeaseTTLIsTwentyMinutes(t *testing.T) {
	if coordination.LeaseTTL != 20*time.Minute {
		t.Fatalf("LeaseTTL = %v, want 20m (spec §59)", coordination.LeaseTTL)
	}
	f, _ := leaseFixture(t)
	session := f.session(t)

	acquired, _, err := f.store.AcquireLease(t.Context(), f.projectID, coordination.NamedSession(session.ID), mustFile(t, "src/a.go"))
	if err != nil {
		t.Fatalf("AcquireLease = %v, want no error", err)
	}
	if got := acquired.Lease.ExpiresAt.Sub(acquired.Lease.AcquiredAt); got != 20*time.Minute {
		t.Errorf("expires_at - acquired_at = %v, want 20m", got)
	}
	if !acquired.Lease.AcquiredAt.Equal(baseInstant) {
		t.Errorf("acquired_at = %v, want the clock's reading %v", acquired.Lease.AcquiredAt, baseInstant.UTC())
	}
}

// TestEveryCellOfTheLeaseTable is AC-04.2: design §6's five situations
// against its three verbs, each cell asserted on the code and, for the
// writes, on the row afterwards. The situations are built the way a
// repository reaches them, not planted.
func TestEveryCellOfTheLeaseTable(t *testing.T) {
	type cell struct {
		name    string
		verb    string // acquire | renew | release
		arrange func(t *testing.T, f fixture, clock *settableClock, me, other coordination.Session) (leaseID string)
		want    app.Code // "" means success
		check   func(t *testing.T, f fixture, before, after coordination.Lease)
	}

	target := func(t *testing.T) coordination.Target { return mustFile(t, "src/auth.go") }
	acquireAs := func(t *testing.T, f fixture, who coordination.Session) coordination.Lease {
		t.Helper()
		got, _, err := f.store.AcquireLease(t.Context(), f.projectID, coordination.NamedSession(who.ID), target(t))
		if err != nil {
			t.Fatalf("arranging: AcquireLease by %s = %v", who.ID, err)
		}
		return got.Lease
	}

	none := func(*testing.T, fixture, *settableClock, coordination.Session, coordination.Session) string {
		return "LSE-00000000000000000000000000"
	}
	mine := func(t *testing.T, f fixture, _ *settableClock, me, _ coordination.Session) string {
		return acquireAs(t, f, me).ID
	}
	theirs := func(t *testing.T, f fixture, _ *settableClock, _, other coordination.Session) string {
		return acquireAs(t, f, other).ID
	}
	expiredTheirs := func(t *testing.T, f fixture, clock *settableClock, _, other coordination.Session) string {
		id := acquireAs(t, f, other).ID
		clock.Advance(coordination.LeaseTTL)
		return id
	}
	expiredMine := func(t *testing.T, f fixture, clock *settableClock, me, _ coordination.Session) string {
		id := acquireAs(t, f, me).ID
		clock.Advance(coordination.LeaseTTL)
		return id
	}
	released := func(t *testing.T, f fixture, _ *settableClock, me, _ coordination.Session) string {
		id := acquireAs(t, f, me).ID
		if _, _, err := f.store.ReleaseLease(t.Context(), id, coordination.NamedSession(me.ID)); err != nil {
			t.Fatalf("arranging: ReleaseLease = %v", err)
		}
		return id
	}

	cells := []cell{
		{"no lease / acquire", "acquire", none, "", nil},
		{"no lease / renew", "renew", none, app.CodeLeaseNotFound, nil},
		{"no lease / release", "release", none, app.CodeLeaseNotFound, nil},

		{"active, mine / acquire", "acquire", mine, "", nil},
		{"active, mine / renew", "renew", mine, "", nil},
		{"active, mine / release", "release", mine, "", nil},

		{"active, another's / acquire", "acquire", theirs, app.CodeLeaseConflict, nil},
		{"active, another's / renew", "renew", theirs, app.CodeLeaseConflict, nil},
		{"active, another's / release", "release", theirs, app.CodeLeaseConflict, nil},

		{"expired, another's / acquire", "acquire", expiredTheirs, "", nil},
		{"expired, mine / renew", "renew", expiredMine, app.CodeLeaseNotHeld, nil},
		{"expired, mine / release", "release", expiredMine, app.CodeLeaseNotHeld, nil},

		{"released / acquire", "acquire", released, "", nil},
		{"released / renew", "renew", released, app.CodeLeaseNotHeld, nil},
		{"released / release", "release", released, app.CodeLeaseNotHeld, nil},
	}

	for _, tc := range cells {
		t.Run(tc.name, func(t *testing.T) {
			f, clock := leaseFixture(t)
			me, other := f.session(t), f.session(t)
			leaseID := tc.arrange(t, f, clock, me, other)

			var before coordination.Lease
			hadRow := false
			if got, err := f.store.FindLease(t.Context(), leaseID); err == nil {
				before, hadRow = got, true
			}
			clock.Advance(time.Minute)

			var (
				err   error
				after coordination.Lease
			)
			switch tc.verb {
			case "acquire":
				var got coordination.Acquisition
				got, _, err = f.store.AcquireLease(t.Context(), f.projectID, coordination.NamedSession(me.ID), target(t))
				after = got.Lease
			case "renew":
				after, _, err = f.store.RenewLease(t.Context(), leaseID, coordination.NamedSession(me.ID))
			case "release":
				after, _, err = f.store.ReleaseLease(t.Context(), leaseID, coordination.NamedSession(me.ID))
			}

			if tc.want == "" {
				if err != nil {
					t.Fatalf("%s = %v, want success", tc.verb, err)
				}
				stored := readLeaseRow(t, f, after.ID)
				if stored.Holder != me.ID {
					t.Errorf("stored holder = %s, want %s", stored.Holder, me.ID)
				}
				switch tc.verb {
				case "release":
					if stored.Status != coordination.LeaseReleased || stored.ReleaseReason != coordination.ReleaseReasonReleased {
						t.Errorf("after release the row is %s (%q), want released (released)", stored.Status, stored.ReleaseReason)
					}
				default:
					if stored.Status != coordination.LeaseActive {
						t.Errorf("after %s the row is %s, want active", tc.verb, stored.Status)
					}
					if !stored.ExpiresAt.Equal(clock.Now().Add(coordination.LeaseTTL)) {
						t.Errorf("after %s expires_at = %v, want now + TTL = %v", tc.verb, stored.ExpiresAt, clock.Now().Add(coordination.LeaseTTL))
					}
				}
				return
			}

			payload := requireCode(t, err, tc.want)
			if payload.Metadata["lease_id"] == "" {
				t.Errorf("%s refusal names no lease_id: %v", tc.verb, payload.Metadata)
			}
			if tc.want == app.CodeLeaseConflict && payload.Metadata["holder"] != other.ID {
				t.Errorf("conflict names holder %q, want %s", payload.Metadata["holder"], other.ID)
			}
			// The row a refusal leaves behind is the row it found.
			if hadRow {
				stored := readLeaseRow(t, f, leaseID)
				if !sameRow(before, stored) {
					t.Errorf("a refused %s changed the row:\n before %+v\n after  %+v", tc.verb, before, stored)
				}
			}
		})
	}
}

// TestAcquiringOverAnExpiredTenureClosesItAndSaysSo is AC-04.3, the recovery
// path (decision D-67): the old row is closed with reason `expired` in the
// same transaction, the result names it, and the table holds one unreleased
// row for the target afterwards.
func TestAcquiringOverAnExpiredTenureClosesItAndSaysSo(t *testing.T) {
	f, clock := leaseFixture(t)
	crashed, next := f.session(t), f.session(t)
	target := mustFile(t, "src/auth.go")

	first, _, err := f.store.AcquireLease(t.Context(), f.projectID, coordination.NamedSession(crashed.ID), target)
	if err != nil {
		t.Fatalf("first AcquireLease = %v, want no error", err)
	}

	// One second short of expiry the holder is still the holder.
	clock.Advance(coordination.LeaseTTL - time.Second)
	if _, _, err := f.store.AcquireLease(t.Context(), f.projectID, coordination.NamedSession(next.ID), target); err == nil {
		t.Fatal("AcquireLease one second before expiry = nil error, want LEASE_CONFLICT")
	} else {
		requireCode(t, err, app.CodeLeaseConflict)
	}

	// At expiry it is not.
	clock.Advance(time.Second)
	took, _, err := f.store.AcquireLease(t.Context(), f.projectID, coordination.NamedSession(next.ID), target)
	if err != nil {
		t.Fatalf("AcquireLease at expiry = %v, want the takeover", err)
	}
	if took.Renewed {
		t.Error("the takeover reports renewed = true; it is a new tenure")
	}
	if took.Superseded == nil || took.Superseded.ID != first.Lease.ID || took.Superseded.Holder != crashed.ID {
		t.Fatalf("the takeover names %+v as superseded, want the crashed holder's tenure %s", took.Superseded, first.Lease.ID)
	}
	if took.Lease.ID == first.Lease.ID {
		t.Error("the takeover reused the expired tenure's id; a takeover is a new row")
	}

	old := readLeaseRow(t, f, first.Lease.ID)
	if old.Status != coordination.LeaseReleased || old.ReleaseReason != coordination.ReleaseReasonExpired || old.ReleasedAt == nil {
		t.Errorf("the superseded row is %s (%q, released_at %v), want released (expired, at the takeover)", old.Status, old.ReleaseReason, old.ReleasedAt)
	}
	if old.ReleasedAt != nil && !old.ReleasedAt.Equal(clock.Now()) {
		t.Errorf("the superseded row was closed at %v, want the takeover's instant %v", old.ReleasedAt, clock.Now())
	}

	var unreleased int
	if err := f.db.QueryRowContext(t.Context(),
		`SELECT count(*) FROM leases WHERE target_kind = ? AND target_key = ? AND released_at IS NULL`,
		string(target.Kind), target.Key).Scan(&unreleased); err != nil {
		t.Fatalf("count = %v, want no error", err)
	}
	if unreleased != 1 {
		t.Errorf("unreleased rows for the target = %d, want exactly 1", unreleased)
	}
}

// TestAnAcquisitionByTheHolderRenewsUnderTheSameID is design §6's second row:
// acquiring what you already hold is a renewal, not a duplicate and not a
// refusal.
func TestAnAcquisitionByTheHolderRenewsUnderTheSameID(t *testing.T) {
	f, clock := leaseFixture(t)
	me := f.session(t)
	target := mustFile(t, "src/auth.go")

	first, _, err := f.store.AcquireLease(t.Context(), f.projectID, coordination.NamedSession(me.ID), target)
	if err != nil {
		t.Fatalf("AcquireLease = %v, want no error", err)
	}
	clock.Advance(5 * time.Minute)
	again, _, err := f.store.AcquireLease(t.Context(), f.projectID, coordination.NamedSession(me.ID), target)
	if err != nil {
		t.Fatalf("second AcquireLease by the holder = %v, want a renewal", err)
	}
	if !again.Renewed || again.Lease.ID != first.Lease.ID || again.Superseded != nil {
		t.Errorf("second acquisition = %+v, want renewed under %s with nothing superseded", again, first.Lease.ID)
	}
	if !again.Lease.ExpiresAt.Equal(clock.Now().Add(coordination.LeaseTTL)) {
		t.Errorf("expires_at = %v, want moved to now + TTL", again.Lease.ExpiresAt)
	}
	if !again.Lease.AcquiredAt.Equal(first.Lease.AcquiredAt) {
		t.Errorf("acquired_at moved to %v on a renewal; want the original %v", again.Lease.AcquiredAt, first.Lease.AcquiredAt)
	}

	var rows int
	if err := f.db.QueryRowContext(t.Context(), `SELECT count(*) FROM leases`).Scan(&rows); err != nil {
		t.Fatal(err)
	}
	if rows != 1 {
		t.Errorf("rows in leases = %d, want 1: a renewal is not a second tenure", rows)
	}
}

// TestListLeasesReportsActiveOnesOldestFirstAndMarksNothing is AC-04.4 and
// decision D-79: expired rows are neither listed nor touched.
func TestListLeasesReportsActiveOnesOldestFirstAndMarksNothing(t *testing.T) {
	f, clock := leaseFixture(t)
	me := f.session(t)

	first, _, err := f.store.AcquireLease(t.Context(), f.projectID, coordination.NamedSession(me.ID), mustFile(t, "a.go"))
	if err != nil {
		t.Fatal(err)
	}
	clock.Advance(time.Minute)
	second, _, err := f.store.AcquireLease(t.Context(), f.projectID, coordination.NamedSession(me.ID), mustFile(t, "b.go"))
	if err != nil {
		t.Fatal(err)
	}
	clock.Advance(time.Minute)
	third, _, err := f.store.AcquireLease(t.Context(), f.projectID, coordination.NamedSession(me.ID), mustFile(t, "c.go"))
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := f.store.ReleaseLease(t.Context(), third.Lease.ID, coordination.NamedSession(me.ID)); err != nil {
		t.Fatal(err)
	}

	// Move past the first lease's expiry and not the second's.
	clock.Advance(coordination.LeaseTTL - 90*time.Second)

	listed, err := f.store.ListLeases(t.Context(), f.projectID)
	if err != nil {
		t.Fatalf("ListLeases = %v, want no error", err)
	}
	if len(listed) != 1 || listed[0].ID != second.Lease.ID || listed[0].Status != coordination.LeaseActive {
		t.Fatalf("ListLeases = %+v, want only the second lease, active", listed)
	}

	// The expired one was judged, not marked.
	var releasedAt *string
	if err := f.db.QueryRowContext(t.Context(), `SELECT released_at FROM leases WHERE lease_id = ?`, first.Lease.ID).Scan(&releasedAt); err != nil {
		t.Fatal(err)
	}
	if releasedAt != nil {
		t.Errorf("ListLeases closed the expired row (released_at = %q); reads write nothing", *releasedAt)
	}
	if got := readLeaseRow(t, f, first.Lease.ID); got.Status != coordination.LeaseExpired {
		t.Errorf("FindLease reports the expired row as %s, want expired", got.Status)
	}

	// Order is acquisition order, before anything expires.
	clock.Advance(-coordination.LeaseTTL)
	listed, err = f.store.ListLeases(t.Context(), f.projectID)
	if err != nil {
		t.Fatal(err)
	}
	if len(listed) != 2 || listed[0].ID != first.Lease.ID || listed[1].ID != second.Lease.ID {
		t.Errorf("ListLeases = %v, want [first second] in acquisition order", listed)
	}

	// Another project's leases are another project's.
	other, err := f.store.ListLeases(t.Context(), "PRJ-nobody")
	if err != nil {
		t.Fatal(err)
	}
	if len(other) != 0 {
		t.Errorf("ListLeases(other project) = %v, want none", other)
	}
}

// TestLeaseRefusalsCarryWhatACallerActsOn is AC-04.5 over the three codes:
// registered, a diagnostic, an impact, a remedy, and the id or target in
// the metadata.
func TestLeaseRefusalsCarryWhatACallerActsOn(t *testing.T) {
	f, clock := leaseFixture(t)
	me, other := f.session(t), f.session(t)
	target := mustFile(t, "src/auth.go")

	held, _, err := f.store.AcquireLease(t.Context(), f.projectID, coordination.NamedSession(other.ID), target)
	if err != nil {
		t.Fatal(err)
	}

	_, _, conflict := f.store.AcquireLease(t.Context(), f.projectID, coordination.NamedSession(me.ID), target)
	_, notFound := f.store.FindLease(t.Context(), "LSE-00000000000000000000000000")
	clock.Advance(coordination.LeaseTTL)
	_, _, notHeld := f.store.RenewLease(t.Context(), held.Lease.ID, coordination.NamedSession(other.ID))

	for _, tc := range []struct {
		name     string
		err      error
		code     app.Code
		sentinel error
		named    map[string]string
	}{
		{"conflict", conflict, app.CodeLeaseConflict, coordination.ErrLeaseConflict,
			map[string]string{"lease_id": held.Lease.ID, "holder": other.ID, "target_key": "src/auth.go", "expires_at": app.FormatTime(held.Lease.ExpiresAt)}},
		{"not found", notFound, app.CodeLeaseNotFound, coordination.ErrLeaseNotFound,
			map[string]string{"lease_id": "LSE-00000000000000000000000000"}},
		{"not held", notHeld, app.CodeLeaseNotHeld, coordination.ErrLeaseNotHeld,
			map[string]string{"lease_id": held.Lease.ID, "status": "expired", "target_key": "src/auth.go"}},
	} {
		payload := requireCode(t, tc.err, tc.code)
		if !app.IsRegistered(payload.Code) {
			t.Errorf("%s: code %s is not registered", tc.name, payload.Code)
		}
		if !errors.Is(tc.err, tc.sentinel) {
			t.Errorf("%s: errors.Is(err, sentinel) = false", tc.name)
		}
		if app.ExitCode(tc.err) != app.ExitFailed {
			t.Errorf("%s: exits %d, want %d", tc.name, app.ExitCode(tc.err), app.ExitFailed)
		}
		if payload.Why == "" || payload.Impact == "" || len(payload.NextAction) == 0 {
			t.Errorf("%s: payload lacks a why, an impact or a remedy: %+v", tc.name, payload)
		}
		for key, want := range tc.named {
			if payload.Metadata[key] != want {
				t.Errorf("%s: metadata.%s = %q, want %q", tc.name, key, payload.Metadata[key], want)
			}
		}
	}

	// The remedy for a file lease that is no longer held is an acquisition
	// naming the key (a task lease's is a move; TASK-04 exercises that arm).
	notHeldPayload, _ := app.PayloadOf(notHeld)
	if !strings.Contains(strings.Join(notHeldPayload.NextAction, " "), "lease acquire --file src/auth.go") {
		t.Errorf("the file remedy does not name the acquisition: %v", notHeldPayload.NextAction)
	}
}

// TestATaskTargetIsNotAcquiredDirectly is decision D-66's refusal arm: the
// task lease has one hook into the lifecycle, and it is not this one.
func TestATaskTargetIsNotAcquiredDirectly(t *testing.T) {
	f, _ := leaseFixture(t)
	me := f.session(t)
	task := f.task(t, me.ID, "a task whose lease comes from moving it")

	_, _, err := f.store.AcquireLease(t.Context(), f.projectID, coordination.NamedSession(me.ID), coordination.TaskTarget(task.ID))
	payload := requireCode(t, err, app.CodeCommandLineInvalid)
	if !strings.Contains(strings.Join(payload.NextAction, " "), "task state") {
		t.Errorf("remedy %v does not send the caller to `task state`", payload.NextAction)
	}
	var rows int
	if err := f.db.QueryRowContext(t.Context(), `SELECT count(*) FROM leases`).Scan(&rows); err != nil {
		t.Fatal(err)
	}
	if rows != 0 {
		t.Errorf("a refused task acquisition wrote %d lease rows", rows)
	}
}

// TestLeaseWritersMintAndRefuseSessionsLikeTheOthers keeps decision D-61 on
// the three new writers: an absent session is minted inside the write and
// reported, an unknown one is refused with nothing written.
func TestLeaseWritersMintAndRefuseSessionsLikeTheOthers(t *testing.T) {
	f, _ := leaseFixture(t)
	target := mustFile(t, "src/auth.go")

	acquired, write, err := f.store.AcquireLease(t.Context(), f.projectID, coordination.MintFor(f.spaceID), target)
	if err != nil {
		t.Fatalf("AcquireLease with a minted session = %v, want no error", err)
	}
	if !write.Minted || write.Session.ID == "" || acquired.Lease.Holder != write.Session.ID {
		t.Errorf("write = %+v, holder = %s; want a minted session holding the lease", write, acquired.Lease.Holder)
	}
	if write.Timing.Held <= 0 {
		t.Errorf("write.Timing = %+v, want the transaction's measurement (decision D-75)", write.Timing)
	}

	var sessions int
	if err := f.db.QueryRowContext(t.Context(), `SELECT count(*) FROM sessions`).Scan(&sessions); err != nil {
		t.Fatal(err)
	}
	for _, attempt := range []func() error{
		func() error {
			_, _, err := f.store.AcquireLease(t.Context(), f.projectID, coordination.NamedSession("SES-nobody"), mustFile(t, "other.go"))
			return err
		},
		func() error {
			_, _, err := f.store.RenewLease(t.Context(), acquired.Lease.ID, coordination.NamedSession("SES-nobody"))
			return err
		},
		func() error {
			_, _, err := f.store.ReleaseLease(t.Context(), acquired.Lease.ID, coordination.NamedSession("SES-nobody"))
			return err
		},
	} {
		requireCode(t, attempt(), app.CodeSessionNotFound)
	}
	var after int
	if err := f.db.QueryRowContext(t.Context(), `SELECT count(*) FROM sessions`).Scan(&after); err != nil {
		t.Fatal(err)
	}
	if after != sessions {
		t.Errorf("refused lease writes left %d session rows behind", after-sessions)
	}
}

// TestSessionOpenIsOneTransactionWithItsWait is the finding TASK-01's Breaker
// carried to this task: OpenSession wrote with a bare ExecContext, so under a
// held lock it waited the whole budget and published no waited_ms where every
// other writer did. It now goes through the same transaction as the others,
// and a refusal at BEGIN carries the wait.
func TestSessionOpenIsOneTransactionWithItsWait(t *testing.T) {
	f, clock := leaseFixture(t)

	// The store under test runs over a second handle with a short budget, so
	// the refusal comes in a tenth of a second; the fixture's handle holds the
	// lock.
	contender, err := storage.Open(t.Context(), storage.Options{Path: f.path, BusyTimeout: 100 * time.Millisecond})
	if err != nil {
		t.Fatalf("storage.Open(contender) = %v, want no error", err)
	}
	t.Cleanup(func() { _ = contender.Close() })
	store := coordination.NewStore(contender.DB, clock)

	holder, err := f.db.BeginTx(t.Context(), nil)
	if err != nil {
		t.Fatalf("BeginTx = %v, want no error", err)
	}
	t.Cleanup(func() { _ = holder.Rollback() })

	_, err = store.OpenSession(t.Context(), f.spaceID, "under a held lock")
	payload := requireCode(t, err, app.CodeBusyRetryable)
	if payload.Metadata["waited_ms"] == "" {
		t.Errorf("session open under a held lock publishes no waited_ms: %v", payload.Metadata)
	}

	_ = holder.Rollback()
	session, err := store.OpenSession(t.Context(), f.spaceID, "after release")
	if err != nil {
		t.Fatalf("OpenSession after release = %v, want no error", err)
	}
	if session.ID == "" {
		t.Error("OpenSession returned no id")
	}
}
