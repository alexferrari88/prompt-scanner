// scanner/types.go
package scanner

import "regexp"

// ScanOptions holds the configuration for a scan.
type ScanOptions struct {
	MinLength           int
	VariableKeywords    []string
	ContentKeywords     []string
	PlaceholderPatterns []string
	ScanConfigs         bool

	compiledVarKeywords  *regexp.Regexp
	compiledContentWords *regexp.Regexp
	compiledPlaceholders []*regexp.Regexp
}

// FoundPrompt represents a potential LLM prompt found in a file.
type FoundPrompt struct {
	Filepath string `json:"filepath"`
	Line     int    `json:"line"`
	Content  string `json:"content"`

	MatchedVariableName string
	MatchedContentWord  string
	MatchedPlaceholder  string
	IsMultiLine         bool
}

// JSONOutput is the structure for the --json flag output
type JSONOutput struct {
	Filepath string `json:"filepath"`
	Line     int    `json:"line"`
	Content  string `json:"content"`
}

// PromptContext provides context to the heuristic checker.
type PromptContext struct {
	Text                   string // The string content itself
	VariableName           string // Variable or key name, if applicable
	IsMultiLineExplicit    bool
	LinesInContent         int
	FileExtension          string
	InvocationFunctionName string // e.g., "log", "info", "print" if string is a direct func arg
	InvocationReceiverName string // e.g., "console", "logger", "fmt" if string is arg to a method call
}
