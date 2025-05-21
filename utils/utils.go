// utils/utils.go
package utils

import (
	"os/exec"
	"strings"
)

// SanitizeStringContent cleans up common string literal artifacts.
// For Go AST, use strconv.Unquote. For tree-sitter, node.Content() is often enough.
// This is a generic helper if needed for simpler parsers or specific cleaning.
func SanitizeStringContent(raw string, quoteChar byte) string {
	content := raw
	if len(content) >= 2 && content[0] == quoteChar && content[len(content)-1] == quoteChar {
		content = content[1 : len(content)-1]
	}
	// Basic unescaping for common cases if not handled by a full unquoter
	// This is very simplified; proper unescaping is complex.
	content = strings.ReplaceAll(content, "\\n", "\n")
	content = strings.ReplaceAll(content, "\\t", "\t")
	if quoteChar == '"' {
		content = strings.ReplaceAll(content, "\\\"", "\"")
	}
	if quoteChar == '\'' {
		content = strings.ReplaceAll(content, "\\'", "'")
	}
	return content
}

// CountNewlines counts newlines in a string.
func CountNewlines(s string) int {
	return strings.Count(s, "\n")
}

// CommandExists checks if a command exists on the system PATH.
func CommandExists(cmd string) bool {
	_, err := exec.LookPath(cmd)
	return err == nil
}