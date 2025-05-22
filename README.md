# prompt-scanner 🔎

**Find LLM prompts in any codebase. No more hunting for the AI’s “needle in the haystack.”**

---

`prompt-scanner` is a blazing-fast™, language-aware CLI tool that **automatically finds Large Language Model (LLM) prompts**—the actual natural language instructions—in codebases and config files. If you’re tired of digging through endless files and wrappers just to see what prompt your AI/LLM code is using, this tool is for you.

---

## Why Use prompt-scanner?

Most modern AI wrappers and frameworks hide the real LLM prompt deep in variables, templates, or config files. Searching for them manually is tedious and error-prone. `prompt-scanner` saves you hours by **recursively scanning your project and surfacing potential prompts instantly**—no matter how buried they are.

---

## Key Features

* **Comprehensive Project Scanning:** Searches through code and configuration files in multiple languages and formats (Go, Python, JavaScript/TypeScript, JSON, YAML, TOML, `.env`).
* **Heuristic Detection:** Intelligent filters exclude irrelevant log messages and errors, identifying genuine prompts efficiently.
* **Native Multi-language Support:** Leverages Go's AST and Tree-sitter for accurate parsing and analysis.
* **GitHub Integration:** Directly scan GitHub repositories—simply provide the URL and `prompt-scanner` does the rest.
* **Flexible Output:** Offers tabular or JSON-formatted outputs, with optional inclusion of file paths and line numbers.
* **Highly Configurable:** Customize keywords, placeholder patterns, prompt length thresholds, and detection sensitivity.
* **Performance Optimized:** Utilizes multi-threading and skips unnecessary directories (e.g., dependencies/build folders).

---

## Quickstart

### Installation

```sh
go install github.com/alexferrari88/prompt-scanner@latest
```

### Basic Usage

```sh
prompt-scanner ./my-project
# or scan directly from GitHub
prompt-scanner https://github.com/user/repo
```

### Common Options

* `--json` – Output results in JSON format.
* `--scan-configs` – Include configuration files (JSON, YAML, TOML, `.env`).
* `--min-len=N` – Set minimum string length to qualify as a prompt (default: 30).
* `--var-keywords=...` – Define keywords for detecting prompt variables.
* `--content-keywords=...` – Specify keywords that must be present in the content.
* `--greedy` – Enable aggressive detection heuristics.

#### Example

```sh
prompt-scanner --json --scan-configs --greedy ./llm-project
```

---

## Sample Output

### Standard View

```
scanner/ai.go:41   You are an expert coding assistant. Your task is to help users...
handlers/llm.py:11 Your task is to summarize the following article for a 12-year-old...
config/prompts.yaml:3 Act as a wise, unbiased career coach. Answer the following...
```

### JSON View

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

* **Include Config Files:**

  ```sh
  prompt-scanner --scan-configs ./my-project
  ```

* **Direct Scanning From a Github URL:**

  ```sh
  prompt-scanner https://github.com/user/repo
  ```

* **Customize Output:**

  ```sh
  prompt-scanner --no-filepath --no-linenumber ./my-project
  ```

* **Adjust Detection Heuristics:**

  ```sh
  prompt-scanner --var-keywords=prompt,message --content-keywords=task,instruct ./my-project
  ```

### View All Options

```sh
prompt-scanner --help
```

---

## How It Works

* **Intelligent Parsing:** Utilizes Go's AST and Tree-sitter for precise, language-aware analysis.
* **Variable Detection:** Targets typical prompt-related variables like `prompt`, `template`, and more.
* **Smart Filtering:** Avoids false positives by excluding common non-prompt strings unless in greedy mode.
* **Supports Complex Prompts:** Handles multi-line strings and templated content effectively.

---

## Contributing 🤝

Pull requests and community contributions are welcome. If you'd like support for additional languages or configurations, please open an issue or submit a pull request with the required changes.

---

## Support Development 💖

If `prompt-scanner` saves you time or headaches, please consider [sponsoring me on GitHub](https://github.com/sponsors/alexferrari88)!
Your support helps me maintain and improve this project.

[![Sponsor me](https://img.shields.io/badge/Sponsor%20me%20%E2%9D%A4%EF%B8%8F-GitHub-blue?style=for-the-badge)](https://github.com/sponsors/alexferrari88)

---

## License 📄

This project is licensed under the [MIT License](LICENSE).

---

**Stop searching, start shipping.**
— [@alexferrari88](https://github.com/alexferrari88)
