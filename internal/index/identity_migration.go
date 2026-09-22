package index

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"sort"
	"strings"

	"github.com/PsyChaos/mindrail/internal/identity"
)

// RenameHint is one file move Git corroborated: absolute old and new paths.
// Hints travel on FileFacts from callers that can afford one git invocation
// per batch; the indexer never runs git itself, and matching without hints is
// structural-only. Malformed hints fail the completion closed.
type RenameHint struct {
	OldPath string
	NewPath string
}

// stagedKey is one incoming symbol reduced to what migration matching reads.
type stagedKey struct {
	Key            string
	Kind           string
	ContainerLocal string
	BodyHash       string
	Path           string
}

// ancestorRow is one old row reduced to what migration matching reads: the
// identity it carries plus the fingerprints the rewrite will abandon.
type ancestorRow struct {
	Key            string
	UID            string
	Kind           string
	ContainerLocal string
	BodyHash       string
	Path           string
}

// localPart splits a qualified key into its path-independent local half.
// The empty container maps to itself; malformed stored keys are damage, since
// this binary wrote every one of them.
func localPart(qualified string) (string, error) {
	if qualified == "" {
		return "", nil
	}
	var pair [2]string
	if err := json.Unmarshal([]byte(qualified), &pair); err != nil {
		return "", err
	}
	return pair[1], nil
}

// orderParentsFirst sorts staged keys so containers resolve before the rows
// that name them: depth follows container links among the staged set, ties
// break by key for determinism. A container no staged key owns is a root for
// ordering purposes — its uid resolves through lookup, not through this
// batch.
func orderParentsFirst(keys []stagedKey) []stagedKey {
	localOf := func(key string) string {
		local, err := localPart(key)
		if err != nil {
			return "\x00" + key
		}
		return local
	}
	byLocal := make(map[string]stagedKey, len(keys))
	for _, key := range keys {
		if _, ok := byLocal[localOf(key.Key)]; !ok {
			byLocal[localOf(key.Key)] = key
		}
	}
	depth := func(key stagedKey) int {
		depth := 0
		seen := map[string]bool{key.Key: true}
		current := key.ContainerLocal
		for current != "" {
			holder, ok := byLocal[current]
			if !ok || seen[holder.Key] {
				break
			}
			seen[holder.Key] = true
			depth++
			current = holder.ContainerLocal
		}
		return depth
	}
	ordered := append([]stagedKey(nil), keys...)
	sort.SliceStable(ordered, func(i, j int) bool {
		depthI, depthJ := depth(ordered[i]), depth(ordered[j])
		if depthI != depthJ {
			return depthI < depthJ
		}
		return ordered[i].Key < ordered[j].Key
	})
	return ordered
}

// matchBar is the 0.1 STRUCTURAL migration bar (decision D-96): body, kind
// and container must agree, and the pair must share a file or a
// Git-corroborated move. Rename moves the signature bytes, so structure is
// deliberately not compared — it changes on every rename by extractor
// design. The ancestor container arrives already remapped through renames
// this batch applied (the cascade).
func matchBar(added stagedKey, ancestor ancestorRow, remappedAncestorContainer string, gitCorroborated bool) bool {
	if added.BodyHash == "" || added.BodyHash != ancestor.BodyHash {
		return false
	}
	if added.Kind != ancestor.Kind {
		return false
	}
	if remappedAncestorContainer != added.ContainerLocal {
		return false
	}
	if added.Path == ancestor.Path {
		return true
	}
	return gitCorroborated
}

// hintMovesFor returns the old paths Git renames into this file: the only
// cross-file ancestor source. Same-file ancestors need no corroboration.
func hintMovesFor(hints []RenameHint, newPath string) []string {
	var olds []string
	for _, hint := range hints {
		if hint.NewPath == newPath && hint.OldPath != "" && hint.OldPath != newPath {
			olds = append(olds, hint.OldPath)
		}
	}
	return olds
}

// resolveIdentitiesTx assigns every staged key its lineage uid inside the
// completion transaction (decision D-95). Lookup hits keep their uid —
// identities are never stolen. For the rest it runs the two-phase rule:
// first every disappeared ancestor counts the added keys meeting the bar
// (two or more records ambiguity and consumes them), then every unclaimed
// added key resolves against at most one remaining ancestor or mints.
// It returns key→uid for claimed keys; ambiguous and minted maps are
// reported separately for the row writer.
func (s *Store) resolveIdentitiesTx(ctx context.Context, tx *sql.Tx, projectID, unitID, language string, staged []stagedKey, ancestors []ancestorRow, hints []RenameHint, now string) (claimed map[string]string, ambiguous map[string]bool, err error) {
	moved := make(map[string]bool)
	for _, hint := range hints {
		if hint.OldPath != "" && hint.NewPath != "" {
			moved[hint.OldPath+"\x00"+hint.NewPath] = true
		}
	}
	corroborated := func(ancestorPath, addedPath string) bool {
		return moved[ancestorPath+"\x00"+addedPath]
	}
	// Phase one: per removed ancestor, count meeting added keys.
	meetingAdded := func(ancestor ancestorRow, remap map[string]string) []string {
		remapped := ancestor.ContainerLocal
		if rewritten, ok := remap[remapped]; ok {
			remapped = rewritten
		}
		var meeting []string
		for _, added := range staged {
			if matchBar(added, ancestor, remapped, corroborated(ancestor.Path, added.Path)) {
				meeting = append(meeting, added.Key)
			}
		}
		return meeting
	}
	consumed := make(map[string]bool)
	remap := make(map[string]string)
	for _, ancestor := range ancestors {
		meeting := meetingAdded(ancestor, nil)
		if len(meeting) < 2 {
			continue
		}
		for _, key := range meeting {
			consumed[key] = true
		}
		if err := s.recordAmbiguityTx(ctx, tx, unitID, ancestor.UID, ancestor.Key, meeting, now); err != nil {
			return nil, nil, err
		}
	}
	// Phase two: parents-first assignment over the unclaimed keys.
	claimed = make(map[string]string)
	ambiguous = make(map[string]bool)
	usedAncestors := make(map[string]bool)
	stagedLocals := make(map[string]string, len(staged))
	for _, key := range staged {
		if local, err := localPart(key.Key); err == nil {
			stagedLocals[key.Key] = local
		}
	}
	for _, added := range orderParentsFirst(staged) {
		if consumed[added.Key] {
			ambiguous[added.Key] = true
			continue
		}
		var uid string
		err := tx.QueryRowContext(ctx, `SELECT symbol_uid FROM symbol_identities
			WHERE project_id = ? AND unit_id = ? AND language = ? AND logical_key = ?`,
			projectID, unitID, language, added.Key).Scan(&uid)
		if err == nil {
			claimed[added.Key] = uid
			continue
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return nil, nil, err
		}
		var match *ancestorRow
		for i := range ancestors {
			ancestor := ancestors[i]
			if usedAncestors[ancestor.Key] {
				continue
			}
			remapped := ancestor.ContainerLocal
			if rewritten, ok := remap[remapped]; ok {
				remapped = rewritten
			}
			if matchBar(added, ancestor, remapped, corroborated(ancestor.Path, added.Path)) {
				match = &ancestors[i]
				break
			}
		}
		if match == nil {
			uid, err := s.mintIdentityTx(ctx, tx, projectID, unitID, language, added.Key, now)
			if err != nil {
				return nil, nil, err
			}
			claimed[added.Key] = uid
			continue
		}
		usedAncestors[match.Key] = true
		uid, err = s.migrateIdentityTx(ctx, tx, projectID, unitID, language, added, *match, remap, stagedLocals, claimed, now)
		if err != nil {
			return nil, nil, err
		}
		claimed[added.Key] = uid
	}
	return claimed, ambiguous, nil
}

// migrateIdentityTx rewrites one identity onto its heir: the key moves, the
// abandoned key joins the lineage memory, and the container pointer follows
// the remapped parent. It never mints for the heir and never steals. An
// ancestor row that predates allocation gets its identity minted first —
// the lineage must exist to be rewritten.
func (s *Store) migrateIdentityTx(ctx context.Context, tx *sql.Tx, projectID, unitID, language string, added stagedKey, ancestor ancestorRow, remap map[string]string, stagedLocals, assigned map[string]string, now string) (string, error) {
	uid := ancestor.UID
	if uid == "" {
		var err error
		uid, err = s.mintIdentityTx(ctx, tx, projectID, unitID, language, ancestor.Key, now)
		if err != nil {
			return "", err
		}
	}
	var previous string
	err := tx.QueryRowContext(ctx, `SELECT previous_keys FROM symbol_identities WHERE symbol_uid = ?`,
		uid).Scan(&previous)
	if err != nil {
		return "", err
	}
	var lineage []string
	if err := json.Unmarshal([]byte(previous), &lineage); err != nil {
		return "", err
	}
	lineage = append(lineage, ancestor.Key)
	encoded, err := json.Marshal(lineage)
	if err != nil {
		return "", err
	}
	ancestorLocal, err := localPart(ancestor.Key)
	if err != nil {
		return "", err
	}
	addedLocal, err := localPart(added.Key)
	if err != nil {
		return "", err
	}
	remap[ancestorLocal] = addedLocal
	// The container pointer follows staged parents through the assignment
	// map (parents resolve first); a pre-existing parent keeps NULL in 0.1 —
	// lineage pointer, not matching input, so the cascade never depends on
	// it.
	containerUID := sql.NullString{}
	if added.ContainerLocal != "" {
		for key, local := range stagedLocals {
			if local == added.ContainerLocal {
				if parent, ok := assigned[key]; ok {
					containerUID = sql.NullString{String: parent, Valid: true}
				}
				break
			}
		}
	}
	if _, err := tx.ExecContext(ctx, `UPDATE symbol_identities
		SET logical_key = ?, container_uid = ?, previous_keys = ? WHERE symbol_uid = ?`,
		added.Key, containerUID, string(encoded), uid); err != nil {
		return "", err
	}
	return uid, nil
}

// recordAmbiguityTx stores one blocked identity decision: which removed
// lineage, which added keys met the bar. Resolution never deletes the row;
// a later refresh supersedes its effect by rebinding.
func (s *Store) recordAmbiguityTx(ctx context.Context, tx *sql.Tx, unitID, removedUID, removedKey string, candidateKeys []string, now string) error {
	encoded, err := json.Marshal(candidateKeys)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO symbol_identity_ambiguities
		(unit_id, removed_uid, removed_key, candidate_keys, created_at)
		VALUES (?, ?, ?, ?, ?)`,
		unitID, removedUID, removedKey, string(encoded), now)
	return err
}

// mintIdentityTx inserts-or-reads-back one uid inside the transaction: the
// same arbiter as MintIdentity, sharing its UNIQUE index across processes.
func (s *Store) mintIdentityTx(ctx context.Context, tx *sql.Tx, projectID, unitID, language, key, now string) (string, error) {
	minted := identity.NewID("SYM")
	if _, err := tx.ExecContext(ctx, `INSERT INTO symbol_identities
		(symbol_uid, project_id, unit_id, language, logical_key, created_at)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(project_id, unit_id, language, logical_key) DO NOTHING`,
		minted, projectID, unitID, language, key, now); err != nil {
		return "", err
	}
	var uid string
	if err := tx.QueryRowContext(ctx, `SELECT symbol_uid FROM symbol_identities
		WHERE project_id = ? AND unit_id = ? AND language = ? AND logical_key = ?`,
		projectID, unitID, language, key).Scan(&uid); err != nil {
		return "", err
	}
	return uid, nil
}

// snapshotFileAncestorsTx reads the old rows a completion is about to
// replace — this file's rows plus the rows of hint-named predecessors — as
// migration ancestors. Same unit and same language only: cross-unit and
// cross-language moves mint anew in 0.1 (documented limitation), because the
// allocation key scopes both dimensions and guessing across them would forge
// lineage.
func (s *Store) snapshotFileAncestorsTx(ctx context.Context, tx *sql.Tx, unitID, language, path string, hints []RenameHint) ([]ancestorRow, error) {
	paths := []string{path}
	for _, hint := range hints {
		if hint.NewPath == path {
			paths = append(paths, hint.OldPath)
		}
	}
	placeholders := make([]string, 0, len(paths))
	args := make([]any, 0, len(paths)+1)
	args = append(args, unitID)
	for _, p := range paths {
		placeholders = append(placeholders, "?")
		args = append(args, p)
	}
	query := `SELECT s.logical_key, s.symbol_uid, s.kind, s.container, s.body_hash, s.path
		FROM symbols s JOIN file_index_state f ON f.unit_id = s.unit_id AND f.path = s.path
		WHERE s.unit_id = ? AND s.path IN (` + strings.Join(placeholders, ", ") + `) AND f.language = ?`
	args = append(args, language)
	rows, err := tx.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ancestors []ancestorRow
	for rows.Next() {
		var ancestor ancestorRow
		var uid sql.NullString
		var container string
		if err := rows.Scan(&ancestor.Key, &uid, &ancestor.Kind, &container, &ancestor.BodyHash, &ancestor.Path); err != nil {
			return nil, err
		}
		ancestor.UID = uid.String
		local, err := localPart(container)
		if err != nil {
			return nil, err
		}
		ancestor.ContainerLocal = local
		ancestors = append(ancestors, ancestor)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return ancestors, nil
}
