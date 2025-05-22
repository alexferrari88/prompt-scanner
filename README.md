Here’s an **updated README.md** that matches the actual features, flags, and internal logic of your latest `prompt-scanner` codebase (as found in the attached code):

---

# prompt-scanner 🔎

**Effortlessly find LLM prompts in any codebase—no more hunting for the AI’s “needle in the haystack.”**

---

`prompt-scanner` is a blazing-fast™, language-aware CLI tool that **automatically surfaces natural-language prompts for LLMs**—even when they’re deeply buried in code, templates, or config files. If you’re tired of digging through endless files of a codebase, let `prompt-scanner` do the heavy lifting for you.

---

## Why Use prompt-scanner?

Modern AI codebases bury prompts in variables, templates, and configs. Manual searching is tedious and unreliable. `prompt-scanner` recursively scans your project to **instantly surface potential prompts**, regardless of where they hide.

---

## Key Features

* **Language-aware Scanning:** Supports Go (native AST), Python, JavaScript/TypeScript (Tree-sitter), plus config files (JSON, YAML, TOML, `.env`).
* **Configurable Heuristics:** Fine-tune how “strict” or “greedy” detection is, set minimum string length, and customize keyword matching.
* **GitHub Repo Scanning:** Provide a repo URL—`prompt-scanner` clones and scans it automatically.
* **Smart Output:** Display as tabular or JSON, optionally include/exclude filepaths and line numbers.
* **.gitignore Respect:** Optionally skip files/directories matched by `.gitignore`.
* **Performance:** Multi-threaded, skips common non-source directories.
* **Verbose Mode:** See detailed logs for debugging and transparency.

---

## Installation

```sh
go install github.com/alexferrari88/prompt-scanner@latest
```

---

## Usage

```sh
prompt-scanner [options] <local_path_or_github_url>
```

### Common Options

* `--json` — Output in JSON format
* `--scan-configs` — Also scan config files (JSON, YAML, TOML, `.env`)
* `--min-len=N` — Minimum prompt string length (default: 30)
* `--var-keywords=...` — Comma-separated variable/key names for prompt detection
* `--content-keywords=...` — Comma-separated keywords to match in content
* `--placeholder-patterns=...` — Comma-separated regexes to detect template placeholders
* `--greedy` — Use more aggressive detection (catches more, more noise)
* `--no-filepath` — Omit filepaths in output
* `--no-linenumber` — Omit line numbers in output
* `--use-gitignore` — Respect `.gitignore` (skip matching files/dirs)
* `--verbose` — Print verbose log output to stderr

### Example

```sh
prompt-scanner --json --scan-configs --greedy ./llm-project
prompt-scanner --scan-configs --use-gitignore --min-len=50 https://github.com/user/repo
```

---

## Sample Output

**Text Output**

```
scanner/ai.go:41   You are an expert coding assistant. Your task is to help users...
handlers/llm.py:11 Your task is to summarize the following article for a 12-year-old...
config/prompts.yaml:3 Act as a wise, unbiased career coach. Answer the following...
```

**JSON Output**

```json
[
  {
    "filepath": "handlers/llm.py",
    "line": 11,
    "content": "Your task is to summarize the following article for a 12-year-old..."
  }
]
```

---

## Advanced Usage

* **Scan config files only:**

  ```sh
  prompt-scanner --scan-configs ./project
  ```
* **Scan GitHub repo:**

  ```sh
  prompt-scanner https://github.com/user/repo
  ```
* **Customize detection:**

  ```sh
  prompt-scanner --var-keywords=prompt,system_message --content-keywords="act as,your task is" ./project
  ```
* **Omit file paths and line numbers:**

  ```sh
  prompt-scanner --no-filepath --no-linenumber ./project
  ```
* **Respect `.gitignore`:**

  ```sh
  prompt-scanner --use-gitignore ./project
  ```
* **Full flag list:**

  ```sh
  prompt-scanner --help
  ```

---

## How It Works

* **Go code:** Uses the Go AST for reliable string literal extraction and context.
* **Python/JS/TS:** Uses Tree-sitter queries for robust parsing and prompt context.
* **Config files:** JSON, YAML, TOML, `.env` handled with special parsers.
* **Heuristics:**

  * By default, only detects strings that *start with* or *contain* key prompt-like phrases and are multi-line/long enough.
  * With `--greedy`, detection is more permissive but may catch more false positives.
  * Variables/keys, content, and placeholder regexes are all tunable.
* **Ignores:** Skips common “junk” directories (`.git`, `node_modules`, etc.), plus `.gitignore` (if enabled).

---

## Contributing 🤝

Pull requests welcome! For new language support, better heuristics, or improvements, open an issue or PR.

---

## Support Development 💖

If `prompt-scanner` saves you time or headaches, [sponsor me on GitHub](https://github.com/sponsors/alexferrari88)!
Your support keeps this project (and me) going.

[![Sponsor me](https://img.shields.io/badge/Sponsor%20me%20%E2%9D%A4%EF%B8%8F-GitHub-blue?style=for-the-badge)](https://github.com/sponsors/alexferrari88)

---

## License 📄

MIT License. See [LICENSE](LICENSE).

---

**Stop searching, start shipping.**
— [@alexferrari88](https://github.com/alexferrari88)