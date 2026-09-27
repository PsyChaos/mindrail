package testguard

import (
	_ "embed"
	"fmt"
	"strings"

	ts "github.com/tree-sitter/go-tree-sitter"
	python "github.com/tree-sitter/tree-sitter-python/bindings/go"
)

//go:embed queries/python/guard.scm
var pythonQuerySource string

// pythonSkipDecorators is the spec marker list, normalized: dotted names as
// written, call forms stripped to the dotted head.
var pythonSkipDecorators = map[string]string{
	"pytest.mark.skip":  "skip",
	"unittest.skip":     "skip",
	"pytest.mark.xfail": "xfail",
}

// pythonAnalyzer parses Python test files structurally: test functions,
// assert statements, skip/xfail decorators and hatch comments. Strings and
// comments never count because only node types match, never text.
type pythonAnalyzer struct {
	parser *ts.Parser
	query  *ts.Query
}

func newPythonAnalyzer() (*pythonAnalyzer, error) {
	parser := ts.NewParser()
	language := ts.NewLanguage(python.Language())
	if err := parser.SetLanguage(language); err != nil {
		parser.Close()
		return nil, err
	}
	query, queryErr := ts.NewQuery(language, pythonQuerySource)
	if queryErr != nil {
		parser.Close()
		return nil, fmt.Errorf("compile python guard query: %w", *queryErr)
	}
	return &pythonAnalyzer{parser: parser, query: query}, nil
}

func (a *pythonAnalyzer) close() {
	if a.query != nil {
		a.query.Close()
	}
	if a.parser != nil {
		a.parser.Close()
	}
}

func (a *pythonAnalyzer) tests(source []byte) (map[string]testFunc, error) {
	out := map[string]testFunc{}
	if len(source) == 0 {
		return out, nil
	}
	tree := a.parser.Parse(source, nil)
	if tree == nil {
		return nil, invalidInput("python parse produced no tree")
	}
	defer tree.Close()
	root := tree.RootNode()
	functions := a.functions(root, source)
	asserts := a.assertsByFunction(root, source, functions)
	comments := a.comments(root, source)
	for _, function := range functions {
		if !isTestName(function.name) {
			continue
		}
		out[function.name] = testFunc{
			name:     function.name,
			asserts:  asserts[function.node.StartByte()],
			markers:  a.markersFor(function, source),
			allowed:  allowedAbove(comments, function.node),
			trivial:  isTrivialBody(function.body, source),
			startRow: function.node.StartPosition().Row,
		}
	}
	return out, nil
}

type pythonFunction struct {
	name string
	node *ts.Node
	body *ts.Node
}

func (a *pythonAnalyzer) functions(root *ts.Node, source []byte) []pythonFunction {
	cursor := ts.NewQueryCursor()
	defer cursor.Close()
	matches := cursor.Matches(a.query, root, source)
	names := a.query.CaptureNames()
	var funcs []pythonFunction
	for match := matches.Next(); match != nil; match = matches.Next() {
		var current pythonFunction
		for _, capture := range match.Captures {
			node := capture.Node
			switch names[capture.Index] {
			case "test.name":
				current.name = node.Utf8Text(source)
			case "test.body":
				body := node
				current.body = &body
			case "test.func":
				function := node
				current.node = &function
			}
		}
		if current.name == "" || current.node == nil || current.body == nil {
			continue
		}
		funcs = append(funcs, current)
	}
	return funcs
}

func (a *pythonAnalyzer) assertsByFunction(root *ts.Node, source []byte, functions []pythonFunction) map[uint]int {
	cursor := ts.NewQueryCursor()
	defer cursor.Close()
	matches := cursor.Matches(a.query, root, source)
	names := a.query.CaptureNames()
	counts := map[uint]int{}
	for match := matches.Next(); match != nil; match = matches.Next() {
		for _, capture := range match.Captures {
			if names[capture.Index] != "assert" {
				continue
			}
			node := capture.Node
			for _, function := range functions {
				if encloses(function.node, &node) {
					counts[function.node.StartByte()]++
					break
				}
			}
		}
	}
	return counts
}

func (a *pythonAnalyzer) comments(root *ts.Node, source []byte) []*ts.Node {
	cursor := ts.NewQueryCursor()
	defer cursor.Close()
	matches := cursor.Matches(a.query, root, source)
	names := a.query.CaptureNames()
	var out []*ts.Node
	for match := matches.Next(); match != nil; match = matches.Next() {
		for _, capture := range match.Captures {
			if names[capture.Index] != "comment" {
				continue
			}
			node := capture.Node
			if strings.Contains(node.Utf8Text(source), AllowMarker) {
				out = append(out, &node)
			}
		}
	}
	return out
}

// markersFor reads the decorators of one function through its parent: a
// decorated_definition parents both the decorators and the definition.
// Undecorated functions carry no markers; unknown parents are not errors.
func (a *pythonAnalyzer) markersFor(function pythonFunction, source []byte) []string {
	parent := function.node.Parent()
	if parent == nil {
		return nil
	}
	if parent.Kind() != "decorated_definition" {
		return nil
	}
	var markers []string
	count := parent.ChildCount()
	for i := uint(0); i < count; i++ {
		child := parent.Child(i)
		if child == nil || child.Kind() != "decorator" {
			continue
		}
		if marker, ok := normalizeDecorator(child.Utf8Text(source)); ok {
			markers = append(markers, marker)
		}
	}
	return markers
}

// normalizeDecorator maps decorator source to a canonical marker kind.
// Call forms strip to the dotted head; anything off the spec list,
// including near-cousins like skipif or todo, reports nothing.
func normalizeDecorator(text string) (string, bool) {
	trimmed := strings.TrimSpace(strings.TrimPrefix(text, "@"))
	if index := strings.Index(trimmed, "("); index >= 0 {
		trimmed = strings.TrimSpace(trimmed[:index])
	}
	if marker, ok := pythonSkipDecorators[trimmed]; ok {
		return marker, true
	}
	return "", false
}

func isTestName(name string) bool {
	return strings.HasPrefix(name, "test")
}

func encloses(outer, inner *ts.Node) bool {
	return outer.StartByte() <= inner.StartByte() && inner.EndByte() <= outer.EndByte()
}

// allowedAbove reports a hatch comment in the lines directly above the
// function: within five rows, never from afar.
func allowedAbove(comments []*ts.Node, function *ts.Node) bool {
	startRow := function.StartPosition().Row
	for _, comment := range comments {
		endRow := comment.EndPosition().Row
		if endRow < startRow && startRow-endRow <= 5 {
			return true
		}
	}
	return false
}

// isTrivialBody reports bodies that prove nothing on their own: a lone
// pass, ellipsis or bare return. Anything else — including a bare
// expression — is not trivial: calling the code still executes it.
func isTrivialBody(body *ts.Node, source []byte) bool {
	if body == nil || body.NamedChildCount() != 1 {
		return false
	}
	child := body.NamedChild(0)
	if child == nil {
		return false
	}
	switch child.Kind() {
	case "pass_statement":
		return true
	case "return_statement":
		return child.NamedChildCount() == 0
	case "expression_statement":
		text := strings.TrimSpace(child.Utf8Text(source))
		return text == "..."
	default:
		return false
	}
}
