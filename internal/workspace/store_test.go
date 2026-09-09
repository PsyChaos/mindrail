package workspace_test

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/PsyChaos/mindrail/internal/app"
	"github.com/PsyChaos/mindrail/internal/migration"
	"github.com/PsyChaos/mindrail/internal/storage"
	"github.com/PsyChaos/mindrail/internal/workspace"
	"github.com/PsyChaos/mindrail/migrations"
)

// baseInstant sits in a non-UTC zone on purpose: a timestamp that comes back
// as UTC anyway proves the conversion happened rather than that the test host
// was already on UTC.
var baseInstant = time.Date(2026, time.September, 5, 9, 30, 15, 0, time.FixedZone("UTC+3", 3*60*60))

// stepClock advances by a fixed step on every read, so a test can tell "the
// row was rewritten" apart from "the row was left alone" without sleeping.
type stepClock struct {
	mu   sync.Mutex
	next time.Time
	step time.Duration
}

func newStepClock() *stepClock {
	return &stepClock{next: baseInstant, step: time.Hour}
}

func (c *stepClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := c.next
	c.next = c.next.Add(c.step)
	return now.UTC()
}

func newStore(t *testing.T, clock app.Clock) (*workspace.Store, *storage.DB) {
	t.Helper()

	db, err := storage.Open(t.Context(), storage.Options{Path: filepath.Join(t.TempDir(), "mindrail.db")})
	if err != nil {
		t.Fatalf("storage.Open = %v, want no error", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	set, err := migration.Load(migrations.FS)
	if err != nil {
		t.Fatalf("migration.Load = %v, want no error", err)
	}
	if _, err := migration.New(db.DB, set, app.FixedClock{Instant: baseInstant}).Up(t.Context()); err != nil {
		t.Fatalf("migration Up = %v, want no error", err)
	}

	return workspace.NewStore(db.DB, clock), db
}

func registration(commonDir, root string, linked bool) workspace.Registration {
	gitDir := commonDir
	if linked {
		gitDir = filepath.Join(commonDir, "worktrees", filepath.Base(root))
	}
	return workspace.Registration{
		CommonDir:        commonDir,
		WorktreeRoot:     root,
		GitDir:           gitDir,
		IsLinkedWorktree: linked,
	}
}

// TestRegisterIsIdempotent is the acceptance criterion behind running
// `mindrail init` twice: the second run must find the same workspace, not mint
// a second identity for the same directory.
func TestRegisterIsIdempotent(t *testing.T) {
	store, _ := newStore(t, newStepClock())
	reg := registration("/repo/.git", "/repo", false)

	firstProject, firstWorkspace, err := store.Register(t.Context(), reg)
	if err != nil {
		t.Fatalf("first Register = %v, want no error", err)
	}

	secondProject, secondWorkspace, err := store.Register(t.Context(), reg)
	if err != nil {
		t.Fatalf("second Register = %v, want no error", err)
	}

	if firstWorkspace.ID != secondWorkspace.ID {
		t.Errorf("workspace id changed: %q -> %q", firstWorkspace.ID, secondWorkspace.ID)
	}
	if firstProject.ID != secondProject.ID {
		t.Errorf("project id changed: %q -> %q", firstProject.ID, secondProject.ID)
	}
	if !firstWorkspace.RegisteredAt.Equal(secondWorkspace.RegisteredAt) {
		t.Errorf("RegisteredAt was rewritten: %v -> %v", firstWorkspace.RegisteredAt, secondWorkspace.RegisteredAt)
	}
	if !secondWorkspace.LastSeenAt.After(firstWorkspace.LastSeenAt) {
		t.Errorf("LastSeenAt = %v, want later than %v", secondWorkspace.LastSeenAt, firstWorkspace.LastSeenAt)
	}
	if !firstProject.RegisteredAt.Equal(secondProject.RegisteredAt) {
		t.Errorf("project RegisteredAt was rewritten: %v -> %v",
			firstProject.RegisteredAt, secondProject.RegisteredAt)
	}

	count, err := store.Count(t.Context())
	if err != nil {
		t.Fatalf("Count = %v, want no error", err)
	}
	if count != 1 {
		t.Errorf("Count = %d, want 1", count)
	}

	found, err := store.FindByRoot(t.Context(), reg.WorktreeRoot)
	if err != nil {
		t.Fatalf("FindByRoot = %v, want no error", err)
	}
	if found.ID != firstWorkspace.ID {
		t.Errorf("FindByRoot id = %q, want %q", found.ID, firstWorkspace.ID)
	}
	if !found.LastSeenAt.Equal(secondWorkspace.LastSeenAt) {
		t.Errorf("FindByRoot LastSeenAt = %v, want the persisted %v", found.LastSeenAt, secondWorkspace.LastSeenAt)
	}
}

// TestTwoWorktreesRegisterTwoWorkspacesOneProject is the linked-worktree case
// of tech-stack §35: both directories share a Git common dir, so they are one
// project with two working locations, not two unrelated repositories.
func TestTwoWorktreesRegisterTwoWorkspacesOneProject(t *testing.T) {
	store, db := newStore(t, newStepClock())
	const commonDir = "/repo/.git"

	main := registration(commonDir, "/repo", false)
	linked := registration(commonDir, "/worktrees/feature", true)

	mainProject, mainWorkspace, err := store.Register(t.Context(), main)
	if err != nil {
		t.Fatalf("Register(main) = %v, want no error", err)
	}
	linkedProject, linkedWorkspace, err := store.Register(t.Context(), linked)
	if err != nil {
		t.Fatalf("Register(linked) = %v, want no error", err)
	}

	if mainProject.ID != linkedProject.ID {
		t.Errorf("project ids differ: %q vs %q; one common dir is one project", mainProject.ID, linkedProject.ID)
	}
	if mainWorkspace.ID == linkedWorkspace.ID {
		t.Errorf("both worktrees got workspace id %q, want distinct ids", mainWorkspace.ID)
	}
	if mainWorkspace.IsLinkedWorktree {
		t.Error("main worktree reported IsLinkedWorktree = true, want false")
	}
	if !linkedWorkspace.IsLinkedWorktree {
		t.Error("linked worktree reported IsLinkedWorktree = false, want true")
	}
	if linkedWorkspace.GitDir != linked.GitDir {
		t.Errorf("linked GitDir = %q, want %q", linkedWorkspace.GitDir, linked.GitDir)
	}

	count, err := store.Count(t.Context())
	if err != nil {
		t.Fatalf("Count = %v, want no error", err)
	}
	if count != 2 {
		t.Errorf("Count = %d, want 2", count)
	}

	listed, err := store.List(t.Context())
	if err != nil {
		t.Fatalf("List = %v, want no error", err)
	}
	roots := make([]string, 0, len(listed))
	for _, ws := range listed {
		roots = append(roots, ws.RootPath)
		if ws.ProjectID != mainProject.ID {
			t.Errorf("workspace %q project = %q, want %q", ws.ID, ws.ProjectID, mainProject.ID)
		}
	}
	want := []string{main.WorktreeRoot, linked.WorktreeRoot}
	slices.Sort(want)
	got := slices.Clone(roots)
	slices.Sort(got)
	if !slices.Equal(got, want) {
		t.Errorf("List roots = %v, want %v", got, want)
	}

	var projects int
	if err := db.QueryRowContext(t.Context(), `SELECT count(*) FROM projects`).Scan(&projects); err != nil {
		t.Fatalf("count projects = %v, want no error", err)
	}
	if projects != 1 {
		t.Errorf("projects rows = %d, want 1", projects)
	}
}

func TestRegisterStoresUTCTimestamps(t *testing.T) {
	store, db := newStore(t, app.FixedClock{Instant: baseInstant})
	reg := registration("/repo/.git", "/repo", false)

	project, ws, err := store.Register(t.Context(), reg)
	if err != nil {
		t.Fatalf("Register = %v, want no error", err)
	}

	want := app.FormatTime(baseInstant)
	columns := map[string]string{
		`SELECT registered_at FROM projects WHERE project_id = ?`:     project.ID,
		`SELECT registered_at FROM workspaces WHERE workspace_id = ?`: ws.ID,
		`SELECT last_seen_at FROM workspaces WHERE workspace_id = ?`:  ws.ID,
	}
	for query, id := range columns {
		var stored string
		if err := db.QueryRowContext(t.Context(), query, id).Scan(&stored); err != nil {
			t.Fatalf("%s = %v, want no error", query, err)
		}
		if stored != want {
			t.Errorf("%s stored %q, want %q", query, stored, want)
		}
		if !strings.HasSuffix(stored, "Z") {
			t.Errorf("%s stored %q, want a UTC (Z) offset", query, stored)
		}
	}

	times := map[string]time.Time{
		"project.RegisteredAt":   project.RegisteredAt,
		"workspace.RegisteredAt": ws.RegisteredAt,
		"workspace.LastSeenAt":   ws.LastSeenAt,
	}
	for name, value := range times {
		if value.Location() != time.UTC {
			t.Errorf("%s location = %v, want UTC", name, value.Location())
		}
		if !value.Equal(baseInstant) {
			t.Errorf("%s = %v, want %v", name, value, baseInstant.UTC())
		}
	}
}

// TestRegisteredIDsAreOpaqueAndPrefixed covers decision D-26: identity is the
// opaque id, never the path. An id that leaked the root path would make every
// downstream reference machine-specific.
//
// This is the half of D-26 that is about the rows this package writes. The
// format half — the alphabet, the length, the sort order — moved to
// internal/identity with the minter (decision D-57), and asserting it here as
// well would be a second copy that could pass while the real one was red. What
// stays is what only this package can answer: that the id the store put in a row
// is the opaque one, and that it carries this package's prefix.
func TestRegisteredIDsAreOpaqueAndPrefixed(t *testing.T) {
	store, _ := newStore(t, app.FixedClock{Instant: baseInstant})
	project, ws, err := store.Register(t.Context(), registration("/very/unusual/repo/.git", "/very/unusual/repo", false))
	if err != nil {
		t.Fatalf("Register = %v, want no error", err)
	}
	for _, id := range []string{project.ID, ws.ID} {
		for _, leak := range []string{
			"repo", "unusual",
			strconv.FormatInt(baseInstant.Unix(), 10),
			strconv.FormatInt(baseInstant.UnixMilli(), 10),
		} {
			if strings.Contains(strings.ToLower(id), strings.ToLower(leak)) {
				t.Errorf("id %q leaks %q", id, leak)
			}
		}
	}
	if !strings.HasPrefix(project.ID, "PRJ-") {
		t.Errorf("project id = %q, want the PRJ- prefix", project.ID)
	}
	if !strings.HasPrefix(ws.ID, "WS-") {
		t.Errorf("workspace id = %q, want the WS- prefix", ws.ID)
	}
}

func TestFindByRootReportsUnregisteredWorkspace(t *testing.T) {
	store, _ := newStore(t, newStepClock())

	_, err := store.FindByRoot(t.Context(), "/never/registered")
	if err == nil {
		t.Fatal("FindByRoot(unknown) = nil error, want ErrNotRegistered")
	}
	if !errors.Is(err, workspace.ErrNotRegistered) {
		t.Errorf("FindByRoot(unknown) = %v, want errors.Is(err, ErrNotRegistered)", err)
	}
}

func TestEmptyStoreListsNothing(t *testing.T) {
	store, _ := newStore(t, newStepClock())

	listed, err := store.List(t.Context())
	if err != nil {
		t.Fatalf("List = %v, want no error", err)
	}
	if len(listed) != 0 {
		t.Errorf("List = %d workspaces, want 0", len(listed))
	}

	count, err := store.Count(t.Context())
	if err != nil {
		t.Fatalf("Count = %v, want no error", err)
	}
	if count != 0 {
		t.Errorf("Count = %d, want 0", count)
	}
}

func TestRegisterRejectsIncompleteRegistration(t *testing.T) {
	store, _ := newStore(t, newStepClock())

	tests := []struct {
		name string
		reg  workspace.Registration
	}{
		{name: "no common dir", reg: workspace.Registration{WorktreeRoot: "/repo", GitDir: "/repo/.git"}},
		{name: "no worktree root", reg: workspace.Registration{CommonDir: "/repo/.git", GitDir: "/repo/.git"}},
		{name: "no git dir", reg: workspace.Registration{CommonDir: "/repo/.git", WorktreeRoot: "/repo"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, _, err := store.Register(t.Context(), tt.reg); err == nil {
				t.Fatalf("Register(%s) = nil error, want a rejection", tt.name)
			} else if payload, ok := app.PayloadOf(err); !ok || payload.Code != app.CodeWorkspaceRegistrationFailed {
				t.Errorf("payload = %+v (ok=%v), want %q", payload, ok, app.CodeWorkspaceRegistrationFailed)
			}
		})
	}
}

// newStoreAt builds a migrated store over a database at a caller-chosen path, so
// a test can go on to interfere with the file itself.
func newStoreAt(t *testing.T, path string) (*workspace.Store, *storage.DB) {
	t.Helper()

	db, err := storage.Open(t.Context(), storage.Options{Path: path})
	if err != nil {
		t.Fatalf("storage.Open = %v, want no error", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	set, err := migration.Load(migrations.FS)
	if err != nil {
		t.Fatalf("migration.Load = %v, want no error", err)
	}
	if _, err := migration.New(db.DB, set, app.FixedClock{Instant: baseInstant}).Up(t.Context()); err != nil {
		t.Fatalf("migration Up = %v, want no error", err)
	}

	return workspace.NewStore(db.DB, newStepClock()), db
}

// TestRegisterOnAnUnwritableDatabaseGivesARemedyThatCanSucceed is finding W2 at
// the point where a user actually meets it.
//
// `mindrail init` against a mindrail.db whose mode bits refuse writes failed
// here with WORKSPACE_REGISTRATION_FAILED at exit 1, and its only remedy was
// "Run `mindrail doctor` to check the runtime database, then re-run `mindrail
// init`". Doctor opens the database read-only, never attempts a write, and
// reported that same installation `Overall: OK` at exit 0 -- so the remedy sent
// the user to a report that agreed with them and then back to the command that
// had just failed. A closed loop over an installation the tool called healthy.
func TestRegisterOnAnUnwritableDatabaseGivesARemedyThatCanSucceed(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("running as root; mode bits do not refuse writes")
	}

	path := filepath.Join(t.TempDir(), "mindrail.db")
	_, initial := newStoreAt(t, path)
	// Closed before the mode changes, because the condition is about what the
	// *next* process can do. A handle that already has the WAL open keeps writing
	// into it; `mindrail init` starts from a cold open, and that is what fails.
	if err := initial.Close(); err != nil {
		t.Fatalf("Close = %v, want no error", err)
	}
	if err := os.Chmod(path, 0o444); err != nil {
		t.Fatalf("Chmod = %v, want no error", err)
	}
	t.Cleanup(func() { _ = os.Chmod(path, 0o644) })

	store, _ := newStoreAt(t, path)
	_, _, err := store.Register(t.Context(), registration("/repo/.git", "/repo", false))
	if err == nil {
		t.Fatal("Register against a read-only database = nil, want a refusal")
	}
	if !errors.Is(err, storage.ErrReadOnly) {
		t.Fatalf("Register = %v, want errors.Is(err, storage.ErrReadOnly)", err)
	}

	payload, ok := app.PayloadOf(err)
	if !ok {
		t.Fatalf("PayloadOf(%v) = _, false, want a domain payload", err)
	}
	if payload.Code != app.CodeRuntimePathUnwritable {
		t.Errorf("payload.Code = %q, want %q", payload.Code, app.CodeRuntimePathUnwritable)
	}
	if got := app.ExitCode(err); got != app.ExitUnavailable {
		t.Errorf("ExitCode = %d, want %d; decision D-03 rates an unwritable runtime path unavailable",
			got, app.ExitUnavailable)
	}
	if payload.Metadata["worktree_root"] != "/repo" {
		t.Errorf("metadata worktree_root = %q, want %q; the caller's own detail must survive",
			payload.Metadata["worktree_root"], "/repo")
	}

	remedy := strings.Join(payload.NextAction, " ")
	if strings.Contains(remedy, "mindrail doctor") {
		t.Errorf("NextAction = %q, which re-enters the loop: doctor reads this database and calls it healthy",
			payload.NextAction)
	}
	if !strings.Contains(remedy, path) {
		t.Errorf("NextAction = %q, want it to name %q", payload.NextAction, path)
	}

	// Carry the remedy out and re-run the command, which is what the next action
	// tells the user to do. A next action that does not clear the condition is
	// not a remedy, and this one had to be proved rather than asserted.
	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatalf("Chmod = %v, want no error", err)
	}
	retried, _ := newStoreAt(t, path)
	if _, _, err := retried.Register(t.Context(), registration("/repo/.git", "/repo", false)); err != nil {
		t.Fatalf("Register after the remedy = %v, want no error; the remedy has to end the loop", err)
	}
}

// TestRegistrationFailureAlwaysCarriesANextAction is the mutation survivor of
// finding W4: deleting the next_action argument from registrationFailure left
// `go test ./...` green, so nothing in the suite required the one field a user
// reads to know what to do.
//
// The failure is deliberately one the driver cannot classify -- a table that is
// gone, not a database that is locked or read-only -- so it lands on the generic
// diagnosis rather than on a named condition. That is precisely the branch the
// mutation lived in.
func TestRegistrationFailureAlwaysCarriesANextAction(t *testing.T) {
	store, db := newStore(t, newStepClock())
	if _, err := db.ExecContext(t.Context(), `DROP TABLE workspaces`); err != nil {
		t.Fatalf("DROP TABLE workspaces = %v, want no error", err)
	}

	_, _, err := store.Register(t.Context(), registration("/repo/.git", "/repo", false))
	if err == nil {
		t.Fatal("Register against a missing table = nil, want a refusal")
	}
	if errors.Is(err, storage.ErrReadOnly) || errors.Is(err, storage.ErrBusy) {
		t.Fatalf("Register = %v, which blames the database for a table that was dropped", err)
	}

	payload, ok := app.PayloadOf(err)
	if !ok {
		t.Fatalf("PayloadOf(%v) = _, false, want a domain payload", err)
	}
	if payload.Code != app.CodeWorkspaceRegistrationFailed {
		t.Errorf("payload.Code = %q, want %q", payload.Code, app.CodeWorkspaceRegistrationFailed)
	}
	if payload.Impact == "" {
		t.Error("payload.Impact is empty; a refusal owes the reader one")
	}
	if len(payload.NextAction) == 0 {
		t.Fatal("payload.NextAction is empty; there is nothing for the user to do")
	}
	for i, action := range payload.NextAction {
		if strings.TrimSpace(action) == "" {
			t.Errorf("NextAction[%d] is blank; a remedy nobody can read is not one", i)
		}
	}
}
