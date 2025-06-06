// main.go
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/alexferrari88/prompt-scanner/scanner"
)

var (
	// VLog is a global logger for verbose output. It's initialized in main.
	VLog *log.Logger
)

func main() {
	startTime := time.Now()
	log.SetFlags(0) // Simpler logging for fatal errors and final summary (goes to stderr)

	// --- Define flags ---
	// Output control
	jsonOutput := flag.Bool("json", false, "Output results in JSON format.")
	noFilepath := flag.Bool("no-filepath", false, "Omit the filepath from the default text output.")
	noLinenumber := flag.Bool("no-linenumber", false, "Omit the line number from the default text output.")
	verbose := flag.Bool("verbose", false, "Enable verbose logging output to stderr.")

	// Scanning behavior
	scanConfigs := flag.Bool("scan-configs", false, "Also scan common config files (JSON, YAML, TOML, .env).")
	useGitignore := flag.Bool("use-gitignore", false, "Skip files and directories listed in .gitignore files.")
	greedy := flag.Bool("greedy", false, "Use aggressive (current) heuristics if true. If false, use stricter rules based on content keywords and multi-line criteria.")

	// Heuristic tuning
	minLength := flag.Int("min-len", scanner.DefaultMinLength, "Minimum character length for a string to be considered a potential prompt.")
	varKeywordsStr := flag.String("var-keywords", scanner.DefaultVarKeywords, "Comma-separated keywords for variable or key names.")
	contentKeywordsStr := flag.String("content-keywords", scanner.DefaultContentKeywords, "Comma-separated keywords to search for within string content.")
	placeholderPatternsStr := flag.String("placeholder-patterns", scanner.DefaultPlaceholderPatterns, "Comma-separated regex patterns to identify templating placeholders.")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "LLM Prompt Scanner\nRecursively scans codebases for potential LLM prompts.\n\nUsage:\n  %s [options] <target_path_or_github_url>\n\nOptions:\n", filepath.Base(os.Args[0]))
		flag.PrintDefaults()
	}
	flag.Parse()

	// Initialize VLog based on the verbose flag
	if *verbose {
		VLog = log.New(os.Stderr, "", 0) // Standard log output to stderr for verbose messages
	} else {
		VLog = log.New(io.Discard, "", 0) // Discard verbose logs if not enabled
	}

	if flag.NArg() == 0 {
		flag.Usage()
		os.Exit(1)
	}
	targetInput := flag.Arg(0)

	scanOpts := scanner.ScanOptions{
		MinLength:           *minLength,
		VariableKeywords:    splitAndTrim(*varKeywordsStr),
		ContentKeywords:     splitAndTrim(*contentKeywordsStr),
		PlaceholderPatterns: splitAndTrim(*placeholderPatternsStr),
		ScanConfigs:         *scanConfigs,
		Greedy:              *greedy,
		UseGitignore:        *useGitignore,
		Verbose:             *verbose, // Pass verbose to scanner package for its own internal logs
	}

	s, err := scanner.New(scanOpts)
	if err != nil {
		log.Fatalf("Error initializing scanner: %v", err) // Fatal, always prints to stderr
	}

	var foundPrompts []scanner.FoundPrompt
	scanPath := targetInput
	isTempDir := false
	originalTargetForDisplay := targetInput

	if looksLikeGitHubURL(targetInput) {
		VLog.Printf("GitHub URL detected: %s", targetInput)
		tempDir, errClone := s.CloneRepo(targetInput)
		if errClone != nil {
			log.Fatalf("Error cloning repository '%s': %v", targetInput, errClone)
		}
		scanPath = tempDir
		isTempDir = true
		defer func() {
			VLog.Printf("Cleaning up temporary directory: %s", tempDir)
			if err := os.RemoveAll(tempDir); err != nil {
				// This is a warning, so it might be useful even if not verbose, but let's gate it too.
				VLog.Printf("Warning: Failed to remove temporary directory %s: %v", tempDir, err)
			}
		}()
		VLog.Printf("Repository cloned. Starting scan in %s...", scanPath)
	} else {
		absTarget, errPath := filepath.Abs(targetInput)
		if errPath != nil {
			log.Fatalf("Error resolving absolute path for '%s': %v", targetInput, errPath)
		}
		scanPath = absTarget
		originalTargetForDisplay = scanPath // Use absolute path for display if local
		fileInfo, errStat := os.Stat(scanPath)
		if errStat != nil {
			log.Fatalf("Error accessing target path '%s': %v", scanPath, errStat)
		}
		if fileInfo.IsDir() {
			VLog.Printf("Scanning local directory: %s", scanPath)
		} else {
			VLog.Printf("Scanning local file: %s", scanPath)
		}
	}

	foundPrompts, err = s.ScanDirectory(scanPath)
	if err != nil {
		log.Fatalf("Error during scan of '%s': %v", scanPath, err)
	}

	if *jsonOutput {
		outputJSON(foundPrompts, scanPath, isTempDir, originalTargetForDisplay)
	} else {
		outputText(foundPrompts, *noFilepath, *noLinenumber, scanPath, isTempDir, originalTargetForDisplay)
	}

	duration := time.Since(startTime)
	// Final summary always prints to stderr, as it's essential info.
	log.Printf("Scan complete. Found %d potential prompts in %.2fs from '%s'.", len(foundPrompts), duration.Seconds(), originalTargetForDisplay)
}

func splitAndTrim(s string) []string {
	if s == "" {
		return []string{}
	}
	parts := strings.Split(s, ",")
	cleanedParts := make([]string, 0, len(parts))
	for _, p := range parts {
		trimmed := strings.TrimSpace(p)
		if trimmed != "" {
			cleanedParts = append(cleanedParts, trimmed)
		}
	}
	return cleanedParts
}

func looksLikeGitHubURL(target string) bool {
	if strings.HasPrefix(target, "git@github.com:") {
		return true
	}
	parsedURL, err := url.ParseRequestURI(target)
	if err != nil {
		return false
	}
	return (parsedURL.Scheme == "http" || parsedURL.Scheme == "https") &&
		(strings.HasSuffix(parsedURL.Host, "github.com")) &&
		(strings.HasSuffix(parsedURL.Path, ".git") || !strings.Contains(parsedURL.Path, ".")) // Broader match for repo URLs
}

func outputJSON(prompts []scanner.FoundPrompt, scanRoot string, isTempScan bool, originalTarget string) {
	outputData := make([]scanner.JSONOutput, len(prompts))
	for i, p := range prompts {
		displayFilepath := p.Filepath
		if isTempScan {
			relPath, err := filepath.Rel(scanRoot, p.Filepath)
			if err == nil {
				displayFilepath = relPath // Show path relative to temp cloned dir root
			}
		} else {
			// If original target was a dir, make path relative to it.
			// If it was a file, displayFilepath will remain absolute (or as is).
			info, _ := os.Stat(originalTarget)
			if info != nil && info.IsDir() {
				relPath, err := filepath.Rel(originalTarget, p.Filepath)
				if err == nil {
					displayFilepath = relPath
				}
			}
		}

		outputData[i] = scanner.JSONOutput{
			Filepath: displayFilepath,
			Line:     p.Line,
			Content:  p.Content,
		}
	}
	jsonData, err := json.MarshalIndent(outputData, "", "  ")
	if err != nil {
		log.Fatalf("Error marshalling JSON: %v", err) // Fatal, always prints to stderr
	}
	fmt.Println(string(jsonData)) // JSON output to stdout
}

func outputText(prompts []scanner.FoundPrompt, noFilepath, noLinenumber bool, scanRoot string, isTempScan bool, originalTarget string) {
	for _, p := range prompts {
		displayFilepath := p.Filepath
		if isTempScan {
			relPath, err := filepath.Rel(scanRoot, p.Filepath)
			if err == nil {
				displayFilepath = relPath
			}
		} else {
			info, _ := os.Stat(originalTarget)
			if info != nil && info.IsDir() {
				relPath, err := filepath.Rel(originalTarget, p.Filepath)
				if err == nil {
					displayFilepath = relPath
				}
			}
		}

		var prefixParts []string
		if !noFilepath {
			prefixParts = append(prefixParts, displayFilepath)
		}
		if !noLinenumber {
			prefixParts = append(prefixParts, fmt.Sprintf("%d", p.Line))
		}

		prefix := strings.Join(prefixParts, ":")
		fullPrefixWithTab := ""
		if prefix != "" {
			fullPrefixWithTab = prefix + "\t"
		}

		normalizedContent := strings.ReplaceAll(p.Content, "\r\n", "\n")
		lines := strings.Split(strings.TrimRight(normalizedContent, "\n"), "\n")

		if len(lines) > 0 {
			// Text output (prompts) to stdout
			fmt.Printf("%s%s%s", fullPrefixWithTab, lines[0], "\n")

			indentation := ""
			if fullPrefixWithTab != "" {
				// Ensure indentation matches the visual start of the first line's content
				indentation = strings.Repeat(" ", len(strings.Split(fullPrefixWithTab, "\t")[0])) + "\t"
			}

			for i := 1; i < len(lines); i++ {
				fmt.Printf("%s%s%s", indentation, lines[i], "\n")
			}
		} else if p.Content == "" && fullPrefixWithTab != "" { // Handle empty content line if prefix exists
			fmt.Printf("%s%s", fullPrefixWithTab, "\n")
		}
	}
}
