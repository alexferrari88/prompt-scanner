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
		"write": true,
	}
	// Common logger object/receiver names or prefixes (case-insensitive)
	loggingReceiverNames = map[string]bool{
		"log": true, "logger": true, "logging": true, "console": true, "fmt": true,
		"logrus": true, "zap": true, "zerolog": true, "tracer": true, "stderr": true, "stdout": true,
		"process": true, "window": true, "self": true,
	}
	// Keywords that, if a string starts with them, make it likely a log/error message (case-insensitive)
	logMessagePrefixes = []string{
		"error:", "error ", "warning:", "warning ", "info:", "info ", "debug:", "debug ",
		"failed to", "unable to", "could not", "exception:", "uncaught", "unhandled",
		"trace:", "notice:", "critical:", "alert:", "emerg:", "emergency:",
	}
	compiledLogMessagePrefixes []*regexp.Regexp
)

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

	// Compile log message prefixes
	compiledLogMessagePrefixes = make([]*regexp.Regexp, 0, len(logMessagePrefixes))
	for _, prefix := range logMessagePrefixes {
		re, err := regexp.Compile(`(?i)^\s*` + regexp.QuoteMeta(prefix))
		if err != nil {
			return fmt.Errorf("compiling log message prefix '%s': %w", prefix, err)
		}
		compiledLogMessagePrefixes = append(compiledLogMessagePrefixes, re)
	}
	return nil
}

func (s *Scanner) IsPotentialPrompt(ctx PromptContext, fp *FoundPrompt) bool {
	text := strings.TrimSpace(ctx.Text)
	if text == "" {
		return false
	}

	// New logic for the 'greedy' flag
	if !s.Options.Greedy {
		lowerText := strings.ToLower(text)
		isMultiLine := ctx.IsMultiLineExplicit || ctx.LinesInContent > 1

		// Condition 1: String starts with a content keyword
		for _, keyword := range s.Options.ContentKeywords {
			if strings.HasPrefix(lowerText, strings.ToLower(keyword)) {
				fp.MatchedContentWord = keyword // Record the keyword that matched
				return true
			}
		}

		// Condition 2: String contains a content keyword AND is multi-line
		if isMultiLine {
			for _, keyword := range s.Options.ContentKeywords {
				if strings.Contains(lowerText, strings.ToLower(keyword)) {
					fp.MatchedContentWord = keyword // Record the keyword that matched
					return true
				}
			}
		}
		// If neither of the greedy=false conditions are met, it's not a prompt under this mode.
		return false
	} else {
		// Original heuristic logic (when greedy is true)
		for _, re := range compiledLogMessagePrefixes {
			if re.MatchString(text) {
				placeholderFound := false
				for _, pRe := range s.Options.compiledPlaceholders {
					if pRe.MatchString(text) {
						placeholderFound = true
						break
					}
				}
				if len(text) < 150 && !placeholderFound {
					return false
				}
			}
		}

		lowerFuncName := strings.ToLower(ctx.InvocationFunctionName)
		lowerReceiverName := strings.ToLower(ctx.InvocationReceiverName)

		if lowerFuncName != "" {
			if (lowerFuncName == "error" && (lowerReceiverName == "" || lowerReceiverName == "new")) ||
				lowerFuncName == "throw" || // Added for JS 'throw "string"' which might be captured by parent type
				(lowerReceiverName == "" && lowerFuncName == "throw_literal") { // Special marker for throw "literal"
				if len(text) < 150 && !strings.Contains(text, "{") {
					return false
				}
			}

			if loggingMethodNames[lowerFuncName] {
				placeholderFound := false
				for _, pRe := range s.Options.compiledPlaceholders {
					if pRe.MatchString(text) {
						placeholderFound = true
						break
					}
				}
				if len(text) < 200 && !placeholderFound {
					return false
				}
			}
			if loggingReceiverNames[lowerReceiverName] && (loggingMethodNames[lowerFuncName] || lowerFuncName == "write") {
				if len(text) < 100 && !strings.Contains(text, "{") {
					return false
				}
			}
		}

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
	} // End of else (greedy == true)
}
