package parser

import (
	"bytes"
	"context"
	"fmt"
	"slices"
	"sync"
	"unicode/utf8"

	ts "github.com/tree-sitter/go-tree-sitter"
)

type adapter struct {
	mu                           sync.RWMutex
	info                         LanguageInfo
	language                     *ts.Language
	symbols, references, imports *ts.Query
	closed                       bool
}

var _ SyntaxAdapter = (*adapter)(nil)

func (a *adapter) Info() LanguageInfo {
	info := a.info
	info.Extensions = slices.Clone(info.Extensions)
	return info
}

func (a *adapter) close() {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.closed {
		return
	}
	if a.symbols != nil {
		a.symbols.Close()
	}
	if a.references != nil {
		a.references.Close()
	}
	if a.imports != nil {
		a.imports.Close()
	}
	a.closed = true
}

func (a *adapter) Parse(ctx context.Context, src SourceFile) (*SyntaxSnapshot, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	a.mu.RLock()
	defer a.mu.RUnlock()
	if a.closed {
		return nil, ErrClosed
	}
	if !utf8.Valid(src.Content) {
		return nil, ErrInvalidUTF8
	}
	source := bytes.Clone(src.Content)
	p := ts.NewParser()
	defer p.Close()
	if err := p.SetLanguage(a.language); err != nil {
		return nil, fmt.Errorf("set %s grammar: %w", a.info.Language, err)
	}
	// v0.25.0 ParseWithOptions leaks its saved progress payload when options are
	// non-nil. Basic Parse uses nil options. Cancellation is checked at file
	// boundaries, as required by the non-preemptive parsing contract (§48).
	tree := p.Parse(source, nil)
	if err := ctx.Err(); err != nil {
		if tree != nil {
			tree.Close()
		}
		return nil, err
	}
	if tree == nil {
		return nil, fmt.Errorf("parse %s: no syntax tree", a.info.Language)
	}
	snapshot := &SyntaxSnapshot{tree: tree, source: source, owner: a, hasErrors: tree.RootNode().HasError()}
	if snapshot.hasErrors {
		return snapshot, fmt.Errorf("%s: %w", a.info.Language, ErrSyntax)
	}
	return snapshot, nil
}

func (a *adapter) Symbols(s *SyntaxSnapshot) ([]Symbol, error) {
	var symbols []Symbol
	err := a.withSnapshot(s, func() error {
		declarations, err := a.declarations(s)
		for _, declaration := range declarations {
			symbols = append(symbols, declaration.symbol)
		}
		return err
	})
	return symbols, err
}

func (a *adapter) References(s *SyntaxSnapshot) ([]Reference, error) {
	var references []Reference
	err := a.withSnapshot(s, func() error {
		var err error
		references, err = a.extractReferences(s)
		return err
	})
	return references, err
}

func (a *adapter) Imports(s *SyntaxSnapshot) ([]Import, error) {
	var imports []Import
	err := a.withSnapshot(s, func() error {
		declarations, err := a.declarations(s)
		if err != nil {
			return err
		}
		imports, err = a.extractImports(s, declarations)
		return err
	})
	return imports, err
}

func (a *adapter) withSnapshot(s *SyntaxSnapshot, fn func() error) error {
	a.mu.RLock()
	defer a.mu.RUnlock()
	if a.closed {
		return ErrClosed
	}
	if s == nil {
		return ErrSnapshot
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.tree == nil || s.owner != a {
		return ErrSnapshot
	}
	return fn()
}
