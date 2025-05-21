// scanner/scanner.go
package scanner

import (
	"bytes"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
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
	if err := options.compileMatchers(); err != nil {
		return nil, fmt.Errorf("failed to compile matchers: %w", err)
	}
	if !utils.CommandExists("git") {
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
	filesToProcess := make(chan string, defaultNumWorkers*2)
	resultsChan := make(chan []FoundPrompt, defaultNumWorkers*2)
	var mu sync.Mutex

	for i := 0; i < defaultNumWorkers; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for filePath := range filesToProcess {
				promptsFromFile, err := s.processFile(filePath)
				if err != nil {
					log.Printf("Warning: Error processing file %q: %v\n", filePath, err)
				}
				if len(promptsFromFile) > 0 {
					resultsChan <- promptsFromFile
				}
			}
		}(i)
	}

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

	walkErr := filepath.WalkDir(rootDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			log.Printf("Warning: Error accessing path %q: %v\n", path, err)
			if d != nil && d.IsDir() && errors.Is(err, os.ErrPermission) {
				return filepath.SkipDir
			}
			return nil
		}

		if d.IsDir() {
			dirName := d.Name()
			if dirName == ".git" || dirName == "node_modules" || dirName == "vendor" ||
				dirName == "dist" || dirName == "build" || dirName == "target" ||
				(strings.HasPrefix(dirName, ".") && len(dirName) > 1 &&
					!strings.Contains(dirName, ".env")) { // Allow .env containing dirs
				return filepath.SkipDir
			}
			return nil
		}
		filesToProcess <- path
		return nil
	})

	close(filesToProcess)
	wg.Wait()
	close(resultsChan)
	collectWg.Wait()

	if walkErr != nil {
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
		return nil, nil
	}

	// Code file types
	switch ext {
	case ".go":
		return s.ParseGoFile(filePath, contentBytes)
	case ".py":
		return s.ParseTreeSitterFile(filePath, contentBytes, "python")
	case ".js", ".jsx":
		return s.ParseTreeSitterFile(filePath, contentBytes, "javascript")
	case ".ts", ".tsx":
		return s.ParseTreeSitterFile(filePath, contentBytes, "typescript")
	}

	// Config file types - only if ScanConfigs is true
	if s.Options.ScanConfigs {
		if strings.HasPrefix(fileName, ".env") { // Check .env by name first
			return s.ParseEnvFile(filePath, contentBytes)
		}
		switch ext {
		case ".json":
			return s.ParseJSONFile(filePath, contentBytes)
		case ".yaml", ".yml":
			return s.ParseYAMLFile(filePath, contentBytes)
		case ".toml":
			return s.ParseTOMLFile(filePath, contentBytes)
		}
	}

	// log.Printf("Skipping unsupported or non-selected file type: %s", filePath)
	return nil, nil
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
	cmd := exec.Command("git", "clone", "--depth", "1", url, tempDir)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		_ = os.RemoveAll(tempDir)
		return "", fmt.Errorf("failed to clone repo '%s' (exit code: %s): %w. Stderr: %s", url, cmd.ProcessState.String(), err, stderr.String())
	}
	log.Println("Repository cloned successfully.")
	return tempDir, nil
}
