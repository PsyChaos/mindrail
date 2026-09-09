// Package workspace records which repository Mindrail is looking at.
//
// A project is a Git common directory; a workspace is one working tree under
// it. Both are addressed by an opaque id, and the paths are stored only so a
// running process can find its own row again (decision D-26). MR-001 persists
// no repository content paths at all.
package workspace

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/PsyChaos/mindrail/internal/app"
	"github.com/PsyChaos/mindrail/internal/identity"
	"github.com/PsyChaos/mindrail/internal/storage"
)

// TableSchemaVersion is the migration that creates the projects and workspaces
// tables.
//
// A reader needs it to tell two conditions apart that were one condition while
// there was only a single migration: a database with no schema at all, where
// there is no workspaces table to query, and a database one or more migrations
// behind this binary, where the table is there and holds the row. Treating the
// second as the first told every worktree registered by an older binary that it
// was not registered (finding F01).
const TableSchemaVersion = 1

// Project is one Git common directory: a repository and every worktree of it.
type Project struct {
	ID           string    `json:"project_id"` // "PRJ-<26>"
	CommonDir    string    `json:"common_dir"`
	RegisteredAt time.Time `json:"registered_at"`
}

// Workspace is one working tree of a project.
type Workspace struct {
	ID               string    `json:"workspace_id"` // "WS-<26>"
	ProjectID        string    `json:"project_id"`
	RootPath         string    `json:"root_path"`
	GitDir           string    `json:"git_dir"`
	IsLinkedWorktree bool      `json:"is_linked_worktree"`
	RegisteredAt     time.Time `json:"registered_at"`
	LastSeenAt       time.Time `json:"last_seen_at"`
}

// Registration is what the Git adapter discovered about the current directory.
// It is a separate type from Workspace because the caller supplies no ids and
// no timestamps: those are the store's to mint.
type Registration struct {
	CommonDir        string
	WorktreeRoot     string
	GitDir           string
	IsLinkedWorktree bool
}

// Store reads and writes the project and workspace rows.
type Store struct {
	db    *sql.DB
	clock app.Clock
}

// NewStore builds a store over an already-migrated database.
func NewStore(db *sql.DB, clock app.Clock) *Store {
	return &Store{db: db, clock: clock}
}

// ErrNotRegistered means no workspace row exists for the given root. It is a
// state `mindrail status` reports, not necessarily a failure (decision D-01),
// so it stays a bare sentinel and the caller decides what it costs.
var ErrNotRegistered = errors.New("workspace is not registered")

// Register records the worktree and returns its project and workspace rows.
//
// It is idempotent on the worktree root: running `mindrail init` twice finds
// the same workspace and only bumps LastSeenAt. The whole thing runs in one
// transaction so a crash between the project insert and the workspace insert
// cannot leave a project nothing points at.
func (s *Store) Register(ctx context.Context, r Registration) (Project, Workspace, error) {
	if err := validateRegistration(r); err != nil {
		return Project{}, Workspace{}, err
	}

	var (
		project   Project
		workspace Workspace
	)
	err := storage.InTx(ctx, s.db, func(ctx context.Context, tx *sql.Tx) error {
		now := s.clock.Now().UTC()

		var err error
		if project, err = upsertProject(ctx, tx, r.CommonDir, now); err != nil {
			return err
		}
		workspace, err = upsertWorkspace(ctx, tx, project.ID, r, now)
		return err
	})
	if err != nil {
		if _, isDomain := app.PayloadOf(err); isDomain {
			return Project{}, Workspace{}, err
		}
		return Project{}, Workspace{}, s.registrationFailure(ctx, r, err)
	}

	return project, workspace, nil
}

// FindByRoot returns the workspace registered for root, or ErrNotRegistered.
func (s *Store) FindByRoot(ctx context.Context, root string) (Workspace, error) {
	row := s.db.QueryRowContext(ctx, selectWorkspace+` WHERE root_path = ?`, root)

	workspace, err := scanWorkspace(row)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return Workspace{}, fmt.Errorf("%w: %s", ErrNotRegistered, root)
	case err != nil:
		return Workspace{}, fmt.Errorf("look up workspace for %q: %w", root, err)
	}

	return workspace, nil
}

// List returns every registered workspace in registration order. The id is the
// tie breaker because it is time-sortable, so rows written under the same
// injected clock instant still come back in a stable order.
func (s *Store) List(ctx context.Context) ([]Workspace, error) {
	rows, err := s.db.QueryContext(ctx, selectWorkspace+` ORDER BY registered_at, workspace_id`)
	if err != nil {
		return nil, fmt.Errorf("list workspaces: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var workspaces []Workspace
	for rows.Next() {
		workspace, err := scanWorkspace(rows)
		if err != nil {
			return nil, fmt.Errorf("scan workspace row: %w", err)
		}
		workspaces = append(workspaces, workspace)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list workspaces: %w", err)
	}

	return workspaces, nil
}

// Count returns the number of registered workspaces without materialising
// them; status reports the number far more often than it needs the rows.
func (s *Store) Count(ctx context.Context) (int, error) {
	var count int
	if err := s.db.QueryRowContext(ctx, `SELECT count(*) FROM workspaces`).Scan(&count); err != nil {
		return 0, fmt.Errorf("count workspaces: %w", err)
	}
	return count, nil
}

const selectWorkspace = `SELECT workspace_id, project_id, root_path, git_dir,
	is_linked_worktree, registered_at, last_seen_at FROM workspaces`

// rowScanner is satisfied by both *sql.Row and *sql.Rows, so the single-row
// and multi-row paths decode identically.
type rowScanner interface{ Scan(dest ...any) error }

func scanWorkspace(row rowScanner) (Workspace, error) {
	var (
		workspace    Workspace
		linked       int
		registeredAt string
		lastSeenAt   string
	)
	if err := row.Scan(&workspace.ID, &workspace.ProjectID, &workspace.RootPath, &workspace.GitDir,
		&linked, &registeredAt, &lastSeenAt); err != nil {
		return Workspace{}, err
	}

	var err error
	if workspace.RegisteredAt, err = app.ParseTime(registeredAt); err != nil {
		return Workspace{}, fmt.Errorf("workspace %s registered_at: %w", workspace.ID, err)
	}
	if workspace.LastSeenAt, err = app.ParseTime(lastSeenAt); err != nil {
		return Workspace{}, fmt.Errorf("workspace %s last_seen_at: %w", workspace.ID, err)
	}
	workspace.IsLinkedWorktree = linked != 0

	return workspace, nil
}

// upsertProject finds or creates the project for a Git common directory. Two
// worktrees of one repository share the common dir, and therefore the project.
func upsertProject(ctx context.Context, tx *sql.Tx, commonDir string, now time.Time) (Project, error) {
	project := Project{CommonDir: commonDir}

	var registeredAt string
	err := tx.QueryRowContext(ctx,
		`SELECT project_id, registered_at FROM projects WHERE common_dir = ?`, commonDir).
		Scan(&project.ID, &registeredAt)
	switch {
	case err == nil:
		project.RegisteredAt, err = app.ParseTime(registeredAt)
		return project, err
	case !errors.Is(err, sql.ErrNoRows):
		return Project{}, err
	}

	project.ID = identity.NewID(projectIDPrefix)
	project.RegisteredAt = now
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO projects (project_id, common_dir, registered_at) VALUES (?, ?, ?)`,
		project.ID, commonDir, app.FormatTime(now)); err != nil {
		return Project{}, err
	}

	return project, nil
}

// upsertWorkspace finds or creates the workspace for a worktree root.
//
// An existing row keeps its id and its registered_at: those are the identity
// and the birth date, and rewriting either would make "the same worktree" mean
// something different on every run. Only last_seen_at moves.
func upsertWorkspace(ctx context.Context, tx *sql.Tx, projectID string, r Registration, now time.Time) (Workspace, error) {
	existing, err := scanWorkspace(tx.QueryRowContext(ctx, selectWorkspace+` WHERE root_path = ?`, r.WorktreeRoot))
	switch {
	case err == nil:
		if _, err := tx.ExecContext(ctx,
			`UPDATE workspaces SET last_seen_at = ? WHERE workspace_id = ?`,
			app.FormatTime(now), existing.ID); err != nil {
			return Workspace{}, err
		}
		existing.LastSeenAt = now
		return existing, nil
	case !errors.Is(err, sql.ErrNoRows):
		return Workspace{}, err
	}

	workspace := Workspace{
		ID:               identity.NewID(workspaceIDPrefix),
		ProjectID:        projectID,
		RootPath:         r.WorktreeRoot,
		GitDir:           r.GitDir,
		IsLinkedWorktree: r.IsLinkedWorktree,
		RegisteredAt:     now,
		LastSeenAt:       now,
	}
	linked := 0
	if workspace.IsLinkedWorktree {
		linked = 1
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO workspaces (workspace_id, project_id, root_path, git_dir,
			is_linked_worktree, registered_at, last_seen_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		workspace.ID, workspace.ProjectID, workspace.RootPath, workspace.GitDir,
		linked, app.FormatTime(now), app.FormatTime(now)); err != nil {
		return Workspace{}, err
	}

	return workspace, nil
}

// validateRegistration rejects an incomplete discovery result before it
// reaches the database. The columns are NOT NULL but not NOT EMPTY, so an
// empty root would insert cleanly and then never match a lookup.
func validateRegistration(r Registration) error {
	missing := ""
	switch {
	case r.CommonDir == "":
		missing = "common_dir"
	case r.WorktreeRoot == "":
		missing = "worktree_root"
	case r.GitDir == "":
		missing = "git_dir"
	default:
		return nil
	}

	return app.NewError(
		app.CodeWorkspaceRegistrationFailed,
		app.KindFailed,
		"the repository discovery result is incomplete: "+missing+" is empty",
		"Mindrail cannot record which worktree it is operating on.",
		"Run `mindrail doctor` to check Git repository discovery.",
	).WithMetadata("missing_field", missing)
}

// registrationFailure reports a registration that did not reach the database.
//
// The driver's result code decides first. `mindrail init` against a mindrail.db
// whose mode bits refuse writes failed here, at exit 1, with the remedy "Run
// `mindrail doctor` to check the runtime database, then re-run `mindrail init`"
// -- and doctor, which only ever reads, reported that same database healthy and
// exited 0. The tool's own advice was a closed loop over an installation it
// called fine (finding W2). A named condition carries a remedy that can succeed
// and the exit 4 decision D-03 gives an unwritable runtime path; the generic
// remedy below survives only for the failures that really are about the rows
// being written, where doctor has something new to say.
func (s *Store) registrationFailure(ctx context.Context, r Registration, cause error) error {
	if named := storage.WriteFailure(ctx, s.db, "register this worktree", cause); named != nil {
		return named.WithMetadata("worktree_root", r.WorktreeRoot)
	}

	return app.NewError(
		app.CodeWorkspaceRegistrationFailed,
		app.KindFailed,
		"the workspace could not be registered in the runtime database",
		"Mindrail cannot associate later work with this worktree.",
		"Run `mindrail doctor` to check the runtime database, then re-run `mindrail init`.",
	).WithMetadata("worktree_root", r.WorktreeRoot).WithCause(cause)
}
