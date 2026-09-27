package changes

import (
	"context"
	"database/sql"
	"errors"

	"github.com/PsyChaos/mindrail/internal/app"
)

// RecordAttribution stores a human/agent assignment of one logical key to
// one Change (decision D-137). Validation is fail-closed and runs before any
// write: empty deciders, unknown changes and malformed keys are refused, so
// the table never holds an assignment that resolves nowhere. Recording for
// the same key supersedes: one row per key, rewritten, never expiring in
// 0.1. This is the write path that closes Breaker B-1 — the database holds
// no CHECK, so this refusal is the enforcement.
func (s *Store) RecordAttribution(ctx context.Context, key, changeID, decidedBy, reason string) error {
	if key == "" {
		return invalidInput("attribution needs a logical key")
	}
	if changeID == "" {
		return invalidInput("attribution needs a change")
	}
	if decidedBy == "" {
		return invalidInput("attribution needs a decider")
	}
	if err := s.requireSchema(ctx); err != nil {
		return err
	}
	var known bool
	err := s.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM changes WHERE change_id = ?)`,
		changeID).Scan(&known)
	if err != nil {
		return corruptState(err)
	}
	if !known {
		return invalidInput("attribution names an unknown change")
	}
	now := app.FormatTime(s.clock.Now())
	if _, err := s.db.ExecContext(ctx, `INSERT INTO scope_attributions
		(logical_key, change_id, decided_by, reason, decided_at)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(logical_key) DO UPDATE SET
		change_id = excluded.change_id, decided_by = excluded.decided_by,
		reason = excluded.reason, decided_at = excluded.decided_at`,
		key, changeID, decidedBy, reason, now); err != nil {
		return writeFailure(ctx, s.db, "scope attribution", err)
	}
	return nil
}

// ReadAttributions returns every recorded assignment as logical key→change.
// Evaluation consults them first: an overridden symbol resolves to its
// recorded Change before candidate counting (decision D-137).
func (s *Store) ReadAttributions(ctx context.Context) (map[string]string, error) {
	if err := s.requireSchema(ctx); err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT logical_key, change_id FROM scope_attributions`)
	if err != nil {
		return nil, corruptState(err)
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var key, changeID string
		if err := rows.Scan(&key, &changeID); err != nil {
			return nil, corruptState(err)
		}
		out[key] = changeID
	}
	if err := rows.Err(); err != nil {
		return nil, corruptState(err)
	}
	return out, nil
}

// LeaseView is one lease the caller knows about, read as input — never
// through a coordination store (decision D-144). Kind mirrors the
// coordination target kinds (`task`, `file`); Key is the task id or the
// absolute path. Leases advise, never attribute (decision D-134): they join
// the guidance sentence, never the candidate count.
type LeaseView struct {
	Holder string
	Kind   string
	Key    string
}

// EvaluateTask returns the blocking set for completion: every
// ambiguous/unregistered/drifted item with code, provenance and next_action.
// Overrides resolve before counting — a recorded symbol's finding is gone,
// while siblings stay blocked. A clean task returns empty: ALLOW-shaped,
// with no verdict invented (MR-013 owns ALLOW/DENY).
//
// The empty task id evaluates ownerless work: NULL-task changes hold rows
// but belong to no baseline, so every one of their symbols rides the
// unregistered track instead of an error path (AC-03.5). A named task with
// no open Change evaluates empty the same way.
func (s *Service) EvaluateTask(ctx context.Context, taskID string, leases []LeaseView) ([]Finding, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	overrides, err := s.store.ReadAttributions(ctx)
	if err != nil {
		return nil, err
	}
	if taskID == "" {
		return s.evaluateOwnerless(ctx, overrides, leases)
	}
	answer, err := s.AttributeTask(ctx, taskID)
	if err != nil {
		return nil, err
	}
	all, err := s.store.ListTaskChanges(ctx)
	if err != nil {
		return nil, err
	}
	owningTask := map[string]string{}
	for _, change := range all {
		owningTask[change.ID] = change.TaskID
	}
	var blocked []Finding
	for _, drift := range answer.Drift {
		blocked = append(blocked, withLeaseGuidance(drift, drift.Provenance.ChangeKey,
			[]string{taskID}, leases))
	}
	for _, symbol := range answer.Symbols {
		if symbol.Finding == nil {
			continue
		}
		if _, overridden := overrides[symbol.Key]; overridden {
			continue
		}
		var candidateTasks []string
		for _, candidate := range symbol.Candidates {
			candidateTasks = append(candidateTasks, owningTask[candidate])
		}
		blocked = append(blocked, withLeaseGuidance(*symbol.Finding, symbol.File,
			candidateTasks, leases))
	}
	return blocked, nil
}

// evaluateOwnerless judges NULL-task change rows: files and symbols a
// discovery recorded without a task. With no baseline task nothing can
// attribute — overrides aside, every symbol is unregistered. Drift does not
// apply: there is no declared scope to drift from.
func (s *Service) evaluateOwnerless(ctx context.Context, overrides map[string]string, leases []LeaseView) ([]Finding, error) {
	if err := s.store.requireSchema(ctx); err != nil {
		return nil, err
	}
	rows, err := s.store.db.QueryContext(ctx, `SELECT f.change_id, s.logical_key, s.discovered_via
		FROM change_symbols s JOIN changes f ON f.change_id = s.change_id
		WHERE f.task_id IS NULL ORDER BY s.logical_key`)
	if err != nil {
		return nil, corruptState(err)
	}
	defer rows.Close()
	var blocked []Finding
	for rows.Next() {
		var changeID, key, via string
		if err := rows.Scan(&changeID, &key, &via); err != nil {
			return nil, corruptState(err)
		}
		if _, overridden := overrides[key]; overridden {
			continue
		}
		var file string
		uid, err := s.symbolUID(ctx, changeID, key)
		if err != nil {
			return nil, err
		}
		if path, found, err := s.indexes.PathForUID(ctx, uid); err != nil {
			return nil, err
		} else if found {
			file = path
		}
		blocked = append(blocked, withLeaseGuidance(
			UnregisteredChange("", changeID, key, via, nil), file, nil, leases))
	}
	if err := rows.Err(); err != nil {
		return nil, corruptState(err)
	}
	return blocked, nil
}

func (s *Service) symbolUID(ctx context.Context, changeID, key string) (string, error) {
	var uid sql.NullString
	err := s.store.db.QueryRowContext(ctx, `SELECT symbol_uid FROM change_symbols
		WHERE change_id = ? AND logical_key = ?`, changeID, key).Scan(&uid)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", nil
		}
		return "", corruptState(err)
	}
	return uid.String, nil
}

// withLeaseGuidance names the lease holder in next_action when exactly one
// lease covers the item — a file lease on its file, or a task lease on one
// of its tasks. Zero or several leases name none, and the outcome never
// follows the lease either way (decision D-134, AC-03.4).
func withLeaseGuidance(finding Finding, file string, tasks []string, leases []LeaseView) Finding {
	var covering []LeaseView
	for _, lease := range leases {
		switch lease.Kind {
		case "file":
			if lease.Key != "" && lease.Key == file {
				covering = append(covering, lease)
			}
		case "task":
			for _, task := range tasks {
				if lease.Key != "" && lease.Key == task {
					covering = append(covering, lease)
					break
				}
			}
		}
	}
	if len(covering) != 1 {
		return finding
	}
	holder := covering[0]
	what := "the file lease on " + file
	if holder.Kind == "task" {
		what = "the task lease on " + holder.Key
	}
	finding.NextAction = append(finding.NextAction,
		"Confirm with "+holder.Holder+", which holds "+what+".")
	return finding
}
