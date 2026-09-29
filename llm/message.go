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

// Message is a chat message. Only the message types in this package implement
// it, so the set of roles is closed. Role returns the message's role.
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

// AssistantMessage is a reply from the model. It may ask for tool calls, and
// carries token usage when [LLM.Chat] returns it.
type AssistantMessage struct {
	// Content is the reply text, empty when the model only asked for tool calls.
	Content string
	// ToolCalls are the tools the model asks the caller to run before it goes on.
	// Empty for a final answer.
	ToolCalls []ToolCall
	// Stats is the token usage of the request that produced this message, zero
	// when [LLM.Chat] did not produce it.
	Stats Stats

	// StopReason is the provider's own reason the model stopped, such as
	// Anthropic's "end_turn" or "max_tokens", OpenRouter's "stop" or "length", or
	// Ollama's "stop". Empty when [LLM.Chat] did not produce the message.
	StopReason string

	raw any
}

// Role returns [AssistantRole].
func (AssistantMessage) Role() RoleName {
	return AssistantRole
}

func (AssistantMessage) isMessage() {}

// ToolMessage returns the result of a [ToolCall] to the model. OpenRouter
// matches it to the call by ID and Ollama by tool name, so it carries both.
type ToolMessage struct {
	// ToolCallID is the ID of the [ToolCall].
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
