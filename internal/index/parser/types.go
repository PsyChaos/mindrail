// Package parser contains the built-in Tree-sitter grammars and converts their
// syntax into path-independent facts. Native nodes never cross this boundary.
package parser

import (
	"context"
	"errors"
	"sync"

	ts "github.com/tree-sitter/go-tree-sitter"
)

// ParseSchemaVersion invalidates snapshots when extraction semantics change.
// It is independent of the database migration version.
const ParseSchemaVersion = 3

var (
	ErrSyntax      = errors.New("source contains syntax errors")
	ErrInvalidUTF8 = errors.New("source is not valid UTF-8")
	ErrClosed      = errors.New("parser registry is closed")
	ErrSnapshot    = errors.New("snapshot is nil, closed, or belongs to another adapter")
)

// SourceFile deliberately contains no path: identical content is reusable
// across files and worktrees. Parse copies Content before retaining it.
type SourceFile struct{ Content []byte }

type LanguageInfo struct {
	Language       string
	GrammarVersion string
	Extensions     []string
}

type SyntaxAdapter interface {
	Info() LanguageInfo
	Parse(context.Context, SourceFile) (*SyntaxSnapshot, error)
	Symbols(*SyntaxSnapshot) ([]Symbol, error)
	Imports(*SyntaxSnapshot) ([]Import, error)
	References(*SyntaxSnapshot) ([]Reference, error)
}

// Range is a half-open byte interval with zero-based row/byte-column positions.
type Range struct {
	StartByte, EndByte                       uint
	StartRow, StartColumn, EndRow, EndColumn uint
}

// Symbol and Reference own their strings and contain no native tree pointers.
// Local keys describe declarations within one source, including duplicate overloads.
// They become path-qualified logical keys only when the indexer persists facts.
type Symbol struct {
	Name, Kind                             string
	Range                                  Range
	LocalKey, ContainerLocalKey            string
	SignatureRange                         Range
	BodyRange                              *Range
	SignatureHash, BodyHash, StructureHash string
}
type Import struct {
	Module           string
	Names            []string
	Alias            string
	IsRelative       bool
	ImporterLocalKey string
	Range            Range
}
type Reference struct {
	Name, Kind                                      string
	Range                                           Range
	ReferrerLocalKey, ScopeLocalKey, TargetLocalKey string
	ImportedModule, ImportedName                    string
	ImportedRelative                                bool
}

type Facts struct {
	Symbols         []Symbol    `json:"symbols"`
	Imports         []Import    `json:"imports"`
	References      []Reference `json:"references"`
	HasSyntaxErrors bool        `json:"has_syntax_errors"`
}

// SyntaxSnapshot owns one native tree. The caller must Close it, including when
// Parse returns ErrSyntax together with a partial snapshot. It must not be copied.
type SyntaxSnapshot struct {
	mu        sync.Mutex
	tree      *ts.Tree
	source    []byte
	owner     *adapter
	hasErrors bool
}

func (s *SyntaxSnapshot) HasErrors() bool { return s != nil && s.hasErrors }

// Close is idempotent and releases native memory without relying on finalizers.
func (s *SyntaxSnapshot) Close() {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.tree != nil {
		s.tree.Close()
		s.tree = nil
		s.source = nil
	}
}
