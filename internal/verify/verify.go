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
	"strings"

	"github.com/PsyChaos/mindrail/internal/changes"
	"github.com/PsyChaos/mindrail/internal/filesystem"
	"github.com/PsyChaos/mindrail/internal/gate"
	"github.com/PsyChaos/mindrail/internal/git"
	"github.com/PsyChaos/mindrail/internal/index"
	"github.com/PsyChaos/mindrail/internal/knowledge/loader"
	"github.com/PsyChaos/mindrail/internal/knowledge/record"
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

// stagedOperationID is versioned because releases before exact projection
// replacement accumulated rows under `verify-staged`. Reusing that identity
// would let a file-only equality shortcut accept a legacy partial symbol set.
const stagedOperationID = "verify-staged-v2"

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
// The fixed staged-verification operation id converges repeat runs onto one
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
	knowledge, err := loadScopes(ctx, root)
	if err != nil {
		return Verdict{}, err
	}
	files, err := s.changes.Store().DiscoverFilesStaged(ctx, runner, root)
	if err != nil {
		return Verdict{}, err
	}
	// Capture baseline referrers before staged indexing replaces a weakened or
	// deleted test file's facts.
	staged, err := s.guardStaged(ctx, root, runner, knowledge, files)
	if err != nil {
		return Verdict{}, err
	}
	result, err := s.changes.ReconcileStagedSnapshot(ctx, projectID, root, "", stagedOperationID, files, runner)
	if err != nil {
		return Verdict{}, err
	}
	composed, err := s.compose(ctx, result.Change.ID, knowledge, staged, true)
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
	records []record.Invariant
}

type scopeEntry struct {
	severity string
	active   bool
	level    string
	target   string
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
		var body record.Invariant
		if err := json.Unmarshal(ref.Body, &body); err != nil || body.Severity == "" {
			scope.entries[ref.ID] = scopeEntry{severity: "CRITICAL", active: true}
			continue
		}
		scope.records = append(scope.records, body)
		scope.entries[ref.ID] = scopeEntry{severity: string(body.Severity), active: body.Status == record.StatusActive,
			level: string(body.Scope.Level), target: body.Scope.Target}
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

// stagedDrift attributes each staged file to durable task scopes by exact path.
// One owner is quiet, no owners fails closed as unregistered scope, and
// multiple owners fail as ambiguous. Completed task scopes remain valid owners
// because the normal workflow completes work before the commit hook runs.
func (s *Service) stagedDrift(ctx context.Context, changeID string, requireOwner bool) ([]changes.Finding, error) {
	if !requireOwner {
		return nil, nil
	}
	scopes, err := s.changes.Store().ReadAllBaselines(ctx)
	if err != nil {
		return nil, err
	}
	states, err := s.changes.Store().ReadTaskStates(ctx)
	if err != nil {
		return nil, err
	}
	files, err := s.changes.Store().ReadChangeFiles(ctx, changeID)
	if err != nil {
		return nil, err
	}
	symbols, err := s.changes.Store().ReadChangeSymbols(ctx, changeID)
	if err != nil {
		return nil, err
	}
	represented := map[string]bool{}
	for _, symbol := range symbols {
		if symbol.UID == "" {
			continue
		}
		path, found, err := s.indexes.PathForUID(ctx, symbol.UID)
		if err != nil {
			return nil, err
		}
		if found {
			represented[path] = true
		}
	}
	var out []changes.Finding
	for _, file := range files {
		// Symbol attribution already fails closed for parsable files. Add a
		// file-level finding only where no symbol can carry ownership, such as
		// documentation or unsupported source.
		if represented[file.Path] {
			continue
		}
		owners := changes.PreferredTaskOwners(scopes, states, file.Path)
		switch len(owners) {
		case 0:
			out = append(out, changes.UnregisteredFile(changeID, file.Path, file.Kind))
		case 1:
			// Exactly one declared owner: no cross-task comparison is needed.
		default:
			out = append(out, changes.AmbiguousFile(changeID, file.Path, owners))
		}
	}
	return out, nil
}

// guardStaged runs the guard over staged test files: before bytes from
// HEAD, after bytes from the index, mapping from bound-uid referrers
// resolved to test names.
func (s *Service) guardStaged(ctx context.Context, root string, runner git.CommandRunner, knowledge knowledgeScope, files []changes.FileChange) ([]testguard.Finding, error) {
	var deltas []testguard.FileDelta
	baselinePaths := map[string]string{}
	for _, file := range files {
		baselinePath := file.Path
		if file.OldPath != "" {
			baselinePath = file.OldPath
		}
		language, ok := guardLanguage(file.Path)
		if !ok {
			language, ok = guardLanguage(baselinePath)
		}
		if !ok || (!isTestFile(file.Path) && !isTestFile(baselinePath)) {
			continue
		}
		beforeRel, err := filepath.Rel(root, baselinePath)
		if err != nil {
			continue
		}
		beforeRel = filepath.ToSlash(beforeRel)
		before, _, err := git.ShowHEAD(ctx, runner, root, beforeRel)
		if err != nil {
			return nil, err
		}
		afterRel, err := filepath.Rel(root, file.Path)
		if err != nil {
			continue
		}
		after, _, err := git.ShowStaged(ctx, runner, root, filepath.ToSlash(afterRel))
		if err != nil {
			return nil, err
		}
		deltas = append(deltas, testguard.FileDelta{
			Path:     file.Path,
			Language: language,
			Before:   before,
			After:    after,
		})
		baselinePaths[file.Path] = baselinePath
	}
	if len(deltas) == 0 {
		return nil, nil
	}
	mappings, err := s.guardMappings(ctx, root, knowledge, deltas, baselinePaths)
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
func (s *Service) guardMappings(ctx context.Context, root string, knowledge knowledgeScope, deltas []testguard.FileDelta, baselinePaths map[string]string) ([]testguard.TestMapping, error) {
	testFiles := make([]string, 0, len(deltas))
	destinations := make(map[string]string, len(deltas))
	for _, delta := range deltas {
		baseline := baselinePaths[delta.Path]
		if baseline == "" {
			baseline = delta.Path
		}
		testFiles = append(testFiles, baseline)
		destinations[baseline] = delta.Path
	}
	refs, err := s.indexes.GuardReferencesInFiles(ctx, testFiles)
	if err != nil {
		return nil, err
	}
	mappings := make([]testguard.TestMapping, 0, len(refs))
	seen := map[string]bool{}
	validTests := make(map[string]map[string]bool, len(deltas))
	for _, delta := range deltas {
		tests, err := s.guard.BaselineTests(ctx, delta)
		if err != nil {
			return nil, err
		}
		validTests[delta.Path] = tests
	}
	appendMapping := func(path, test, uid, invariantID string) {
		if !validTests[path][test] {
			return
		}
		severity, active := knowledge.scopeOf(invariantID)
		if _, known := knowledge.entries[invariantID]; !known {
			return
		}
		key := path + "\x00" + test + "\x00" + uid + "\x00" + invariantID
		if seen[key] {
			return
		}
		seen[key] = true
		mappings = append(mappings, testguard.TestMapping{Path: path, Test: test,
			ProductionUID: uid, InvariantID: invariantID, Severity: severity, Active: active})
	}
	for _, ref := range refs {
		path := destinations[ref.Path]
		if path == "" {
			path = ref.Path
		}
		appendMapping(path, ref.Test, ref.ProductionUID, ref.InvariantID)
	}

	type parsedRef struct {
		path, baseline, test, targetModule, targetText, targetLanguage string
		targetRelative                                                 bool
		targetKeys                                                     []string
	}
	var parsed []parsedRef
	var targetKeys []string
	for _, delta := range deltas {
		baseline := baselinePaths[delta.Path]
		if baseline == "" {
			baseline = delta.Path
		}
		rows, err := s.changes.ExtractGuardReferences(ctx, root, baseline, delta.Before)
		if err != nil {
			return nil, err
		}
		for _, row := range rows {
			var keys []string
			if row.TargetKey != "" {
				keys = append(keys, row.TargetKey)
				targetKeys = append(targetKeys, row.TargetKey)
			}
			if row.TargetKey != "" && baseline != delta.Path {
				moved, ok, err := s.changes.RequalifyGuardKey(ctx, root, delta.Path, row.TargetKey)
				if err != nil {
					return nil, err
				}
				if ok && moved != row.TargetKey {
					keys = append(keys, moved)
					targetKeys = append(targetKeys, moved)
				}
			}
			parsed = append(parsed, parsedRef{path: delta.Path, test: row.Test,
				baseline: baseline, targetModule: row.TargetModule,
				targetText: row.TargetText, targetLanguage: row.TargetLanguage,
				targetRelative: row.TargetRelative, targetKeys: keys})
		}
	}
	bindings, err := s.indexes.BindingsForLogicalKeys(ctx, targetKeys)
	if err != nil {
		return nil, err
	}
	byKey := map[string][]index.LogicalBinding{}
	for _, binding := range bindings {
		byKey[binding.LogicalKey] = append(byKey[binding.LogicalKey], binding)
	}
	for _, ref := range parsed {
		for _, key := range ref.targetKeys {
			for _, binding := range byKey[key] {
				appendMapping(ref.path, ref.test, binding.ProductionUID, binding.InvariantID)
			}
		}
		for invariantID, entry := range knowledge.entries {
			if entry.level != string(record.ScopeSymbol) || entry.target == "" || ref.targetText == "" || ref.targetModule == "" {
				continue
			}
			targetFile, name := entry.target, ""
			if at := strings.LastIndex(targetFile, ":"); at >= 0 {
				name, targetFile = targetFile[at+1:], targetFile[:at]
			}
			if dot := strings.LastIndex(name, "."); dot >= 0 {
				name = name[dot+1:]
			}
			if name != ref.targetText || !importTargetsFile(root, ref.baseline, ref.targetLanguage,
				ref.targetModule, ref.targetRelative, targetFile) {
				continue
			}
			bindings, err := s.indexes.ListBindingsForInvariant(ctx, invariantID)
			if err != nil {
				return nil, err
			}
			for _, binding := range bindings {
				appendMapping(ref.path, ref.test, binding.UID, invariantID)
			}
		}
	}
	return mappings, nil
}

func importTargetsFile(root, baselinePath, language, module string, relative bool, targetFile string) bool {
	if module == "" || targetFile == "" {
		return false
	}
	target := strings.TrimSuffix(filepath.ToSlash(filepath.Clean(targetFile)), filepath.Ext(targetFile))
	relBaseline, err := filepath.Rel(root, baselinePath)
	if err != nil || strings.HasPrefix(filepath.ToSlash(relBaseline), "../") {
		return false
	}
	baseDir := filepath.ToSlash(filepath.Dir(relBaseline))
	cleanCandidate := func(candidate string) string {
		candidate = filepath.ToSlash(filepath.Clean(filepath.FromSlash(candidate)))
		return strings.TrimSuffix(candidate, filepath.Ext(candidate))
	}
	switch language {
	case testguard.LanguagePython:
		if relative {
			dots := len(module) - len(strings.TrimLeft(module, "."))
			if dots == 0 {
				return false
			}
			anchor := baseDir
			for level := 1; level < dots; level++ {
				anchor = filepath.ToSlash(filepath.Dir(filepath.FromSlash(anchor)))
			}
			rest := strings.ReplaceAll(strings.TrimLeft(module, "."), ".", "/")
			return target == cleanCandidate(filepath.ToSlash(filepath.Join(filepath.FromSlash(anchor), filepath.FromSlash(rest))))
		}
		absolute := cleanCandidate(strings.ReplaceAll(module, ".", "/"))
		local := cleanCandidate(filepath.ToSlash(filepath.Join(filepath.FromSlash(baseDir), filepath.FromSlash(absolute))))
		return target == absolute || target == local
	case testguard.LanguageJavaScript, testguard.LanguageTypeScript, testguard.LanguageTSX:
		module = filepath.ToSlash(module)
		if relative || strings.HasPrefix(module, ".") {
			local := cleanCandidate(filepath.ToSlash(filepath.Join(filepath.FromSlash(baseDir), filepath.FromSlash(module))))
			return target == local
		}
		return target == cleanCandidate(module)
	default:
		return false
	}
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
