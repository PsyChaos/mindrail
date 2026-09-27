package changes

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"sort"
	"time"

	"github.com/PsyChaos/mindrail/internal/app"
	"github.com/PsyChaos/mindrail/internal/storage"
)

// AutomaticScope records the dirty files present before automatic work began.
// ForeignPaths are currently leased to another run and are not this run's work.
// The scope applies only to the returned service view, never legacy callers.
type AutomaticScope struct {
	Initial      []FileChange
	ForeignPaths []string
	StartedAt    time.Time
}

func (s *Service) WithAutomaticScope(scope AutomaticScope) *Service {
	view := *s
	view.automatic = &AutomaticScope{Initial: append([]FileChange(nil), scope.Initial...), ForeignPaths: append([]string(nil), scope.ForeignPaths...), StartedAt: scope.StartedAt}
	return &view
}

// ExtendBaseline adds paths without recapturing previously declared files.
// Retry after edits therefore cannot erase the original before-change truth.
func (s *Store) ExtendBaseline(ctx context.Context, taskID string, paths []string) (BaselineSummary, error) {
	if err := s.requireTask(ctx, taskID); err != nil {
		return BaselineSummary{}, err
	}
	hashes := make(map[string]string, len(paths))
	for _, path := range paths {
		if !filepath.IsAbs(path) || filepath.Clean(path) != path {
			return BaselineSummary{}, invalidInput("baseline scope needs clean absolute paths")
		}
		hash, err := contentHash(path)
		if err != nil {
			return BaselineSummary{}, err
		}
		hashes[path] = hash
	}
	err := storage.InTx(ctx, s.db, func(ctx context.Context, tx *sql.Tx) error {
		for path, hash := range hashes {
			if _, err := tx.ExecContext(ctx, `INSERT INTO change_baselines (task_id,path,content_hash,captured_at) VALUES (?,?,?,?) ON CONFLICT(task_id,path) DO NOTHING`, taskID, path, hash, app.FormatTime(s.clock.Now())); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return BaselineSummary{}, writeFailure(ctx, s.db, "baseline extension", err)
	}
	baseline, err := s.ReadBaseline(ctx, taskID)
	return BaselineSummary{TaskID: taskID, Files: len(baseline)}, err
}

func (s *Service) automaticEntries(ctx context.Context, taskID string, entries []FileChange) ([]FileChange, error) {
	baseline, err := s.store.ReadBaseline(ctx, taskID)
	if err != nil {
		return nil, err
	}
	paths := make([]string, 0, len(baseline))
	for p := range baseline {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	delta, err := s.store.BaselineFileDelta(ctx, taskID, paths)
	if err != nil {
		return nil, err
	}
	initial := make(map[string]FileChange, len(s.automatic.Initial))
	for _, f := range s.automatic.Initial {
		initial[f.Path] = f
	}
	foreign := make(map[string]bool, len(s.automatic.ForeignPaths))
	for _, p := range s.automatic.ForeignPaths {
		foreign[p] = true
	}
	seen := make(map[string]bool, len(entries))
	for _, entry := range entries {
		seen[entry.Path] = true
		if _, own := baseline[entry.Path]; own {
			continue
		}
		if foreign[entry.Path] {
			continue
		}
		if before, ok := initial[entry.Path]; ok && before == entry {
			continue
		}
		settled, err := s.store.completedFileMatches(ctx, entry, s.automatic.StartedAt)
		if err != nil {
			return nil, err
		}
		if settled {
			continue
		}
		delta = append(delta, entry)
	}
	// Restoring old dirty bytes to HEAD disappears from Git status, but it is
	// still a mutation made during this run. Do not silently lose that drift.
	for path, before := range initial {
		if seen[path] || foreign[path] {
			continue
		}
		if _, own := baseline[path]; own {
			continue
		}
		hash, err := contentHash(path)
		if err != nil {
			return nil, err
		}
		if hash == before.Hash {
			continue
		}
		kind := FileModified
		if hash == "" {
			kind = FileDeleted
		}
		entry := FileChange{Path: path, Kind: kind, Hash: hash, Via: ViaReconcile}
		settled, err := s.store.completedFileMatches(ctx, entry, s.automatic.StartedAt)
		if err != nil {
			return nil, err
		}
		if settled {
			continue
		}
		delta = append(delta, entry)
	}
	for i := range delta {
		delta[i].Via = ViaReconcile
	}
	sort.Slice(delta, func(i, j int) bool { return delta[i].Path < delta[j].Path })
	return delta, nil
}

// activeTaskChanges excludes terminal tasks only from ownership competition;
// their rows and the public historical listing remain intact.
func (s *Store) activeTaskChanges(ctx context.Context, taskID string) ([]Change, error) {
	all, err := s.ListTaskChanges(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT task_id FROM tasks WHERE state NOT IN ('COMPLETED','ABANDONED')`)
	if err != nil {
		return nil, corruptState(err)
	}
	defer rows.Close()
	active := map[string]bool{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, corruptState(err)
		}
		active[id] = true
	}
	if err := rows.Err(); err != nil {
		return nil, corruptState(err)
	}
	out := make([]Change, 0, len(all))
	for _, c := range all {
		if active[c.TaskID] || c.TaskID == taskID {
			out = append(out, c)
		}
	}
	return out, nil
}

// A parallel run may finish and release its lease before this run reconciles.
// Its last completed bytes remain attributable to that task, even when both
// runs began before either edit. Only the newest completed result is consulted.
func (s *Store) completedFileMatches(ctx context.Context, entry FileChange, startedAt time.Time) (bool, error) {
	if startedAt.IsZero() {
		return false, nil
	}
	var hash, kind, stamp string
	err := s.db.QueryRowContext(ctx, `SELECT f.content_hash, f.kind,t.updated_at FROM change_files f
		JOIN changes c ON c.change_id=f.change_id JOIN tasks t ON t.task_id=c.task_id
		WHERE f.path=? AND t.state='COMPLETED' ORDER BY t.updated_at DESC, t.rowid DESC LIMIT 1`, entry.Path).Scan(&hash, &kind, &stamp)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, corruptState(err)
	}
	completedAt, err := app.ParseTime(stamp)
	if err != nil {
		return false, corruptState(err)
	}
	return !completedAt.Before(startedAt) && hash == entry.Hash && (hash != "" || kind == entry.Kind), nil
}
