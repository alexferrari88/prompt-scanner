// scanner/heuristics.go
package scanner

import (
	"fmt"
	"regexp"
	"strings"
)

// CompileMatchers pre-compiles regex patterns for keywords and placeholders.
// Ensure this is called when ScanOptions is created.
func (so *ScanOptions) compileMatchers() error { // Made private, called by NewScanner
	if len(so.VariableKeywords) > 0 {
		pattern := `(?i)\b(` + strings.Join(so.VariableKeywords, "|") + `)\b`
		re, err := regexp.Compile(pattern)
		if err != nil {
			return fmt.Errorf("compiling variable keywords regex: %w", err)
		}
		so.compiledVarKeywords = re
	}

	if len(so.ContentKeywords) > 0 {
		pattern := `(?i)(` + strings.Join(so.ContentKeywords, "|") + `)` // Allow partial matches within text
		re, err := regexp.Compile(pattern)
		if err != nil {
			return fmt.Errorf("compiling content keywords regex: %w", err)
		}
		so.compiledContentWords = re
	}

	so.compiledPlaceholders = make([]*regexp.Regexp, 0, len(so.PlaceholderPatterns))
	for _, pStr := range so.PlaceholderPatterns {
		if pStr == "" {
			continue
		}
		re, err := regexp.Compile(pStr)
		if err != nil {
			return fmt.Errorf("compiling placeholder pattern '%s': %w", pStr, err)
		}
		so.compiledPlaceholders = append(so.compiledPlaceholders, re)
	}
	return nil
}

// IsPotentialPrompt applies heuristics to determine if a string is a potential prompt.
// Returns true if likely a prompt, and populates match details in the FoundPrompt struct.
func (s *Scanner) IsPotentialPrompt(ctx PromptContext, fp *FoundPrompt) bool {
	text := ctx.Text

	// 0. Basic filter: if text is empty after potential processing, it's not a prompt.
	if strings.TrimSpace(text) == "" {
		return false
	}

	// 1. Strong Indicator: Variable Name Match
	if ctx.VariableName != "" && s.Options.compiledVarKeywords != nil {
		match := s.Options.compiledVarKeywords.FindString(ctx.VariableName)
		if match != "" {
			// If var name matches, and text is reasonably long or multi-line, it's a strong signal
			if len(text) > s.Options.MinLength/3 || ctx.IsMultiLineExplicit || ctx.LinesInContent > 1 {
				fp.MatchedVariableName = match
				// Don't return immediately; other indicators can also be true
			}
		}
	}

	// 2. Content Keywords Match
	if s.Options.compiledContentWords != nil {
		match := s.Options.compiledContentWords.FindString(text)
		if match != "" {
			fp.MatchedContentWord = match
		}
	}

	// 3. Placeholder Presence
	for _, re := range s.Options.compiledPlaceholders {
		match := re.FindString(text)
		if match != "" {
			fp.MatchedPlaceholder = match
			break // One placeholder match is enough for this category
		}
	}

	// Score based on findings:
	score := 0
	if fp.MatchedVariableName != "" {
		score += 3 // Strongest indicator
	}
	if fp.MatchedContentWord != "" {
		score += 2
	}
	if fp.MatchedPlaceholder != "" {
		score += 2
	}

	// Consider length and multi-line status for additional scoring or as standalone indicators
	isLongEnough := len(text) >= s.Options.MinLength
	isMultiLine := ctx.IsMultiLineExplicit || ctx.LinesInContent > 1

	if isMultiLine {
		score += 1
	}
	if isLongEnough {
		score += 1
	}

	// Decision based on score or specific combinations:
	if fp.MatchedVariableName != "" && (isLongEnough || isMultiLine || fp.MatchedContentWord != "" || fp.MatchedPlaceholder != "") {
		return true
	}
	if fp.MatchedContentWord != "" && (isLongEnough || isMultiLine || fp.MatchedPlaceholder != "") {
		return true
	}
	if fp.MatchedPlaceholder != "" && (isLongEnough || isMultiLine) {
		return true
	}

	// If it's multi-line AND long enough, even without keywords/placeholders, consider it.
	if isMultiLine && isLongEnough && score >= 1 { // Ensure at least one other small signal or just these two combined
		return true
	}

	// A sufficiently long string with at least one keyword or placeholder if not variable match
	if isLongEnough && (fp.MatchedContentWord != "" || fp.MatchedPlaceholder != "") {
		return true
	}

	// Default to false if no strong combinations are met.
	// Adjust scoring and thresholds as needed based on testing.
	// Example: require a minimum score
	if score >= 2 && isLongEnough { // A general threshold
		return true
	}
	if score >= 3 { // High confidence matches
		return true
	}

	// Fallback for very long strings that might be prompts but don't hit other markers.
	// Be cautious with this, as it can be noisy.
	if len(text) > s.Options.MinLength*3 && (isMultiLine || strings.ContainsAny(text, ".?!:")) { // Require some sentence structure
		// Only if no other flags were set and score is low
		if score < 2 {
			fp.MatchedContentWord = "long_string" // Special marker for this case
			return true
		}
	}

	return false
}
