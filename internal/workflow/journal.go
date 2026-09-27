package workflow

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/PsyChaos/mindrail/internal/app"
	"github.com/PsyChaos/mindrail/internal/changes"
	"github.com/PsyChaos/mindrail/internal/git"
	"github.com/PsyChaos/mindrail/internal/storage"
)

type journal struct {
	Input     StartInput           `json:"input"`
	Initial   []changes.FileChange `json:"initial"`
	StartedAt time.Time            `json:"started_at"`
}

func digest(value []byte) string { sum := sha256.Sum256(value); return hex.EncodeToString(sum[:]) }
func (s *Service) operation(key, phase string) string {
	return "WFL:" + digest([]byte(s.options.ProjectID+"\x00"+s.options.WorkspaceID+"\x00"+key)) + ":" + phase
}

func (s *Service) rememberStart(ctx context.Context, in StartInput) (journal, error) {
	encoded, err := json.Marshal(in)
	if err != nil {
		return journal{}, err
	}
	hash := digest(encoded)
	// Replay is read before touching Git: an interrupted process must keep the
	// original dirty-tree boundary, even after the agent has edited files.
	if j, recorded, found, err := s.lookupStart(ctx, in.RunKey); err != nil {
		return journal{}, err
	} else if found {
		if recorded != hash {
			return journal{}, runConflict()
		}
		return j, nil
	}
	started := s.options.Clock.Now().UTC()
	initial, err := s.options.Changes.Store().DiscoverFilesGit(ctx, git.NewExecRunner(), s.options.Root)
	if err != nil {
		return journal{}, err
	}
	j := journal{Input: in, Initial: initial, StartedAt: started}
	body, err := json.Marshal(j)
	if err != nil {
		return journal{}, err
	}
	err = storage.InTx(ctx, s.options.DB, func(ctx context.Context, tx *sql.Tx) error {
		var prior string
		err := tx.QueryRowContext(ctx, `SELECT request_hash FROM operations WHERE operation_id=?`, s.operation(in.RunKey, "start")).Scan(&prior)
		if err == nil {
			if prior != hash {
				return runConflict()
			}
			return nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO operations(operation_id,command,request_hash,result,recorded_at) VALUES(?,?,?,?,?)`, s.operation(in.RunKey, "start"), "workflow start", hash, string(body), app.FormatTime(started))
		return err
	})
	if err != nil {
		return journal{}, err
	}
	return s.loadStart(ctx, in.RunKey)
}

func runConflict() error {
	return app.NewError(app.CodeOperationIDConflict, app.KindFailed, "run_key already identifies a different automatic request", "No new automatic run was started.", "Retry the original goal and scope, or use a new run_key for new work.")
}

func (s *Service) lookupStart(ctx context.Context, key string) (journal, string, bool, error) {
	var command, hash, body string
	err := s.options.DB.QueryRowContext(ctx, `SELECT command,request_hash,result FROM operations WHERE operation_id=?`, s.operation(key, "start")).Scan(&command, &hash, &body)
	if errors.Is(err, sql.ErrNoRows) {
		return journal{}, "", false, nil
	}
	if err != nil {
		return journal{}, "", false, err
	}
	if command != "workflow start" {
		return journal{}, "", false, runConflict()
	}
	var j journal
	if err := json.Unmarshal([]byte(body), &j); err != nil {
		return journal{}, "", false, fmt.Errorf("decode automatic start: %w", err)
	}
	if j.Input.RunKey != key || j.Input.Goal == "" {
		return journal{}, "", false, invalid("automatic start journal is corrupt")
	}
	return j, hash, true, nil
}

func (s *Service) loadStart(ctx context.Context, key string) (journal, error) {
	j, _, found, err := s.lookupStart(ctx, key)
	if err != nil {
		return journal{}, err
	}
	if !found {
		return journal{}, invalid("unknown run_key; bootstrap the automatic run first")
	}
	return j, nil
}

func (s *Service) readPhase(ctx context.Context, key, phase string, out any) (bool, error) {
	var body string
	err := s.options.DB.QueryRowContext(ctx, `SELECT result FROM operations WHERE operation_id=?`, s.operation(key, phase)).Scan(&body)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	var record struct {
		Result json.RawMessage `json:"result"`
	}
	if err := json.Unmarshal([]byte(body), &record); err != nil {
		return false, err
	}
	if len(record.Result) == 0 {
		return false, invalid("automatic phase journal is corrupt")
	}
	if err := json.Unmarshal(record.Result, out); err != nil {
		return false, err
	}
	return true, nil
}

func (s *Service) recordFailure(ctx context.Context, key string, failure error) error {
	s.failureMu.Lock()
	s.failures[key] = failure
	s.failureMu.Unlock()
	_, err := s.options.DB.ExecContext(ctx, `INSERT INTO operations(operation_id,command,request_hash,result,recorded_at) VALUES(?,?,?,?,?) ON CONFLICT(operation_id) DO NOTHING`, s.operation(key, "failure"), "workflow failure", digest([]byte(key)), failure.Error(), app.FormatTime(s.options.Clock.Now()))
	return err
}

func (s *Service) health(ctx context.Context, key string) error {
	s.failureMu.Lock()
	err := s.failures[key]
	s.failureMu.Unlock()
	if err != nil {
		return err
	}
	var failure string
	err = s.options.DB.QueryRowContext(ctx, `SELECT result FROM operations WHERE operation_id=?`, s.operation(key, "failure")).Scan(&failure)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	return invalid("automatic run is stopped: " + failure + "; explicitly resume the task with a new run_key")
}

// reconcileOperation binds replay to the declared baseline and current bytes,
// not to task revision (editing files does not advance that revision).
func (s *Service) reconcileOperation(ctx context.Context, r Run) (string, error) {
	entries, err := s.options.Changes.Store().DiscoverFilesGit(ctx, git.NewExecRunner(), s.options.Root)
	if err != nil {
		return "", err
	}
	baseline, err := s.options.Changes.Store().ReadBaseline(ctx, r.TaskID)
	if err != nil {
		return "", err
	}
	delta, err := s.options.Changes.Store().BaselineFileDelta(ctx, r.TaskID, r.Paths)
	if err != nil {
		return "", err
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Path < entries[j].Path })
	sort.Slice(delta, func(i, j int) bool { return delta[i].Path < delta[j].Path })
	leases, err := s.options.Coordination.ListLeases(ctx, s.options.ProjectID)
	if err != nil {
		return "", err
	}
	owners := map[string]string{}
	for _, lease := range leases {
		owners[lease.Target().String()] = lease.Holder
	}
	body, err := json.Marshal(struct {
		Task       string
		Baseline   map[string]string
		Owners     map[string]string
		Git, Delta []changes.FileChange
	}{r.TaskID, baseline, owners, entries, delta})
	if err != nil {
		return "", err
	}
	return s.operation(r.RunKey, "reconcile:"+digest(body)[:32]), nil
}

// reconcile records the whole discovered result after all idempotent upserts
// succeed. The change store's operation record only owns change creation; this
// record owns subsequent discoveries of new bytes under that same change.
// A historical phase cannot replace applying the current snapshot: A -> B -> A
// must restore A in the file and symbol indexes even though A is already logged.
func (s *Service) reconcile(ctx context.Context, r Run, view *changes.Service) (changes.ReconcileResult, error) {
	id, err := s.reconcileOperation(ctx, r)
	if err != nil {
		return changes.ReconcileResult{}, err
	}
	runner := git.NewExecRunner()
	baseline, err := view.Store().EnsureGuardBaseline(ctx, s.options.ProjectID, s.options.Root, runner)
	if err != nil {
		return changes.ReconcileResult{}, err
	}
	out, err := view.Reconcile(ctx, s.options.ProjectID, s.options.Root, r.TaskID, id, runner)
	if err != nil {
		return out, err
	}
	if err := baseline.CheckHead(ctx, s.options.Root, runner); err != nil {
		return out, err
	}
	encoded, err := json.Marshal(out)
	if err != nil {
		return out, err
	}
	err = storage.InTx(ctx, s.options.DB, func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `INSERT INTO operations(operation_id,command,request_hash,result,recorded_at) VALUES(?,?,?,?,?) ON CONFLICT(operation_id) DO NOTHING`, id, "workflow reconcile", digest([]byte(id)), string(encoded), app.FormatTime(s.options.Clock.Now()))
		return err
	})
	return out, err
}
