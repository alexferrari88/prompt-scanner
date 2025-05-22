// scanner/defaults.go
package scanner

import "strings"

// DefaultMinLength is the default minimum character length for a string to be considered a potential prompt.
const DefaultMinLength = 30

// --- Variable Keywords ---

// DefaultVarKeywordsList provides the default keywords for variable names as a slice for readability and easy management.
var DefaultVarKeywordsList = []string{
	"prompt",
	"template",
	"system_message",
	"user_message",
	"instruction",
	"persona",
	"query",
	"question",
	"task_description",
	"context_str",
}

// DefaultVarKeywords is the comma-separated string version of DefaultVarKeywordsList, used for flag defaults.
var DefaultVarKeywords = strings.Join(DefaultVarKeywordsList, ",")

// --- Content Keywords ---

// DefaultContentKeywordsList provides the default keywords for content matching as a slice for readability and easy management.
var DefaultContentKeywordsList = []string{
	"you are a",
	"you are an",
	"you are the",
	"act as",
	"from the following",
	"from this",
	"your task is to",
	"you need to",
	"break down",
	"translate the",
	"summarize the",
	"given the",
	"answer the following question",
	"extract entities from",
	"generate code for",
	"what is the",
	"explain the",
	"act as a",
	"respond with",
	"based on the provided text",
	"here's",
	"here is",
	"here are",
	"consider this",
	"consider the following",
	"analyze this",
	"analyze the following",
}

// DefaultContentKeywords is the comma-separated string version of DefaultContentKeywordsList, used for flag defaults.
var DefaultContentKeywords = strings.Join(DefaultContentKeywordsList, ",")

// --- Placeholder Patterns ---

// DefaultPlaceholderPatternsList provides the default regex patterns for identifying templating placeholders as a slice for readability and easy management.
// Each string in this slice is a separate regex pattern.
var DefaultPlaceholderPatternsList = []string{
	`\{[^{}]*?\}`,        // Matches {placeholder}
	`\{\{[^{}]*?\}\}`,    // Matches {{placeholder}}
	`<[^<>]*?>`,          // Matches <placeholder>
	`\$[A-Z_][A-Z0-9_]*`, // Matches $PLACEHOLDER or $PLACEHOLDER_123
	`\%[sdfeuxg]`,        // Matches printf-style format specifiers like %s, %d
	`\[[A-Z_]+\]`,        // Matches [PLACEHOLDER_IN_CAPS_AND_BRACKETS]
}

// DefaultPlaceholderPatterns is the comma-separated string version of DefaultPlaceholderPatternsList, used for flag defaults.
// This allows users to provide comma-separated regex patterns via the command line.
var DefaultPlaceholderPatterns = strings.Join(DefaultPlaceholderPatternsList, ",")
