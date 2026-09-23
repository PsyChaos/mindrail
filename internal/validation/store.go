package validation

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"

	"github.com/PsyChaos/mindrail/internal/app"
	"github.com/PsyChaos/mindrail/internal/identity"
	"github.com/PsyChaos/mindrail/internal/storage"
)

// TableSchemaVersion is the migration that creates the evidence table.
//
// It exists for the same reason every other store's version does: a reader
// of this table must be able to tell "the schema does not hold it yet"
// apart from "it holds nothing", from the migration ledger, which is what
// the migrator itself is answerable for.
const TableSchemaVersion = 8

// Store owns only validation evidence; profiles live in internal/config
// and process facts in the runner.
type Store struct {
	db    *sql.DB
	clock app.Clock
}

// NewStore builds a Store over an open runtime database handle.
func NewStore(db *sql.DB, clock app.Clock) (*Store, error) {
	if db == nil {
		return nil, errors.New("validation: needs a database handle")
	}
	return &Store{db: db, clock: clock}, nil
}

func (s *Store) requireSchema(ctx context.Context) error {
	var applied sql.NullInt64
	if err := s.db.QueryRowContext(ctx, `SELECT max(version) FROM schema_migrations`).Scan(&applied); err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return err
		}
		var tables int
		if probeErr := s.db.QueryRowContext(ctx, `SELECT count(*) FROM sqlite_master WHERE type = 'table' AND name NOT GLOB 'sqlite_*'`).Scan(&tables); probeErr == nil && tables == 0 {
			return schemaBehind(0)
		}
		return corruptState(err)
	}
	if applied.Int64 < TableSchemaVersion {
		return schemaBehind(applied.Int64)
	}
	return nil
}

func schemaBehind(applied int64) error {
	return app.NewError(app.CodeMigrationFailed, app.KindFailed,
		"Schema is behind this binary: the runtime database is at migration "+strconv.FormatInt(applied, 10)+" and the validation store needs 8",
		"Evidence cannot be recorded; nothing was read and nothing was written.",
		"Run `mindrail init` to apply the pending migrations, then re-run the command.")
}

func corruptState(err error) error {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	return app.NewError(app.CodeIndexStateCorrupt, app.KindFailed,
		"the persisted validation facts could not be read consistently",
		"Evidence answers cannot be trusted until the state is repaired.",
		"Preserve the runtime database and restore its evidence state from a known-good backup before retrying.").WithCause(err)
}

func writeFailure(ctx context.Context, db *sql.DB, what string, err error) error {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	if _, ok := app.PayloadOf(err); ok {
		return err
	}
	wrapped := fmt.Errorf("write %s: %w", what, err)
	if classified := storage.WriteFailure(ctx, db, what, wrapped); classified != nil {
		return classified
	}
	return wrapped
}

// Evidence is one recorded command run: profile, redacted command and
// output, normalized result, the snapshot it ran against, and provenance
// binding them together (spec §45, MR-010 subset).
type Evidence struct {
	ID           string
	Profile      string
	Type         string
	Argv         []string
	Status       string
	ExitCode     int
	Output       string
	SnapshotHash string
	Provenance   string
	CreatedAt    string
	OperationID  string
}

// Record stores one evidence row. Redaction applies to output AND argv
// before insert: argv is redacted element-wise (never as serialized JSON,
// where escaping would defeat literal matching — Breaker B-3), so a secret
// passed as an argument cannot survive in the command record (AC-24). The
// request hash covers the raw argv: a retry is the same run only with the
// same words. Operation-id replay mirrors D-121: the same id with the same
// request hash returns the stored row without a new write; the same id with
// a different hash is a conflict, never a second row. An empty operation id
// records without idempotency.
func (s *Store) Record(ctx context.Context, profile, typ string, argv []string, result Result, snapshotHash, provenance, operationID string, redactor *Redactor) (Evidence, error) {
	if profile == "" || typ == "" || len(argv) == 0 || snapshotHash == "" {
		return Evidence{}, invalidInput("evidence needs a profile, a type, a command and a snapshot")
	}
	if redactor == nil {
		return Evidence{}, invalidInput("evidence needs a redactor")
	}
	if err := s.requireSchema(ctx); err != nil {
		return Evidence{}, err
	}
	argvJSON, err := json.Marshal(argv)
	if err != nil {
		return Evidence{}, invalidInput("evidence command is not representable")
	}
	hash := requestHash(profile, typ, string(argvJSON), snapshotHash)
	if operationID != "" {
		if replayed, ok, err := s.replayLogged(ctx, operationID, hash); err != nil {
			return Evidence{}, err
		} else if ok {
			return replayed, nil
		}
	}
	now := app.FormatTime(s.clock.Now())
	redactedArgv := make([]string, len(argv))
	for i, word := range argv {
		redactedArgv[i] = redactor.Redact(word)
	}
	redactedJSON, err := json.Marshal(redactedArgv)
	if err != nil {
		return Evidence{}, invalidInput("evidence command is not representable")
	}
	record := Evidence{
		ID:           identity.NewID("EVD"),
		Profile:      profile,
		Type:         typ,
		Argv:         redactedArgv,
		Status:       result.Status,
		ExitCode:     result.ExitCode,
		Output:       redactor.Redact(result.Stdout + result.Stderr),
		SnapshotHash: snapshotHash,
		Provenance:   provenance,
		CreatedAt:    now,
		OperationID:  operationID,
	}
	err = storage.InTx(ctx, s.db, func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `INSERT INTO evidence
			(evidence_id, profile, type, command_argv, status, exit_code, output,
			snapshot_hash, provenance, created_at, operation_id, request_hash)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, NULLIF(?, ''), ?)
			ON CONFLICT(operation_id) DO NOTHING`,
			record.ID, profile, typ, string(redactedJSON), record.Status, record.ExitCode,
			record.Output, snapshotHash, provenance, now, operationID, hash)
		return err
	})
	if err != nil {
		return Evidence{}, writeFailure(ctx, s.db, "evidence", err)
	}
	return s.readRecorded(ctx, record.ID, operationID, hash)
}

// replayLogged returns the stored row when the operation id already names a
// run: the same request hash replays it, a different hash conflicts — the
// retry changed the question, so answering from the old row would lie.
func (s *Store) replayLogged(ctx context.Context, operationID, hash string) (Evidence, bool, error) {
	var storedHash string
	err := s.db.QueryRowContext(ctx, `SELECT request_hash FROM evidence
		WHERE operation_id = ?`, operationID).Scan(&storedHash)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Evidence{}, false, nil
		}
		return Evidence{}, false, corruptState(err)
	}
	if storedHash != hash {
		return Evidence{}, false, operationConflict(operationID)
	}
	record, err := s.readByOperation(ctx, operationID)
	if err != nil {
		return Evidence{}, false, err
	}
	return record, true, nil
}

func operationConflict(operationID string) error {
	return app.NewError(app.CodeOperationIDConflict, app.KindFailed,
		"operation "+operationID+" already recorded different evidence",
		"Retrying the same operation id with a different command or snapshot would duplicate evidence.",
		"Mint a new operation id and retry the command.").WithMetadata("operation_id", operationID)
}

// readRecorded loads the row this call stored: by id, or — when a racer won
// the operation id first — by operation id after verifying the hash.
func (s *Store) readRecorded(ctx context.Context, id, operationID, hash string) (Evidence, error) {
	record, err := s.readByID(ctx, id)
	if err == nil {
		return record, nil
	}
	if operationID == "" {
		return Evidence{}, err
	}
	byOp, opErr := s.readByOperation(ctx, operationID)
	if opErr != nil {
		return Evidence{}, err
	}
	stored, err := s.requestHashOf(ctx, byOp.ID)
	if err != nil {
		return Evidence{}, err
	}
	if stored != hash {
		return Evidence{}, operationConflict(operationID)
	}
	return byOp, nil
}

func (s *Store) requestHashOf(ctx context.Context, id string) (string, error) {
	var hash string
	if err := s.db.QueryRowContext(ctx, `SELECT request_hash FROM evidence
		WHERE evidence_id = ?`, id).Scan(&hash); err != nil {
		return "", corruptState(err)
	}
	return hash, nil
}

func (s *Store) readByID(ctx context.Context, id string) (Evidence, error) {
	return s.readOne(ctx, `evidence_id = ?`, id)
}

func (s *Store) readByOperation(ctx context.Context, operationID string) (Evidence, error) {
	return s.readOne(ctx, `operation_id = ?`, operationID)
}

func (s *Store) readOne(ctx context.Context, predicate, arg string) (Evidence, error) {
	var record Evidence
	var argvJSON, operation sql.NullString
	err := s.db.QueryRowContext(ctx, `SELECT evidence_id, profile, type, command_argv,
		status, exit_code, output, snapshot_hash, provenance, created_at, operation_id
		FROM evidence WHERE `+predicate, arg).Scan(
		&record.ID, &record.Profile, &record.Type, &argvJSON, &record.Status,
		&record.ExitCode, &record.Output, &record.SnapshotHash, &record.Provenance,
		&record.CreatedAt, &operation)
	if err != nil {
		return Evidence{}, corruptState(err)
	}
	record.OperationID = operation.String
	if err := json.Unmarshal([]byte(argvJSON.String), &record.Argv); err != nil {
		return Evidence{}, corruptState(err)
	}
	return record, nil
}

func requestHash(profile, typ, argvJSON, snapshotHash string) string {
	digest := sha256.New()
	digest.Write([]byte(profile))
	digest.Write([]byte{0})
	digest.Write([]byte(typ))
	digest.Write([]byte{0})
	digest.Write([]byte(argvJSON))
	digest.Write([]byte{0})
	digest.Write([]byte(snapshotHash))
	return hex.EncodeToString(digest.Sum(nil))
}
