# Project Overview

**Project Name:** `prompt-scanner` (GitHub: `github.com/alexferrari88/prompt-scanner`)

**Goal:**
Create a standalone Go CLI tool that recursively scans a given codebase (local directory or public GitHub URL) to extract potential LLM prompts. It should display the filepath, line number, and content of these prompts.

**Core Requirements:**
1.  **Supported Languages (Code):** Python, JavaScript, TypeScript, Go.
2.  **Supported Config Files (Optional Scan):** JSON, YAML, TOML, `.env` files. (Scanning these is off by default, enabled with `--scan-configs`).
3.  **Output Formats & CLI Customization:**
    *   **Output Formats:**
        *   **Text (Default):** `filepath:linenumber\tcontent` (with multi-line content indented).
        *   **JSON:** Array of objects: `{"filepath": "...", "line": 123, "content": "..."}`.
    *   **Key CLI Flags:**
        *   `--json`: Output results in JSON format.
        *   `--no-filepath`: Omit the filepath from the default text output.
        *   `--no-linenumber`: Omit the line number from the default text output.
        *   `--scan-configs`: Also scan common config files (JSON, YAML, TOML, .env). Off by default.
        *   `--min-len <int>`: Minimum character length for a string to be considered a potential prompt (default 30). (Primarily affects `--greedy=true` mode).
        *   `--var-keywords <csv>`: Comma-separated keywords for variable/key names (e.g., `prompt,template`).
        *   `--content-keywords <csv>`: Comma-separated keywords to search for within string content (e.g., `you are a,act as`).
        *   `--placeholder-patterns <csv_regex>`: Comma-separated regex patterns for templating placeholders.
        *   `--greedy`: (Default: `false`) If `true`, use aggressive, broader heuristics. If `false`, use stricter rules: prompt if string starts with a content keyword, or contains a content keyword AND is multi-line.
4.  **Input:** Local path or public GitHub URL (cloned to a temp directory).
5.  **Core Technology:**
    *   CLI tool written in Go.
    *   Go's `go/ast` for parsing Go files.
    *   `tree-sitter` (via `github.com/smacker/go-tree-sitter`) for Python, JavaScript, and TypeScript for more accurate AST-based parsing.
6.  **Definition of "Potential LLM Prompt" (Heuristics):** The approach to identifying prompts is controlled by the `--greedy` flag (see Key CLI Flags).
    *   **When `--greedy=false` (Default Mode):**
        *   A string is identified as a potential prompt if:
            *   It **starts with** one of the `--content-keywords` (case-insensitive).
            *   OR, it **contains** one of the `--content-keywords` (case-insensitive) AND is a **multi-line string**.
        *   General exclusions (see below) may still apply if these conditions are met.
    *   **When `--greedy=true` (Aggressive Mode):**
        *   A string is identified as a potential prompt based on a broader combination of factors, including:
            *   Variable/key names matching keywords (e.g., `prompt`, `template`).
            *   Multi-line strings.
            *   String length (above `--min-len`).
            *   Presence of templating placeholders (e.g., `{var}`).
            *   Presence of content keywords (e.g., "You are a helpful assistant") anywhere in the string.
    *   **General Exclusions (Applied in both modes to reduce false positives, particularly relevant if a string passes initial checks):**
        *   Strings passed directly to common logging functions (e.g., `console.log`, `logger.error`).
        *   Strings passed to `throw new Error(...)` or similar error-throwing constructs.
        *   Strings that start with "error:", "warning:", "info:", "debug:" (case-insensitive), unless very long or complex (e.g., containing placeholders or being exceptionally long).
        *   Source code comments are ignored.