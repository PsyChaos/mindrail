package index

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"

	"github.com/PsyChaos/mindrail/internal/index/parser"
	"github.com/PsyChaos/mindrail/internal/index/snapshot"
	"github.com/PsyChaos/mindrail/internal/storage"
)

// Timing is local instrumentation for D-86. It is deliberately excluded from
// JSON and durable facts; cache hits have zero parse/extract time.
type Timing struct {
	Parse   time.Duration `json:"-"`
	Extract time.Duration `json:"-"`
	Write   time.Duration `json:"-"`
}

type IndexResult struct {
	State      FileIndexState
	Skipped    bool
	Stale      bool
	Timing     Timing          `json:"-"`
	WriteStats storage.TxStats `json:"-"`
}

type Indexer struct {
	store    *Store
	registry *parser.Registry
	cache    *snapshot.Cache
	extract  func(context.Context, parser.SyntaxAdapter, parser.SourceFile) (parser.Facts, error)
	recheck  func(string, sourceVersion) (sourceVersion, bool, error)
}

func NewIndexer(store *Store, registry *parser.Registry, cache *snapshot.Cache) *Indexer {
	return &Indexer{store: store, registry: registry, cache: cache, extract: parser.Extract, recheck: sourceStillMatches}
}

type sourceVersion struct {
	info  os.FileInfo
	hash  string
	bytes []byte
}

func contentHash(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func readSource(path string) (sourceVersion, error) {
	before, err := os.Lstat(path)
	if err != nil {
		return sourceVersion{}, err
	}
	if !before.Mode().IsRegular() {
		return sourceVersion{}, invalidInput("index source must be a regular file, not a symlink or device")
	}
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return sourceVersion{}, err
	}
	if resolved != path {
		return sourceVersion{}, invalidInput("index source path traverses a symlink")
	}
	bytes, err := os.ReadFile(path)
	if err != nil {
		return sourceVersion{}, err
	}
	after, err := os.Lstat(path)
	if err != nil {
		return sourceVersion{}, err
	}
	if !os.SameFile(before, after) || before.Size() != after.Size() || !before.ModTime().Equal(after.ModTime()) {
		return sourceVersion{}, errSourceChanged
	}
	return sourceVersion{info: after, hash: contentHash(bytes), bytes: bytes}, nil
}

var errSourceChanged = errors.New("source changed during indexing")

func sourceStillMatches(path string, captured sourceVersion) (sourceVersion, bool, error) {
	current, err := readSource(path)
	if err != nil {
		if errors.Is(err, errSourceChanged) || errors.Is(err, os.ErrNotExist) {
			return sourceVersion{}, false, nil
		}
		return sourceVersion{}, false, err
	}
	return current, os.SameFile(captured.info, current.info) && captured.hash == current.hash, nil
}

// IndexFile registers target bytes, computes path-independent facts outside a
// SQLite transaction, and CAS-replaces only this file's durable rows. A stale
// attempt never deletes newer facts. Cancellation leaves its pending target
// hash for resume instead of recording a failed parse.
//
// projectID scopes every identity this call mints or reuses: callers resolve
// it from the workspace (one explicit parameter, never ambient), and an empty
// project is refused before anything is read. Optional rename hints corroborate
// cross-file moves; without them matching is structural-only.
func (i *Indexer) IndexFile(ctx context.Context, projectID string, unit ProjectUnit, path string, hints ...RenameHint) (IndexResult, error) {
	if err := ctx.Err(); err != nil {
		return IndexResult{}, err
	}
	if i == nil || i.store == nil || i.registry == nil || i.cache == nil {
		return IndexResult{}, invalidInput("indexer needs a store, parser registry and snapshot cache")
	}
	if projectID == "" {
		return IndexResult{}, invalidInput("indexing needs a project ID for identity scope")
	}
	if unit.ID == "" || !isCleanAbsolutePath(unit.Path) || !isCleanAbsolutePath(path) || !pathInRoot(unit.Path, path) || path == unit.Path {
		return IndexResult{}, invalidInput("source path must be clean, absolute and inside its project unit")
	}
	adapter, ok := i.registry.Lookup(path)
	if !ok {
		return IndexResult{}, UnsupportedLanguage(path)
	}
	observed, err := i.store.ReadFileState(ctx, path)
	if err != nil {
		return IndexResult{}, err
	}
	source, err := readSource(path)
	if err != nil {
		return IndexResult{}, err
	}
	if observed.Exists && observed.State.UnitID == unit.ID && observed.State.Language == adapter.Info().Language && observed.State.State == StateIndexed && observed.State.ContentHash == source.hash {
		return IndexResult{State: observed.State, Skipped: true}, nil
	}
	if _, same, err := i.recheck(path, source); err != nil {
		return IndexResult{}, err
	} else if !same {
		return IndexResult{State: observed.State, Stale: true}, nil
	}
	registered, applied, err := i.store.RegisterFileCAS(ctx, observed, unit.ID, path, adapter.Info().Language, source.hash)
	if err != nil {
		return IndexResult{}, err
	}
	if !applied {
		return IndexResult{State: registered.State, Stale: true}, nil
	}
	result := IndexResult{State: registered.State}
	var timing Timing
	facts, parseErr := i.cache.GetOrCompute(ctx, adapter.Info(), source.bytes, func(ctx context.Context, bytes []byte) (parser.Facts, error) {
		wrapped := &timedAdapter{SyntaxAdapter: adapter}
		started := time.Now()
		parsed, err := i.extract(ctx, wrapped, parser.SourceFile{Content: bytes})
		elapsed := time.Since(started)
		timing.Parse = wrapped.parseTime
		timing.Extract = elapsed - wrapped.parseTime
		return parsed, err
	})
	result.Timing = timing
	if err := ctx.Err(); err != nil {
		return result, err
	}
	if errors.Is(parseErr, context.Canceled) || errors.Is(parseErr, context.DeadlineExceeded) {
		return result, parseErr
	}
	state := StateIndexed
	lastError := ""
	if parseErr != nil {
		state = StateFailed
		lastError = parseErr.Error()
	}
	if _, same, err := i.recheck(path, source); err != nil {
		return result, err
	} else if !same {
		result.Stale = true
		return result, nil
	}
	fileFacts := mapFacts(projectID, unit, path, adapter.Info().Language, source.hash, state, lastError, facts, hints)
	started := time.Now()
	completed, stats, applied, err := i.store.ReplaceFileFactsCAS(ctx, registered, fileFacts)
	result.Timing.Write = time.Since(started)
	result.WriteStats = stats
	if err != nil {
		return result, err
	}
	if !applied {
		result.State = completed.State
		result.Stale = true
		return result, nil
	}
	result.State = completed.State
	current, same, err := i.recheck(path, source)
	if err != nil || !same {
		result.Stale = true
		if err == nil && current.hash != "" {
			newState, _, err := i.store.RegisterFileCAS(ctx, completed, unit.ID, path, adapter.Info().Language, current.hash)
			if err != nil {
				return result, err
			}
			result.State = newState.State
		} else if !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
			newState, _, invalidateErr := i.store.InvalidateFileCAS(ctx, completed)
			if invalidateErr != nil {
				return result, errors.Join(err, invalidateErr)
			}
			result.State = newState.State
		}
		if err != nil {
			return result, err
		}
	}
	if parseErr != nil {
		return result, ParseFailed(path, parseErr)
	}
	return result, nil
}

type timedAdapter struct {
	parser.SyntaxAdapter
	parseTime time.Duration
}

func (a *timedAdapter) Parse(ctx context.Context, src parser.SourceFile) (*parser.SyntaxSnapshot, error) {
	started := time.Now()
	snapshot, err := a.SyntaxAdapter.Parse(ctx, src)
	a.parseTime += time.Since(started)
	return snapshot, err
}

func qualifiedKey(unit ProjectUnit, path, local string) string {
	if local == "" {
		return ""
	}
	rel, _ := filepath.Rel(unit.Path, path)
	key, _ := json.Marshal([2]string{filepath.ToSlash(rel), local})
	return string(key)
}

// ExtractCurrent parses source bytes without touching the database: change
// discovery needs current facts beside stored ones before indexing replaces
// them. Unsupported languages return no symbols and no error — there is no
// delta to compute, and that is a terminal answer, not a failure. Malformed
// scope is refused; extraction errors propagate (a delta from a tree the
// parser disowned would be fabricated).
func (i *Indexer) ExtractCurrent(ctx context.Context, unit ProjectUnit, path string, content []byte) ([]Symbol, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if i == nil || i.registry == nil {
		return nil, invalidInput("extraction needs a parser registry")
	}
	if unit.ID == "" || !isCleanAbsolutePath(unit.Path) || !isCleanAbsolutePath(path) || !pathInRoot(unit.Path, path) || path == unit.Path {
		return nil, invalidInput("source path must be clean, absolute and inside its project unit")
	}
	adapter, ok := i.registry.Lookup(path)
	if !ok {
		return nil, nil
	}
	facts, err := i.extract(ctx, adapter, parser.SourceFile{Content: content})
	if err != nil {
		return nil, err
	}
	symbols := make([]Symbol, 0, len(facts.Symbols))
	for _, sym := range facts.Symbols {
		symbols = append(symbols, Symbol{LogicalKey: qualifiedKey(unit, path, sym.LocalKey), Kind: sym.Kind, Name: sym.Name,
			Container: qualifiedKey(unit, path, sym.ContainerLocalKey), StartLine: int(sym.Range.StartRow), StartCol: int(sym.Range.StartColumn),
			EndLine: int(sym.Range.EndRow), EndCol: int(sym.Range.EndColumn), SignatureHash: sym.SignatureHash, BodyHash: sym.BodyHash, StructureHash: sym.StructureHash})
	}
	return symbols, nil
}

func mapFacts(projectID string, unit ProjectUnit, path, language, hash string, state FileState, lastError string, facts parser.Facts, hints []RenameHint) FileFacts {
	result := FileFacts{ProjectID: projectID, UnitID: unit.ID, Path: path, Language: language, ContentHash: hash, State: state, LastError: lastError, RenameHints: hints}
	for _, sym := range facts.Symbols {
		result.Symbols = append(result.Symbols, Symbol{LogicalKey: qualifiedKey(unit, path, sym.LocalKey), Kind: sym.Kind, Name: sym.Name,
			Container: qualifiedKey(unit, path, sym.ContainerLocalKey), StartLine: int(sym.Range.StartRow), StartCol: int(sym.Range.StartColumn),
			EndLine: int(sym.Range.EndRow), EndCol: int(sym.Range.EndColumn), SignatureHash: sym.SignatureHash, BodyHash: sym.BodyHash, StructureHash: sym.StructureHash})
	}
	for _, imp := range facts.Imports {
		result.Imports = append(result.Imports, Import{ImporterKey: qualifiedKey(unit, path, imp.ImporterLocalKey), Module: imp.Module, Names: imp.Names, Alias: imp.Alias, IsRelative: imp.IsRelative})
	}
	for _, ref := range facts.References {
		result.References = append(result.References, Reference{ReferrerKey: qualifiedKey(unit, path, ref.ReferrerLocalKey), TargetText: ref.Name,
			TargetLogicalKey: qualifiedKey(unit, path, ref.TargetLocalKey), ScopeText: qualifiedKey(unit, path, ref.ScopeLocalKey), Label: "STRUCTURAL_NAME_MATCH", Confidence: 0.5})
	}
	return result
}
