package completion

import (
	"context"
	"encoding/json"

	"github.com/PsyChaos/mindrail/internal/app"
	"github.com/PsyChaos/mindrail/internal/filesystem"
	"github.com/PsyChaos/mindrail/internal/gate"
	"github.com/PsyChaos/mindrail/internal/knowledge/loader"
	"github.com/PsyChaos/mindrail/internal/knowledge/schema"
	"github.com/PsyChaos/mindrail/internal/knowledge/validate"
	"github.com/PsyChaos/mindrail/internal/testguard"
	"github.com/PsyChaos/mindrail/internal/validation"
	schemas "github.com/PsyChaos/mindrail/schemas"
)

// compose discovers every gate input server-side: attribution findings,
// evidence rows checked for freshness, bindings with knowledge severity,
// anchored ambiguities, and guard findings over git deltas of changed test
// files. No family arrives as caller input (decision D-206).
func (s *Service) compose(ctx context.Context, taskID string, required []string, knowledge knowledgeScope, prior []testguard.TestMapping) (gate.Input, error) {
	var input gate.Input
	attribution, err := s.changes.EvaluateTask(ctx, taskID, nil)
	if err != nil {
		return gate.Input{}, err
	}
	input.Attribution = attribution

	var rows []validation.Evidence
	for _, profile := range required {
		listed, err := s.evidence.EvidenceForProfile(ctx, profile)
		if err != nil {
			return gate.Input{}, err
		}
		rows = append(rows, listed...)
	}
	_, coverage, err := validation.Check(s.root, rows, required)
	if err != nil {
		return gate.Input{}, err
	}
	input.Coverage = coverage

	changeID, symbols, err := s.taskSymbols(ctx, taskID)
	if err != nil {
		return gate.Input{}, err
	}
	for _, uid := range symbols {
		bindings, err := s.indexes.BindingsWithStatus(ctx, uid)
		if err != nil {
			return gate.Input{}, err
		}
		for _, binding := range bindings {
			severity, active := knowledge.scopeOf(binding.InvariantID)
			input.Bindings = append(input.Bindings, gate.InvariantBlock{
				InvariantID: binding.InvariantID,
				UID:         uid,
				Status:      binding.Status,
				Severity:    severity,
				Active:      active,
			})
		}
		ambiguities, err := s.indexes.ListAmbiguitiesForUID(ctx, uid)
		if err != nil {
			return gate.Input{}, err
		}
		for _, ambiguity := range ambiguities {
			invariant, severity, active := s.anchorAmbiguity(ctx, knowledge, uid, ambiguity.CandidateKeys)
			input.Ambiguities = append(input.Ambiguities, gate.Ambiguity{
				Key:         ambiguity.RemovedKey,
				Candidates:  ambiguity.CandidateKeys,
				InvariantID: invariant,
				Severity:    severity,
				Active:      active,
			})
		}
	}
	guard, err := s.guardFiles(ctx, changeID, symbols, knowledge, prior)
	if err != nil {
		return gate.Input{}, err
	}
	input.Guard = guard
	return input, nil
}

// knowledgeScope is severity/active per invariant id, parsed only after the
// current store passes loader and schema/lineage checks.
type knowledgeScope struct {
	entries map[string]scopeEntry
}

type scopeEntry struct {
	severity string
	active   bool
}

func (s *Service) loadKnowledge(ctx context.Context) (knowledgeScope, error) {
	fsRoot, err := filesystem.NewRoot(s.root)
	if err != nil {
		return knowledgeScope{}, err
	}
	reg, err := schema.NewRegistry(schemas.KnowledgeFS)
	if err != nil {
		return knowledgeScope{}, err
	}
	store, err := loader.New(fsRoot, reg).Load(ctx)
	if err != nil {
		return knowledgeScope{}, err
	}
	// A refused record disappears from the readable subset. Even a nonfatal
	// problem must stop completion rather than silently drop its constraints.
	for _, problem := range store.Problems {
		return knowledgeScope{}, knowledgeError(problem.Code, problem.Path, problem.Message)
	}
	validator, err := schema.NewValidator(reg)
	if err != nil {
		return knowledgeScope{}, err
	}
	for _, finding := range validate.Check(store, validator) {
		return knowledgeScope{}, knowledgeError(finding.Code, finding.Path, finding.Message)
	}
	scope := knowledgeScope{entries: map[string]scopeEntry{}}
	for _, ref := range store.Invariants {
		var body struct {
			Severity string `json:"severity"`
			Status   string `json:"status"`
		}
		if err := json.Unmarshal(ref.Body, &body); err != nil || body.Severity == "" {
			scope.entries[ref.ID] = scopeEntry{severity: "CRITICAL", active: true}
			continue
		}
		scope.entries[ref.ID] = scopeEntry{severity: body.Severity, active: body.Status == "active"}
	}
	return scope, nil
}

func (k knowledgeScope) scopeOf(id string) (string, bool) {
	if entry, ok := k.entries[id]; ok {
		return entry.severity, entry.active
	}
	return "", false
}

// anchorAmbiguity links an ambiguity to the first active HIGH/CRITICAL
// invariant bound to the removed uid or any candidate uid. None found
// means warn-scope silence (gate semantics).
func (s *Service) anchorAmbiguity(ctx context.Context, knowledge knowledgeScope, removedUID string, candidates []string) (string, string, bool) {
	uids := append([]string{removedUID}, candidateUIDs(ctx, s, candidates)...)
	for _, uid := range uids {
		bound, err := s.indexes.BindingsForUID(ctx, uid)
		if err != nil || len(bound) == 0 {
			continue
		}
		for _, id := range bound {
			if severity, active := knowledge.scopeOf(id); active && (severity == "HIGH" || severity == "CRITICAL") {
				return id, severity, true
			}
		}
	}
	return "", "", false
}

func candidateUIDs(ctx context.Context, s *Service, candidates []string) []string {
	var out []string
	for _, key := range candidates {
		uids, err := s.indexes.KeyToUIDs(ctx, key)
		if err != nil {
			continue
		}
		out = append(out, uids...)
	}
	return out
}

// taskSymbols returns the open change id (empty when none) and the symbol
// uids of the task's change rows.
func (s *Service) taskSymbols(ctx context.Context, taskID string) (string, []string, error) {
	all, err := s.store.ListTaskChanges(ctx)
	if err != nil {
		return "", nil, err
	}
	var changeID string
	for _, change := range all {
		if change.TaskID == taskID {
			changeID = change.ID
			break
		}
	}
	if changeID == "" {
		return "", nil, nil
	}
	rows, err := s.store.ReadChangeSymbols(ctx, changeID)
	if err != nil {
		return "", nil, err
	}
	var uids []string
	for _, row := range rows {
		if row.UID != "" {
			uids = append(uids, row.UID)
		}
	}
	return changeID, uids, nil
}

func knowledgeError(code app.Code, path, message string) error {
	return app.NewError(code, app.KindFailed, path+": "+message,
		"Completion is refused because repository knowledge cannot be trusted.",
		"Repair "+path+" and retry completion.").WithMetadata("path", path)
}
