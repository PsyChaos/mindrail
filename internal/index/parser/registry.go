package parser

import (
	"embed"
	"fmt"
	"io/fs"
	"path/filepath"
	"slices"

	ts "github.com/tree-sitter/go-tree-sitter"
	javascript "github.com/tree-sitter/tree-sitter-javascript/bindings/go"
	python "github.com/tree-sitter/tree-sitter-python/bindings/go"
	typescript "github.com/tree-sitter/tree-sitter-typescript/bindings/go"
)

//go:embed queries/*/*.scm
var queries embed.FS

// Registry owns the compiled queries of its four immutable adapters. Close it
// when its users are done. Snapshots have independent ownership and must also close.
type Registry struct{ adapters []*adapter }

func NewRegistry() (*Registry, error) { return newRegistry(queries) }

func newRegistry(queryFS fs.FS) (*Registry, error) {
	r := &Registry{adapters: []*adapter{
		{info: LanguageInfo{"python", "v0.25.0", []string{".py"}}, language: ts.NewLanguage(python.Language())},
		{info: LanguageInfo{"javascript", "v0.25.0", []string{".js", ".mjs", ".cjs"}}, language: ts.NewLanguage(javascript.Language())},
		{info: LanguageInfo{"typescript", "v0.23.2", []string{".ts", ".mts", ".cts"}}, language: ts.NewLanguage(typescript.LanguageTypescript())},
		{info: LanguageInfo{"tsx", "v0.23.2", []string{".tsx"}}, language: ts.NewLanguage(typescript.LanguageTSX())},
	}}
	for _, a := range r.adapters {
		for _, kind := range []string{"symbols", "references"} {
			path := "queries/" + a.info.Language + "/" + kind + ".scm"
			content, err := fs.ReadFile(queryFS, path)
			if err != nil {
				r.Close()
				return nil, fmt.Errorf("read embedded query %s: %w", path, err)
			}
			query, queryErr := ts.NewQuery(a.language, string(content))
			if queryErr != nil {
				r.Close()
				return nil, fmt.Errorf("compile embedded query %s: %w", path, *queryErr)
			}
			if kind == "symbols" {
				a.symbols = query
			} else {
				a.references = query
			}
		}
	}
	return r, nil
}

// Lookup uses exact extensions and returns the first matching entry in the
// fixed Python, JavaScript, TypeScript, TSX order. Unsupported files allocate no parser.
func (r *Registry) Lookup(path string) (SyntaxAdapter, bool) {
	ext := filepath.Ext(path)
	for _, a := range r.adapters {
		if slices.Contains(a.info.Extensions, ext) {
			return a, true
		}
	}
	return nil, false
}

func (r *Registry) Entries() []LanguageInfo {
	entries := make([]LanguageInfo, 0, len(r.adapters))
	for _, a := range r.adapters {
		entries = append(entries, a.Info())
	}
	return entries
}

func (r *Registry) Close() {
	for _, a := range r.adapters {
		a.close()
	}
}
