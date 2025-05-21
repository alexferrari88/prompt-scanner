// scanner/treesitter_parser.go
package scanner

import (
	"context"
	"fmt" // Keep for debugging if necessary
	"path/filepath"

	// "regexp" // Not used here anymore
	"strings"

	sitter "github.com/smacker/go-tree-sitter"
	"github.com/smacker/go-tree-sitter/javascript"
	"github.com/smacker/go-tree-sitter/python"
	"github.com/smacker/go-tree-sitter/typescript/typescript"

	"github.com/alexferrari88/prompt-scanner/utils" // Adjust import path
)

var (
	langToGrammar = map[string]*sitter.Language{
		"python":     python.GetLanguage(),
		"javascript": javascript.GetLanguage(),
		"typescript": typescript.GetLanguage(),
	}

	// Tree-sitter queries targeting string literals and their assignments.
	// These queries are designed NOT to match comment nodes.
	langToQueries = map[string]string{
		"python": `
			(string) @string_node
			(assignment
				left: (identifier) @var.name
				right: (string) @string_node)
            (call
                function: (identifier) @func.name
                arguments: (argument_list (string) @string_node))
		`,
		"javascript": `
			[
				(string_fragment) ;; for template string parts
				(string) ;; for regular strings "" ''
				(template_string) @string_node_ts ;; for full template strings ` + "`" + `...` + "`" + `
			] @string_node
			(assignment_expression
				left: [ (identifier) (member_expression) ] @var.name
				right: [ (string) (template_string) ] @string_node)
			(variable_declarator
				name: (identifier) @var.name
				value: [ (string) (template_string) ] @string_node)
            (call_expression
                arguments: (arguments [ (string) (template_string) ] @string_node))
		`,
		"typescript": `
			[
				(string_fragment)
				(string)
				(template_string) @string_node_ts
			] @string_node
			(assignment_expression
				left: [ (identifier) (member_expression) ] @var.name
				right: [ (string) (template_string) ] @string_node)
			(lexical_declaration
				(variable_declarator
					name: (identifier) @var.name
					value: [ (string) (template_string) ] @string_node))
            (call_expression
                arguments: (arguments [ (string) (template_string) ] @string_node))
		`,
	}
)

// unescapePythonString (simplified)
func unescapePythonString(s string) string {
	s = strings.ReplaceAll(s, "\\n", "\n")
	s = strings.ReplaceAll(s, "\\t", "\t")
	s = strings.ReplaceAll(s, "\\'", "'")
	s = strings.ReplaceAll(s, "\\\"", "\"")
	s = strings.ReplaceAll(s, "\\\\", "\\")
	return s
}

// unescapeJSString (simplified)
func unescapeJSString(s string) string {
	s = strings.ReplaceAll(s, "\\n", "\n")
	s = strings.ReplaceAll(s, "\\t", "\t")
	s = strings.ReplaceAll(s, "\\'", "'")
	s = strings.ReplaceAll(s, "\\\"", "\"")
	s = strings.ReplaceAll(s, "\\`", "`") // For template literals
	s = strings.ReplaceAll(s, "\\\\", "\\")
	return s
}

func (s *Scanner) ParseTreeSitterFile(filePath string, contentBytes []byte, langName string) ([]FoundPrompt, error) {
	lang, supported := langToGrammar[langName]
	if !supported {
		return nil, fmt.Errorf("tree-sitter grammar for language '%s' not supported/loaded", langName)
	}
	queryString, hasQuery := langToQueries[langName]
	if !hasQuery {
		return nil, fmt.Errorf("tree-sitter query for language '%s' not defined", langName)
	}

	parser := sitter.NewParser()
	parser.SetLanguage(lang)
	tree, err := parser.ParseCtx(context.Background(), nil, contentBytes)
	if err != nil {
		return nil, fmt.Errorf("tree-sitter parsing error for %s: %w", filePath, err)
	}
	defer tree.Close()

	q, err := sitter.NewQuery([]byte(queryString), lang)
	if err != nil {
		return nil, fmt.Errorf("tree-sitter query compilation error for %s: %w", langName, err)
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

		varName := ""
		stringNode := (*sitter.Node)(nil)

		for _, capture := range m.Captures {
			captureName := q.CaptureNameForId(capture.Index)
			node := capture.Node
			nodeType := node.Type()

			if strings.Contains(nodeType, "comment") { // Explicitly skip comment node types
				stringNode = nil // Ensure stringNode is not set if a comment was somehow captured
				break            // Break from inner capture loop for this match
			}

			switch captureName {
			case "var.name":
				varName = node.Content(contentBytes)
			case "string_node", "string_node_ts":
				if strings.Contains(nodeType, "string") || nodeType == "template_string" || nodeType == "string_fragment" {
					stringNode = node
				}
			}
		}

		if stringNode == nil { // If no valid string node was found for this match (e.g., skipped due to being a comment)
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
			isMultiLineExplicit = strings.Contains(rawStringNodeContent, "\n") // Initial check based on raw content

			// Determine prefix and if it's a raw string
			var prefixLen int
			isRawString := false
			// Order matters: check for longest prefixes first
			if strings.HasPrefix(rawStringNodeContent, "fr\"\"\"") || strings.HasPrefix(rawStringNodeContent, "Fr\"\"\"") ||
				strings.HasPrefix(rawStringNodeContent, "rf\"\"\"") || strings.HasPrefix(rawStringNodeContent, "Rf\"\"\"") ||
				strings.HasPrefix(rawStringNodeContent, "fr'''") || strings.HasPrefix(rawStringNodeContent, "Fr'''") ||
				strings.HasPrefix(rawStringNodeContent, "rf'''") || strings.HasPrefix(rawStringNodeContent, "Rf'''") {
				prefixLen = 5
				isRawString = true
				isMultiLineExplicit = true
			} else if strings.HasPrefix(rawStringNodeContent, "r\"\"\"") || strings.HasPrefix(rawStringNodeContent, "R\"\"\"") ||
				strings.HasPrefix(rawStringNodeContent, "f\"\"\"") || strings.HasPrefix(rawStringNodeContent, "F\"\"\"") ||
				strings.HasPrefix(rawStringNodeContent, "u\"\"\"") || strings.HasPrefix(rawStringNodeContent, "U\"\"\"") ||
				strings.HasPrefix(rawStringNodeContent, "r'''") || strings.HasPrefix(rawStringNodeContent, "R'''") ||
				strings.HasPrefix(rawStringNodeContent, "f'''") || strings.HasPrefix(rawStringNodeContent, "F'''") ||
				strings.HasPrefix(rawStringNodeContent, "u'''") || strings.HasPrefix(rawStringNodeContent, "U'''") {
				prefixLen = 4
				isMultiLineExplicit = true
				if strings.HasPrefix(rawStringNodeContent, "r") || strings.HasPrefix(rawStringNodeContent, "R") {
					isRawString = true
				}
			} else if strings.HasPrefix(rawStringNodeContent, "\"\"\"") || strings.HasPrefix(rawStringNodeContent, "'''") {
				prefixLen = 3
				isMultiLineExplicit = true
			} else if strings.HasPrefix(rawStringNodeContent, "fr") || strings.HasPrefix(rawStringNodeContent, "Fr") ||
				strings.HasPrefix(rawStringNodeContent, "rf") || strings.HasPrefix(rawStringNodeContent, "Rf") {
				prefixLen = 3
				isRawString = true
				isMultiLineExplicit = false // single quoted raw f-string
			} else if strings.HasPrefix(rawStringNodeContent, "r") || strings.HasPrefix(rawStringNodeContent, "R") ||
				strings.HasPrefix(rawStringNodeContent, "f") || strings.HasPrefix(rawStringNodeContent, "F") ||
				strings.HasPrefix(rawStringNodeContent, "u") || strings.HasPrefix(rawStringNodeContent, "U") {
				prefixLen = 2
				isMultiLineExplicit = false // single quoted
				if strings.HasPrefix(rawStringNodeContent, "r") || strings.HasPrefix(rawStringNodeContent, "R") {
					isRawString = true
				}
			} else if strings.HasPrefix(rawStringNodeContent, "\"") || strings.HasPrefix(rawStringNodeContent, "'") {
				prefixLen = 1
				isMultiLineExplicit = false // single quoted
			} else {
				prefixLen = 0 // Should not happen for valid strings from grammar
			}

			// Determine suffix length
			suffixLen := 0
			if isMultiLineExplicit && (strings.HasSuffix(rawStringNodeContent, "\"\"\"") || strings.HasSuffix(rawStringNodeContent, "'''")) {
				suffixLen = 3
			} else if !isMultiLineExplicit && (strings.HasSuffix(rawStringNodeContent, "\"") || strings.HasSuffix(rawStringNodeContent, "'")) {
				suffixLen = 1
			}

			if len(rawStringNodeContent) >= prefixLen+suffixLen {
				actualContent = rawStringNodeContent[prefixLen : len(rawStringNodeContent)-suffixLen]
			} else {
				actualContent = "" // Invalid string format after prefix/suffix logic
			}

			if !isRawString { // Unescape only if not a raw string
				actualContent = unescapePythonString(actualContent)
			}

		case "javascript", "typescript":
			// JS/TS is simpler with template literals vs regular strings
			if nodeType == "template_string" || (strings.HasPrefix(rawStringNodeContent, "`") && strings.HasSuffix(rawStringNodeContent, "`")) {
				isMultiLineExplicit = true          // Template literals are inherently multi-line capable
				if len(rawStringNodeContent) >= 2 { // `content`
					actualContent = rawStringNodeContent[1 : len(rawStringNodeContent)-1]
					actualContent = unescapeJSString(actualContent)
				} else {
					actualContent = ""
				} // Just ``
			} else if nodeType == "string_fragment" { // Part of a template string, content is usually raw from TS parser
				actualContent = unescapeJSString(rawStringNodeContent) // Unescape potential escape sequences within the fragment
			} else if (strings.HasPrefix(rawStringNodeContent, "\"") && strings.HasSuffix(rawStringNodeContent, "\"")) ||
				(strings.HasPrefix(rawStringNodeContent, "'") && strings.HasSuffix(rawStringNodeContent, "'")) {
				isMultiLineExplicit = false         // Explicitly single quoted form
				if len(rawStringNodeContent) >= 2 { // "content" or 'content'
					actualContent = rawStringNodeContent[1 : len(rawStringNodeContent)-1]
					actualContent = unescapeJSString(actualContent)
				} else {
					actualContent = ""
				} // Just "" or ''
			}
			// Fallback for isMultiLineExplicit if not set but content has newlines
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
			Text:                actualContent,
			VariableName:        varName,
			IsMultiLineExplicit: isMultiLineExplicit,
			LinesInContent:      linesInContent,
			FileExtension:       ext,
		}

		if s.IsPotentialPrompt(context, &fp) {
			prompts = append(prompts, fp)
		}
	}
	return prompts, nil
}
