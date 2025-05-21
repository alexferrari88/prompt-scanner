# Project Overview

**Project Name:** `prompt-scanner` (GitHub: `github.com/alexferrari88/prompt-scanner`)

**Goal:**
Create a standalone Go CLI tool that recursively scans a given codebase (local directory or public GitHub URL) to extract potential LLM prompts. It should display the filepath, line number, and content of these prompts.

**Core Requirements:**
1.  **Supported Languages (Code):** Python, JavaScript, TypeScript, Go.
2.  **Supported Config Files (Optional Scan):** JSON, YAML, TOML, `.env` files. (Scanning these is off by default, enabled with `--scan-configs`).
3.  **Output Formats:**
    *   **Text (Default):** `filepath:linenumber\tcontent` (with multi-line content indented). Flags `--no-filepath` and `--no-linenumber` to customize. Filepaths relative to scan root.
    *   **JSON:** Array of objects: `{"filepath": "...", "line": 123, "content": "..."}` (enabled with `--json`).
4.  **Input:** Local path or public GitHub URL (cloned to a temp directory).
5.  **Core Technology:**
    *   CLI tool written in Go.
    *   Go's `go/ast` for parsing Go files.
    *   `tree-sitter` (via `github.com/smacker/go-tree-sitter`) for Python, JavaScript, and TypeScript for more accurate AST-based parsing.
6.  **Definition of "Potential LLM Prompt" (Heuristics):** Based on a combination of:
    *   Variable/key names matching keywords (e.g., `prompt`, `template`).
    *   Multi-line strings.
    *   String length.
    *   Presence of templating placeholders (e.g., `{var}`).
    *   Presence of content keywords (e.g., "You are a helpful assistant").
    *   **Exclusions (False Positive Reduction):**
        *   Strings passed directly to common logging functions (e.g., `console.log`, `logger.error`).
        *   Strings passed to `throw new Error(...)` or similar error-throwing constructs.
        *   Strings that start with "error:", "warning:", "info:", "debug:" (case-insensitive), unless very long or complex.
        *   Comments should be ignored.