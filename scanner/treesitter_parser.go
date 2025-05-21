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

	"github.com/alexferrari88/prompt-scanner/utils" // Adjust import path
)

var (
	langToGrammar = map[string]*sitter.Language{
		"python":     python.GetLanguage(),
		"javascript": javascript.GetLanguage(),
		"typescript": typescript.GetLanguage(),
	}

	// Tree-sitter queries. These are crucial and can be complex.
	// We aim to capture strings assigned to variables or standalone strings.
	// @string.content allows for capturing the part of the node that is the actual content.
	// Some grammars might have specific nodes for content excluding quotes.
	langToQueries = map[string]string{
		"python": `
			(string) @string_node
			(assignment
				left: (identifier) @var.name
				right: (string) @string_node)
            (call
                function: (identifier) @func.name
                arguments: (argument_list (string) @string_node)) ; String as function arg
		`,
		"javascript": `
			[
				(string_fragment) ;; for template string parts
				(string) ;; for regular strings "" ''
				(template_string) @string_node ;; for full template strings ` + "`" + `...` + "`" + `
			] @string_node_alt 
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
				(template_string) @string_node
			] @string_node_alt
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

// unescapePythonString is a simplified unescaper.
// Python's `ast.literal_eval` is the true way, but not available here.
func unescapePythonString(s string) string {
	s = strings.ReplaceAll(s, "\\n", "\n")
	s = strings.ReplaceAll(s, "\\t", "\t")
	s = strings.ReplaceAll(s, "\\'", "'")
	s = strings.ReplaceAll(s, "\\\"", "\"")
	s = strings.ReplaceAll(s, "\\\\", "\\")
	return s
}

// unescapeJSString is a simplified unescaper for JS/TS.
func unescapeJSString(s string) string {
	s = strings.ReplaceAll(s, "\\n", "\n")
	s = strings.ReplaceAll(s, "\\t", "\t")
	s = strings.ReplaceAll(s, "\\'", "'")
	s = strings.ReplaceAll(s, "\\\"", "\"")
	s = strings.ReplaceAll(s, "\\`", "`")
	s = strings.ReplaceAll(s, "\\\\", "\\")
	// TODO: Add more comprehensive unescaping (e.g., \uXXXX, \xXX) if needed
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
	processedNodeIDs := make(map[uintptr]bool) // Use node ID to avoid reprocessing

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
			switch captureName {
			case "var.name":
				varName = node.Content(contentBytes)
			case "string_node", "string_node_alt": // Prioritize explicitly named string nodes
				stringNode = node
			}
		}

		if stringNode == nil {
			continue // No string content captured for this match
		}

		if processedNodeIDs[stringNode.ID()] {
			continue // Already processed this specific string node
		}
		processedNodeIDs[stringNode.ID()] = true

		rawStringNodeContent := stringNode.Content(contentBytes)
		actualContent := rawStringNodeContent
		isMultiLineExplicit := false
		nodeType := stringNode.Type()

		// String content extraction logic per language and node type
		switch langName {
		case "python":
			// Python 'string' nodes often contain multiple 'string_content' or 'escape_sequence' children.
			// The .Content() of the 'string' node itself includes quotes.
			// Example: (string content: (string_start) (string_content) (string_end))
			if strings.HasPrefix(rawStringNodeContent, "r\"\"\"") || strings.HasPrefix(rawStringNodeContent, "R\"\"\"") ||
				strings.HasPrefix(rawStringNodeContent, "r'''") || strings.HasPrefix(rawStringNodeContent, "R'''") {
				isMultiLineExplicit = true
				if len(rawStringNodeContent) >= 7 { // r"""..."""
					actualContent = rawStringNodeContent[4 : len(rawStringNodeContent)-3]
				} else {
					actualContent = ""
				}
			} else if strings.HasPrefix(rawStringNodeContent, "\"\"\"") || strings.HasPrefix(rawStringNodeContent, "'''") {
				isMultiLineExplicit = true
				if len(rawStringNodeContent) >= 6 {
					actualContent = rawStringNodeContent[3 : len(rawStringNodeContent)-3]
					actualContent = unescapePythonString(actualContent)
				} else {
					actualContent = ""
				}
			} else if (strings.HasPrefix(rawStringNodeContent, "r\"") && strings.HasSuffix(rawStringNodeContent, "\"")) ||
				(strings.HasPrefix(rawStringNodeContent, "R\"") && strings.HasSuffix(rawStringNodeContent, "\"")) ||
				(strings.HasPrefix(rawStringNodeContent, "r'") && strings.HasSuffix(rawStringNodeContent, "'")) ||
				(strings.HasPrefix(rawStringNodeContent, "R'") && strings.HasSuffix(rawStringNodeContent, "'")) {
				if len(rawStringNodeContent) >= 3 {
					actualContent = rawStringNodeContent[2 : len(rawStringNodeContent)-1]
				} else {
					actualContent = ""
				}
			} else if (strings.HasPrefix(rawStringNodeContent, "\"") && strings.HasSuffix(rawStringNodeContent, "\"")) ||
				(strings.HasPrefix(rawStringNodeContent, "'") && strings.HasSuffix(rawStringNodeContent, "'")) {
				if len(rawStringNodeContent) >= 2 {
					actualContent = rawStringNodeContent[1 : len(rawStringNodeContent)-1]
					actualContent = unescapePythonString(actualContent)
				} else {
					actualContent = ""
				}
			}
			// F-strings are complex; the query for (string) might get the whole f-string.
			// For simplicity, if it's an f-string, we take its content as is.
			if strings.HasPrefix(rawStringNodeContent, "f\"") || strings.HasPrefix(rawStringNodeContent, "F\"") ||
				strings.HasPrefix(rawStringNodeContent, "f'") || strings.HasPrefix(rawStringNodeContent, "F'") {
				// Already handled by single/triple quote logic above if quotes are present.
				// This ensures f-string content is unescaped if not raw.
				if !(strings.HasPrefix(rawStringNodeContent, "fr") || strings.HasPrefix(rawStringNodeContent, "Fr") ||
					strings.HasPrefix(rawStringNodeContent, "rf") || strings.HasPrefix(rawStringNodeContent, "Rf")) {
					// if it's not a raw f-string, try to unescape the content derived from quote stripping
					if len(rawStringNodeContent) >= 3 && (rawStringNodeContent[1] == '"' || rawStringNodeContent[1] == '\'') {
						actualContent = unescapePythonString(actualContent) // actualContent was already stripped
					}
				}
			}

		case "javascript", "typescript":
			if nodeType == "template_string" || (strings.HasPrefix(rawStringNodeContent, "`") && strings.HasSuffix(rawStringNodeContent, "`")) {
				isMultiLineExplicit = true
				if len(rawStringNodeContent) >= 2 {
					actualContent = rawStringNodeContent[1 : len(rawStringNodeContent)-1]
					actualContent = unescapeJSString(actualContent) // Unescape sequences like \n, \`, etc.
				} else {
					actualContent = ""
				}
			} else if nodeType == "string_fragment" { // Part of a template string
				actualContent = unescapeJSString(rawStringNodeContent) // Already unquoted by grammar usually
			} else if (strings.HasPrefix(rawStringNodeContent, "\"") && strings.HasSuffix(rawStringNodeContent, "\"")) ||
				(strings.HasPrefix(rawStringNodeContent, "'") && strings.HasSuffix(rawStringNodeContent, "'")) {
				if len(rawStringNodeContent) >= 2 {
					actualContent = rawStringNodeContent[1 : len(rawStringNodeContent)-1]
					actualContent = unescapeJSString(actualContent)
				} else {
					actualContent = ""
				}
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
