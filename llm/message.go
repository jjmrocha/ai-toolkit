package llm

// RoleName identifies the role of a chat [Message].
type RoleName string

const (
	// SystemRole marks a [SystemMessage].
	SystemRole RoleName = "system"
	// UserRole marks a [UserMessage].
	UserRole RoleName = "user"
	// AssistantRole marks an [AssistantMessage].
	AssistantRole RoleName = "assistant"
	// ToolRole marks a [ToolMessage].
	ToolRole RoleName = "tool"
)

// Message is a sealed interface: only the message types declared in this
// package satisfy it, so the set of chat roles is closed. Role reports the
// message's role for inspection.
type Message interface {
	Role() RoleName
	isMessage()
}

func messageValue[T Message](m Message) T {
	if v, ok := m.(T); ok {
		return v
	}

	return *(any(m).(*T))
}

// SystemMessage carries instructions that steer the model's behavior.
type SystemMessage struct {
	// Content is the instruction text.
	Content string
}

// Role returns [SystemRole].
func (SystemMessage) Role() RoleName {
	return SystemRole
}

func (SystemMessage) isMessage() {}

// UserMessage carries input from the end user.
type UserMessage struct {
	// Content is the user's input text.
	Content string
}

// Role returns [UserRole].
func (UserMessage) Role() RoleName {
	return UserRole
}

func (UserMessage) isMessage() {}

// AssistantMessage is a reply from the model. It may request tool calls, and
// carries token-usage [Stats] when returned by [LLM.Chat].
type AssistantMessage struct {
	// Content is the model's reply text. Empty when the model replied with tool
	// calls only.
	Content string
	// ToolCalls are the tools the model asks the caller to run before it
	// continues. Empty when the model produced a final answer.
	ToolCalls []ToolCall
	// Stats reports the token usage of the request that produced this message.
	// Zero for messages not produced by [LLM.Chat].
	Stats Stats

	// StopReason is the provider's native reason the model stopped generating
	// (e.g. Anthropic's "end_turn" or "max_tokens", OpenRouter's "stop" or
	// "length", Ollama's "stop"). Empty for messages not produced by [LLM.Chat].
	StopReason string

	raw any
}

// Role returns [AssistantRole].
func (AssistantMessage) Role() RoleName {
	return AssistantRole
}

func (AssistantMessage) isMessage() {}

// ToolMessage carries the result of a [ToolCall] back to the model. Providers
// correlate the result either by call ID (e.g. OpenRouter) or by tool name
// (e.g. Ollama), so both fields are available; set whichever the provider uses.
type ToolMessage struct {
	// ToolCallID matches the ID of the originating [ToolCall].
	ToolCallID string
	// ToolName is the name of the tool that produced the result.
	ToolName string
	// Content is the tool's result.
	Content string
}

// Role returns [ToolRole].
func (ToolMessage) Role() RoleName {
	return ToolRole
}

func (ToolMessage) isMessage() {}
