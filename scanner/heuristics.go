// scanner/heuristics.go
package scanner

import (
	"fmt"
	"regexp"
	"strings"
)

var (
	// Common logging method names (case-insensitive)
	loggingMethodNames = map[string]bool{
		"log": true, "info": true, "warn": true, "warning": true, "error": true,
		"debug": true, "fatal": true, "trace": true, "print": true, "println": true,
		"printf": true, "exception": true, "verbose": true, "notice": true,
		"critical": true, "alert": true, "emerg": true, "emergency": true,
		// Specific ones that might not be caught by simple method name
		"write": true, // sometimes used for logging, e.g. process.stdout.write
	}
	// Common logger object/receiver names or prefixes (case-insensitive)
	// These are harder to rely on solely but can add weight.
	loggingReceiverNames = map[string]bool{
		"log": true, "logger": true, "logging": true, "console": true, "fmt": true,
		"logrus": true, "zap": true, "zerolog": true, "tracer": true, "stderr": true, "stdout": true,
		"process": true, "window": true, "self": true, // common global-ish objects
	}
)

// compileMatchers (no change from previous version, ensure it's called)
func (so *ScanOptions) compileMatchers() error {
	if len(so.VariableKeywords) > 0 {
		pattern := `(?i)\b(` + strings.Join(so.VariableKeywords, "|") + `)\b`
		re, err := regexp.Compile(pattern)
		if err != nil {
			return fmt.Errorf("compiling variable keywords regex: %w", err)
		}
		so.compiledVarKeywords = re
	}
	if len(so.ContentKeywords) > 0 {
		pattern := `(?i)(` + strings.Join(so.ContentKeywords, "|") + `)`
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
func (s *Scanner) IsPotentialPrompt(ctx PromptContext, fp *FoundPrompt) bool {
	text := ctx.Text
	if strings.TrimSpace(text) == "" {
		return false
	}

	// **NEW: Check if the string is an argument to a common logging function**
	lowerFuncName := strings.ToLower(ctx.InvocationFunctionName)
	lowerReceiverName := strings.ToLower(ctx.InvocationReceiverName)

	if lowerFuncName != "" {
		if loggingMethodNames[lowerFuncName] {
			// If the method name itself is a strong logging indicator (like .log, .error, .print)
			// then it's very likely a log message.
			// We can be more aggressive in filtering these.
			// Exception: if the text is extremely long and has template vars, it might still be a prompt used for logging.
			if len(text) < 200 && !strings.Contains(text, "{") && !strings.Contains(text, "%") { // Heuristic length/template check
				return false // Likely a log message, not a prompt
			}
		}
		// If receiver is a known logger, and method is generic like 'write'
		if loggingReceiverNames[lowerReceiverName] && (lowerFuncName == "write" || lowerFuncName == "send" || lowerFuncName == "put") {
			if len(text) < 100 && !strings.Contains(text, "{") {
				return false
			}
		}
	}
	// End of new logging check section

	score := 0
	if ctx.VariableName != "" && s.Options.compiledVarKeywords != nil {
		match := s.Options.compiledVarKeywords.FindString(ctx.VariableName)
		if match != "" {
			fp.MatchedVariableName = match
			score += 3
		}
	}

	if s.Options.compiledContentWords != nil {
		match := s.Options.compiledContentWords.FindString(text)
		if match != "" {
			fp.MatchedContentWord = match
			score += 2
		}
	}

	for _, re := range s.Options.compiledPlaceholders {
		match := re.FindString(text)
		if match != "" {
			fp.MatchedPlaceholder = match
			score += 2
			break
		}
	}

	isLongEnough := len(text) >= s.Options.MinLength
	isMultiLine := ctx.IsMultiLineExplicit || ctx.LinesInContent > 1

	if isMultiLine {
		score += 1
	}
	if isLongEnough {
		score += 1
	}

	if fp.MatchedVariableName != "" && (isLongEnough || isMultiLine || fp.MatchedContentWord != "" || fp.MatchedPlaceholder != "") {
		return true
	}
	if fp.MatchedContentWord != "" && (isLongEnough || isMultiLine || fp.MatchedPlaceholder != "") {
		return true
	}
	if fp.MatchedPlaceholder != "" && (isLongEnough || isMultiLine) {
		return true
	}
	if isMultiLine && isLongEnough && score >= 1 {
		return true
	}
	if isLongEnough && (fp.MatchedContentWord != "" || fp.MatchedPlaceholder != "") {
		return true
	}
	if score >= 2 && isLongEnough {
		return true
	}
	if score >= 3 {
		return true
	}

	if len(text) > s.Options.MinLength*3 && (isMultiLine || strings.ContainsAny(text, ".?!:")) {
		if score < 2 {
			fp.MatchedContentWord = "long_string"
			return true
		}
	}
	return false
}
