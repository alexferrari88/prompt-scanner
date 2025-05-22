// scanner/defaults.go
package scanner

const (
	// DefaultMinLength is the default minimum character length for a string to be considered a potential prompt.
	DefaultMinLength = 30

	// DefaultVarKeywords lists common variable names that might hold prompts.
	DefaultVarKeywords = "prompt,template,system_message,user_message,instruction,persona,query,question,task_description,context_str"

	// DefaultContentKeywords lists common phrases found within prompt strings.
	DefaultContentKeywords = "you are a,you are an,you are the,act as,from the following,from this,your task is to,you need to,break down,translate the,summarize the,given the,answer the following question,extract entities from,generate code for,what is the,explain the,act as a,respond with,based on the provided text,here's,here is,here are,consider this,consider the following,analyze this,analyze the following"

	// DefaultPlaceholderPatterns lists common regex patterns for identifying templating placeholders.
	DefaultPlaceholderPatterns = `\{[^{}]*?\}|\{\{[^{}]*?\}\}|<[^<>]*?>|\$[A-Z_][A-Z0-9_]*|\%[sdfeuxg]|\[[A-Z_]+\]`
)
