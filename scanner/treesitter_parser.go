// scanner/treesitter_parser.go
package scanner

import (
	"bufio"
	"context"
	"fmt"
	"path/filepath"
	"strings"

	sitter "github.com/smacker/go-tree-sitter"
	"github.com/smacker/go-tree-sitter/javascript"
	"github.com/smacker/go-tree-sitter/python"
	"github.com/smacker/go-tree-sitter/typescript/typescript"

	"github.com/alexferrari88/prompt-scanner/utils"
)

// cleanQuery removes comment lines (starting with ';') and trims whitespace from a tree-sitter query string.
func cleanQuery(query string) string {
	var cleanedQuery strings.Builder
	scanner := bufio.NewScanner(strings.NewReader(query))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" && !strings.HasPrefix(line, ";") {
			cleanedQuery.WriteString(line)
			cleanedQuery.WriteString("\n")
		}
	}
	finalStr := strings.TrimSpace(cleanedQuery.String())
	if finalStr == "" {
		return ""
	}
	return finalStr
}

var (
	langToGrammar = map[string]*sitter.Language{
		"python":     python.GetLanguage(),
		"javascript": javascript.GetLanguage(),
		"typescript": typescript.GetLanguage(),
	}

	rawLangToQueries = map[string]string{
		"python": `
			(string) @string_node
			(assignment
				left: (identifier) @var.name ; Context from AST walk
				right: (string) @string_node)
            (call
                function: [
					(identifier) @call.function  ; Context from AST walk
					(attribute
						object: (identifier) @call.receiver ; Context from AST walk
						attribute: (identifier) @call.function) ; Context from AST walk
				]
                arguments: (argument_list (string) @string_node))
			(raise_statement (string) @string_node) ; Context from AST walk
			(raise_statement
				(call
					function: (identifier) @call.function ; Context from AST walk
					arguments: (argument_list (string) @string_node)))
		`,
		"javascript": `
			[ (string_fragment) (string) (template_string) ] @string_node

			(assignment_expression
				left: [ (identifier) (member_expression) ] @var.name ; Context from AST walk
				right: [ (string) (template_string) ] @string_node)
			(variable_declarator
				name: (identifier) @var.name ; Context from AST walk
				value: [ (string) (template_string) ] @string_node)

            (call_expression
				function: (_) @call.invoked_function_or_method ; Context from AST walk
                arguments: (arguments ([ (string) (template_string) ] @string_node)))

			(throw_statement
				(new_expression
					constructor: (_) @call.new_constructor ; Context from AST walk
					arguments: (arguments ([ (string) (template_string) ] @string_node))
				)
			)
			(throw_statement
				(call_expression
					function: (identifier) @call.error_function ; Context from AST walk
					arguments: (arguments ([ (string) (template_string) ] @string_node))
				)
			)
			(throw_statement (string) @string_node) ; Context from AST walk
			(throw_statement (template_string) @string_node) ; Context from AST walk
		`,
		"typescript": `
			[ (string_fragment) (string) (template_string) ] @string_node

			(assignment_expression
				left: [ (identifier) (member_expression) ] @var.name ; Context from AST walk
				right: [ (string) (template_string) ] @string_node)
			(lexical_declaration
				(variable_declarator
					name: (identifier) @var.name ; Context from AST walk
					value: [ (string) (template_string) ] @string_node))

            (call_expression
				function: (_) @call.invoked_function_or_method ; Context from AST walk
                arguments: (arguments ([ (string) (template_string) ] @string_node)))

			(throw_statement
				(new_expression
					constructor: (_) @call.new_constructor ; Context from AST walk
					arguments: (arguments ([ (string) (template_string) ] @string_node))
				)
			)
			(throw_statement
				(call_expression
					function: (identifier) @call.error_function ; Context from AST walk
					arguments: (arguments ([ (string) (template_string) ] @string_node))
				)
			)
			(throw_statement (string) @string_node) ; Context from AST walk
			(throw_statement (template_string) @string_node) ; Context from AST walk
		`,
	}
	langToQueries map[string]string
)

func init() {
	langToQueries = make(map[string]string)
	for lang, query := range rawLangToQueries {
		cleaned := cleanQuery(query)
		if cleaned != "" {
			langToQueries[lang] = cleaned
		}
	}
}

// determineContextAroundNode walks the AST upwards from stringNode to find its context.
func determineContextAroundNode(stringNode *sitter.Node, contentBytes []byte, langName string) (varName, invFuncName, invReceiverName string) {
	current := stringNode
	// Limit upward traversal to avoid excessively deep searches. 3-4 levels should cover most common cases.
	for depth := 0; depth < 4 && current != nil && current.Parent() != nil; depth++ {
		parentNode := current.Parent()
		if parentNode == nil {
			break
		}

		// Variable assignment context
		// Only consider if 'current' (our stringNode or its direct wrapper) is the value being assigned.
		if current.ID() == stringNode.ID() { // Check on first iteration
			switch parentNode.Type() {
			case "assignment_expression": // JS/TS: foo = "string" or obj.prop = "string"
				if rhs := parentNode.ChildByFieldName("right"); rhs != nil && rhs.ID() == current.ID() {
					if left := parentNode.ChildByFieldName("left"); left != nil {
						varName = left.Content(contentBytes)
					}
				}
			case "variable_declarator": // JS/TS: var foo = "string"
				if value := parentNode.ChildByFieldName("value"); value != nil && value.ID() == current.ID() {
					if nameNode := parentNode.ChildByFieldName("name"); nameNode != nil {
						varName = nameNode.Content(contentBytes)
					}
				}
			case "assignment": // Python: foo = "string"
				if rhs := parentNode.ChildByFieldName("right"); rhs != nil && rhs.ID() == current.ID() {
					if leftNode := parentNode.ChildByFieldName("left"); leftNode != nil {
						varName = leftNode.Content(contentBytes)
					}
				}
			case "pair": // JSON: "key": "value" (value is our string)
				if valNode := parentNode.ChildByFieldName("value"); valNode != nil && valNode.ID() == current.ID() {
					if keyNode := parentNode.ChildByFieldName("key"); keyNode != nil {
						keyContent := keyNode.Content(contentBytes)
						if len(keyContent) >= 2 && keyContent[0] == '"' && keyContent[len(keyContent)-1] == '"' {
							varName = keyContent[1 : len(keyContent)-1]
						} else {
							varName = keyContent
						}
					}
				}
			}
		}

		isArg := false
		if parentNode.Type() == "arguments" || parentNode.Type() == "argument_list" || parentNode.Type() == "tuple" {
			for i := 0; i < int(parentNode.ChildCount()); i++ {
				child := parentNode.Child(i)
				if child != nil && child.ID() == current.ID() {
					isArg = true
					break
				}
			}

			if isArg {
				callLikeNode := parentNode.Parent()
				if callLikeNode != nil {
					switch callLikeNode.Type() {
					case "call_expression", "call":
						var funcNode *sitter.Node
						if langName == "python" && callLikeNode.Type() == "call" {
							if callLikeNode.ChildCount() > 0 {
								funcNode = callLikeNode.Child(0)
							}
						} else {
							funcNode = callLikeNode.ChildByFieldName("function")
						}

						if funcNode != nil {
							if funcNode.Type() == "identifier" {
								invFuncName = funcNode.Content(contentBytes)
							} else if funcNode.Type() == "member_expression" {
								objN := funcNode.ChildByFieldName("object")
								propN := funcNode.ChildByFieldName("property")
								if objN != nil {
									invReceiverName = objN.Content(contentBytes)
								}
								if propN != nil {
									invFuncName = propN.Content(contentBytes)
								}
							} else if funcNode.Type() == "attribute" {
								objN := funcNode.ChildByFieldName("object")
								attrN := funcNode.ChildByFieldName("attribute")
								if objN != nil {
									invReceiverName = objN.Content(contentBytes)
								}
								if attrN != nil {
									invFuncName = attrN.Content(contentBytes)
								}
							}
						}
					case "new_expression":
						invReceiverName = "new"
						if constructorNode := callLikeNode.ChildByFieldName("constructor"); constructorNode != nil {
							invFuncName = constructorNode.Content(contentBytes)
						}
					}
					if invFuncName != "" || invReceiverName != "" {
						return varName, invFuncName, invReceiverName
					}
				}
			}
		}

		if parentNode.Type() == "throw_statement" {
			if argFieldNode := parentNode.ChildByFieldName("argument"); argFieldNode != nil && argFieldNode.ID() == current.ID() {
				if invFuncName == "" && invReceiverName == "" {
					invFuncName = "throw_literal"
				}
				return varName, invFuncName, invReceiverName
			}
		} else if parentNode.Type() == "raise_statement" {
			isDirectRaiseArg := false
			for i := 0; i < int(parentNode.ChildCount()); i++ {
				child := parentNode.Child(i)
				if child != nil && child.ID() == current.ID() && parentNode.FieldNameForChild(i) == "" {
					isDirectRaiseArg = true
					break
				}
			}
			if isDirectRaiseArg {
				if invFuncName == "" && invReceiverName == "" {
					invFuncName = "raise_literal"
				}
				return varName, invFuncName, invReceiverName
			}
		}
		current = parentNode
	}
	return
}

func unescapePythonString(s string) string {
	s = strings.ReplaceAll(s, "\\n", "\n")
	s = strings.ReplaceAll(s, "\\t", "\t")
	s = strings.ReplaceAll(s, "\\'", "'")
	s = strings.ReplaceAll(s, "\\\"", "\"")
	s = strings.ReplaceAll(s, "\\\\", "\\")
	return s
}

func unescapeJSString(s string) string {
	s = strings.ReplaceAll(s, "\\n", "\n")
	s = strings.ReplaceAll(s, "\\t", "\t")
	s = strings.ReplaceAll(s, "\\'", "'")
	s = strings.ReplaceAll(s, "\\\"", "\"")
	s = strings.ReplaceAll(s, "\\`", "`")
	s = strings.ReplaceAll(s, "\\\\", "\\")
	return s
}

func (s *Scanner) ParseTreeSitterFile(filePath string, contentBytes []byte, langName string) ([]FoundPrompt, error) {
	lang, supported := langToGrammar[langName]
	if !supported {
		return nil, fmt.Errorf("tree-sitter grammar for '%s' not supported", langName)
	}
	queryString, hasQuery := langToQueries[langName]
	if !hasQuery {
		return nil, fmt.Errorf("tree-sitter query for '%s' not defined or empty after cleaning", langName)
	}

	parser := sitter.NewParser()
	parser.SetLanguage(lang)
	tree, err := parser.ParseCtx(context.Background(), nil, contentBytes)
	if err != nil {
		return nil, fmt.Errorf("ts parsing error for %s: %w", filePath, err)
	}
	defer tree.Close()

	q, err := sitter.NewQuery([]byte(queryString), lang)
	if err != nil {
		return nil, fmt.Errorf("ts query compilation error for %s (cleaned query: \n%s\nError: %w)", langName, queryString, err)
	}
	defer q.Close()

	qc := sitter.NewQueryCursor()
	qc.Exec(q, tree.RootNode())
	defer qc.Close()

	var prompts []FoundPrompt
	ext := filepath.Ext(filePath)
	processedNodeIDs := make(map[uintptr]bool)

	for {
		m, ok := qc.NextMatch()
		if !ok {
			break
		}

		stringNode := (*sitter.Node)(nil)
		for _, capture := range m.Captures {
			node := capture.Node
			captureName := q.CaptureNameForId(capture.Index)
			nodeTypeStr := node.Type()

			if strings.Contains(nodeTypeStr, "comment") {
				stringNode = nil
				break
			}
			if captureName == "string_node" {
				if strings.Contains(nodeTypeStr, "string") || nodeTypeStr == "template_string" || nodeTypeStr == "string_fragment" {
					stringNode = node
				}
			}
		}

		if stringNode == nil {
			continue
		}

		// If this node is a string_fragment and its parent is a template_string,
		// skip it because the entire template_string will be processed.
		if stringNode.Type() == "string_fragment" {
			parentNode := stringNode.Parent()
			if parentNode != nil && parentNode.Type() == "template_string" {
				continue
			}
		}

		if processedNodeIDs[stringNode.ID()] {
			continue
		}
		processedNodeIDs[stringNode.ID()] = true

		varName, invFuncName, invReceiverName := determineContextAroundNode(stringNode, contentBytes, langName)

		rawStringNodeContent := stringNode.Content(contentBytes)
		actualContent := ""
		isMultiLineExplicit := false
		nodeType := stringNode.Type()

		switch langName {
		case "python":
			var prefixLen int
			var quoteLen int
			var isRawString bool
			var isBytes bool
			var quoteChar string

			tempStrData := rawStringNodeContent

			if len(tempStrData) > 0 {
				c1 := tempStrData[0]
				if c1 == 'r' || c1 == 'R' {
					isRawString = true
					prefixLen = 1
				}
				if c1 == 'f' || c1 == 'F' {
					prefixLen = 1
				} // f-string, not necessarily raw
				if c1 == 'u' || c1 == 'U' {
					prefixLen = 1
				} // Python 2 unicode, effectively no-op for Python 3 content
				if c1 == 'b' || c1 == 'B' {
					isBytes = true
					prefixLen = 1
				} // Bytes literal

				if len(tempStrData) > prefixLen {
					charNext := tempStrData[prefixLen]
					// Check for fr, rf, Fr, Rf etc.
					if (c1 == 'f' || c1 == 'F') && (charNext == 'r' || charNext == 'R') {
						isRawString = true
						prefixLen = 2
					}
					if (c1 == 'r' || c1 == 'R') && (charNext == 'f' || charNext == 'F') {
						isRawString = true
						prefixLen = 2
					}
				}
			}

			contentAfterPrefix := rawStringNodeContent
			if prefixLen > 0 && len(rawStringNodeContent) >= prefixLen {
				contentAfterPrefix = rawStringNodeContent[prefixLen:]
			} else if prefixLen > 0 && len(rawStringNodeContent) < prefixLen { // e.g. just "r"
				actualContent = ""
				goto endPythonStringProcessing
			}

			if strings.HasPrefix(contentAfterPrefix, "\"\"\"") {
				quoteChar = "\"\"\""
				quoteLen = 3
				isMultiLineExplicit = true
			}
			if strings.HasPrefix(contentAfterPrefix, "'''") {
				quoteChar = "'''"
				quoteLen = 3
				isMultiLineExplicit = true
			}
			if quoteLen == 0 {
				if strings.HasPrefix(contentAfterPrefix, "\"") {
					quoteChar = "\""
					quoteLen = 1
				}
				if strings.HasPrefix(contentAfterPrefix, "'") {
					quoteChar = "'"
					quoteLen = 1
				}
			}

			if quoteLen > 0 {
				if len(contentAfterPrefix) >= 2*quoteLen && strings.HasSuffix(contentAfterPrefix, quoteChar) {
					actualContent = contentAfterPrefix[quoteLen : len(contentAfterPrefix)-quoteLen]
				} else {
					actualContent = contentAfterPrefix[quoteLen:]
					if len(actualContent) > 0 && actualContent[len(actualContent)-1] == contentAfterPrefix[0] && quoteLen == 1 {
						// Handle simple case of missing closing quote for single quoted strings, e.g. "abc' -> abc
						// This is a simple heuristic, might not be perfectly robust for all malformed strings
						// actualContent = actualContent[:len(actualContent)-1] // This line is risky. Better to take as is or clear.
					} else if len(contentAfterPrefix) < 2*quoteLen { // e.g. " or ""
						actualContent = ""
					}
				}
			} else {
				actualContent = contentAfterPrefix
			}

			if !isRawString && !isBytes {
				actualContent = unescapePythonString(actualContent)
			}

			if !isMultiLineExplicit && stringNode.StartPoint().Row != stringNode.EndPoint().Row {
				isMultiLineExplicit = true
			}
		endPythonStringProcessing:
			{
			}

		case "javascript", "typescript":
			if nodeType == "template_string" {
				isMultiLineExplicit = true
				if len(rawStringNodeContent) >= 2 && rawStringNodeContent[0] == '`' && rawStringNodeContent[len(rawStringNodeContent)-1] == '`' {
					actualContent = rawStringNodeContent[1 : len(rawStringNodeContent)-1]
				} else {
					actualContent = rawStringNodeContent
				}
				actualContent = unescapeJSString(actualContent)
			} else if nodeType == "string_fragment" {
				// This case should now only be hit if the string_fragment is NOT part of a template_string
				// (e.g. if the query or grammar changes to allow standalone fragments).
				// For the current setup, the check at the beginning of the loop body handles fragments within template_strings.
				actualContent = unescapeJSString(rawStringNodeContent)
				if strings.Contains(rawStringNodeContent, "\n") {
					isMultiLineExplicit = true
				}
			} else if (strings.HasPrefix(rawStringNodeContent, "\"") && strings.HasSuffix(rawStringNodeContent, "\"")) ||
				(strings.HasPrefix(rawStringNodeContent, "'") && strings.HasSuffix(rawStringNodeContent, "'")) {
				isMultiLineExplicit = false
				if len(rawStringNodeContent) >= 2 {
					actualContent = rawStringNodeContent[1 : len(rawStringNodeContent)-1]
					actualContent = unescapeJSString(actualContent)
				} else {
					actualContent = ""
				}
				if strings.Contains(actualContent, "\n") {
					isMultiLineExplicit = true
				}
			} else {
				actualContent = rawStringNodeContent
			}

			if !isMultiLineExplicit && stringNode.StartPoint().Row != stringNode.EndPoint().Row {
				isMultiLineExplicit = true
			}
		}

		startLine := int(stringNode.StartPoint().Row + 1)
		linesInContent := utils.CountNewlines(actualContent) + 1

		fp := FoundPrompt{
			Filepath:    filePath,
			Line:        startLine,
			Content:     actualContent,
			IsMultiLine: isMultiLineExplicit || linesInContent > 1,
		}
		context := PromptContext{
			Text:                   actualContent,
			VariableName:           varName,
			IsMultiLineExplicit:    isMultiLineExplicit,
			LinesInContent:         linesInContent,
			FileExtension:          ext,
			InvocationFunctionName: invFuncName,
			InvocationReceiverName: invReceiverName,
		}

		if s.IsPotentialPrompt(context, &fp) {
			prompts = append(prompts, fp)
		}
	}
	return prompts, nil
}
