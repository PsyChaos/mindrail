package completion

import (
	"context"
	"os"
	"path/filepath"

	"github.com/PsyChaos/mindrail/internal/changes"
	"github.com/PsyChaos/mindrail/internal/git"
	"github.com/PsyChaos/mindrail/internal/testguard"
)

// Rehydrate only policy from the current, validated store. The persisted
// snapshot keeps exact proof edges, never a stale severity or active flag.
func rehydrateMappings(saved []changes.ProofMapping, knowledge knowledgeScope) []testguard.TestMapping {
	mappings := make([]testguard.TestMapping, 0, len(saved))
	for _, mapping := range saved {
		severity, active := knowledge.scopeOf(mapping.InvariantID)
		mappings = append(mappings, testguard.TestMapping{
			Path: mapping.Path, Test: mapping.Test, ProductionUID: mapping.ProductionUID,
			InvariantID: mapping.InvariantID, Severity: severity, Active: active,
		})
	}
	return mappings
}

// guardFiles builds one testguard evaluation from the task's changed test
// files: before bytes from git HEAD (missing in HEAD reads as added and
// quiet), after bytes from disk (missing reads as removed), mapping from
// bound-uid referrers resolved to names. Files that cannot be judged
// (unknown language, unreadable after, unresolvable names) are skipped,
// never invented.
func (s *Service) guardFiles(ctx context.Context, changeID string, uids []string, knowledge knowledgeScope, prior []testguard.TestMapping) ([]testguard.Finding, error) {
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
		if !ok || !changes.IsVerificationTest(file.Path) {
			continue
		}
		after, err := os.ReadFile(file.Path)
		if err != nil {
			if !os.IsNotExist(err) {
				continue
			}
			after = nil
		}
		before, err := s.headVersion(ctx, file.Path)
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
	mappings, err := s.guardMappings(ctx, knowledge, uids, deltas)
	if err != nil {
		return nil, err
	}
	mappings = append(mappings, prior...)
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
func (s *Service) guardMappings(ctx context.Context, knowledge knowledgeScope, uids []string, deltas []testguard.FileDelta) ([]testguard.TestMapping, error) {
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
				if !changes.IsVerificationTest(referrer.Path) || !testFiles[referrer.Path] {
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

func (s *Service) referrerName(ctx context.Context, unitID, path, key string) (string, bool) {
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

// headVersion reads the committed bytes through the shared Git adapter.
// Absence is a new file; an unreadable HEAD is not proof of absence.
func (s *Service) headVersion(ctx context.Context, path string) ([]byte, error) {
	rel, err := filepath.Rel(s.root, path)
	if err != nil {
		return nil, err
	}
	out, _, err := git.ShowHEAD(ctx, git.NewExecRunner(), s.root, filepath.ToSlash(rel))
	return out, err
}
