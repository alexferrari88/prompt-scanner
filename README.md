# prompt-scanner 🔎

**Effortlessly find LLM prompts in any codebase—no more hunting for the AI’s “needle in the haystack.”**

`prompt-scanner` is a blazing-fast™, language-aware CLI tool that **automatically surfaces natural-language prompts for LLMs**—even when they’re deeply buried in code, templates, or config files.

---

## Why Use prompt-scanner?

Modern AI codebases bury prompts in variables, templates, and configs. Manual searching is tedious and unreliable. `prompt-scanner` recursively scans your project to **instantly surface potential prompts**, regardless of where they hide.

---

## Key Features

* **Language-aware Scanning:** Supports Go (native AST), Python, JavaScript/TypeScript (Tree-sitter), plus config files (JSON, YAML, TOML, `.env`).
* **Configurable Heuristics:** Fine-tune how “strict” or “greedy” detection is, set minimum string length, and customize keyword matching.
* **GitHub Repo Scanning:** Provide a repo URL—`prompt-scanner` clones and scans it automatically.
* **Smart Output:** Display as tabular or JSON, optionally include/exclude file paths and line numbers.
* **.gitignore Respect:** Optionally skip files/directories matched by `.gitignore`.
* **Performance:** Multi-threaded, skips common non-source directories.
* **Verbose Mode:** See detailed logs for debugging and transparency.

---

## Installation

Install the latest `prompt-scanner` CLI via Go:

```sh
go install github.com/alexferrari88/prompt-scanner@latest
```

---

## Downloading Prebuilt Binaries

Prebuilt executables for **macOS (x64/arm64)** and **Linux (x64)** are available on the [Releases page](https://github.com/alexferrari88/prompt-scanner/releases).

> **Note:** Windows and Linux on ARM64 binaries are not yet provided. If you’re proficient with GitHub Actions and would like to help add these targets, please propose changes to the [release workflow](https://github.com/alexferrari88/prompt-scanner/blob/main/.github/workflows/release.yml) and open a pull request.

### Adding the Binary to Your PATH

Once downloaded, you can place the executable in a directory that’s included in your system’s `PATH` so you can run `prompt-scanner` from anywhere.

* **Linux & macOS**

  1. Unpack the archive if necessary (e.g., `tar xzf prompt-scanner_<version>_$(uname | tr '[:upper:]' '[:lower:]')_amd64.tar.gz`).
  2. Move the binary to `/usr/local/bin` (or another directory in your `PATH`):

     ```sh
     sudo mv prompt-scanner /usr/local/bin/
     ```
  3. Ensure `/usr/local/bin` is in your `PATH`:

     ```sh
     echo $PATH
     ```

* **Windows**

  1. Download and unzip the `.zip` file.
  2. Copy `prompt-scanner.exe` into a directory on your `PATH`, for example:

     * `C:\\Windows\\System32`
     * Or create a dedicated tools folder (e.g., `C:\\Tools\\prompt-scanner`) and add it via **System Properties → Environment Variables → Path → Edit**.

After adding to your `PATH`, open a new terminal or PowerShell window and verify:

```sh
prompt-scanner --help
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
* `--no-filepath` — Omit file paths in output
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

Pull requests welcome! For new language support, better heuristics, or improvements, open an issue or PR. If you’d like to help publish Windows or Linux ARM64 binaries, you can start by updating the [release workflow](https://github.com/alexferrari88/prompt-scanner/blob/main/.github/workflows/release.yml) with those targets.

---

## Support 💖

If `prompt-scanner` saves you time or headaches, [sponsor me on GitHub](https://github.com/sponsors/alexferrari88)!

[![Sponsor me](https://img.shields.io/badge/Sponsor%20me%20❤️-GitHub-blue?style=for-the-badge)](https://github.com/sponsors/alexferrari88)

---

## License 📄

MIT License. See [LICENSE](LICENSE).

---

**Stop searching, start shipping.**
