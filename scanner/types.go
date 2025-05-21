// scanner/types.go
package scanner

import "regexp"

// ScanOptions holds the configuration for a scan.
type ScanOptions struct {
	MinLength           int
	VariableKeywords    []string
	ContentKeywords     []string
	PlaceholderPatterns []string
	ScanConfigs         bool // New flag: whether to scan config files (JSON, YAML, TOML, .env)

	// Compiled regexes for efficiency, initialized by CompileMatchers
	compiledVarKeywords  *regexp.Regexp
	compiledContentWords *regexp.Regexp
	compiledPlaceholders []*regexp.Regexp
}

// FoundPrompt represents a potential LLM prompt found in a file.
type FoundPrompt struct {
	Filepath string `json:"filepath"`
	Line     int    `json:"line"`    // Starting line number of the prompt
	Content  string `json:"content"` // The actual prompt text

	// Internal fields, not for direct JSON output unless transformed
	MatchedVariableName string // If found via variable assignment
	MatchedContentWord  string // If found via content keyword
	MatchedPlaceholder  string // If found via placeholder
	IsMultiLine         bool   // Was the original string multi-line (approximated)
}

// JSONOutput is the structure for the --json flag output
type JSONOutput struct {
	Filepath string `json:"filepath"`
	Line     int    `json:"line"`
	Content  string `json:"content"`
}

// PromptContext provides context to the heuristic checker.
type PromptContext struct {
	Text                string // The string content itself
	VariableName        string // Variable or key name, if applicable
	IsMultiLineExplicit bool   // If the original string literal was explicitly a multi-line type (e.g., Python """str""", JS `str`)
	LinesInContent      int    // Number of lines in the *extracted* string content
	FileExtension       string // e.g., ".py", ".go"
}
