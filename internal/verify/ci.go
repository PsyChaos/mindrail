// Package verify's CI path (MR-018): the same composition as VerifyStaged
// over a committed merge-base range instead of the index. Knowledge first
// (fail-closed), then range reconciliation into rows, global attribution,
// the guard over base-vs-head test bytes, bindings with knowledge
// severity, and the gate. Pure composition — no store of its own.
package verify

import (
	"context"
	"path/filepath"
	"strconv"

	"github.com/PsyChaos/mindrail/internal/app"
	"github.com/PsyChaos/mindrail/internal/gate"
	"github.com/PsyChaos/mindrail/internal/git"
	"github.com/PsyChaos/mindrail/internal/testguard"
)

// VerifyCI evaluates a committed range: knowledge first (fail-closed),
// then base/head resolution, merge-base computation, range reconciliation
// into rows, and the shared gate. baseRev empty selects the documented
// default chain (decision D-223); headRev empty means HEAD. Resolution and
// merge-base failures refuse with usage codes before any source check — a
// range that cannot be named must never read as a clean one.
func (s *Service) VerifyCI(ctx context.Context, projectID, root string, runner git.CommandRunner, baseRev, headRev string) (Verdict, error) {
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
	head := headRev
	if head == "" {
		head = "HEAD"
	}
	headSHA, err := git.ResolveRev(ctx, runner, root, head)
	if err != nil {
		return Verdict{}, rangeUsage("head revision "+head+" does not resolve",
			"The range under judgment cannot be named, so nothing about it can be certified.",
			"Pass an existing commit as --head, or omit it to judge HEAD.", err)
	}
	var baseSHA string
	baseDefaulted := baseRev == ""
	if baseRev != "" {
		baseSHA, err = git.ResolveRev(ctx, runner, root, baseRev)
		if err != nil {
			return Verdict{}, rangeUsage("base revision "+baseRev+" does not resolve",
				"The range under judgment cannot be named, so nothing about it can be certified.",
				"Pass an existing commit or branch as --base.", err)
		}
	} else {
		baseSHA, err = git.DefaultBase(ctx, runner, root)
		if err != nil {
			return Verdict{}, rangeUsage("no default base resolves",
				"The range under judgment cannot be named, so nothing about it can be certified.",
				"Pass --base explicitly.", err)
		}
	}
	mergeBase, err := git.MergeBase(ctx, runner, root, baseSHA, headSHA)
	if err != nil {
		return Verdict{}, rangeUsage("merge-base of the base and head does not resolve",
			"Histories without a common ancestor have no range to judge.",
			"Pass a --base that shares history with --head.", err)
	}
	if mergeBase == headSHA && baseSHA != headSHA {
		return Verdict{}, rangeUsage("base "+shortSHA(baseSHA)+" is a descendant of head "+shortSHA(headSHA)+" (inverted range)",
			"A range that ends where it starts judges nothing; certifying it green would be a lie.",
			"Swap --base and --head, or pass a base that is an ancestor of head.", nil)
	}
	if baseSHA == headSHA && baseDefaulted {
		return Verdict{}, rangeUsage("default base "+shortSHA(baseSHA)+" equals head (nothing to judge)",
			"An empty default range would certify any content green — including a bypassed commit.",
			"Push commits the base does not contain, or pass explicit --base and --head.", nil)
	}
	if err := refuseDeskMismatch(ctx, runner, root, headSHA); err != nil {
		return Verdict{}, err
	}
	result, err := s.changes.ReconcileRange(ctx, projectID, root, mergeBase, headSHA, runner)
	if err != nil {
		return Verdict{}, err
	}
	knowledge, err := loadScopes(ctx, root)
	if err != nil {
		return Verdict{}, err
	}
	ranged, err := s.guardRange(ctx, root, runner, mergeBase, headSHA, result.Change.ID, knowledge)
	if err != nil {
		return Verdict{}, err
	}
	composed, err := s.compose(ctx, result.Change.ID, knowledge, ranged)
	if err != nil {
		return Verdict{}, err
	}
	composed.Knowledge = verdict.Knowledge
	return composed, nil
}

// compose runs the gate over one reconciled change: global attribution,
// bindings with knowledge severity, ambiguities, the caller's guard
// findings, and staged-scope drift. It is the half VerifyStaged and
// VerifyCI share (decision D-225); only the file source differs.
func (s *Service) compose(ctx context.Context, changeID string, knowledge knowledgeScope, guard []testguard.Finding) (Verdict, error) {
	attributed, err := s.changes.AttributeChanges(ctx, []string{changeID})
	if err != nil {
		return Verdict{}, err
	}
	var input gate.Input
	for _, symbol := range attributed.Symbols {
		if symbol.Finding != nil {
			input.Attribution = append(input.Attribution, *symbol.Finding)
		}
		if symbol.UID == "" {
			continue
		}
		bindings, err := s.indexes.BindingsWithStatus(ctx, symbol.UID)
		if err != nil {
			return Verdict{}, err
		}
		for _, binding := range bindings {
			severity, active := knowledge.scopeOf(binding.InvariantID)
			input.Bindings = append(input.Bindings, gate.InvariantBlock{
				InvariantID: binding.InvariantID,
				UID:         symbol.UID,
				Status:      binding.Status,
				Severity:    severity,
				Active:      active,
			})
		}
		ambiguities, err := s.indexes.ListAmbiguitiesForUID(ctx, symbol.UID)
		if err != nil {
			return Verdict{}, err
		}
		for _, ambiguity := range ambiguities {
			invariant, severity, active := anchor(ctx, s.indexes, knowledge, symbol.UID, ambiguity.CandidateKeys)
			input.Ambiguities = append(input.Ambiguities, gate.Ambiguity{
				Key:         ambiguity.RemovedKey,
				Candidates:  ambiguity.CandidateKeys,
				InvariantID: invariant,
				Severity:    severity,
				Active:      active,
			})
		}
	}
	input.Guard = guard
	drifted, err := s.stagedDrift(ctx, changeID)
	if err != nil {
		return Verdict{}, err
	}
	input.Attribution = append(input.Attribution, drifted...)
	decision, err := gate.New().Evaluate(input)
	if err != nil {
		return Verdict{}, err
	}
	return Verdict{Allow: decision.Allow, Denials: decision.Denials}, nil
}

// guardRange runs the guard over ranged test files: before bytes from the
// merge-base SHA, after bytes from head, mapping from bound-uid referrers
// resolved to test names — the staged rule with committed sides.
func (s *Service) guardRange(ctx context.Context, root string, runner git.CommandRunner, mergeBaseSHA, headSHA, changeID string, knowledge knowledgeScope) ([]testguard.Finding, error) {
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
		before, _, err := git.ShowRev(ctx, runner, root, mergeBaseSHA, rel)
		if err != nil {
			return nil, err
		}
		after, _, err := git.ShowRev(ctx, runner, root, headSHA, rel)
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
		Trigger:  testguard.TriggerCI,
	})
	if err != nil {
		return nil, err
	}
	return result.Findings, nil
}

// refuseDeskMismatch enforces the CI precondition the disk-reading half
// of the composition needs: the worktree must be a clean checkout of the
// judged head. Indexing and knowledge load from desk bytes, so a desk that
// is behind head, ahead of it, or dirty would have the verdict certify
// bytes it never read. Fresh clones satisfy this by construction; anything
// else is a structured refusal, never a silent judgment.
func refuseDeskMismatch(ctx context.Context, runner git.CommandRunner, root, headSHA string) error {
	worktreeHEAD, err := git.ResolveRev(ctx, runner, root, "HEAD")
	if err != nil || worktreeHEAD != headSHA {
		return rangeUsage("worktree checkout does not match judged head "+shortSHA(headSHA),
			"Indexing and knowledge read desk bytes; a desk on another commit would certify unread content.",
			"Check out "+headSHA+", or re-run with a --head that names the checkout.", err)
	}
	dirt, err := git.DeskDirt(ctx, runner, root)
	if err != nil {
		return rangeUsage("worktree dirt cannot be listed",
			"A desk that cannot be inspected cannot be certified clean.",
			"Re-run once git answers again.", err)
	}
	if len(dirt) > 0 {
		named := dirt
		if len(named) > 5 {
			named = named[:5]
		}
		return rangeUsage("worktree is dirty ("+joinDirt(named, len(dirt))+")",
			"Indexing and knowledge read desk bytes; uncommitted edits would certify unread content.",
			"Commit, stash or discard the listed paths — or judge a clean checkout — then re-run.", nil)
	}
	return nil
}

// joinDirt names the first paths plus the remainder count.
func joinDirt(named []string, total int) string {
	s := ""
	for i, path := range named {
		if i > 0 {
			s += ", "
		}
		s += path
	}
	if total > len(named) {
		s += ", ... (+" + strconv.Itoa(total-len(named)) + " more)"
	}
	return s
}

func shortSHA(sha string) string {
	if len(sha) > 7 {
		return sha[:7]
	}
	return sha
}

// caller mistyped or omitted a revision, and the remedy names the fix. The
// underlying git answer rides the message — it is what names the tried
// candidates — alongside the structured code. No new code (decision D-227).
func rangeUsage(message, why, remedy string, cause error) error {
	text := "verify --ci refuses: " + message
	if cause != nil {
		text += ": " + cause.Error()
	}
	err := app.NewError(
		app.CodeCommandLineInvalid,
		app.KindUsage,
		text,
		why,
		remedy,
	)
	if cause != nil {
		return err.WithCause(cause)
	}
	return err
}
