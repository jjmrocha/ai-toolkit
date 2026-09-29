package llm

// ToolCall is the model's request to run a [Tool].
type ToolCall struct {
	// ID correlates the call with its [ToolMessage] result.
	ID string
	// Name is the tool's name.
	Name string
	// Arguments are the decoded arguments the model passed.
	Arguments map[string]any
}

// Stats is the token usage of one response.
type Stats struct {
	// PromptTokens is the number of tokens in the request, including any read from
	// or written to the prompt cache.
	PromptTokens int
	// OutputTokens is the number of tokens generated in the response.
	OutputTokens int
	// TotalTokens is the number of tokens billed for the request.
	TotalTokens int
	// CacheWriteTokens is the number of prompt tokens written to the prompt cache.
	// Zero for providers without prompt caching.
	CacheWriteTokens int
	// CacheReadTokens is the number of prompt tokens read from the prompt cache.
	// Zero for providers without prompt caching.
	CacheReadTokens int
}
