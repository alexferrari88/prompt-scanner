// scanner/treesitter_parser.go
package scanner

import (
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

var (
	langToGrammar = map[string]*sitter.Language{
		"python":     python.GetLanguage(),
		"javascript": javascript.GetLanguage(),
		"typescript": typescript.GetLanguage(),
	}

	// Final attempt at robust JS/TS queries
	langToQueries = map[string]string{
		"python": `
			(string) @string_node
			(assignment
				left: (identifier) @var.name
				right: (string) @string_node)
            (call
                function: [
					(identifier) @call.function
					(attribute
						object: (identifier) @call.receiver
						attribute: (identifier) @call.function)
				]
                arguments: (argument_list (string) @string_node))
		`,
		"javascript": `
			; Basic string literals that are not part of other specific matches below
			[ (string_fragment) (string) (template_string) ] @string_node

			; Strings in assignments
			(assignment_expression
				left: [ (identifier) (member_expression) ] @var.name
				right: [ (string) (template_string) ] @string_node)
			(variable_declarator
				name: (identifier) @var.name
				value: [ (string) (template_string) ] @string_node)

			; Strings as arguments to direct function calls: func("string")
            (call_expression
				function: (identifier) @call.function
                arguments: (arguments ([ (string) (template_string) ] @string_node)))

			; Strings as arguments to member function calls: obj.method("string")
			(call_expression
				function: (member_expression
					object: (_) @call.receiver ; Capture any node as receiver
					property: (_) @call.function_prop ; Capture any node as property
				)
				arguments: (arguments ([ (string) (template_string) ] @string_node)))
		`,
		"typescript": ` ;; Similar to JavaScript
			[ (string_fragment) (string) (template_string) ] @string_node
			(assignment_expression
				left: [ (identifier) (member_expression) ] @var.name
				right: [ (string) (template_string) ] @string_node)
			(lexical_declaration
				(variable_declarator
					name: (identifier) @var.name
					value: [ (string) (template_string) ] @string_node))
            (call_expression
				function: (identifier) @call.function
                arguments: (arguments ([ (string) (template_string) ] @string_node)))
			(call_expression
				function: (member_expression
					object: (_) @call.receiver
					property: (_) @call.function_prop
				)
				arguments: (arguments ([ (string) (template_string) ] @string_node)))
		`,
	}
)

// unescapePythonString (no change)
func unescapePythonString(s string) string {
	s = strings.ReplaceAll(s, "\\n", "\n")
	s = strings.ReplaceAll(s, "\\t", "\t")
	s = strings.ReplaceAll(s, "\\'", "'")
	s = strings.ReplaceAll(s, "\\\"", "\"")
	s = strings.ReplaceAll(s, "\\\\", "\\")
	return s
}

// unescapeJSString (no change)
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
		return nil, fmt.Errorf("tree-sitter query for '%s' not defined", langName)
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
		return nil, fmt.Errorf("ts query compilation error for %s (query: \n%s\nError: %w)", langName, queryString, err)
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

		var varName, currentInvFuncName, currentInvReceiverName string
		stringNode := (*sitter.Node)(nil)

		// Process captures for the current match `m`
		for _, capture := range m.Captures {
			node := capture.Node
			captureName := q.CaptureNameForId(capture.Index)
			nodeTypeStr := node.Type()

			if strings.Contains(nodeTypeStr, "comment") {
				stringNode = nil
				break
			}

			switch captureName {
			case "var.name":
				varName = node.Content(contentBytes)
			case "string_node", "string_node_ts": // string_node_ts was mainly for template_string within choice
				// Basic string types are now captured by the first pattern or as part of others.
				// Ensure stringNode is only set if the node is a string type.
				if strings.Contains(nodeTypeStr, "string") || nodeTypeStr == "template_string" || nodeTypeStr == "string_fragment" {
					stringNode = node
				}
			case "call.function":
				currentInvFuncName = node.Content(contentBytes)
			case "call.function_prop":
				currentInvFuncName = node.Content(contentBytes)
			case "call.receiver":
				currentInvReceiverName = node.Content(contentBytes)
			}
		}

		if stringNode == nil {
			continue
		}

		if processedNodeIDs[stringNode.ID()] {
			continue
		}
		processedNodeIDs[stringNode.ID()] = true

		rawStringNodeContent := stringNode.Content(contentBytes)
		actualContent := rawStringNodeContent
		isMultiLineExplicit := false
		nodeType := stringNode.Type()

		switch langName {
		case "python":
			isMultiLineExplicit = strings.Contains(rawStringNodeContent, "\n")
			var prefixLen int
			isRawString := false
			if strings.HasPrefix(rawStringNodeContent, "fr\"\"\"") || strings.HasPrefix(rawStringNodeContent, "Fr\"\"\"") || strings.HasPrefix(rawStringNodeContent, "rf\"\"\"") || strings.HasPrefix(rawStringNodeContent, "Rf\"\"\"") || strings.HasPrefix(rawStringNodeContent, "fr'''") || strings.HasPrefix(rawStringNodeContent, "Fr'''") || strings.HasPrefix(rawStringNodeContent, "rf'''") || strings.HasPrefix(rawStringNodeContent, "Rf'''") {
				prefixLen = 5
				isRawString = true
				isMultiLineExplicit = true
			} else if strings.HasPrefix(rawStringNodeContent, "r\"\"\"") || strings.HasPrefix(rawStringNodeContent, "R\"\"\"") || strings.HasPrefix(rawStringNodeContent, "f\"\"\"") || strings.HasPrefix(rawStringNodeContent, "F\"\"\"") || strings.HasPrefix(rawStringNodeContent, "u\"\"\"") || strings.HasPrefix(rawStringNodeContent, "U\"\"\"") || strings.HasPrefix(rawStringNodeContent, "r'''") || strings.HasPrefix(rawStringNodeContent, "R'''") || strings.HasPrefix(rawStringNodeContent, "f'''") || strings.HasPrefix(rawStringNodeContent, "F'''") || strings.HasPrefix(rawStringNodeContent, "u'''") || strings.HasPrefix(rawStringNodeContent, "U'''") {
				prefixLen = 4
				isMultiLineExplicit = true
				if strings.HasPrefix(rawStringNodeContent, "r") || strings.HasPrefix(rawStringNodeContent, "R") {
					isRawString = true
				}
			} else if strings.HasPrefix(rawStringNodeContent, "\"\"\"") || strings.HasPrefix(rawStringNodeContent, "'''") {
				prefixLen = 3
				isMultiLineExplicit = true
			} else if strings.HasPrefix(rawStringNodeContent, "fr") || strings.HasPrefix(rawStringNodeContent, "Fr") || strings.HasPrefix(rawStringNodeContent, "rf") || strings.HasPrefix(rawStringNodeContent, "Rf") {
				prefixLen = 3
				isRawString = true
				isMultiLineExplicit = false
			} else if strings.HasPrefix(rawStringNodeContent, "r") || strings.HasPrefix(rawStringNodeContent, "R") || strings.HasPrefix(rawStringNodeContent, "f") || strings.HasPrefix(rawStringNodeContent, "F") || strings.HasPrefix(rawStringNodeContent, "u") || strings.HasPrefix(rawStringNodeContent, "U") {
				prefixLen = 2
				isMultiLineExplicit = false
				if strings.HasPrefix(rawStringNodeContent, "r") || strings.HasPrefix(rawStringNodeContent, "R") {
					isRawString = true
				}
			} else if strings.HasPrefix(rawStringNodeContent, "\"") || strings.HasPrefix(rawStringNodeContent, "'") {
				prefixLen = 1
				isMultiLineExplicit = false
			} else {
				prefixLen = 0
			}
			suffixLen := 0
			if isMultiLineExplicit && (strings.HasSuffix(rawStringNodeContent, "\"\"\"") || strings.HasSuffix(rawStringNodeContent, "'''")) {
				suffixLen = 3
			} else if !isMultiLineExplicit && (strings.HasSuffix(rawStringNodeContent, "\"") || strings.HasSuffix(rawStringNodeContent, "'")) {
				suffixLen = 1
			}
			if len(rawStringNodeContent) >= prefixLen+suffixLen {
				actualContent = rawStringNodeContent[prefixLen : len(rawStringNodeContent)-suffixLen]
			} else {
				actualContent = ""
			}
			if !isRawString {
				actualContent = unescapePythonString(actualContent)
			}

		case "javascript", "typescript":
			if nodeType == "template_string" || (strings.HasPrefix(rawStringNodeContent, "`") && strings.HasSuffix(rawStringNodeContent, "`")) {
				isMultiLineExplicit = true
				if len(rawStringNodeContent) >= 2 {
					actualContent = rawStringNodeContent[1 : len(rawStringNodeContent)-1]
					actualContent = unescapeJSString(actualContent)
				} else {
					actualContent = ""
				}
			} else if nodeType == "string_fragment" {
				actualContent = unescapeJSString(rawStringNodeContent)
			} else if (strings.HasPrefix(rawStringNodeContent, "\"") && strings.HasSuffix(rawStringNodeContent, "\"")) || (strings.HasPrefix(rawStringNodeContent, "'") && strings.HasSuffix(rawStringNodeContent, "'")) {
				isMultiLineExplicit = false
				if len(rawStringNodeContent) >= 2 {
					actualContent = rawStringNodeContent[1 : len(rawStringNodeContent)-1]
					actualContent = unescapeJSString(actualContent)
				} else {
					actualContent = ""
				}
			}
			if !isMultiLineExplicit && strings.Contains(actualContent, "\n") {
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
			InvocationFunctionName: currentInvFuncName,
			InvocationReceiverName: currentInvReceiverName,
		}

		if s.IsPotentialPrompt(context, &fp) {
			prompts = append(prompts, fp)
		}
	}
	return prompts, nil
}
