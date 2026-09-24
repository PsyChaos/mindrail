package mcp

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/PsyChaos/mindrail/internal/filesystem"
	"github.com/PsyChaos/mindrail/internal/gate"
	"github.com/PsyChaos/mindrail/internal/knowledge/loader"
	"github.com/PsyChaos/mindrail/internal/knowledge/schema"
	"github.com/PsyChaos/mindrail/internal/testguard"
	"github.com/PsyChaos/mindrail/internal/validation"
	schemas "github.com/PsyChaos/mindrail/schemas"
)

// CompleteIn asks local completion. Required names evidence profiles;
// budget/escalation/approval are version-error parameters (decision
// D-208): present means refusal, absent means the default flow.
type CompleteIn struct {
	TaskID     string   `json:"task_id"`
	Required   []string `json:"required,omitempty"`
	Budget     *string  `json:"budget,omitempty"`
	Escalation *string  `json:"escalation,omitempty"`
	Approval   *string  `json:"approval,omitempty"`
}

// CompleteDenial is one gate denial on the wire.
type CompleteDenial struct {
	Code       string   `json:"code"`
	Reason     string   `json:"reason"`
	Key        string   `json:"key"`
	Provenance string   `json:"provenance"`
	NextAction []string `json:"next_action"`
}

// CompleteOut is the gate Decision verbatim: composition proves parity by
// construction (decision D-206).
type CompleteOut struct {
	Allow   bool             `json:"allow"`
	Denials []CompleteDenial `json:"denials"`
	Refusal *Refusal         `json:"refusal,omitempty"`
}

func (s *Server) complete(ctx context.Context, _ *sdk.CallToolRequest, in CompleteIn) (*sdk.CallToolResult, CompleteOut, error) {
	if refusal := versionedParam("budget", in.Budget); refusal != nil {
		return nil, CompleteOut{Denials: []CompleteDenial{}, Refusal: refusal}, nil
	}
	if refusal := versionedParam("escalation", in.Escalation); refusal != nil {
		return nil, CompleteOut{Denials: []CompleteDenial{}, Refusal: refusal}, nil
	}
	if refusal := versionedParam("approval", in.Approval); refusal != nil {
		return nil, CompleteOut{Denials: []CompleteDenial{}, Refusal: refusal}, nil
	}
	if in.TaskID == "" {
		return nil, CompleteOut{}, Invalid("complete needs a task")
	}
	if _, err := s.projectID(ctx, in.TaskID); err != nil {
		return nil, CompleteOut{}, err
	}
	composed, err := s.compose(ctx, in.TaskID, in.Required)
	if err != nil {
		return nil, CompleteOut{}, err
	}
	decision, err := gate.New().Evaluate(composed)
	if err != nil {
		return nil, CompleteOut{}, err
	}
	out := CompleteOut{Allow: decision.Allow}
	for _, denial := range decision.Denials {
		out.Denials = append(out.Denials, CompleteDenial{
			Code:       string(denial.Code),
			Reason:     denial.Reason,
			Key:        denial.Key,
			Provenance: denial.Provenance,
			NextAction: denial.NextAction,
		})
	}
	if out.Denials == nil {
		out.Denials = []CompleteDenial{}
	}
	return nil, out, nil
}

// compose discovers every gate input server-side: attribution findings,
// evidence rows checked for freshness, bindings with knowledge severity,
// anchored ambiguities, and guard findings over git deltas of changed test
// files. No family arrives as caller input (decision D-206).
func (s *Server) compose(ctx context.Context, taskID string, required []string) (gate.Input, error) {
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

	knowledge, err := s.loadKnowledge(ctx)
	if err != nil {
		return gate.Input{}, err
	}
	changeID, symbols, err := s.taskSymbols(ctx, taskID)
	if err != nil {
		return gate.Input{}, err
	}
	_ = changeID
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
	guard, err := s.guardChangedTests(ctx, taskID, changeID, symbols, knowledge)
	if err != nil {
		return gate.Input{}, err
	}
	input.Guard = guard
	return input, nil
}

// knowledgeScope is severity/active per invariant id, parsed from loaded
// bodies. Unparseable entries fail safe to active CRITICAL with the id
// named: a safety gate denies on uncertainty, never shrugs (design §4).
type knowledgeScope struct {
	entries map[string]scopeEntry
}

type scopeEntry struct {
	severity string
	active   bool
}

func (s *Server) loadKnowledge(ctx context.Context) (knowledgeScope, error) {
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
func (s *Server) anchorAmbiguity(ctx context.Context, knowledge knowledgeScope, removedUID string, candidates []string) (string, string, bool) {
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

func candidateUIDs(ctx context.Context, s *Server, candidates []string) []string {
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
func (s *Server) taskSymbols(ctx context.Context, taskID string) (string, []string, error) {
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

// guardChangedTests runs the guard over git deltas of the task's changed
// test files with reference-derived mapping: every uid's bindings, every
// binding's referrers in test files, every referrer resolved to a test
// name. Unresolvable names are skipped, never guessed.
func (s *Server) guardChangedTests(ctx context.Context, taskID, changeID string, uids []string, knowledge knowledgeScope) ([]testguard.Finding, error) {
	return s.guardFiles(ctx, changeID, uids, knowledge)
}

// guardFiles builds one testguard evaluation from the task's changed test
// files: before bytes from git HEAD (missing in HEAD reads as added and
// quiet), after bytes from disk (missing reads as removed), mapping from
// bound-uid referrers resolved to names. Files that cannot be judged
// (unknown language, unreadable after, unresolvable names) are skipped,
// never invented.
func (s *Server) guardFiles(ctx context.Context, changeID string, uids []string, knowledge knowledgeScope) ([]testguard.Finding, error) {
	if changeID == "" {
		return nil, nil
	}
	files, err := s.store.ReadChangeFiles(ctx, changeID)
	if err != nil {
		return nil, err
	}
	var deltas []testguard.FileDelta
	for _, file := range files {
		language, ok := guardLanguage(file.Path)
		if !ok || !isTestFile(file.Path) {
			continue
		}
		after, err := os.ReadFile(file.Path)
		if err != nil {
			if !os.IsNotExist(err) {
				continue
			}
			after = nil
		}
		deltas = append(deltas, testguard.FileDelta{
			Path:     file.Path,
			Language: language,
			Before:   s.headVersion(ctx, file.Path),
			After:    after,
		})
	}
	if len(deltas) == 0 {
		return nil, nil
	}
	mappings, err := s.guardMappings(ctx, knowledge, uids, deltas)
	if err != nil {
		return nil, err
	}
	result, err := s.guard.Evaluate(ctx, testguard.Request{
		Files:    deltas,
		Mappings: mappings,
		Trigger:  testguard.TriggerComplete,
	})
	if err != nil {
		return nil, err
	}
	return result.Findings, nil
}

// guardMappings links tests to invariants through bound production uids:
// every referrer of a bound uid whose file is a test file, resolved to
// its test name. Unresolvable names are skipped, never guessed.
func (s *Server) guardMappings(ctx context.Context, knowledge knowledgeScope, uids []string, deltas []testguard.FileDelta) ([]testguard.TestMapping, error) {
	testFiles := map[string]bool{}
	for _, delta := range deltas {
		testFiles[delta.Path] = true
	}
	var mappings []testguard.TestMapping
	for _, uid := range uids {
		bindings, err := s.indexes.BindingsWithStatus(ctx, uid)
		if err != nil {
			return nil, err
		}
		for _, binding := range bindings {
			severity, active := knowledge.scopeOf(binding.InvariantID)
			referrers, err := s.indexes.ReferrersOfUID(ctx, uid)
			if err != nil {
				return nil, err
			}
			for _, referrer := range referrers {
				if !testFiles[referrer.Path] {
					continue
				}
				name, ok := s.referrerName(ctx, referrer.UnitID, referrer.Path, referrer.Key)
				if !ok {
					continue
				}
				mappings = append(mappings, testguard.TestMapping{
					Path:          referrer.Path,
					Test:          name,
					ProductionUID: uid,
					InvariantID:   binding.InvariantID,
					Severity:      severity,
					Active:        active,
				})
			}
		}
	}
	return mappings, nil
}

func (s *Server) referrerName(ctx context.Context, unitID, path, key string) (string, bool) {
	uid, ok, err := s.indexes.UIDForKey(ctx, unitID, path, key)
	if err != nil || !ok {
		return "", false
	}
	_, name, _, found, err := s.indexes.FactsForUID(ctx, uid)
	if err != nil || !found || name == "" {
		return "", false
	}
	return name, true
}

// guardLanguage maps extensions to guard languages. Unknown extensions
// are not test shapes.
func guardLanguage(path string) (string, bool) {
	switch filepath.Ext(path) {
	case ".py":
		return testguard.LanguagePython, true
	case ".ts":
		return testguard.LanguageTypeScript, true
	case ".js":
		return testguard.LanguageJavaScript, true
	case ".tsx":
		return testguard.LanguageTSX, true
	default:
		return "", false
	}
}

// isTestFile is the D-207 rule: test/test directories or test affixes.
// Anything else is not a verification test in 0.1, recorded not guessed.
func isTestFile(path string) bool {
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

// headVersion reads a path's HEAD bytes. Missing in HEAD reads as empty
// (added files are quiet); git failures read as empty too — after bytes
// carry the judgment, and an unreadable HEAD cannot fabricate weakness.
func (s *Server) headVersion(ctx context.Context, path string) []byte {
	rel, err := filepath.Rel(s.root, path)
	if err != nil {
		return nil
	}
	out, err := exec.CommandContext(ctx, "git", "-C", s.root, "show", "HEAD:"+filepath.ToSlash(rel)).Output()
	if err != nil {
		return nil
	}
	return out
}
