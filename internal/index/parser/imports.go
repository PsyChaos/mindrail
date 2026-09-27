package parser

import (
	"strings"

	ts "github.com/tree-sitter/go-tree-sitter"
)

func (a *adapter) extractImports(s *SyntaxSnapshot, declarations []declaration) ([]Import, error) {
	captures, err := queryNodes(s, a.imports)
	if err != nil {
		return nil, err
	}
	var imports []Import
	for _, c := range captures {
		n := c.node
		scope := ""
		if parent := enclosing(declarations, n.StartByte(), n.EndByte()); parent != nil {
			scope = parent.symbol.LocalKey
		}
		add := func(module string, names []string, alias string) {
			imports = append(imports, Import{Module: module, Names: names, Alias: alias, IsRelative: strings.HasPrefix(module, "."), ImporterLocalKey: scope, Range: nodeRange(n)})
		}
		if a.info.Language == "python" {
			module := n.ChildByFieldName("module_name")
			moduleName := ""
			if module != nil {
				moduleName = module.Utf8Text(s.source)
			}
			if n.Kind() == "future_import_statement" {
				moduleName = "__future__"
			}
			for i := uint(0); i < n.NamedChildCount(); i++ {
				child := n.NamedChild(i)
				if module != nil && child.Id() == module.Id() {
					continue
				}
				var name, alias string
				switch child.Kind() {
				case "aliased_import":
					name = textField(*child, "name", s.source)
					alias = textField(*child, "alias", s.source)
				case "dotted_name", "wildcard_import":
					name = child.Utf8Text(s.source)
				default:
					continue
				}
				if moduleName == "" {
					add(name, nil, alias)
				} else {
					add(moduleName, []string{name}, alias)
				}
			}
			continue
		}
		module := unquoteModule(textField(n, "source", s.source))
		before := len(imports)
		for i := uint(0); i < n.NamedChildCount(); i++ {
			clause := n.NamedChild(i)
			if clause.Kind() != "import_clause" {
				continue
			}
			for j := uint(0); j < clause.NamedChildCount(); j++ {
				binding := clause.NamedChild(j)
				switch binding.Kind() {
				case "identifier":
					add(module, []string{"default"}, binding.Utf8Text(s.source))
				case "namespace_import":
					if name := binding.NamedChild(0); name != nil {
						add(module, []string{"*"}, name.Utf8Text(s.source))
					}
				case "named_imports":
					for k := uint(0); k < binding.NamedChildCount(); k++ {
						spec := binding.NamedChild(k)
						if spec.Kind() == "import_specifier" {
							add(module, []string{unquoteModule(textField(*spec, "name", s.source))}, textField(*spec, "alias", s.source))
						}
					}
				}
			}
		}
		if len(imports) == before {
			add(module, nil, "")
		}
	}
	return imports, nil
}

func textField(n ts.Node, field string, source []byte) string {
	if child := n.ChildByFieldName(field); child != nil {
		return child.Utf8Text(source)
	}
	return ""
}
func unquoteModule(s string) string {
	if len(s) >= 2 && (s[0] == '\'' || s[0] == '"') && s[len(s)-1] == s[0] {
		return s[1 : len(s)-1]
	}
	return s
}
