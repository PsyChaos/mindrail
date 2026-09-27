package changes

import (
	"context"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"

	"github.com/PsyChaos/mindrail/internal/app"
	"github.com/PsyChaos/mindrail/internal/filesystem"
	"github.com/PsyChaos/mindrail/internal/git"
	"github.com/PsyChaos/mindrail/internal/storage"
)

const guardBaselineVersion = 1

// ProofMapping preserves an exact, already-resolved test edge. Policy fields
// are deliberately absent: current knowledge decides severity and active state.
type ProofMapping struct {
	Path          string `json:"path"`
	Test          string `json:"test"`
	ProductionUID string `json:"production_uid"`
	InvariantID   string `json:"invariant_id"`
}

// GuardBaseline accumulates exact proof edges for one workspace and Git HEAD.
// A present empty Mappings slice is a captured empty set, not an absent row.
type GuardBaseline struct {
	HeadOID  string
	Mappings []ProofMapping
}

// EnsureGuardBaseline must finish before task-scoped index mutation. It unions
// current exact edges with the stored set in one serialized transaction; a later
// binding can add protection but reindexing cannot erase historical proof. This
// store never owns SDK types or testguard policy and never guesses an edge.
func (s *Store) EnsureGuardBaseline(ctx context.Context, projectID, root string, runner git.CommandRunner) (GuardBaseline, error) {
	boundary, err := filesystem.NewRoot(root)
	if err != nil {
		return GuardBaseline{}, err
	}
	root = boundary.Path()
	var workspace string
	if err := s.db.QueryRowContext(ctx, `SELECT workspace_id FROM workspaces WHERE project_id = ? AND root_path = ?`, projectID, root).Scan(&workspace); err != nil {
		return GuardBaseline{}, corruptState(err)
	}
	head, err := guardHead(ctx, root, runner)
	if err != nil {
		return GuardBaseline{}, err
	}
	_, found, err := s.readGuardBaseline(ctx, projectID, workspace, head)
	if err != nil {
		return GuardBaseline{}, err
	}
	if !found {
		if err := s.trustGuardIndex(ctx, root, head, runner); err != nil {
			// A concurrent caller may have captured the trustworthy index and
			// then reconciled it while this caller was checking hashes.
			_, found, readErr := s.readGuardBaseline(ctx, projectID, workspace, head)
			if readErr != nil {
				return GuardBaseline{}, readErr
			}
			if !found {
				return GuardBaseline{}, err
			}
		}
	}
	mappings, err := s.captureProofMappings(ctx, root)
	if err != nil {
		return GuardBaseline{}, err
	}
	baseline := GuardBaseline{HeadOID: head, Mappings: mappings}
	if err := baseline.CheckHead(ctx, root, runner); err != nil {
		return GuardBaseline{}, err
	}
	err = storage.InTx(ctx, s.db, func(ctx context.Context, tx *sql.Tx) error {
		var previous string
		err := tx.QueryRowContext(ctx, `SELECT mappings_json FROM guard_baselines
			WHERE project_id = ? AND workspace_id = ? AND head_oid = ? AND format_version = ?`,
			projectID, workspace, head, guardBaselineVersion).Scan(&previous)
		merged := slices.Clone(mappings)
		if err == nil {
			saved, err := decodeGuardBaseline(head, previous)
			if err != nil {
				return err
			}
			merged = append(merged, saved.Mappings...)
		} else if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		body, err := json.Marshal(canonicalMappings(merged))
		if err != nil || string(body) == previous {
			return err
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO guard_baselines
			(project_id, workspace_id, head_oid, format_version, mappings_json, captured_at)
			VALUES (?, ?, ?, ?, ?, ?)
			ON CONFLICT(project_id, workspace_id, head_oid, format_version)
			DO UPDATE SET mappings_json = excluded.mappings_json`,
			projectID, workspace, head, guardBaselineVersion, string(body), app.FormatTime(s.clock.Now()))
		return err
	})
	if err != nil {
		return GuardBaseline{}, writeFailure(ctx, s.db, "guard baseline capture", err)
	}
	saved, found, err := s.readGuardBaseline(ctx, projectID, workspace, head)
	if err != nil {
		return GuardBaseline{}, err
	}
	if !found {
		return GuardBaseline{}, corruptState(errors.New("committed guard baseline is missing"))
	}
	return saved, nil
}

// CheckHead refuses an observation spanning two Git baselines. Call it after
// reconciliation, before any verdict or completion transition is published.
func (b GuardBaseline) CheckHead(ctx context.Context, root string, runner git.CommandRunner) error {
	head, err := guardHead(ctx, root, runner)
	if err != nil {
		return err
	}
	if head != b.HeadOID {
		return guardBaselineUntrusted("Git HEAD moved while the task was being reconciled")
	}
	return nil
}

func (s *Store) readGuardBaseline(ctx context.Context, projectID, workspace, head string) (GuardBaseline, bool, error) {
	var body string
	err := s.db.QueryRowContext(ctx, `SELECT mappings_json FROM guard_baselines
		WHERE project_id = ? AND workspace_id = ? AND head_oid = ? AND format_version = ?`,
		projectID, workspace, head, guardBaselineVersion).Scan(&body)
	if errors.Is(err, sql.ErrNoRows) {
		return GuardBaseline{}, false, nil
	}
	if err != nil {
		return GuardBaseline{}, false, corruptState(err)
	}
	baseline, err := decodeGuardBaseline(head, body)
	return baseline, err == nil, err
}

func decodeGuardBaseline(head, body string) (GuardBaseline, error) {
	var mappings []ProofMapping
	if err := json.Unmarshal([]byte(body), &mappings); err != nil || mappings == nil {
		return GuardBaseline{}, corruptState(fmt.Errorf("invalid guard baseline JSON: %v", err))
	}
	for _, mapping := range mappings {
		if mapping.Path == "" || mapping.Test == "" || mapping.ProductionUID == "" || mapping.InvariantID == "" {
			return GuardBaseline{}, corruptState(errors.New("incomplete guard baseline mapping"))
		}
	}
	canonical := canonicalMappings(mappings)
	encoded, err := json.Marshal(canonical)
	if err != nil || string(encoded) != body {
		return GuardBaseline{}, corruptState(errors.New("noncanonical guard baseline mappings"))
	}
	return GuardBaseline{HeadOID: head, Mappings: canonical}, nil
}

// trustGuardIndex detects an upgraded workspace already indexed after edits.
// Its old resolved edges cannot be reconstructed by relabeling the live index
// as HEAD. Every tracked indexed file must still carry HEAD's content hash;
// newly added files have no earlier committed proof to lose.
func (s *Store) trustGuardIndex(ctx context.Context, root, head string, runner git.CommandRunner) error {
	if strings.HasPrefix(head, "unborn:") {
		return nil
	}
	rows, err := s.db.QueryContext(ctx, `SELECT path, COALESCE(content_hash, ''), state FROM file_index_state ORDER BY path`)
	if err != nil {
		return corruptState(err)
	}
	type fileState struct{ path, hash, state string }
	var files []fileState
	for rows.Next() {
		var file fileState
		if err := rows.Scan(&file.path, &file.hash, &file.state); err != nil {
			_ = rows.Close()
			return corruptState(err)
		}
		if isBelow(root, file.path) {
			files = append(files, file)
		}
	}
	readErr := rows.Err()
	_ = rows.Close()
	if readErr != nil {
		return corruptState(readErr)
	}
	for _, file := range files {
		rel, err := filepath.Rel(root, file.path)
		if err != nil {
			return err
		}
		content, found, err := git.ShowHEAD(ctx, runner, root, filepath.ToSlash(rel))
		if err != nil {
			return err
		}
		if found && (file.state != "indexed" || file.hash != contentHashBytes(content)) {
			return guardBaselineUntrusted("no trustworthy pre-mutation guard baseline exists for " + filepath.ToSlash(rel))
		}
	}
	return nil
}

func (s *Store) captureProofMappings(ctx context.Context, root string) ([]ProofMapping, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT r.path, test.name, production.symbol_uid, binding.invariant_id
		FROM invariant_symbol_bindings binding
		JOIN symbols production ON production.symbol_uid = binding.symbol_uid
		JOIN symbol_references r ON r.resolved_symbol_id = production.id
		JOIN symbols test ON test.unit_id = r.unit_id AND test.path = r.path AND test.logical_key = r.referrer_key
		ORDER BY r.path, test.name, production.symbol_uid, binding.invariant_id`)
	if err != nil {
		return nil, corruptState(err)
	}
	defer rows.Close()
	mappings := []ProofMapping{}
	for rows.Next() {
		var mapping ProofMapping
		if err := rows.Scan(&mapping.Path, &mapping.Test, &mapping.ProductionUID, &mapping.InvariantID); err != nil {
			return nil, corruptState(err)
		}
		if isBelow(root, mapping.Path) && IsVerificationTest(mapping.Path) {
			mappings = append(mappings, mapping)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, corruptState(err)
	}
	return canonicalMappings(mappings), nil
}

func canonicalMappings(mappings []ProofMapping) []ProofMapping {
	slices.SortFunc(mappings, func(a, b ProofMapping) int {
		for _, pair := range [][2]string{{a.Path, b.Path}, {a.Test, b.Test}, {a.ProductionUID, b.ProductionUID}, {a.InvariantID, b.InvariantID}} {
			if order := strings.Compare(pair[0], pair[1]); order != 0 {
				return order
			}
		}
		return 0
	})
	return slices.Compact(mappings)
}

// IsVerificationTest is the shared D-207 directory/affix rule. Both capture
// and evaluation must agree about which indexed referrers are proof tests.
func IsVerificationTest(path string) bool {
	parts := strings.Split(filepath.ToSlash(path), "/")
	for _, part := range parts[:len(parts)-1] {
		if part == "test" || part == "tests" {
			return true
		}
	}
	base := parts[len(parts)-1]
	return strings.HasPrefix(base, "test_") || strings.HasSuffix(base, "_test.py") ||
		strings.HasSuffix(base, ".test.ts") || strings.HasSuffix(base, ".test.js")
}

func guardHead(ctx context.Context, root string, runner git.CommandRunner) (string, error) {
	if runner == nil {
		return "", invalidInput("guard baseline needs a Git runner")
	}
	out, stderr, err := runner.Run(ctx, root, "rev-parse", "--verify", "--quiet", "HEAD^{commit}")
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 && len(stderr) == 0 {
			ref, _, refErr := runner.Run(ctx, root, "symbolic-ref", "--quiet", "HEAD")
			if refErr == nil && strings.HasPrefix(strings.TrimSpace(string(ref)), "refs/heads/") {
				return "unborn:" + strings.TrimSpace(string(ref)), nil
			}
		}
		return "", fmt.Errorf("guard baseline HEAD: %w", err)
	}
	head := strings.TrimSpace(string(out))
	if _, err := hex.DecodeString(head); err != nil || len(head) != 40 {
		return "", invalidInput("guard baseline HEAD is not a commit object id")
	}
	return head, nil
}

func guardBaselineUntrusted(why string) error {
	return app.NewError(app.CodeIndexStateCorrupt, app.KindFailed, why,
		"Completion cannot safely reconstruct the previous verification-test mappings.",
		"Preserve the current diff and runtime database; restore or reconstruct the index from the committed baseline before retrying.")
}
