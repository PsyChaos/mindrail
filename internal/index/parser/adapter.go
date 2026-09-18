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
	mu                  sync.RWMutex
	info                LanguageInfo
	language            *ts.Language
	symbols, references *ts.Query
	closed              bool
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
	a.mu.RLock()
	defer a.mu.RUnlock()
	var symbols []Symbol
	err := a.capture(s, a.symbols, func(name, kind string, span Range) {
		symbols = append(symbols, Symbol{Name: name, Kind: kind, Range: span})
	})
	return symbols, err
}

func (a *adapter) References(s *SyntaxSnapshot) ([]Reference, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	var references []Reference
	err := a.capture(s, a.references, func(name, kind string, span Range) {
		references = append(references, Reference{Name: name, Kind: kind, Range: span})
	})
	return references, err
}

// capture runs a compiled query while the adapter read lock is held. Every
// match captures @name plus a normalized kind (@function, @class or @call).
func (a *adapter) capture(s *SyntaxSnapshot, query *ts.Query, emit func(string, string, Range)) error {
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
	cursor := ts.NewQueryCursor()
	defer cursor.Close()
	matches := cursor.Matches(query, s.tree.RootNode(), s.source)
	names := query.CaptureNames()
	for match := matches.Next(); match != nil; match = matches.Next() {
		var name, kind string
		var span Range
		for _, capture := range match.Captures {
			label := names[capture.Index]
			if label == "name" {
				name = capture.Node.Utf8Text(s.source)
			} else {
				kind = label
				n := capture.Node
				start, end := n.StartPosition(), n.EndPosition()
				span = Range{n.StartByte(), n.EndByte(), start.Row, start.Column, end.Row, end.Column}
			}
		}
		if name != "" && kind != "" {
			emit(name, kind, span)
		}
	}
	if cursor.DidExceedMatchLimit() {
		return fmt.Errorf("%s query exceeded match limit", a.info.Language)
	}
	return nil
}
