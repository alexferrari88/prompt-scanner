// scanner/config_parser.go
package scanner

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/BurntSushi/toml"
	"gopkg.in/yaml.v3"

	"github.com/alexferrari88/prompt-scanner/utils" // Adjust import path
)

// ParseJSONFile parses JSON files for potential prompts.
// Note: Line numbers for specific values within JSON are hard to get accurately
// without a more sophisticated streaming parser or custom unmarshaler.
// Current implementation defaults to line 1 or the line of the containing object if known.
func (s *Scanner) ParseJSONFile(filePath string, contentBytes []byte) ([]FoundPrompt, error) {
	var data interface{}
	// Using json.Decoder to potentially get more info in the future, but line numbers are still tricky.
	decoder := json.NewDecoder(bytes.NewReader(contentBytes))
	if err := decoder.Decode(&data); err != nil {
		return nil, fmt.Errorf("unmarshalling JSON from %s: %w", filePath, err)
	}

	var prompts []FoundPrompt
	ext := filepath.Ext(filePath)

	// Recursive helper to find strings
	var findStrings func(currentJSONPath string, node interface{}, lineHint int)
	findStrings = func(currentJSONPath string, node interface{}, lineHint int) {
		switch v := node.(type) {
		case map[string]interface{}:
			for key, val := range v {
				newPath := key
				if currentJSONPath != "" {
					newPath = currentJSONPath + "." + key
				}
				findStrings(newPath, val, lineHint) // Line hint propagation is approximate
			}
		case []interface{}:
			for i, item := range v {
				newPath := fmt.Sprintf("%s[%d]", currentJSONPath, i)
				findStrings(newPath, item, lineHint)
			}
		case string:
			if v == "" { // Skip empty strings early
				return
			}
			linesInContent := utils.CountNewlines(v) + 1
			isMultiLineExplicit := strings.Contains(v, "\n") // Simple check for JSON

			fp := FoundPrompt{
				Filepath:    filePath,
				Line:        lineHint, // Approximate line number
				Content:     v,
				IsMultiLine: isMultiLineExplicit || linesInContent > 1,
			}
			context := PromptContext{
				Text:                v,
				VariableName:        currentJSONPath, // Using JSON path as "variable name"
				IsMultiLineExplicit: isMultiLineExplicit,
				LinesInContent:      linesInContent,
				FileExtension:       ext,
			}
			if s.IsPotentialPrompt(context, &fp) {
				prompts = append(prompts, fp)
			}
		}
	}

	findStrings("", data, 1) // Start with line 1 as a general hint
	return prompts, nil
}

// ParseYAMLFile parses YAML files using gopkg.in/yaml.v3, which provides line numbers.
func (s *Scanner) ParseYAMLFile(filePath string, contentBytes []byte) ([]FoundPrompt, error) {
	var root yaml.Node
	if err := yaml.Unmarshal(contentBytes, &root); err != nil {
		return nil, fmt.Errorf("unmarshalling YAML from %s: %w", filePath, err)
	}

	var prompts []FoundPrompt
	ext := filepath.Ext(filePath)

	var findYAMLStrings func(node *yaml.Node, keyPath string)
	findYAMLStrings = func(node *yaml.Node, keyPath string) {
		if node == nil {
			return
		}
		currentKeyName := keyPath // Default to inherited key path

		if node.Kind == yaml.ScalarNode && (node.Tag == "!!str" || node.Tag == "") { // Tag can be empty for plain scalars
			val := node.Value
			if val == "" { // Skip empty strings early
				return
			}
			linesInContent := utils.CountNewlines(val) + 1
			// literal style means multi-line, folded also usually implies it with newlines
			isMultiLineExplicit := node.Style == yaml.LiteralStyle || node.Style == yaml.FoldedStyle || (node.Style == 0 && strings.Contains(val, "\n"))

			fp := FoundPrompt{
				Filepath:    filePath,
				Line:        node.Line, // yaml.v3 provides this
				Content:     val,
				IsMultiLine: isMultiLineExplicit || linesInContent > 1,
			}
			context := PromptContext{
				Text:                val,
				VariableName:        currentKeyName,
				IsMultiLineExplicit: isMultiLineExplicit,
				LinesInContent:      linesInContent,
				FileExtension:       ext,
			}
			if s.IsPotentialPrompt(context, &fp) {
				prompts = append(prompts, fp)
			}
		} else if node.Kind == yaml.MappingNode {
			for i := 0; i < len(node.Content); i += 2 {
				keyNode := node.Content[i]
				valueNode := node.Content[i+1]
				fullKeyPath := keyNode.Value
				if keyPath != "" {
					fullKeyPath = keyPath + "." + keyNode.Value
				}
				findYAMLStrings(valueNode, fullKeyPath)
			}
		} else if node.Kind == yaml.SequenceNode {
			for i, itemNode := range node.Content {
				// For sequences, the "key" is often the parent key with an index.
				indexedKeyPath := fmt.Sprintf("%s[%d]", keyPath, i)
				findYAMLStrings(itemNode, indexedKeyPath)
			}
		}
	}

	// The root node itself is a DocumentNode, its content is usually a single MappingNode or SequenceNode
	if len(root.Content) > 0 {
		findYAMLStrings(root.Content[0], "") // Start with an empty key path
	}
	return prompts, nil
}

// ParseTOMLFile parses TOML files.
// Note: Line numbers for specific values are not easily available from BurntSushi/toml's basic Decode.
// Defaults to line 1.
func (s *Scanner) ParseTOMLFile(filePath string, contentBytes []byte) ([]FoundPrompt, error) {
	var data map[string]interface{}
	if _, err := toml.Decode(string(contentBytes), &data); err != nil {
		return nil, fmt.Errorf("decoding TOML from %s: %w", filePath, err)
	}

	var prompts []FoundPrompt
	ext := filepath.Ext(filePath)

	var findTOMLStrings func(currentTOMLPath string, node interface{})
	findTOMLStrings = func(currentTOMLPath string, node interface{}) {
		switch v := node.(type) {
		case map[string]interface{}:
			for key, val := range v {
				newPath := key
				if currentTOMLPath != "" {
					newPath = currentTOMLPath + "." + key
				}
				findTOMLStrings(newPath, val)
			}
		case []interface{}:
			for i, item := range v {
				newPath := fmt.Sprintf("%s[%d]", currentTOMLPath, i)
				findTOMLStrings(newPath, item)
			}
		case string:
			if v == "" {
				return
			}
			linesInContent := utils.CountNewlines(v) + 1
			// TOML multi-line strings are `"""..."""` or `'''...'''`
			// A simple check for contained newlines can also indicate multi-line presentation.
			isMultiLineExplicit := strings.Contains(v, "\n")

			fp := FoundPrompt{
				Filepath:    filePath,
				Line:        1, // Approximate line number for TOML values
				Content:     v,
				IsMultiLine: isMultiLineExplicit || linesInContent > 1,
			}
			context := PromptContext{
				Text:                v,
				VariableName:        currentTOMLPath,
				IsMultiLineExplicit: isMultiLineExplicit,
				LinesInContent:      linesInContent,
				FileExtension:       ext,
			}
			if s.IsPotentialPrompt(context, &fp) {
				prompts = append(prompts, fp)
			}
		}
	}
	findTOMLStrings("", data)
	return prompts, nil
}

// ParseEnvFile parses .env files for potential prompts.
func (s *Scanner) ParseEnvFile(filePath string, contentBytes []byte) ([]FoundPrompt, error) {
	var prompts []FoundPrompt
	scanner := bufio.NewScanner(bytes.NewReader(contentBytes))
	lineNumber := 0
	ext := filepath.Ext(filePath) // Though usually no ext, could be .env.local

	for scanner.Scan() {
		lineNumber++
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		parts := strings.SplitN(line, "=", 2)
		if len(parts) == 2 {
			key := strings.TrimSpace(parts[0])
			valueStr := strings.TrimSpace(parts[1])
			actualValue := valueStr

			// Attempt to unquote .env values if they are quoted
			if (strings.HasPrefix(valueStr, `"`) && strings.HasSuffix(valueStr, `"`)) ||
				(strings.HasPrefix(valueStr, `'`) && strings.HasSuffix(valueStr, `'`)) {
				if len(valueStr) >= 2 {
					parsedVal, err := strconv.Unquote(valueStr) // Handles basic escapes within quotes
					if err == nil {
						actualValue = parsedVal
					} else {
						// Fallback to simple trim if Unquote fails (e.g. mismatched quotes)
						actualValue = valueStr[1 : len(valueStr)-1]
					}
				} else {
					actualValue = "" // Empty if just "" or ''
				}
			} else {
				// If not quoted, treat backslash escapes literally as per some .env parsers,
				// or unescape common ones if that's the desired behavior.
				// For now, assume standard .env doesn't do much unescaping outside quotes.
				// Python-dotenv, for example, does unescape \n, \t etc. if value is quoted.
				// If we want to replicate that for unquoted values, add it here.
				// Example: actualValue = strings.ReplaceAll(actualValue, "\\n", "\n")
			}

			if actualValue == "" {
				continue
			}

			linesInContent := utils.CountNewlines(actualValue) + 1
			// .env values are typically single line unless explicitly containing \n (from parsing)
			isMultiLineExplicit := strings.Contains(actualValue, "\n")

			fp := FoundPrompt{
				Filepath:    filePath,
				Line:        lineNumber,
				Content:     actualValue,
				IsMultiLine: isMultiLineExplicit || linesInContent > 1,
			}
			context := PromptContext{
				Text:                actualValue,
				VariableName:        key,
				IsMultiLineExplicit: isMultiLineExplicit,
				LinesInContent:      linesInContent,
				FileExtension:       ext, // Could be empty if filename is just ".env"
			}
			if s.IsPotentialPrompt(context, &fp) {
				prompts = append(prompts, fp)
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("reading .env file %s: %w", filePath, err)
	}
	return prompts, nil
}
