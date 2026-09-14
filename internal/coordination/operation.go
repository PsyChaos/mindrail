package coordination

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"regexp"
	"time"

	"github.com/PsyChaos/mindrail/internal/app"
)

// Operation is the idempotency key of one mutation (decision D-71): an id the
// caller minted — a ULID, a UUID, a counter — under which the store answers a
// second delivery of the same request from the first. The zero value is no
// key, and a write under it runs as every write did before MR-004.
type Operation struct {
	ID string
}

// operationIDPattern is the grammar of a caller-minted id: something a shell,
// a log line and a JSON string can all carry unchanged.
var operationIDPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$`)

// ValidOperationID reports whether id is one this store accepts. The command
// line judges it before the application starts; the store judges it again,
// because it is the surface MR-015's tools call without a command line.
func ValidOperationID(id string) bool { return operationIDPattern.MatchString(id) }

// Idempotent returns a view of the store under which the next write is the
// operation named: looked up before it runs, recorded after it commits. The
// binding is per call — a caller binds an id, performs one write, and lets the
// view go — so that two writes cannot share one id by accident, which would be
// answered as OPERATION_ID_CONFLICT on the second.
//
// A view rather than a parameter on every writer, because seven signatures
// and every test that calls them would otherwise carry a value that is the
// zero value at every call but the command line's (recorded as a departure
// from AC-06.1's letter in the findings).
func (s *Store) Idempotent(operationID string) *Store {
	bound := *s
	bound.op = Operation{ID: operationID}
	return &bound
}

// operationRecord is what the operations table holds for one mutation: the
// session the write was attributed to and whether it was minted, and the
// writer's own result as it was returned. On replay the caller gets exactly
// these, plus Replayed on the Write.
type operationRecord struct {
	Session Session         `json:"session"`
	Minted  bool            `json:"minted"`
	Result  json.RawMessage `json:"result"`
}

// requestHash is SHA-256 over a canonical JSON encoding of the command name
// and the parameters the store received, hex-encoded. It is computed by the
// store, not by the command, so MR-015's tools inherit the rule; the session
// handle is one of the parameters, so a "retry" with a different handle is a
// different request wearing the same id.
func requestHash(command string, params any) (string, error) {
	encoded, err := json.Marshal(struct {
		Command string `json:"command"`
		Params  any    `json:"params"`
	}{command, params})
	if err != nil {
		return "", fmt.Errorf("encode the request for hashing: %w", err)
	}
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:]), nil
}

// attributionKey renders an Attribution for the request hash: which session
// the write is for, or which workspace a session is minted for.
func attributionKey(by Attribution) string {
	if by.mint {
		return "mint:" + by.workspaceID
	}
	return "named:" + by.handle
}

// replay looks the bound operation up inside the transaction that would
// otherwise perform the write, before any other statement (decision D-71).
// The bool reports whether a record was found and decoded into result; a
// record for a different request is OPERATION_ID_CONFLICT.
func (s *Store) replay(ctx context.Context, tx *sql.Tx, command, hash string, result any) (Write, bool, error) {
	if s.op.ID == "" {
		return Write{}, false, nil
	}

	var (
		recordedCommand string
		recordedHash    string
		body            string
	)
	err := tx.QueryRowContext(ctx,
		`SELECT command, request_hash, result FROM operations WHERE operation_id = ?`, s.op.ID).
		Scan(&recordedCommand, &recordedHash, &body)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return Write{}, false, nil
	case err != nil:
		return Write{}, false, readFailed("the operation", s.op.ID, err)
	}
	if recordedCommand != command || recordedHash != hash {
		return Write{}, false, operationConflict(s.op.ID, recordedCommand, command)
	}

	var record operationRecord
	if err := json.Unmarshal([]byte(body), &record); err != nil {
		return Write{}, false, readFailed("the operation", s.op.ID, fmt.Errorf("operation %s result: %w", s.op.ID, err))
	}
	// Decoded into a zero value, never into the value the writer prepared
	// for a fresh run: a record whose result lacks the entity — a hand-edited
	// row, or one a later binary wrote in another shape — would otherwise
	// replay as the freshly minted id the writer had ready, a task that exists
	// nowhere, different on every replay (TASK-05's Breaker). What the record
	// holds is what is returned, and a record that holds no entity is damage.
	zero := reflect.New(reflect.TypeOf(result).Elem())
	if err := json.Unmarshal(record.Result, zero.Interface()); err != nil {
		return Write{}, false, readFailed("the operation", s.op.ID, fmt.Errorf("operation %s result: %w", s.op.ID, err))
	}
	if identity, ok := zero.Interface().(recorded); !ok || identity.recordedID() == "" {
		return Write{}, false, readFailed("the operation", s.op.ID,
			fmt.Errorf("operation %s result: the record names no %s", s.op.ID, reflect.TypeOf(result).Elem().Name()))
	}
	reflect.ValueOf(result).Elem().Set(zero.Elem())
	return Write{Session: record.Session, Minted: record.Minted, Replayed: true, OperationID: s.op.ID}, true, nil
}

// recorded is what every replayable result can answer: the id of the entity
// it is about, empty when the record carries none.
type recorded interface{ recordedID() string }

func (s Session) recordedID() string     { return s.ID }
func (t Task) recordedID() string        { return t.ID }
func (m Move) recordedID() string        { return m.Task.ID }
func (n Noted) recordedID() string       { return n.Checkpoint.ID }
func (a Acquisition) recordedID() string { return a.Lease.ID }
func (l Lease) recordedID() string       { return l.ID }

// record inserts the bound operation's row after the write, in the same
// transaction, so a refusal takes the record back with the write and a commit
// carries both (decision D-71). No-op without a bound operation.
func (s *Store) record(ctx context.Context, tx *sql.Tx, command, hash string, write Write, result any, now time.Time) error {
	if s.op.ID == "" {
		return nil
	}

	encoded, err := json.Marshal(result)
	if err != nil {
		return fmt.Errorf("encode the operation's result: %w", err)
	}
	body, err := json.Marshal(operationRecord{Session: write.Session, Minted: write.Minted, Result: encoded})
	if err != nil {
		return fmt.Errorf("encode the operation record: %w", err)
	}
	_, err = tx.ExecContext(ctx,
		`INSERT INTO operations (operation_id, command, request_hash, result, recorded_at) VALUES (?, ?, ?, ?, ?)`,
		s.op.ID, command, hash, string(body), app.FormatTime(now))
	return err
}

// refuseInvalidOperation is the store's own judgment of a bound id, for the
// callers that have no command line in front of them.
func (s *Store) refuseInvalidOperation() error {
	if s.op.ID == "" || ValidOperationID(s.op.ID) {
		return nil
	}
	return app.NewError(
		app.CodeCommandLineInvalid,
		app.KindUsage,
		fmt.Sprintf("the operation id %q is not an identifier this store accepts", s.op.ID),
		"Nothing was read and nothing was written.",
		"Pass --operation-id as up to 128 letters, digits, dots, underscores, colons or dashes, starting with a letter or digit.",
	).WithMetadata("operation_id", s.op.ID)
}
