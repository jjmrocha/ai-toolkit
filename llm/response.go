package llm

// ToolCall is a request from the assistant to invoke a [Tool].
type ToolCall struct {
	// ID correlates the call with its [ToolMessage] result.
	ID string
	// Name is the invoked tool's name.
	Name string
	// Arguments are the decoded arguments the model supplied for the call.
	Arguments map[string]any
}

// Stats reports token usage for a response.
type Stats struct {
	// PromptTokens is the number of tokens in the request.
	PromptTokens int
	// OutputTokens is the number of tokens generated in the response.
	OutputTokens int
	// TotalTokens is the total number of tokens billed for the request.
	TotalTokens int
}
