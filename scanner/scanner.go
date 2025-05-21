// scanner/scanner.go
package scanner

import (
	"bytes"
	"errors" // For checking specific error types in WalkDir
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime" // To get NumCPU
	"strings"
	"sync"

	"github.com/alexferrari88/prompt-scanner/utils" // Adjust import path
)

var defaultNumWorkers = runtime.NumCPU()

// Scanner orchestrates the scanning process.
type Scanner struct {
	Options ScanOptions
}

// New creates a new Scanner instance.
func New(options ScanOptions) (*Scanner, error) {
	if err := options.compileMatchers(); err != nil { // Changed to private method
		return nil, fmt.Errorf("failed to compile matchers: %w", err)
	}
	if !utils.CommandExists("git") {
		// This is a warning, not a fatal error, as local scans are still possible.
		log.Println("Warning: 'git' command not found in PATH. GitHub URL cloning will be unavailable.")
	}
	return &Scanner{
		Options: options,
	}, nil
}

// ScanDirectory recursively scans a directory for prompts.
func (s *Scanner) ScanDirectory(rootDir string) ([]FoundPrompt, error) {
	var allPrompts []FoundPrompt
	var wg sync.WaitGroup
	// Buffer channels to prevent deadlocks if workers are slower than WalkDir or vice-versa.
	filesToProcess := make(chan string, defaultNumWorkers*2)
	resultsChan := make(chan []FoundPrompt, defaultNumWorkers*2) // Store slices of prompts
	var mu sync.Mutex                                            // Protects allPrompts

	for i := 0; i < defaultNumWorkers; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			// log.Printf("Worker %d started", workerID)
			for filePath := range filesToProcess {
				// log.Printf("Worker %d processing %s", workerID, filePath)
				promptsFromFile, err := s.processFile(filePath)
				if err != nil {
					// Log specific file processing errors but continue with other files.
					log.Printf("Warning: Error processing file %q: %v\n", filePath, err)
				}
				if len(promptsFromFile) > 0 {
					resultsChan <- promptsFromFile
				}
			}
			// log.Printf("Worker %d finished", workerID)
		}(i)
	}

	// Goroutine to collect results from resultsChan
	var collectWg sync.WaitGroup
	collectWg.Add(1)
	go func() {
		defer collectWg.Done()
		for promptsSlice := range resultsChan {
			mu.Lock()
			allPrompts = append(allPrompts, promptsSlice...)
			mu.Unlock()
		}
	}()

	// Walk the directory and send file paths to workers.
	walkErr := filepath.WalkDir(rootDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			// Handle errors during walking (e.g., permission denied).
			log.Printf("Warning: Error accessing path %q: %v\n", path, err)
			if d != nil && d.IsDir() && errors.Is(err, os.ErrPermission) {
				return filepath.SkipDir // Skip directories we can't read.
			}
			// For other errors, we might choose to skip the file or stop.
			// Returning the error would stop WalkDir. Returning nil continues.
			return nil // Continue walking even if some files/dirs are inaccessible.
		}

		if d.IsDir() {
			dirName := d.Name()
			// More targeted skipping
			if dirName == ".git" || dirName == "node_modules" || dirName == "vendor" ||
				dirName == "dist" || dirName == "build" || dirName == "target" || // Common build/dependency dirs
				(strings.HasPrefix(dirName, ".") && len(dirName) > 1 && // Hidden dirs
					!strings.Contains(dirName, ".env")) { // Allow .env files/dirs
				// log.Printf("Skipping directory: %s", path)
				return filepath.SkipDir
			}
			return nil // It's a directory we want to explore, continue.
		}

		// It's a file, send it for processing.
		filesToProcess <- path
		return nil
	})

	close(filesToProcess) // Signal workers that no more files will be sent.
	wg.Wait()             // Wait for all file processing workers to complete.
	close(resultsChan)    // Signal the collector goroutine to complete.
	collectWg.Wait()      // Wait for the collector to process all results.

	if walkErr != nil {
		// This error is from WalkDir itself, not from file processing.
		return allPrompts, fmt.Errorf("error walking directory %s: %w", rootDir, walkErr)
	}

	return allPrompts, nil
}

// processFile determines the file type and calls the appropriate parser.
func (s *Scanner) processFile(filePath string) ([]FoundPrompt, error) {
	ext := strings.ToLower(filepath.Ext(filePath))
	fileName := strings.ToLower(filepath.Base(filePath))

	contentBytes, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("reading file %s: %w", filePath, err)
	}
	if len(contentBytes) == 0 {
		return nil, nil // Skip empty files
	}

	// Specific handling for .env files by full name or prefix, as they might lack ".env" extension (e.g. .env.local)
	if strings.HasPrefix(fileName, ".env") {
		return s.ParseEnvFile(filePath, contentBytes)
	}

	switch ext {
	case ".go":
		return s.ParseGoFile(filePath, contentBytes)
	case ".py":
		return s.ParseTreeSitterFile(filePath, contentBytes, "python")
	case ".js", ".jsx":
		return s.ParseTreeSitterFile(filePath, contentBytes, "javascript")
	case ".ts", ".tsx":
		return s.ParseTreeSitterFile(filePath, contentBytes, "typescript")
	case ".json":
		return s.ParseJSONFile(filePath, contentBytes)
	case ".yaml", ".yml":
		return s.ParseYAMLFile(filePath, contentBytes)
	case ".toml":
		return s.ParseTOMLFile(filePath, contentBytes)
	default:
		// log.Printf("Skipping unsupported file type: %s", filePath)
		return nil, nil
	}
}

// CloneRepo clones a public GitHub repository to a temporary directory.
func (s *Scanner) CloneRepo(url string) (string, error) {
	if !utils.CommandExists("git") {
		return "", fmt.Errorf("'git' command not found in PATH. Cannot clone repository. Please install git or ensure it's in your system's PATH")
	}

	tempDir, err := os.MkdirTemp("", "llm-prompt-scan-repo-")
	if err != nil {
		return "", fmt.Errorf("failed to create temp directory: %w", err)
	}

	log.Printf("Cloning %s into %s...", url, tempDir)
	// Use --depth 1 for a shallow clone, reducing download time/data.
	// Use --quiet to reduce log noise unless there's an error.
	cmd := exec.Command("git", "clone", "--depth", "1", url, tempDir)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr // Capture stderr for error reporting.

	if err := cmd.Run(); err != nil {
		// Attempt to clean up the temp directory on cloning failure.
		// Not critical if it fails, but good practice.
		_ = os.RemoveAll(tempDir)
		return "", fmt.Errorf("failed to clone repo '%s' (exit code: %s): %w. Stderr: %s", url, cmd.ProcessState.String(), err, stderr.String())
	}

	log.Println("Repository cloned successfully.")
	return tempDir, nil
}
