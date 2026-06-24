package llm

type ToolCall struct {
	ID        string
	Name      string
	Arguments map[string]any
}

type Stats struct {
	PromptTokens int
	OutputTokens int
	TotalTokens  int
}
