package testguard

import (
	_ "embed"
	"fmt"
	"strings"

	ts "github.com/tree-sitter/go-tree-sitter"
	javascript "github.com/tree-sitter/tree-sitter-javascript/bindings/go"
	typescript "github.com/tree-sitter/tree-sitter-typescript/bindings/go"
)

//go:embed queries/typescript/guard.scm
var typescriptQuerySource string

//go:embed queries/javascript/guard.scm
var javascriptQuerySource string

// ecmaAnalyzer parses TypeScript/JavaScript test files structurally: named
// it/test calls, expect calls, skip member variants and hatch comments.
// Bare names only — near-cousins (xit, xdescribe, test.todo, test.only)
// do not exist in 0.1 and report nothing (decision D-171).
type ecmaAnalyzer struct {
	parser *ts.Parser
	query  *ts.Query
}

func newTypescriptAnalyzer() (*ecmaAnalyzer, error) {
	return newEcmaAnalyzer(ts.NewLanguage(typescript.LanguageTypescript()), typescriptQuerySource)
}

func newJavascriptAnalyzer() (*ecmaAnalyzer, error) {
	return newEcmaAnalyzer(ts.NewLanguage(javascript.Language()), javascriptQuerySource)
}

func newEcmaAnalyzer(language *ts.Language, source string) (*ecmaAnalyzer, error) {
	parser := ts.NewParser()
	if err := parser.SetLanguage(language); err != nil {
		parser.Close()
		return nil, err
	}
	query, queryErr := ts.NewQuery(language, source)
	if queryErr != nil {
		parser.Close()
		return nil, fmt.Errorf("compile ecma guard query: %w", *queryErr)
	}
	return &ecmaAnalyzer{parser: parser, query: query}, nil
}

func (a *ecmaAnalyzer) close() {
	if a.query != nil {
		a.query.Close()
	}
	if a.parser != nil {
		a.parser.Close()
	}
}

type ecmaCall struct {
	funcText string
	funcNode *ts.Node
	args     *ts.Node
	node     *ts.Node
}

func (a *ecmaAnalyzer) tests(source []byte) (map[string]testFunc, error) {
	out := map[string]testFunc{}
	if len(source) == 0 {
		return out, nil
	}
	tree := a.parser.Parse(source, nil)
	if tree == nil {
		return nil, invalidInput("ecma parse produced no tree")
	}
	defer tree.Close()
	root := tree.RootNode()
	calls := a.calls(root, source)
	type testDef struct {
		name     string
		node     *ts.Node
		callback *ts.Node
		markers  []string
	}
	var defs []testDef
	var suites []suiteBlock
	var excluded []*ts.Node
	for _, call := range calls {
		head, skipped, shape := splitMember(call.funcText)
		if !shape {
			if head == "xdescribe" {
				if callback := callbackArg(call, source); callback != nil {
					excluded = append(excluded, callback)
				}
			}
			continue
		}
		switch {
		case head == "describe" && skipped:
			if callback := callbackArg(call, source); callback != nil {
				suites = append(suites, suiteBlock{node: callback})
			}
		case head == "it" || head == "test":
			name, callback := testNameAndCallback(call, source)
			if name == "" {
				continue
			}
			var markers []string
			if skipped {
				markers = []string{"skip"}
			}
			defs = append(defs, testDef{name: name, node: call.node, callback: callback, markers: markers})
		}
	}
	expects := a.expectsByCallback(root, source)
	comments := a.comments(root, source)
	for _, def := range defs {
		if insideAny(def.node, excluded) {
			continue
		}
		markers := append([]string{}, def.markers...)
		for _, suite := range suites {
			// Suite skip disables the unit, not the test: contained
			// tests inherit a disabled marker, never a skipped one —
			// the spec's removed/disabled hardness applies to suites.
			if enclosesNode(suite.node, def.node) && !hasMarker(markers, "disabled") {
				markers = append(markers, "disabled")
			}
		}
		count := 0
		if def.callback != nil {
			count = expects[def.callback.StartByte()]
		}
		out[def.name] = testFunc{
			name:       def.name,
			asserts:    count,
			markers:    markers,
			allowed:    allowedAbove(comments, def.node),
			trivial:    isTrivialCallback(def.callback, source),
			startRow:   def.node.StartPosition().Row,
			unit:       "expects",
			decSignal:  SignalAssertionCountDecreased,
			zeroSignal: SignalExpectationRemoved,
		}
	}
	return out, nil
}

type suiteBlock struct {
	node *ts.Node
}

func (a *ecmaAnalyzer) calls(root *ts.Node, source []byte) []ecmaCall {
	cursor := ts.NewQueryCursor()
	defer cursor.Close()
	matches := cursor.Matches(a.query, root, source)
	names := a.query.CaptureNames()
	var out []ecmaCall
	for match := matches.Next(); match != nil; match = matches.Next() {
		var call ecmaCall
		for _, capture := range match.Captures {
			node := capture.Node
			switch names[capture.Index] {
			case "call.func":
				call.funcText = node.Utf8Text(source)
				function := node
				call.funcNode = &function
			case "call.args":
				args := node
				call.args = &args
			case "call":
				whole := node
				call.node = &whole
			}
		}
		if call.funcText == "" || call.node == nil {
			continue
		}
		out = append(out, call)
	}
	return out
}

// splitMember classifies a call head: bare it/test defines a test,
// it.skip/test.skip defines a skipped one, describe.skip a skipped suite.
// Anything else (it.only, xit, test.todo, deeper chains) is not a test
// shape in 0.1 and reports nothing.
func splitMember(text string) (head string, skipped, shape bool) {
	parts := strings.Split(text, ".")
	if len(parts) == 1 && (parts[0] == "it" || parts[0] == "test") {
		return parts[0], false, true
	}
	if len(parts) == 2 && parts[1] == "skip" {
		switch parts[0] {
		case "it", "test":
			return parts[0], true, true
		case "describe":
			return parts[0], true, true
		}
	}
	return parts[0], false, false
}

// testNameAndCallback reads the string-literal name and the function
// callback of an it/test call. Non-string names are not tests in 0.1:
// dynamic names cannot be tracked across versions, so they report nothing.
func testNameAndCallback(call ecmaCall, source []byte) (string, *ts.Node) {
	if call.args == nil {
		return "", nil
	}
	var name string
	var callback *ts.Node
	count := call.args.NamedChildCount()
	for i := uint(0); i < count; i++ {
		child := call.args.NamedChild(i)
		if child == nil {
			continue
		}
		if name == "" && child.Kind() == "string" {
			name = unquote(child.Utf8Text(source))
			continue
		}
		if callback == nil {
			switch child.Kind() {
			case "arrow_function", "function_expression", "function":
				callback = child
			}
		}
	}
	if name == "" {
		return "", nil
	}
	return name, callback
}

func callbackArg(call ecmaCall, source []byte) *ts.Node {
	if call.args == nil {
		return nil
	}
	count := call.args.NamedChildCount()
	for i := uint(0); i < count; i++ {
		child := call.args.NamedChild(i)
		if child == nil {
			continue
		}
		switch child.Kind() {
		case "arrow_function", "function_expression", "function":
			return child
		}
	}
	return nil
}

func unquote(text string) string {
	if len(text) >= 2 {
		if (text[0] == '"' && text[len(text)-1] == '"') ||
			(text[0] == '\'' && text[len(text)-1] == '\'') ||
			(text[0] == '`' && text[len(text)-1] == '`') {
			return text[1 : len(text)-1]
		}
	}
	return text
}

// expectsByCallback bins every bare or trailing expect call into the
// innermost enclosing test callback, keyed by callback start byte.
func (a *ecmaAnalyzer) expectsByCallback(root *ts.Node, source []byte) map[uint]int {
	calls := a.calls(root, source)
	type callback struct {
		node *ts.Node
	}
	var callbacks []*ts.Node
	for _, call := range calls {
		if callback := callbackArg(call, source); callback != nil {
			callbacks = append(callbacks, callback)
		}
	}
	counts := map[uint]int{}
	for _, call := range calls {
		head := strings.Split(call.funcText, ".")[0]
		if head != "expect" && !strings.HasSuffix(call.funcText, ".expect") {
			continue
		}
		var best *ts.Node
		for _, callback := range callbacks {
			if enclosesNode(callback, call.node) {
				if best == nil || rangeSize(callback) < rangeSize(best) {
					best = callback
				}
			}
		}
		if best != nil {
			counts[best.StartByte()]++
		}
	}
	return counts
}

func (a *ecmaAnalyzer) comments(root *ts.Node, source []byte) []*ts.Node {
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

func enclosesNode(outer, inner *ts.Node) bool {
	return outer.StartByte() <= inner.StartByte() && inner.EndByte() <= outer.EndByte()
}

func insideAny(node *ts.Node, blocks []*ts.Node) bool {
	for _, block := range blocks {
		if enclosesNode(block, node) {
			return true
		}
	}
	return false
}

func rangeSize(node *ts.Node) uint {
	return node.EndByte() - node.StartByte()
}

func hasMarker(markers []string, want string) bool {
	for _, marker := range markers {
		if marker == want {
			return true
		}
	}
	return false
}

// isTrivialCallback reports callbacks that prove nothing: empty blocks or
// bare returns. Expression-bodied arrows still execute code, so they are
// never trivial.
func isTrivialCallback(callback *ts.Node, source []byte) bool {
	if callback == nil {
		return false
	}
	_ = source
	body := callback.ChildByFieldName("body")
	if body == nil {
		return false
	}
	if body.Kind() != "statement_block" {
		return false
	}
	if body.NamedChildCount() != 1 {
		return body.NamedChildCount() == 0
	}
	only := body.NamedChild(0)
	return only != nil && only.Kind() == "return_statement" && only.NamedChildCount() == 0
}
