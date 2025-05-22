## Plan Summary: Implement `greedy` Flag for Enhanced Prompt Scanning

**1. Overall Objective:**
To introduce a new command-line flag, `--greedy` (defaulting to `false`), to the `prompt-scanner` tool. This flag will allow users to switch between the current, more aggressive prompt detection heuristics and a new, stricter set of rules designed to reduce false positives.

**2. Flag Behavior:**

*   **`--greedy=true` (or if the flag is present):**
    *   The tool will use the existing, comprehensive set of heuristics to identify potential prompts. This includes checks for variable names, content keywords (anywhere in the string), placeholder patterns, string length, and multi-line status, along with exclusions for common logging and error patterns.

*   **`--greedy=false` (default behavior):**
    *   A string will be considered a potential LLM prompt **only if** it meets one of the following stricter conditions (case-insensitive for keyword matching):
        1.  The string **starts with** any of the keywords defined in the `--content-keywords` list (from `main.go:29`).
        2.  OR, if the string does not start with a content keyword, it **contains** any of the `--content-keywords` AND the string is **multi-line**.
    *   If neither of these conditions is met, the string is not considered a prompt when `greedy` is false.

**3. Affected Files and Planned Changes:**

*   **`main.go`:**
    *   Define the new boolean flag: `greedy := flag.Bool("greedy", false, "Use aggressive (current) heuristics if true. If false, use stricter rules based on content keywords and multi-line criteria.")`.
    *   Add this new flag to the `flag.Usage` message for user visibility.
    *   Pass the value of the `*greedy` flag to the `Greedy` field when initializing `scanner.ScanOptions`.

*   **`scanner/types.go`:**
    *   Add a new boolean field `Greedy bool` to the `ScanOptions` struct definition.

*   **`scanner/heuristics.go`:**
    *   This file requires the most significant changes, specifically within the `IsPotentialPrompt(ctx PromptContext, fp *FoundPrompt) bool` function.
    *   The function will first check the `s.Options.Greedy` flag.
    *   **If `s.Options.Greedy` is `false`:**
        *   Implement the new two-part stricter logic:
            *   Convert the input text to lowercase for case-insensitive matching.
            *   Iterate through `s.Options.ContentKeywords` (the raw string slice).
            *   Check if the lowercased text `strings.HasPrefix` with the lowercased keyword. If yes, return `true`.
            *   If no "starts with" match, determine if the string is multi-line using `ctx.IsMultiLineExplicit || ctx.LinesInContent > 1`.
            *   If it is multi-line, iterate through `s.Options.ContentKeywords` again. Check if the lowercased text `strings.Contains` the lowercased keyword. If yes, return `true`.
            *   If neither of these conditions is met, return `false`.
    *   **If `s.Options.Greedy` is `true`:**
        *   Execute the existing heuristic logic currently present in the `IsPotentialPrompt` function (approximately `lines 81-189` in the version of `scanner/heuristics.go` reviewed). This includes checks for log message prefixes, error function calls, scoring based on variable keywords, content keywords (using the compiled regex `s.Options.compiledContentWords`), placeholders, string length, and multi-line status.

**4. Logic Flow Diagram:**
The overall decision logic within `IsPotentialPrompt` can be visualized as:

```mermaid
graph TD
    A[Start: IsPotentialPrompt(ctx, fp)] --> B{Input text empty?};
    B -- Yes --> X[Return false];
    B -- No --> C{s.Options.Greedy == false?};
    C -- Yes (Greedy is False) --> D{Text starts w/ contentKeyword? (case-insens.)};
    D -- Yes --> G[Return true];
    D -- No --> E{Is string multi-line?};
    E -- No --> H[Return false];
    E -- Yes --> F{Text contains contentKeyword? (case-insens.)};
    F -- Yes --> G;
    F -- No --> H;
    C -- No (Greedy is True) --> I[Execute Original Heuristic Logic (approx. lines 81-189 in current heuristics.go)];
    I --> J{Original Heuristic: Is Prompt?};
    J -- Yes --> G;
    J -- No --> H;
```

This plan aims to clearly separate the two modes of operation within the core heuristic evaluation, ensuring that the `greedy=false` mode adheres strictly to the new, more precise rules.