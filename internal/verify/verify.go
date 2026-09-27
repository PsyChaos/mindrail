// Package verify evaluates staged trees through the shared services
// (MR-017): staged discovery reconciled into rows, global attribution,
// guard over staged test files, bindings with knowledge severity, and the
// gate over all of it. Pure composition — no store of its own, no clock —
// over caller-provided services.
package verify

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/PsyChaos/mindrail/internal/changes"
	"github.com/PsyChaos/mindrail/internal/filesystem"
	"github.com/PsyChaos/mindrail/internal/gate"
	"github.com/PsyChaos/mindrail/internal/git"
	"github.com/PsyChaos/mindrail/internal/index"
	"github.com/PsyChaos/mindrail/internal/knowledge/loader"
	"github.com/PsyChaos/mindrail/internal/knowledge/schema"
	"github.com/PsyChaos/mindrail/internal/testguard"
	schemas "github.com/PsyChaos/mindrail/schemas"
)

// Verdict is one staged evaluation: the gate decision beside the
// knowledge problems that gate it (fatal problems deny before any source
// check — decision D-217).
type Verdict struct {
	Allow      bool
	Denials    []gate.Denial
	Knowledge  []Problem
	FatalKnown bool
}

// Problem is one knowledge record the loader refused.
type Problem struct {
	Path    string
	Code    string
	Message string
	Fatal   bool
}

// Service composes staged verification over shared services.
type Service struct {
	changes *changes.Service
	indexes *index.Store
	guard   *testguard.Service
}

// New builds a Service over the services it composes.
func New(changeService *changes.Service, indexes *index.Store, guard *testguard.Service) (*Service, error) {
	if changeService == nil || indexes == nil || guard == nil {
		return nil, errNeedsServices()
	}
	return &Service{changes: changeService, indexes: indexes, guard: guard}, nil
}

// VerifyStaged evaluates the index: knowledge first (fail-closed), then
// staged discovery reconciled into rows, global attribution, guard over
// staged test files, bindings with knowledge severity, and the gate.
// Unstaged worktree edits are invisible by construction (decision D-216).
// The fixed "verify-staged" operation id converges repeat runs onto one
// NULL change instead of minting a fresh retroactive row per call.
func (s *Service) VerifyStaged(ctx context.Context, projectID, root string, runner git.CommandRunner) (Verdict, error) {
	problems, err := loadProblems(ctx, root)
	if err != nil {
		return Verdict{}, err
	}
	verdict := Verdict{Knowledge: problems}
	for _, problem := range problems {
		if problem.Fatal {
			verdict.FatalKnown = true
			return verdict, nil
		}
	}
	result, err := s.changes.ReconcileStaged(ctx, projectID, root, "", "verify-staged", runner)
	if err != nil {
		return Verdict{}, err
	}
	knowledge, err := loadScopes(ctx, root)
	if err != nil {
		return Verdict{}, err
	}
	staged, err := s.guardStaged(ctx, root, runner, result.Change.ID, knowledge)
	if err != nil {
		return Verdict{}, err
	}
	composed, err := s.compose(ctx, result.Change.ID, knowledge, staged)
	if err != nil {
		return Verdict{}, err
	}
	composed.Knowledge = verdict.Knowledge
	return composed, nil
}

// loadProblems reads knowledge problems without judging source: any fatal
// entry stops the gate before it starts.
func loadProblems(ctx context.Context, root string) ([]Problem, error) {
	fsRoot, err := filesystem.NewRoot(root)
	if err != nil {
		return nil, err
	}
	reg, err := schema.NewRegistry(schemas.KnowledgeFS)
	if err != nil {
		return nil, err
	}
	store, err := loader.New(fsRoot, reg).Load(ctx)
	if err != nil {
		return nil, err
	}
	var out []Problem
	for _, problem := range store.Problems {
		out = append(out, Problem{
			Path:    problem.Path,
			Code:    string(problem.Code),
			Message: problem.Message,
			Fatal:   problem.Fatal,
		})
	}
	return out, nil
}

type knowledgeScope struct {
	entries map[string]scopeEntry
}

type scopeEntry struct {
	severity string
	active   bool
}

func loadScopes(ctx context.Context, root string) (knowledgeScope, error) {
	fsRoot, err := filesystem.NewRoot(root)
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

func anchor(ctx context.Context, indexes *index.Store, knowledge knowledgeScope, removedUID string, candidates []string) (string, string, bool) {
	var uids []string
	uids = append(uids, removedUID)
	for _, key := range candidates {
		resolved, err := indexes.KeyToUIDs(ctx, key)
		if err != nil {
			continue
		}
		uids = append(uids, resolved...)
	}
	for _, uid := range uids {
		bound, err := indexes.BindingsForUID(ctx, uid)
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

// stagedDrift names staged files outside each baselined task's declared
// scope: the MR-008 drift rule applied to the commit under judgment.
// Tasks without baselines declare nothing and never drift; the global
// attribution above covers ownership, this covers scope.
func (s *Service) stagedDrift(ctx context.Context, changeID string) ([]changes.Finding, error) {
	scopes, err := s.changes.Store().ReadAllBaselines(ctx)
	if err != nil {
		return nil, err
	}
	files, err := s.changes.Store().ReadChangeFiles(ctx, changeID)
	if err != nil {
		return nil, err
	}
	var out []changes.Finding
	for taskID, scope := range scopes {
		inScope := map[string]bool{}
		for _, path := range scope {
			inScope[path] = true
		}
		for _, file := range files {
			if inScope[file.Path] {
				continue
			}
			out = append(out, changes.ScopeDrift(taskID, changeID, file.Path,
				file.Kind, "staged outside "+strconv.Itoa(len(scope))+" baseline paths", scope))
		}
	}
	return out, nil
}

// guardStaged runs the guard over staged test files: before bytes from
// HEAD, after bytes from the index, mapping from bound-uid referrers
// resolved to test names.
func (s *Service) guardStaged(ctx context.Context, root string, runner git.CommandRunner, changeID string, knowledge knowledgeScope) ([]testguard.Finding, error) {
	files, err := s.changes.Store().ReadChangeFiles(ctx, changeID)
	if err != nil {
		return nil, err
	}
	var deltas []testguard.FileDelta
	for _, file := range files {
		language, ok := guardLanguage(file.Path)
		if !ok || !isTestFile(file.Path) {
			continue
		}
		rel, err := filepath.Rel(root, file.Path)
		if err != nil {
			continue
		}
		rel = filepath.ToSlash(rel)
		before, _, err := git.ShowHEAD(ctx, runner, root, rel)
		if err != nil {
			return nil, err
		}
		after, _, err := git.ShowStaged(ctx, runner, root, rel)
		if err != nil {
			return nil, err
		}
		deltas = append(deltas, testguard.FileDelta{
			Path:     file.Path,
			Language: language,
			Before:   before,
			After:    after,
		})
	}
	if len(deltas) == 0 {
		return nil, nil
	}
	mappings, err := s.guardMappings(ctx, changeID, knowledge, deltas)
	if err != nil {
		return nil, err
	}
	result, err := s.guard.Evaluate(ctx, testguard.Request{
		Files:    deltas,
		Mappings: mappings,
		Trigger:  testguard.TriggerStaged,
	})
	if err != nil {
		return nil, err
	}
	return result.Findings, nil
}

// guardMappings resolves bound-uid referrers inside the changed test files
// to (path, test, production, invariant) tuples the guard triggers on. It
// is the half guardStaged and guardRange share; only the byte source
// differs.
func (s *Service) guardMappings(ctx context.Context, changeID string, knowledge knowledgeScope, deltas []testguard.FileDelta) ([]testguard.TestMapping, error) {
	testFiles := map[string]bool{}
	for _, delta := range deltas {
		testFiles[delta.Path] = true
	}
	symbols, err := s.changes.Store().ReadChangeSymbols(ctx, changeID)
	if err != nil {
		return nil, err
	}
	var mappings []testguard.TestMapping
	for _, row := range symbols {
		if row.UID == "" {
			continue
		}
		bindings, err := s.indexes.BindingsWithStatus(ctx, row.UID)
		if err != nil {
			return nil, err
		}
		for _, binding := range bindings {
			severity, active := knowledge.scopeOf(binding.InvariantID)
			referrers, err := s.indexes.ReferrersOfUID(ctx, row.UID)
			if err != nil {
				return nil, err
			}
			for _, referrer := range referrers {
				if !testFiles[referrer.Path] {
					continue
				}
				name, ok := referrerName(ctx, s.indexes, referrer.UnitID, referrer.Path, referrer.Key)
				if !ok {
					continue
				}
				mappings = append(mappings, testguard.TestMapping{
					Path:          referrer.Path,
					Test:          name,
					ProductionUID: row.UID,
					InvariantID:   binding.InvariantID,
					Severity:      severity,
					Active:        active,
				})
			}
		}
	}
	return mappings, nil
}

func referrerName(ctx context.Context, indexes *index.Store, unitID, path, key string) (string, bool) {
	uid, ok, err := indexes.UIDForKey(ctx, unitID, path, key)
	if err != nil || !ok {
		return "", false
	}
	_, name, _, found, err := indexes.FactsForUID(ctx, uid)
	if err != nil || !found || name == "" {
		return "", false
	}
	return name, true
}

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

func errNeedsServices() error {
	return errors.New("verify: needs a change service, an index store and a guard")
}
