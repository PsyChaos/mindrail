package parser

import (
	"cmp"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"slices"
	"strings"

	ts "github.com/tree-sitter/go-tree-sitter"
)

type capture struct {
	name, kind string
	node       ts.Node
}
type declaration struct {
	symbol   Symbol
	node     ts.Node
	callable ts.Node
}

// queryNodes borrows the tree while withSnapshot holds both lifetime locks.
func queryNodes(s *SyntaxSnapshot, query *ts.Query) ([]capture, error) {
	cursor := ts.NewQueryCursor()
	defer cursor.Close()
	matches := cursor.Matches(query, s.tree.RootNode(), s.source)
	names := query.CaptureNames()
	var captures []capture
	for match := matches.Next(); match != nil; match = matches.Next() {
		var result capture
		for _, c := range match.Captures {
			if names[c.Index] == "name" {
				result.name = c.Node.Utf8Text(s.source)
			} else {
				result.kind = names[c.Index]
				result.node = c.Node
			}
		}
		if result.kind != "" {
			captures = append(captures, result)
		}
	}
	if cursor.DidExceedMatchLimit() {
		return captures, fmt.Errorf("syntax query exceeded match limit")
	}
	return captures, nil
}

func (a *adapter) declarations(s *SyntaxSnapshot) ([]declaration, error) {
	captures, err := queryNodes(s, a.symbols)
	if err != nil {
		return nil, err
	}
	slices.SortStableFunc(captures, func(a, b capture) int {
		if a.node.StartByte() != b.node.StartByte() {
			return cmp.Compare(a.node.StartByte(), b.node.StartByte())
		}
		return cmp.Compare(b.node.EndByte(), a.node.EndByte())
	})
	var declarations []declaration
	for _, c := range captures {
		if c.name == "" {
			continue
		} // missing names in a recovered partial tree
		parent := enclosing(declarations, c.node.StartByte(), c.node.EndByte())
		container := ""
		if parent != nil {
			container = parent.symbol.LocalKey
			if a.info.Language == "python" && c.kind == "function" && parent.symbol.Kind == "class" {
				c.kind = "method"
			}
		}
		callable := c.node
		if c.node.Kind() == "variable_declarator" {
			callable = *c.node.ChildByFieldName("value")
		}
		body := callable.ChildByFieldName("body")
		span := nodeRange(c.node)
		signature := span
		var bodyRange *Range
		var bodyBytes []byte
		if body != nil {
			b := nodeRange(*body)
			// Python's block starts at the first statement, after leading body
			// comments/indentation. The declaration's direct ':' token marks the
			// true header boundary; those body bytes must not enter its signature.
			if a.info.Language == "python" {
				for i := uint(0); i < callable.ChildCount(); i++ {
					child := callable.Child(i)
					if child.Kind() == ":" && child.EndByte() <= body.StartByte() {
						b.StartByte = child.EndByte()
						point := child.EndPosition()
						b.StartRow, b.StartColumn = point.Row, point.Column
					}
				}
			}
			bodyRange = &b
			signature.EndByte = b.StartByte
			signature.EndRow = b.StartRow
			signature.EndColumn = b.StartColumn
			bodyBytes = s.source[b.StartByte:b.EndByte]
		}
		signatureBytes := s.source[signature.StartByte:signature.EndByte]
		// Structure is the declaration/signature projection, not the executable
		// body. Including signature bytes distinguishes equal-shaped signatures.
		var shape strings.Builder
		shape.WriteString(c.kind)
		shape.WriteByte(0)
		shape.Write(signatureBytes)
		shape.WriteByte(0)
		walkNodes(c.node, func(n ts.Node) bool {
			if bodyRange != nil && n.StartByte() >= bodyRange.StartByte {
				return false
			}
			shape.WriteString(n.Kind())
			shape.WriteByte(0)
			return true
		})
		sym := Symbol{Name: c.name, Kind: c.kind, Range: span, LocalKey: LocalKey(container, c.kind, c.name), ContainerLocalKey: container, SignatureRange: signature, BodyRange: bodyRange, SignatureHash: hashBytes(signatureBytes), BodyHash: hashBytes(bodyBytes), StructureHash: hashBytes([]byte(shape.String()))}
		declarations = append(declarations, declaration{symbol: sym, node: c.node, callable: callable})
	}
	return declarations, nil
}

func enclosing(declarations []declaration, start, end uint) *declaration {
	for i := len(declarations) - 1; i >= 0; i-- {
		s := declarations[i].symbol.Range
		if s.StartByte <= start && s.EndByte >= end && (s.StartByte < start || s.EndByte > end) {
			return &declarations[i]
		}
	}
	return nil
}

func nodeRange(n ts.Node) Range {
	start, end := n.StartPosition(), n.EndPosition()
	return Range{n.StartByte(), n.EndByte(), start.Row, start.Column, end.Row, end.Column}
}
func hashBytes(data []byte) string { sum := sha256.Sum256(data); return hex.EncodeToString(sum[:]) }

// NamedChild returns borrowed nodes and allocates no native cursor.
func walkNodes(n ts.Node, visit func(ts.Node) bool) {
	if !visit(n) {
		return
	}
	for i := uint(0); i < n.NamedChildCount(); i++ {
		if child := n.NamedChild(i); child != nil {
			walkNodes(*child, visit)
		}
	}
}
