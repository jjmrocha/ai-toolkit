package llm

// Native Ollama API wire types (POST /api/chat, POST /api/show).

type ollamaChatRequest struct {
	Model    string          `json:"model"`
	Messages []ollamaMessage `json:"messages"`
	Tools    []ollamaTool    `json:"tools,omitempty"`
	Stream   bool            `json:"stream"`
	Options  *ollamaOptions  `json:"options,omitempty"`
}

type ollamaOptions struct {
	NumPredict int `json:"num_predict,omitempty"`
}

type ollamaMessage struct {
	Role      string           `json:"role"`
	Content   string           `json:"content"`
	ToolCalls []ollamaToolCall `json:"tool_calls,omitempty"`
	ToolName  string           `json:"tool_name,omitempty"`
}

type ollamaToolCall struct {
	Function ollamaToolCallFunction `json:"function"`
}

type ollamaToolCallFunction struct {
	Name string `json:"name"`
	// Arguments is a JSON object, unlike the OpenAI family which stringifies it.
	Arguments map[string]any `json:"arguments"`
}

type ollamaTool struct {
	Type     string             `json:"type"`
	Function ollamaToolFunction `json:"function"`
}

type ollamaToolFunction struct {
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	Parameters  map[string]any `json:"parameters,omitempty"`
}

type ollamaChatResponse struct {
	Message         ollamaResponseMessage `json:"message"`
	PromptEvalCount int                   `json:"prompt_eval_count"`
	EvalCount       int                   `json:"eval_count"`
	Error           string                `json:"error,omitempty"`
}

type ollamaResponseMessage struct {
	Content   string           `json:"content"`
	ToolCalls []ollamaToolCall `json:"tool_calls"`
}

type ollamaShowRequest struct {
	Model string `json:"model"`
}

type ollamaShowResponse struct {
	// ModelInfo is a flat map; the context length lives under
	// "<general.architecture>.context_length".
	ModelInfo map[string]any `json:"model_info"`
	Error     string         `json:"error,omitempty"`
}
