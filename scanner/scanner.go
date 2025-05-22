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

	"github.com/alexferrari88/prompt-scanner/utils"
	gitignore "github.com/sabhiram/go-gitignore"
)

var defaultNumWorkers = runtime.NumCPU()

// Scanner orchestrates the scanning process.
type Scanner struct {
	Options        ScanOptions
	gitIgnoreCache map[string]gitignore.IgnoreParser // Key: absolute path to directory containing .gitignore
	cacheMutex     sync.Mutex
}

// New creates a new Scanner instance.
func New(options ScanOptions) (*Scanner, error) {
	if err := options.compileMatchers(); err != nil {
		return nil, fmt.Errorf("failed to compile matchers: %w", err)
	}
	s := &Scanner{
		Options:        options,
		gitIgnoreCache: make(map[string]gitignore.IgnoreParser),
	}
	if !utils.CommandExists("git") && options.Verbose {
		// This log is already conditional due to options.Verbose
		log.Println("Warning: 'git' command not found in PATH. GitHub URL cloning might be affected if not using a shallow clone mechanism that relies on it, though direct cloning often still works.")
	}
	return s, nil
}

// isIgnored checks if a given path should be ignored based on .gitignore files.
// It traverses up from the path's directory to the rootDir, checking .gitignore files.
// Paths are handled as absolute paths for consistency with the gitignore library.
func (s *Scanner) isIgnored(path string, rootDir string) (bool, error) {
	if !s.Options.UseGitignore {
		return false, nil
	}

	absPath, err := filepath.Abs(path)
	if err != nil {
		return false, fmt.Errorf("isIgnored: failed to get absolute path for target %s: %w", path, err)
	}

	var currentSearchDir string
	fi, statErr := os.Stat(absPath)
	if statErr != nil {
		currentSearchDir = filepath.Dir(absPath)
	} else {
		if fi.IsDir() {
			currentSearchDir = absPath
		} else {
			currentSearchDir = filepath.Dir(absPath)
		}
	}
	currentSearchDir, err = filepath.Abs(currentSearchDir)
	if err != nil {
		return false, fmt.Errorf("isIgnored: failed to get absolute path for search base %s: %w", filepath.Dir(absPath), err)
	}

	absRootDir, err := filepath.Abs(rootDir)
	if err != nil {
		return false, fmt.Errorf("isIgnored: failed to get absolute path for rootDir %s: %w", rootDir, err)
	}

	for {
		if currentSearchDir == "" || (!strings.HasPrefix(currentSearchDir, absRootDir) && currentSearchDir != absRootDir) {
			break
		}

		gitIgnoreFilePath := filepath.Join(currentSearchDir, ".gitignore")

		s.cacheMutex.Lock()
		ignorer, foundInCache := s.gitIgnoreCache[currentSearchDir]
		s.cacheMutex.Unlock()

		if !foundInCache {
			compiledIgnorer, compileErr := gitignore.CompileIgnoreFile(gitIgnoreFilePath)
			if compileErr != nil {
				if s.Options.Verbose {
					log.Printf("Warning: Error compiling .gitignore file %s: %v. It will be skipped.", gitIgnoreFilePath, compileErr)
				}
				dummyLines := []string{}
				compiledIgnorer = gitignore.CompileIgnoreLines(dummyLines...) // Corrected assignment
			}
			if compiledIgnorer == nil {
				dummyLines := []string{}
				compiledIgnorer = gitignore.CompileIgnoreLines(dummyLines...) // Corrected assignment
			}
			ignorer = compiledIgnorer

			s.cacheMutex.Lock()
			s.gitIgnoreCache[currentSearchDir] = ignorer
			s.cacheMutex.Unlock()
		}

		if ignorer != nil && ignorer.MatchesPath(absPath) {
			return true, nil
		}

		if currentSearchDir == absRootDir {
			break
		}

		parentDir := filepath.Dir(currentSearchDir)
		if parentDir == currentSearchDir {
			break
		}
		currentSearchDir = parentDir
	}

	return false, nil
}

// ScanDirectory recursively scans a directory for prompts.
func (s *Scanner) ScanDirectory(rootDir string) ([]FoundPrompt, error) {
	var allPrompts []FoundPrompt
	var wg sync.WaitGroup
	filesToProcess := make(chan string, defaultNumWorkers*2)     // Buffered channel
	resultsChan := make(chan []FoundPrompt, defaultNumWorkers*2) // Buffered channel
	var mu sync.Mutex                                            // Mutex for allPrompts slice

	for i := 0; i < defaultNumWorkers; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for filePath := range filesToProcess {
				promptsFromFile, err := s.processFile(filePath)
				if err != nil {
					if s.Options.Verbose {
						log.Printf("Worker %d: Error processing file %q: %v\n", workerID, filePath, err)
					}
				}
				if len(promptsFromFile) > 0 {
					resultsChan <- promptsFromFile
				}
			}
		}(i)
	}

	// Goroutine to collect results
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

	// Walk the directory
	walkErr := filepath.WalkDir(rootDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			if s.Options.Verbose {
				log.Printf("Warning: Error accessing path %q: %v\n", path, err)
			}
			if d != nil && d.IsDir() && errors.Is(err, os.ErrPermission) {
				return filepath.SkipDir
			}
			return nil
		}

		absRootDir, rootErr := filepath.Abs(rootDir)
		if rootErr != nil {
			if s.Options.Verbose {
				log.Printf("Warning: Could not get absolute path for rootDir %s: %v. Gitignore may not work correctly.", rootDir, rootErr)
			}
			absRootDir = rootDir
		}

		if ignored, gitignoreErr := s.isIgnored(path, absRootDir); gitignoreErr != nil {
			if s.Options.Verbose {
				log.Printf("Warning: Error checking .gitignore for path %q: %v. Path will be processed.\n", path, gitignoreErr)
			}
		} else if ignored {
			if s.Options.Verbose {
				log.Printf("Skipping path due to .gitignore: %s\n", path)
			}
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		if d.IsDir() {
			dirName := d.Name()
			if dirName == ".git" || dirName == "node_modules" || dirName == "vendor" ||
				dirName == "dist" || dirName == "build" || dirName == "target" ||
				dirName == "tmp" || dirName == "temp" || dirName == "__pycache__" ||
				dirName == ".venv" || dirName == "venv" || dirName == "env" ||
				dirName == ".next" || dirName == ".nuxt" || dirName == ".svelte-kit" {
				if s.Options.Verbose {
					log.Printf("Skipping common non-source directory: %s\n", path)
				}
				return filepath.SkipDir
			}
			if strings.HasPrefix(dirName, ".") && len(dirName) > 1 && dirName != ".config" && dirName != ".github" {
				if s.Options.Verbose {
					log.Printf("Skipping hidden directory: %s\n", path)
				}
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

	if s.Options.ScanConfigs {
		if strings.HasPrefix(fileName, ".env") {
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
	return nil, nil
}

// CloneRepo clones a public GitHub repository to a temporary directory.
func (s *Scanner) CloneRepo(url string) (string, error) {
	if !utils.CommandExists("git") {
		return "", fmt.Errorf("'git' command not found in PATH. Cannot clone repository. Please install git or ensure it's in your system's PATH")
	}
	tempDir, err := os.MkdirTemp("", "prompt-scan-repo-")
	if err != nil {
		return "", fmt.Errorf("failed to create temp directory: %w", err)
	}

	if s.Options.Verbose {
		log.Printf("Cloning %s into %s...", url, tempDir)
	}

	cmd := exec.Command("git", "clone", "--depth", "1", url, tempDir)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		_ = os.RemoveAll(tempDir)
		return "", fmt.Errorf("failed to clone repo '%s' (git command exit status: %s): %w. Stderr: %s", url, cmd.ProcessState.String(), err, stderr.String())
	}

	if s.Options.Verbose {
		log.Println("Repository cloned successfully.")
	}
	return tempDir, nil
}
