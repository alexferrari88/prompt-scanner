// main.go
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/url" // For more robust URL parsing
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/alexferrari88/prompt-scanner/scanner" // Adjust import path
)

func main() {
	startTime := time.Now()
	log.SetFlags(0) // Simpler logging, no timestamps from log package itself

	// Define flags
	jsonOutput := flag.Bool("json", false, "Output results in JSON format.")
	noFilepath := flag.Bool("no-filepath", false, "Omit the filepath from the default text output.")
	noLinenumber := flag.Bool("no-linenumber", false, "Omit the line number from the default text output.")
	minLength := flag.Int("min-len", 30, "Minimum character length for a string to be considered a potential prompt.")
	varKeywordsStr := flag.String("var-keywords", "prompt,template,system_message,user_message,instruction,persona,query,question,task_description,context_str", "Comma-separated keywords for variable or key names.")
	contentKeywordsStr := flag.String("content-keywords", "you are a,your task is to,translate the,summarize the,given the,answer the following question,extract entities from,generate code for,what is the,explain the,act as a,respond with,based on the provided text", "Comma-separated keywords to search for within string content.")
	placeholderPatternsStr := flag.String("placeholder-patterns", `\{[^{}]*?\}|\{\{[^{}]*?\}\}|<[^<>]*?>|\$[A-Z_][A-Z0-9_]*|\%[sdfeuxg]|\[[A-Z_]+\]`, "Comma-separated regex patterns to identify templating placeholders.")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "LLM Prompt Scanner\nRecursively scans codebases for potential LLM prompts.\n\nUsage:\n  %s [options] <target_path_or_github_url>\n\nOptions:\n", filepath.Base(os.Args[0]))
		flag.PrintDefaults()
	}
	flag.Parse()

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
	}

	s, err := scanner.New(scanOpts)
	if err != nil {
		log.Fatalf("Error initializing scanner: %v", err)
	}

	var foundPrompts []scanner.FoundPrompt
	scanPath := targetInput // This will be the actual path scanned (local or temp cloned repo)
	isTempDir := false
	originalTargetForDisplay := targetInput // Used for display if it's a URL

	if looksLikeGitHubURL(targetInput) {
		log.Printf("GitHub URL detected: %s", targetInput)
		tempDir, errClone := s.CloneRepo(targetInput)
		if errClone != nil {
			log.Fatalf("Error cloning repository '%s': %v", targetInput, errClone)
		}
		scanPath = tempDir
		isTempDir = true
		defer func() {
			log.Printf("Cleaning up temporary directory: %s", tempDir)
			if err := os.RemoveAll(tempDir); err != nil {
				log.Printf("Warning: Failed to remove temporary directory %s: %v", tempDir, err)
			}
		}()
		log.Printf("Repository cloned. Starting scan in %s...", scanPath)
	} else {
		absTarget, errPath := filepath.Abs(targetInput)
		if errPath != nil {
			log.Fatalf("Error resolving absolute path for '%s': %v", targetInput, errPath)
		}
		scanPath = absTarget
		originalTargetForDisplay = scanPath // Use the absolute path for local display
		fileInfo, errStat := os.Stat(scanPath)
		if errStat != nil {
			log.Fatalf("Error accessing target path '%s': %v", scanPath, errStat)
		}
		if fileInfo.IsDir() {
			log.Printf("Scanning local directory: %s", scanPath)
		} else {
			log.Printf("Scanning local file: %s", scanPath)
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
		return false // Not a valid URI
	}
	return (parsedURL.Scheme == "http" || parsedURL.Scheme == "https") &&
		(strings.HasSuffix(parsedURL.Host, "github.com")) &&
		(strings.HasSuffix(parsedURL.Path, ".git") || !strings.Contains(parsedURL.Path, ".")) // Basic check for repo path vs file path
}

func outputJSON(prompts []scanner.FoundPrompt, scanRoot string, isTempScan bool, originalTarget string) {
	outputData := make([]scanner.JSONOutput, len(prompts))
	for i, p := range prompts {
		displayFilepath := p.Filepath
		if isTempScan {
			relPath, err := filepath.Rel(scanRoot, p.Filepath)
			if err == nil {
				// For GitHub URLs, make the path relative to the repo root.
				// The originalTarget might be the URL itself, or we could try to derive repo name.
				// For simplicity, using relPath directly under the assumption scanRoot is the repo root.
				displayFilepath = relPath
			}
		} else {
			// For local paths, make them relative to the scanned path if it was a directory.
			// If originalTarget was a file, displayFilepath is already correct.
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
		log.Fatalf("Error marshalling JSON: %v", err)
	}
	fmt.Println(string(jsonData))
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

		// Normalize newlines for splitting and then print with OS-specific newlines
		normalizedContent := strings.ReplaceAll(p.Content, "\r\n", "\n")
		lines := strings.Split(strings.TrimRight(normalizedContent, "\n"), "\n")

		if len(lines) > 0 {
			fmt.Printf("%s%s%s", fullPrefixWithTab, lines[0], "\n") // Use explicit newline for consistency

			indentation := ""
			if fullPrefixWithTab != "" {
				// Calculate indentation based on the visual length of the prefix.
				// This is a simplification; true visual length can be complex with non-ASCII.
				indentation = strings.Repeat(" ", len(prefix)) + "\t"
			}

			for i := 1; i < len(lines); i++ {
				fmt.Printf("%s%s%s", indentation, lines[i], "\n")
			}
		} else if p.Content == "" && fullPrefixWithTab != "" {
			fmt.Printf("%s%s", fullPrefixWithTab, "\n")
		} else if p.Content == "" && fullPrefixWithTab == "" {
			// Don't print anything for a completely empty prompt with no location info
		}
	}
}
