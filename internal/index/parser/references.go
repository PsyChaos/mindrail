package parser

import (
	"strings"

	ts "github.com/tree-sitter/go-tree-sitter"
)

func (a *adapter) extractReferences(s *SyntaxSnapshot) ([]Reference, error) {
	declarations, err := a.declarations(s)
	if err != nil {
		return nil, err
	}
	imports, err := a.extractImports(s, declarations)
	if err != nil {
		return nil, err
	}
	redirects := bindingScopeRedirects(s, declarations, a.info.Language)
	blocked := shadowedNames(s, declarations, imports, redirects, a.info.Language)
	nonImportBlocked := shadowedNames(s, declarations, nil, redirects, a.info.Language)
	calls, err := queryNodes(s, a.references)
	if err != nil {
		return nil, err
	}
	var references []Reference
	for _, call := range calls {
		if call.name == "" {
			continue
		}
		referrer := ""
		if parent := enclosing(declarations, call.node.StartByte(), call.node.EndByte()); parent != nil {
			referrer = parent.symbol.LocalKey
		}
		scope := evaluationScope(declarations, call.node, a.info.Language)
		ref := Reference{Name: call.name, Kind: call.kind, Range: nodeRange(call.node), ReferrerLocalKey: referrer, ScopeLocalKey: scope}
		// Member receivers require semantic knowledge; matching the final name
		// alone would invent edges to unrelated same-named declarations.
		function := call.node.ChildByFieldName("function")
		if function != nil && function.Kind() == "identifier" {
			ref.TargetLocalKey = resolveLocal(call.name, scope, declarations, blocked, call.node.StartByte(), a.info.Language)
			if ref.TargetLocalKey == "" {
				if imported, ok := resolveImport(call.name, scope, declarations, imports, redirects, nonImportBlocked, call.node.StartByte(), a.info.Language); ok {
					ref.ImportedModule = imported.Module
					ref.ImportedName = importedName(imported, call.name)
					ref.ImportedRelative = imported.IsRelative
				}
			}
		}
		references = append(references, ref)
	}
	return references, nil
}

type bindingRedirect struct {
	scope    string
	declared bool
}

type bindingRedirects map[string]map[string]bindingRedirect

func (r bindingRedirects) target(scope, name string) (string, bool) {
	redirect, ok := r[scope][name]
	return redirect.scope, ok && redirect.declared
}

func bindingScopeRedirects(s *SyntaxSnapshot, declarations []declaration, language string) bindingRedirects {
	redirects := bindingRedirects{}
	if language != "python" {
		return redirects
	}
	declarationByKey := map[string]declaration{}
	for _, d := range declarations {
		declarationByKey[d.symbol.LocalKey] = d
	}
	walkNodes(*s.tree.RootNode(), func(n ts.Node) bool {
		if n.Kind() != "global_statement" && n.Kind() != "nonlocal_statement" {
			return true
		}
		owner := enclosing(declarations, n.StartByte(), n.EndByte())
		if owner == nil {
			return false
		}
		target := ""
		if n.Kind() == "nonlocal_statement" {
			for key := owner.symbol.ContainerLocalKey; key != ""; {
				parent, ok := declarationByKey[key]
				if !ok {
					break
				}
				if parent.symbol.Kind != "class" {
					target = key
					break
				}
				key = parent.symbol.ContainerLocalKey
			}
		}
		bindingNames(n, func(name string) {
			if redirects[owner.symbol.LocalKey] == nil {
				redirects[owner.symbol.LocalKey] = map[string]bindingRedirect{}
			}
			redirects[owner.symbol.LocalKey][name] = bindingRedirect{scope: target, declared: true}
		}, s.source)
		return false
	})
	return redirects
}

func importedName(imp Import, visible string) string {
	if imp.Alias == visible && len(imp.Names) == 1 {
		return imp.Names[0]
	}
	for _, name := range imp.Names {
		if name == visible {
			return name
		}
	}
	return ""
}

func importVisibleAs(imp Import, visible, language string) bool {
	if imp.Alias != "" {
		return imp.Alias == visible
	}
	for _, name := range imp.Names {
		if name == visible {
			return true
		}
	}
	if language == "python" && len(imp.Names) == 0 {
		return strings.Split(imp.Module, ".")[0] == visible
	}
	return false
}

// resolveImport returns only imports that are the unambiguous lexical binding
// for this call. A declaration, parameter or assignment in a nearer scope
// suppresses import provenance instead of inventing a cross-file edge.
func resolveImport(name, scope string, declarations []declaration, imports []Import, redirects bindingRedirects, nonImportBlocked bindings, position uint, language string) (Import, bool) {
	for {
		var owner *Symbol
		for i := range declarations {
			if declarations[i].symbol.LocalKey == scope {
				owner = &declarations[i].symbol
				break
			}
		}
		if owner == nil || owner.Kind != "class" {
			if bindingsContain(nonImportBlocked[scope][name], position) || bindingsContain(nonImportBlocked[scope]["*"], position) {
				return Import{}, false
			}
			for _, d := range declarations {
				if d.symbol.ContainerLocalKey != scope || d.symbol.Name != name {
					continue
				}
				if language == "python" || rangeContains(lexicalExtent(d.node, true), position) {
					return Import{}, false
				}
			}
			var chosen Import
			found := false
			declaredImport := false
			wildcardStart := uint(0)
			wildcardFound := false
			for _, imp := range imports {
				importScope := imp.ImporterLocalKey
				if redirected, ok := redirects.target(importScope, name); ok {
					importScope = redirected
				}
				if importScope != scope {
					continue
				}
				if language == "python" && imp.Alias == "" && imp.Range.StartByte <= position {
					for _, importedName := range imp.Names {
						if importedName == "*" && (!wildcardFound || imp.Range.StartByte > wildcardStart) {
							wildcardStart, wildcardFound = imp.Range.StartByte, true
						}
					}
				}
				if !importVisibleAs(imp, name, language) {
					continue
				}
				declaredImport = true
				if imp.Range.StartByte > position {
					continue
				}
				if !found || imp.Range.StartByte > chosen.Range.StartByte {
					chosen, found = imp, true
				}
			}
			if found && (!wildcardFound || chosen.Range.StartByte > wildcardStart) && importedName(chosen, name) != "" {
				return chosen, true
			}
			if wildcardFound {
				return Import{}, false
			}
			if language == "python" && declaredImport {
				return Import{}, false
			}
		}
		if scope == "" || owner == nil {
			return Import{}, false
		}
		scope = owner.ContainerLocalKey
	}
}

type bindings map[string]map[string][]Range

// Python declaration headers (defaults, annotations and class bases) are
// evaluated outside the declared body. Textual ownership is still retained in
// ReferrerLocalKey; only the lookup scope excludes the declaration being built.
func evaluationScope(declarations []declaration, n ts.Node, language string) string {
	if language != "python" {
		if parent := enclosing(declarations, n.StartByte(), n.EndByte()); parent != nil {
			return parent.symbol.LocalKey
		}
		return ""
	}
	for i := len(declarations) - 1; i >= 0; i-- {
		body := declarations[i].symbol.BodyRange
		if body != nil && body.StartByte <= n.StartByte() && n.EndByte() <= body.EndByte {
			return declarations[i].symbol.LocalKey
		}
	}
	return ""
}

func shadowedNames(s *SyntaxSnapshot, declarations []declaration, imports []Import, redirects bindingRedirects, language string) bindings {
	blocked := bindings{}
	add := func(scope, name string, extent Range) {
		if name == "" {
			return
		}
		if blocked[scope] == nil {
			blocked[scope] = map[string][]Range{}
		}
		blocked[scope][name] = append(blocked[scope][name], extent)
	}
	for _, imp := range imports {
		extent := nodeRange(*s.tree.RootNode())
		for _, d := range declarations {
			if d.symbol.LocalKey == imp.ImporterLocalKey {
				extent = d.symbol.Range
				break
			}
		}
		addImportBinding := func(name string) {
			scope := imp.ImporterLocalKey
			bindingExtent := extent
			if redirected, ok := redirects.target(scope, name); ok {
				scope = redirected
				bindingExtent.StartByte = imp.Range.StartByte
				bindingExtent.StartRow = imp.Range.StartRow
				bindingExtent.StartColumn = imp.Range.StartColumn
			}
			add(scope, name, bindingExtent)
		}
		if imp.Alias != "" {
			addImportBinding(imp.Alias)
		} else if len(imp.Names) > 0 {
			for _, name := range imp.Names {
				addImportBinding(name)
			}
		} else if language == "python" {
			addImportBinding(strings.Split(imp.Module, ".")[0])
		}
	}
	for _, d := range declarations {
		parameters := d.callable.ChildByFieldName("parameters")
		if parameters == nil {
			parameters = d.callable.ChildByFieldName("parameter")
		}
		if parameters != nil {
			bindingNames(*parameters, func(name string) { add(d.symbol.LocalKey, name, nodeRange(d.callable)) }, s.source)
		}
	}
	walkNodes(*s.tree.RootNode(), func(n ts.Node) bool {
		// Anonymous callbacks/lambdas have lexical bindings even though they do
		// not produce named Symbol rows. Their extent must stay local to the
		// callback, otherwise a sibling call loses its valid outer binding.
		if n.Kind() == "arrow_function" || n.Kind() == "function_expression" || n.Kind() == "generator_function" || n.Kind() == "lambda" {
			scope := evaluationScope(declarations, n, language)
			extent := nodeRange(n)
			if n.Kind() == "lambda" {
				if body := n.ChildByFieldName("body"); body != nil {
					extent = nodeRange(*body)
				}
			}
			parameters := n.ChildByFieldName("parameters")
			if parameters == nil {
				parameters = n.ChildByFieldName("parameter")
			}
			if parameters != nil {
				bindingNames(*parameters, func(name string) { add(scope, name, extent) }, s.source)
			}
			if name := n.ChildByFieldName("name"); name != nil {
				add(scope, name.Utf8Text(s.source), nodeRange(n))
			}
		}
		var target *ts.Node
		switch n.Kind() {
		case "variable_declarator":
			// Named function-valued declarators are already declaration candidates.
			for _, d := range declarations {
				if d.node.Id() == n.Id() {
					return true
				}
			}
			target = n.ChildByFieldName("name")
		case "assignment", "augmented_assignment", "assignment_expression", "augmented_assignment_expression", "for_statement", "for_in_statement", "for_in_clause":
			target = n.ChildByFieldName("left")
		case "as_pattern":
			target = n.ChildByFieldName("alias")
		case "named_expression":
			target = n.ChildByFieldName("name")
		case "catch_clause":
			target = n.ChildByFieldName("parameter")
		case "case_pattern":
			if language == "python" {
				target = &n
			}
		case "delete_statement":
			if language == "python" {
				target = &n
			}
		}
		if target != nil {
			scope := ""
			if parent := enclosing(declarations, n.StartByte(), n.EndByte()); parent != nil {
				scope = parent.symbol.LocalKey
			}
			extent := bindingExtent(n, language)
			if n.Kind() == "case_pattern" {
				patternBindings(*target, func(name string) { add(scope, name, extent) }, s.source)
				return false // nested patterns were handled above
			}
			bindingNames(*target, func(name string) {
				bindingScope := scope
				bindingExtent := extent
				if redirected, ok := redirects.target(scope, name); ok {
					bindingScope = redirected
					bindingExtent.StartByte = n.StartByte()
					bindingExtent.StartRow = n.StartPosition().Row
					bindingExtent.StartColumn = n.StartPosition().Column
				}
				add(bindingScope, name, bindingExtent)
			}, s.source)
		}
		return true
	})
	return blocked
}

func isBindingIdentifier(n ts.Node) bool {
	return n.Kind() == "identifier" || n.Kind() == "shorthand_property_identifier_pattern"
}

// Binding patterns have expression children too: defaults, computed property
// keys and type annotations are reads, not additional declarations.
func bindingNames(n ts.Node, add func(string), source []byte) {
	if isBindingIdentifier(n) {
		add(n.Utf8Text(source))
		return
	}
	switch n.Kind() {
	case "attribute", "subscript", "member_expression", "subscript_expression", "type_annotation", "computed_property_name":
		return
	case "pair_pattern":
		if value := n.ChildByFieldName("value"); value != nil {
			bindingNames(*value, add, source)
		}
		return
	}
	for i := uint(0); i < n.NamedChildCount(); i++ {
		field := n.FieldNameForNamedChild(uint32(i))
		if field == "value" || field == "right" || field == "type" || field == "key" {
			continue
		}
		bindingNames(*n.NamedChild(i), add, source)
	}
}

func rangeContains(span Range, position uint) bool {
	return span.StartByte <= position && position < span.EndByte
}
func bindingsContain(spans []Range, position uint) bool {
	for _, span := range spans {
		if rangeContains(span, position) {
			return true
		}
	}
	return false
}

// lexicalExtent distinguishes JavaScript block bindings from function-local
// Python bindings and JavaScript var/assignment bindings. All nodes are borrowed.
func lexicalExtent(n ts.Node, blockScoped bool) Range {
	for parent := n.Parent(); parent != nil; parent = parent.Parent() {
		switch parent.Kind() {
		case "program", "module", "function_definition", "function_declaration", "function_expression", "generator_function_declaration", "generator_function", "arrow_function", "method_definition", "lambda":
			return nodeRange(*parent)
		case "statement_block", "switch_body", "for_statement", "for_in_statement", "catch_clause", "class_body":
			if blockScoped {
				return nodeRange(*parent)
			}
		}
	}
	return nodeRange(n)
}

func bindingExtent(n ts.Node, language string) Range {
	if language == "python" {
		return lexicalExtent(n, false)
	}
	if n.Kind() == "catch_clause" || n.Kind() == "for_in_statement" {
		return nodeRange(n)
	}
	blockScoped := false
	if n.Kind() == "variable_declarator" {
		if parent := n.Parent(); parent != nil {
			blockScoped = parent.Kind() == "lexical_declaration"
		}
	}
	return lexicalExtent(n, blockScoped)
}

// Python match value/class names are reads; only capture-pattern names bind.
func patternBindings(n ts.Node, add func(string), source []byte) {
	switch n.Kind() {
	case "identifier":
		add(n.Utf8Text(source))
		return
	case "dotted_name":
		if n.NamedChildCount() == 1 {
			add(n.Utf8Text(source))
		}
		return
	case "string", "concatenated_string":
		return
	}
	for i := uint(0); i < n.NamedChildCount(); i++ {
		if (n.Kind() == "class_pattern" || n.Kind() == "keyword_pattern") && i == 0 {
			continue
		}
		if n.Kind() == "dict_pattern" && n.FieldNameForNamedChild(uint32(i)) == "key" {
			continue
		}
		patternBindings(*n.NamedChild(i), add, source)
	}
}

func resolveLocal(name, scope string, declarations []declaration, blocked bindings, position uint, language string) string {
	for {
		var owner *Symbol
		for i := range declarations {
			if declarations[i].symbol.LocalKey == scope {
				owner = &declarations[i].symbol
				break
			}
		}
		// Class members are not lexical bare-name bindings inside methods.
		if owner == nil || owner.Kind != "class" {
			if bindingsContain(blocked[scope][name], position) || bindingsContain(blocked[scope]["*"], position) {
				return ""
			}
			key := ""
			for _, d := range declarations {
				if d.symbol.ContainerLocalKey == scope && d.symbol.Name == name {
					if language != "python" {
						extent := lexicalExtent(d.node, true)
						if d.node.Kind() == "variable_declarator" {
							extent = bindingExtent(d.node, language)
						}
						if !rangeContains(extent, position) {
							continue
						}
					}
					if key != "" && key != d.symbol.LocalKey {
						return ""
					}
					key = d.symbol.LocalKey
				}
			}
			if key != "" {
				return key
			} // same-key overload count is resolved by Store
		}
		if scope == "" || owner == nil {
			return ""
		}
		scope = owner.ContainerLocalKey
	}
}
