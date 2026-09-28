package changes

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"time"

	"github.com/PsyChaos/mindrail/internal/app"
	"github.com/PsyChaos/mindrail/internal/coordination"
)

// Attribution outcomes. One candidate attributes; zero is unregistered; two
// or more is ambiguous (decision D-133). The outcome names what happened —
// the finding it carries names what to do.
const (
	AttributedOutcome   = "attributed"
	AmbiguousOutcome    = "ambiguous"
	UnregisteredOutcome = "unregistered"
)

// SymbolAttribution is one changed symbol's ownership answer. AttributedTo
// names the owner's Change when present, otherwise its uniquely owning Task;
// ambiguous and unregistered carry exactly one blocking finding (decision D-138).
type SymbolAttribution struct {
	Key          string
	UID          string
	Via          string
	File         string
	Outcome      string
	AttributedTo string
	Candidates   []string
	Finding      *Finding
}

// TaskAttribution is the whole ownership answer for one task: per-symbol
// outcomes plus per-file drift findings. Evaluation is read-only and pure —
// the same rows always give the same answer, so CI verify re-runs it
// instead of trusting it (decision D-136).
type TaskAttribution struct {
	TaskID        string
	ChangeID      string
	BaselineScope []string
	Symbols       []SymbolAttribution
	Drift         []Finding
}

// ListTaskChanges returns every task-bound open Change. In 0.1 every changes
// row is open by construction (decision D-140); NULL-task rows belong to no
// task, hold no baseline, and can never candidate — so they are excluded
// here and evaluated through the unregistered track instead.
func (s *Store) ListTaskChanges(ctx context.Context) ([]Change, error) {
	if err := s.requireSchema(ctx); err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT change_id, task_id, operation_id, updated_at
		FROM changes WHERE task_id IS NOT NULL ORDER BY change_id`)
	if err != nil {
		return nil, corruptState(err)
	}
	defer rows.Close()
	var out []Change
	for rows.Next() {
		var change Change
		var operation sql.NullString
		if err := rows.Scan(&change.ID, &change.TaskID, &operation, &change.UpdatedAt); err != nil {
			return nil, corruptState(err)
		}
		change.OperationID = operation.String
		out = append(out, change)
	}
	if err := rows.Err(); err != nil {
		return nil, corruptState(err)
	}
	return out, nil
}

// ReadAllBaselines returns every task's declared scope as sorted paths.
// Scopes are compared by exact path equality in Go — file paths are
// absolute, so no LIKE is needed (the D-84 lesson).
func (s *Store) ReadAllBaselines(ctx context.Context) (map[string][]string, error) {
	if err := s.requireSchema(ctx); err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT task_id, path FROM change_baselines
		ORDER BY task_id, path`)
	if err != nil {
		return nil, corruptState(err)
	}
	defer rows.Close()
	scopes := map[string][]string{}
	for rows.Next() {
		var taskID, path string
		if err := rows.Scan(&taskID, &path); err != nil {
			return nil, corruptState(err)
		}
		scopes[taskID] = append(scopes[taskID], path)
	}
	if err := rows.Err(); err != nil {
		return nil, corruptState(err)
	}
	return scopes, nil
}

// TaskOwnership carries the lifecycle facts used to resolve a durable task
// scope. UpdatedAt orders completed owners without changing the schema.
type TaskOwnership struct {
	State     string
	UpdatedAt time.Time
}

// ReadTaskOwnership returns the lifecycle facts beside each durable task
// scope. Commit attribution prefers current owners; among completed owners the
// uniquely most recently updated task remains the owner of work committed
// after completion.
func (s *Store) ReadTaskOwnership(ctx context.Context) (map[string]TaskOwnership, error) {
	if err := s.requireSchema(ctx); err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT task_id, state, updated_at FROM tasks ORDER BY task_id`)
	if err != nil {
		return nil, corruptState(err)
	}
	defer rows.Close()
	ownership := map[string]TaskOwnership{}
	for rows.Next() {
		var taskID, state, stamp string
		if err := rows.Scan(&taskID, &state, &stamp); err != nil {
			return nil, corruptState(err)
		}
		if !coordination.State(state).Valid() {
			return nil, corruptState(fmt.Errorf("task %s has invalid state %q", taskID, state))
		}
		updatedAt, err := app.ParseTime(stamp)
		if err != nil {
			return nil, corruptState(err)
		}
		ownership[taskID] = TaskOwnership{State: state, UpdatedAt: updatedAt}
	}
	if err := rows.Err(); err != nil {
		return nil, corruptState(err)
	}
	return ownership, nil
}

// PreferredTaskOwners resolves exact file ownership. Non-terminal owners take
// precedence over historical owners so a completed overlapping task does not
// compete with current work. When no current owner exists, the uniquely most
// recently updated completed owner remains valid for the normal
// complete-then-commit workflow. A latest timestamp tie returns every tied
// owner so downstream attribution fails closed as ambiguous.
func PreferredTaskOwners(scopes map[string][]string, ownership map[string]TaskOwnership, file string) []string {
	var current, historical []string
	var latest time.Time
	missing := false
	for taskID, scope := range scopes {
		for _, candidate := range scope {
			if candidate != file {
				continue
			}
			facts, ok := ownership[taskID]
			if !ok {
				// A baseline without a readable task owner is corrupt durable
				// state. Do not let the missing row collapse several owners to
				// one and accidentally authorize the file.
				missing = true
				break
			}
			if facts.State == "COMPLETED" {
				switch {
				case facts.UpdatedAt.After(latest):
					latest = facts.UpdatedAt
					historical = []string{taskID}
				case facts.UpdatedAt.Equal(latest):
					historical = append(historical, taskID)
				}
			} else if facts.State != "ABANDONED" {
				current = append(current, taskID)
			}
			break
		}
	}
	if missing {
		return nil
	}
	if len(current) > 0 {
		sort.Strings(current)
		return current
	}
	sort.Strings(historical)
	return historical
}

// AttributeTask answers which Change owns each of one task's changed
// symbols, and which of its changed files drift outside its declared scope.
// A task with no open Change has no discovery rows to judge: the answer is
// empty, never an error — undiscovered work is outside every verdict
// (decision D-145).
func (s *Service) AttributeTask(ctx context.Context, taskID string) (TaskAttribution, error) {
	if taskID == "" {
		return TaskAttribution{}, invalidInput("attribution needs a task")
	}
	answer := TaskAttribution{TaskID: taskID}
	baseline, err := s.store.ReadBaseline(ctx, taskID)
	if err != nil {
		return TaskAttribution{}, err
	}
	for path := range baseline {
		answer.BaselineScope = append(answer.BaselineScope, path)
	}
	sort.Strings(answer.BaselineScope)

	change, ok, err := s.openTaskChange(ctx, taskID)
	if err != nil {
		return TaskAttribution{}, err
	}
	if !ok {
		return answer, nil
	}
	answer.ChangeID = change.ID

	scopes, err := s.store.ReadAllBaselines(ctx)
	if err != nil {
		return TaskAttribution{}, err
	}
	all, err := s.store.activeTaskChanges(ctx, taskID)
	if err != nil {
		return TaskAttribution{}, err
	}

	files, err := s.store.ReadChangeFiles(ctx, change.ID)
	if err != nil {
		return TaskAttribution{}, err
	}
	inScope := func(path string) bool {
		for _, scope := range answer.BaselineScope {
			if path == scope {
				return true
			}
		}
		return false
	}
	for _, file := range files {
		if inScope(file.Path) {
			continue
		}
		answer.Drift = append(answer.Drift, ScopeDrift(
			taskID, change.ID, file.Path, file.Kind,
			"changed outside "+strconv.Itoa(len(answer.BaselineScope))+" baseline paths",
			answer.BaselineScope))
	}

	symbols, err := s.store.ReadChangeSymbols(ctx, change.ID)
	if err != nil {
		return TaskAttribution{}, err
	}
	for _, row := range symbols {
		attributed := SymbolAttribution{Key: row.Key, UID: row.UID, Via: row.Via}
		file, found, err := s.indexes.PathForUID(ctx, row.UID)
		if err != nil {
			return TaskAttribution{}, err
		}
		if found {
			attributed.File = file
		}
		var candidates []string
		if found {
			for _, other := range all {
				for _, scope := range scopes[other.TaskID] {
					if scope == file {
						candidates = append(candidates, other.ID)
						break
					}
				}
			}
			sort.Strings(candidates)
		}
		switch len(candidates) {
		case 1:
			attributed.Outcome = AttributedOutcome
			attributed.AttributedTo = candidates[0]
		case 0:
			attributed.Outcome = UnregisteredOutcome
			finding := UnregisteredChange(taskID, change.ID, row.Key, row.Via, answer.BaselineScope)
			attributed.Finding = &finding
		default:
			attributed.Outcome = AmbiguousOutcome
			attributed.Candidates = candidates
			finding := AmbiguousAttribution(taskID, change.ID, row.Key, candidates, answer.BaselineScope)
			attributed.Finding = &finding
		}
		answer.Symbols = append(answer.Symbols, attributed)
	}
	return answer, nil
}

// openTaskChange loads one task's Change without failing on absence: the
// second return reports whether a row exists. Callers treat absence as
// "nothing discovered", never as corruption.
func (s *Service) openTaskChange(ctx context.Context, taskID string) (Change, bool, error) {
	if err := s.store.requireSchema(ctx); err != nil {
		return Change{}, false, err
	}
	var change Change
	var operation sql.NullString
	err := s.store.db.QueryRowContext(ctx, `SELECT change_id, task_id, operation_id, updated_at
		FROM changes WHERE task_id = ?`, taskID).
		Scan(&change.ID, &change.TaskID, &operation, &change.UpdatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Change{}, false, nil
		}
		return Change{}, false, corruptState(err)
	}
	change.OperationID = operation.String
	return change, true, nil
}
