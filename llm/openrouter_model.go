package llm

type orChatRequest struct {
	Model     string       `json:"model"`
	Messages  []orMessage  `json:"messages"`
	Tools     []orTool     `json:"tools,omitempty"`
	MaxTokens int          `json:"max_tokens,omitempty"`
	Reasoning *orReasoning `json:"reasoning,omitempty"`
}

type orReasoning struct {
	Effort  string `json:"effort,omitempty"`
	Enabled *bool  `json:"enabled,omitempty"`
}

type orMessage struct {
	Role       string       `json:"role"`
	Content    string       `json:"content,omitempty"`
	ToolCalls  []orToolCall `json:"tool_calls,omitempty"`
	ToolCallID string       `json:"tool_call_id,omitempty"`
}

type orToolCall struct {
	ID       string             `json:"id"`
	Type     string             `json:"type"`
	Function orToolCallFunction `json:"function"`
}

type orToolCallFunction struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type orTool struct {
	Type     string         `json:"type"`
	Function orToolFunction `json:"function"`
}

type orToolFunction struct {
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	Parameters  map[string]any `json:"parameters,omitempty"`
}

type orChatResponse struct {
	Choices []orChoice `json:"choices"`
	Usage   orUsage    `json:"usage"`
	Error   *orError   `json:"error,omitempty"`
}

type orChoice struct {
	Message orResponseMessage `json:"message"`
}

type orResponseMessage struct {
	Content   string       `json:"content"`
	ToolCalls []orToolCall `json:"tool_calls"`
}

type orUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

type orError struct {
	Message string `json:"message"`
	Code    int    `json:"code"`
}

type orModelsResponse struct {
	Data []orModel `json:"data"`
}

type orModel struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	ContextLength int    `json:"context_length"`
}
